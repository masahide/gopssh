package pssh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	pkgerrors "github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	one                = 1
	connectFailureCode = 255
	// DefaultConcurrency limits simultaneous SSH connections.
	DefaultConcurrency = 32
	// DefaultMaxAgentConns limits simultaneous SSH Agent socket connections.
	DefaultMaxAgentConns = 50
	// DefaultMaxOutputBytes limits each host's stdout and stderr buffer.
	DefaultMaxOutputBytes int64 = 10 << 20
)

type prn interface {
	Print(a ...interface{}) (n int, err error)
	Printf(format string, a ...interface{}) (n int, err error)
}

type print struct {
	colorMode bool
	stdout    io.Writer
	stderr    io.Writer
	red       prn
	boldRed   prn
	green     prn
}

// ToSlice comma separated to slice
func ToSlice(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	values := strings.Split(s, ",")
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func newPrint(stdout, stderr io.Writer, colorMode bool) *print {
	p := &print{
		colorMode: colorMode,
		stdout:    stdout,
		stderr:    stderr,
	}
	p.init()
	return p
}

func (p *print) init() {
	if p.colorMode {
		p.red = color.New(color.FgRed)
		p.red.(*color.Color).SetWriter(p.stderr)
		p.boldRed = color.New(color.FgRed).Add(color.Bold)
		p.boldRed.(*color.Color).SetWriter(p.stderr)
		p.green = color.New(color.FgGreen)
		p.green.(*color.Color).SetWriter(p.stdout)
		return
	}
	p.red = writerPrinter{p.stderr}
	p.boldRed = writerPrinter{p.stderr}
	p.green = writerPrinter{p.stdout}
}

func (p *print) Print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(p.stdout, a...)
}
func (p *print) Printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(p.stdout, format, a...)
}

type writerPrinter struct {
	io.Writer
}

func (p writerPrinter) Print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(p.Writer, a...)
}

func (p writerPrinter) Printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(p.Writer, format, a...)
}

type sshDialIface interface {
	Dial(network, addr string, config *ssh.ClientConfig) (sshClientIface, error)
}
type sshDial struct{}

func (n sshDial) Dial(network, addr string, config *ssh.ClientConfig) (sshClientIface, error) {
	return ssh.Dial(network, addr, config)
}

type sshClientIface interface {
	ssh.Conn
	Dial(n, addr string) (net.Conn, error)
	DialTCP(n string, laddr, raddr *net.TCPAddr) (net.Conn, error)
	HandleChannelOpen(channelType string) <-chan ssh.NewChannel
	Listen(n, addr string) (net.Listener, error)
	ListenTCP(laddr *net.TCPAddr) (net.Listener, error)
	ListenUnix(socketPath string) (net.Listener, error)
	NewSession() (*ssh.Session, error)
}

// Pssh pssh struct
type Pssh struct {
	*Config
	*print
	concurrentGoroutines chan struct{}
	stdoutPool           sync.Pool
	stderrPool           sync.Pool
	//netDial              func(network, address string) (net.Conn, error)
	sshDialer     sshDialIface
	cws           []*conWork
	clientConf    ssh.ClientConfig
	identFileData [][]byte
	conns         *connPools
}

// Config pssh config
type Config struct {
	Concurrency      int
	MaxAgentConns    int
	MaxOutputBytes   int64
	User             string
	Hostsfile        string
	ShowHostName     bool
	ColorMode        bool
	IgnoreHostKey    bool
	Debug            bool
	StdinFlag        bool
	IdentityFileOnly bool
	SortPrint        bool
	Timeout          time.Duration
	KexFlag          string
	SSHAuthSocket    string

	IdentFiles []string
	// ciphers
	Kex     []string
	Ciphers []string
	Macs    []string
}

func newBytesBuf() interface{} { return new(bytes.Buffer) }

// Init Pssh
func (p *Pssh) Init() {
	concurrency := p.Concurrency
	if concurrency < 0 {
		concurrency = 0
	}
	p.concurrentGoroutines = make(chan struct{}, concurrency)
	p.print = newPrint(os.Stdout, os.Stderr, p.ColorMode)
	p.stdoutPool = sync.Pool{New: newBytesBuf}
	p.stderrPool = sync.Pool{New: newBytesBuf}
	p.sshDialer = sshDial{}
	p.identFileData = p.readIdentFiles()
}

// Validate checks configuration values that would otherwise panic or block.
func (p *Pssh) Validate() error {
	if p.Config == nil {
		return errors.New("config is required")
	}
	if p.Concurrency <= 0 {
		return errors.New("concurrency must be greater than zero")
	}
	if p.MaxAgentConns <= 0 {
		return errors.New("max agent connections must be greater than zero")
	}
	if p.MaxOutputBytes <= 0 {
		return errors.New("max output bytes must be greater than zero")
	}
	return nil
}

type input struct {
	id      int
	command string
	stdin   string
	results chan<- *result
}
type result struct {
	conID           int
	sessionID       int
	code            int
	err             error
	stdout          *bytes.Buffer
	stderr          *bytes.Buffer
	stdoutTruncated bool
	stderrTruncated bool
}

func (p *Pssh) newResult(conID, sessionID int) *result {
	r := &result{
		conID:     conID,
		sessionID: sessionID,
		stdout:    p.stdoutPool.Get().(*bytes.Buffer),
		stderr:    p.stderrPool.Get().(*bytes.Buffer),
	}
	r.stdout.Reset()
	r.stderr.Reset()
	return r
}
func (p *Pssh) delReslt(r *result) {
	p.stdoutPool.Put(r.stdout)
	p.stderrPool.Put(r.stderr)
}

func readHosts(fileName string) ([]string, error) {
	// nolint: gosec
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	var result []string
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.SplitN(scanner.Text(), "#", 2)[0]
		for _, value := range strings.Fields(line) {
			host, err := normalizeHost(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", fileName, lineNumber, err)
			}
			result = append(result, host)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func normalizeHost(value string) (string, error) {
	if host, port, err := net.SplitHostPort(value); err == nil {
		if host == "" || port == "" {
			return "", fmt.Errorf("invalid host %q", value)
		}
		return net.JoinHostPort(host, port), nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		host := strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
		if net.ParseIP(host) == nil {
			return "", fmt.Errorf("invalid host %q", value)
		}
		return net.JoinHostPort(host, "22"), nil
	}
	if net.ParseIP(value) != nil {
		return net.JoinHostPort(value, "22"), nil
	}
	if strings.Contains(value, ":") {
		return "", fmt.Errorf("invalid host %q", value)
	}
	if value == "" {
		return "", errors.New("host must not be empty")
	}
	return net.JoinHostPort(value, "22"), nil
}

func getHostKeyCallback(insecure bool) (ssh.HostKeyCallback, error) {
	if insecure {
		// nolint: gosec
		return ssh.InsecureIgnoreHostKey(), nil
	}
	file := path.Join(os.Getenv("HOME"), ".ssh/known_hosts")
	cb, err := knownhosts.New(file)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "knownhosts.New")
	}
	return cb, nil
}

func (p *Pssh) newConWork(id int, host string) *conWork {
	c := &conWork{Pssh: p, id: id, host: host, command: make(chan input, one)}
	c.startSession = c.startSessionWorker
	return c
}
func (p *Pssh) setConnPool() {
	if len(p.SSHAuthSocket) == 0 {
		return
	}
	p.conns = newConnPools(p.SSHAuthSocket, p.MaxAgentConns)
}

// Run main task
func (p *Pssh) Run() int {
	if err := p.Validate(); err != nil {
		log.Printf("invalid config: %s", err)
		return one
	}
	hosts, err := readHosts(p.Hostsfile)
	if err != nil {
		// nolint: errcheck,gosec
		log.Printf("read hosts file err: %s", err)
		return one
	}
	hc, err := getHostKeyCallback(p.IgnoreHostKey)
	if err != nil {
		// nolint: errcheck,gosec
		log.Printf("read hosts file err: %s", err)
		return one
	}
	p.setConnPool()
	p.clientConf = ssh.ClientConfig{
		User: p.User,
		//Auth:            p.getAuthMethods(),
		Timeout:         p.Timeout,
		HostKeyCallback: hc,
		Config:          ssh.Config{KeyExchanges: p.Kex, Ciphers: p.Ciphers, MACs: p.Macs},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.cws = make([]*conWork, len(hosts))
	for i, host := range hosts {
		p.cws[i] = p.newConWork(i, host)
	}
	go p.runConWorkers(ctx)

	stdin := []byte{}
	if p.StdinFlag {
		if stdin, err = io.ReadAll(os.Stdin); err != nil {
			log.Fatal(err)
		}
	}
	results := make(chan *result, len(hosts))
	in := input{
		command: strings.Join(flag.Args(), " "),
		stdin:   string(stdin),
		results: results,
	}
	for i := range p.cws {
		p.cws[i].command <- in
	}
	code := p.outputFunc()(ctx, results, p.cws)
	cancel()

	return code
}

func (p *Pssh) runConWorkers(ctx context.Context) int {
	for i := range p.cws {
		if p.Concurrency > 0 {
			p.concurrentGoroutines <- struct{}{}
		}
		go p.cws[i].conWorker(ctx, p.clientConf)
	}
	return len(p.cws)
}

func (p *Pssh) printSortResults(ctx context.Context, results chan *result, cws []*conWork) int {
	var firstCode int
	resSlise := make([]*result, len(cws))
	cur := 0
	for i := 0; i < len(cws); i++ {
		select {
		case res := <-results:
			resSlise[res.conID] = res
		L1:
			for j := cur; j < len(cws); j++ {
				if resSlise[j] == nil {
					break L1
				}
				p.printResult(resSlise[j], cws[resSlise[j].conID].host)
				if firstCode == 0 && resSlise[j].code != 0 {
					firstCode = resSlise[j].code
				}
				p.delReslt(resSlise[j])
				resSlise[j] = nil
				cur = j + one
			}
		case <-ctx.Done():
			firstCode = one
		}
	}
	return firstCode
}

func (p *Pssh) outputFunc() func(ctx context.Context, results chan *result, cws []*conWork) int {
	if p.SortPrint {
		return p.printSortResults
	}
	return p.printResults
}

func (p *Pssh) printResults(ctx context.Context, results chan *result, cws []*conWork) int {
	var firstCode int
	for i := 0; i < len(cws); i++ {
		select {
		case res := <-results:
			p.printResult(res, cws[res.conID].host)
			if firstCode == 0 && res.code != 0 {
				firstCode = res.code
			}
			p.delReslt(res)
		case <-ctx.Done():
			firstCode = one
		}
	}
	return firstCode
}

func (p *Pssh) printResult(res *result, host string) {
	if p.ShowHostName {
		var c prn
		if res.code != 0 || res.err != nil {
			c = p.boldRed
		} else {
			c = p.green
		}
		// nolint: errcheck,gosec
		c.Printf("%s  result code %d\n", host, res.code)
	}
	if res.err != nil {
		// nolint: errcheck,gosec
		e := res.err.Error()
		if !strings.HasSuffix(e, "\n") {
			e += "\n"
		}
		_, _ = p.red.Printf("result err: %s", e)
	}
	if res.stdout.Len() > 0 {
		// nolint: errcheck,gosec
		_, _ = res.stdout.WriteTo(p.stdout)
	}
	if res.stderr.Len() > 0 {
		// nolint: errcheck,gosec
		p.red.Print(res.stderr.String())
	}
}

type client interface {
	NewSession() (*ssh.Session, error)
}

func getErr(ctx context.Context, errCh <-chan error) error {
	var err error
L1:
	for {
		select {
		case e, ok := <-errCh:
			if !ok {
				break L1
			}
			err = e
		case <-ctx.Done():
			return err
		}
	}
	return err
}

func readStream(ctx context.Context, out io.Writer, r io.Reader, errCh chan<- error) {
	_, err := io.Copy(out, r)
	select {
	case errCh <- err:
	case <-ctx.Done():
	}
	close(errCh)
}

type cappedWriter struct {
	dst       io.Writer
	remaining int64
	truncated *bool
}

func (w *cappedWriter) Write(data []byte) (int, error) {
	originalLen := len(data)
	if w.remaining <= 0 {
		*w.truncated = true
		return originalLen, nil
	}
	toWrite := data
	if int64(len(toWrite)) > w.remaining {
		toWrite = toWrite[:w.remaining]
		*w.truncated = true
	}
	written, err := w.dst.Write(toWrite)
	w.remaining -= int64(written)
	if err != nil {
		return written, err
	}
	return originalLen, nil
}

func (p *Pssh) sshKeyAgentCallback() ssh.AuthMethod {
	if p.conns == nil {
		return nil
	}
	agentClient := newAgentClient(p.conns)
	return ssh.PublicKeysCallback(agentClient.Signers)
}

func (p *Pssh) mergeAuthMethods(identMethods []ssh.AuthMethod) []ssh.AuthMethod {
	res := make([]ssh.AuthMethod, 0, len(identMethods)+one)
	if !p.IdentityFileOnly {
		if keyAgentMehod := p.sshKeyAgentCallback(); keyAgentMehod != nil {
			res = append(res, keyAgentMehod)
		}
	}
	return append(res, identMethods...)
}

func (p *Pssh) getIdentFileAuthMethods(identFileData [][]byte) []ssh.AuthMethod {
	res := make([]ssh.AuthMethod, 0, len(identFileData))
	for _, data := range identFileData {
		key, err := ssh.ParsePrivateKey(data)
		if err != nil {
			continue
		}
		res = append(res, ssh.PublicKeys(key))
	}
	return res
}

func (p *Pssh) readIdentFiles() [][]byte {
	res := make([][]byte, 0, len(p.IdentFiles))
	home := os.Getenv("HOME")
	for _, filePath := range p.IdentFiles {
		// nolint: gosec
		filePath = strings.Replace(filePath, "~", home, one)
		buffer, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		res = append(res, buffer)
	}
	return res
}
