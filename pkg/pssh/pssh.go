package pssh

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"strconv"
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
	// DefaultMaxBufferMemory limits total in-memory buffered remote output.
	DefaultMaxBufferMemory int64 = 128 << 20
	// DefaultMaxSpoolSize limits total remote output spilled to disk.
	DefaultMaxSpoolSize int64 = 10 << 30
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
		p.red = colorPrinter{color: color.New(color.FgRed), writer: p.stderr}
		p.boldRed = colorPrinter{color: color.New(color.FgRed).Add(color.Bold), writer: p.stderr}
		p.green = colorPrinter{color: color.New(color.FgGreen), writer: p.stdout}
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

type colorPrinter struct {
	color  *color.Color
	writer io.Writer
}

func (p colorPrinter) Print(a ...interface{}) (n int, err error) {
	return p.color.Fprint(p.writer, a...)
}

func (p colorPrinter) Printf(format string, a ...interface{}) (n int, err error) {
	return p.color.Fprintf(p.writer, format, a...)
}

func (p writerPrinter) Print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(p.Writer, a...)
}

func (p writerPrinter) Printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(p.Writer, format, a...)
}

type sshDialIface interface {
	DialContext(ctx context.Context, network, addr string, config *ssh.ClientConfig) (sshClientIface, error)
}

type contextDialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type sshDial struct {
	netDialer contextDialer
}

func (n sshDial) DialContext(
	ctx context.Context,
	network string,
	addr string,
	config *ssh.ClientConfig,
) (sshClientIface, error) {
	dialer := n.netDialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: config.Timeout}
	}
	netConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	stopCancel := context.AfterFunc(ctx, func() {
		_ = netConn.Close()
	})
	clientConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, config)
	stopCancel()
	if err != nil {
		_ = netConn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = clientConn.Close()
		return nil, err
	}
	return ssh.NewClient(clientConn, chans, reqs), nil
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
	outputMemory         *memoryBudget
	outputSpool          *memoryBudget
	outputSpoolOnce      sync.Once
	outputSpoolDir       string
	outputSpoolErr       error
	workerWG             sync.WaitGroup
	sshDialer            sshDialIface
	cws                  []*conWork
	clientConf           ssh.ClientConfig
	identFileData        [][]byte
	conns                *connPools
}

// Config pssh config
type Config struct {
	Concurrency      int
	MaxAgentConns    int
	MaxBufferMemory  int64
	MaxSpoolSize     int64
	SpoolDir         string
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

// Init Pssh
func (p *Pssh) Init() {
	concurrency := p.Concurrency
	if concurrency < 0 {
		concurrency = 0
	}
	p.concurrentGoroutines = make(chan struct{}, concurrency)
	p.print = newPrint(os.Stdout, os.Stderr, p.ColorMode)
	p.sshDialer = sshDial{}
	p.identFileData = p.readIdentFiles()
	p.prepareOutputStorage()
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
	if p.MaxBufferMemory <= 0 {
		return errors.New("max buffer memory must be greater than zero")
	}
	if p.MaxSpoolSize <= 0 {
		return errors.New("max spool size must be greater than zero")
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
	conID     int
	sessionID int
	code      int
	err       error
	stdout    resultOutput
	stderr    resultOutput
}

func (p *Pssh) newResult(conID, sessionID int) *result {
	return &result{
		conID:     conID,
		sessionID: sessionID,
		stdout:    newSpillBuffer(p.outputMemory, p.outputSpool, p.createOutputSpoolFile),
		stderr:    newSpillBuffer(p.outputMemory, p.outputSpool, p.createOutputSpoolFile),
	}
}

func (p *Pssh) delReslt(r *result) error {
	return errors.Join(r.stdout.Close(), r.stderr.Close())
}

func (p *Pssh) prepareOutputStorage() {
	p.outputMemory = newMemoryBudget(p.MaxBufferMemory)
	p.outputSpool = newMemoryBudget(p.MaxSpoolSize)
	p.outputSpoolOnce = sync.Once{}
	p.outputSpoolDir = ""
	p.outputSpoolErr = nil
}

func (p *Pssh) createOutputSpoolFile() (*os.File, error) {
	p.outputSpoolOnce.Do(func() {
		p.outputSpoolDir, p.outputSpoolErr = os.MkdirTemp(p.SpoolDir, "gopssh-*")
		if p.outputSpoolErr == nil {
			p.outputSpoolErr = os.Chmod(p.outputSpoolDir, 0o700)
		}
	})
	if p.outputSpoolErr != nil {
		return nil, p.outputSpoolErr
	}
	file, err := os.CreateTemp(p.outputSpoolDir, "output-*")
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	return file, nil
}

func (p *Pssh) cleanupOutputStorage() error {
	if p.outputSpoolDir == "" {
		return nil
	}
	err := os.RemoveAll(p.outputSpoolDir)
	p.outputSpoolDir = ""
	return err
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
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", fmt.Errorf("invalid port %q", port)
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
	return p.RunContext(context.Background())
}

// RunContext runs the main task until completion or context cancellation.
func (p *Pssh) RunContext(parent context.Context) int {
	if err := p.Validate(); err != nil {
		log.Printf("invalid config: %s", err)
		return one
	}
	p.prepareOutputStorage()
	defer func() {
		if err := p.cleanupOutputStorage(); err != nil {
			log.Printf("cleanup output spool err: %s", err)
		}
	}()
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
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	p.workerWG = sync.WaitGroup{}

	p.cws = make([]*conWork, len(hosts))
	for i, host := range hosts {
		p.cws[i] = p.newConWork(i, host)
	}
	p.workerWG.Add(len(p.cws))
	go p.launchConWorkers(ctx)

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
	p.workerWG.Wait()

	return code
}

func (p *Pssh) runConWorkers(ctx context.Context) int {
	p.workerWG.Add(len(p.cws))
	return p.launchConWorkers(ctx)
}

func (p *Pssh) launchConWorkers(ctx context.Context) int {
	for i, cw := range p.cws {
		if ctx.Err() != nil {
			p.finishUnlaunchedWorkers(i)
			return i
		}
		if p.Concurrency > 0 {
			select {
			case p.concurrentGoroutines <- struct{}{}:
			case <-ctx.Done():
				p.finishUnlaunchedWorkers(i)
				return i
			}
		}
		go func(cw *conWork) {
			defer p.workerWG.Done()
			if p.Concurrency > 0 {
				defer func() { <-p.concurrentGoroutines }()
			}
			cw.conWorker(ctx, p.clientConf)
		}(cw)
	}
	return len(p.cws)
}

func (p *Pssh) finishUnlaunchedWorkers(start int) {
	for range p.cws[start:] {
		p.workerWG.Done()
	}
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
				printErr := p.printResult(resSlise[j], cws[resSlise[j].conID].host)
				if firstCode == 0 && resSlise[j].code != 0 {
					firstCode = resSlise[j].code
				}
				if firstCode == 0 && printErr != nil {
					firstCode = one
				}
				cleanupErr := p.delReslt(resSlise[j])
				if firstCode == 0 && cleanupErr != nil {
					firstCode = one
				}
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
			printErr := p.printResult(res, cws[res.conID].host)
			if firstCode == 0 && res.code != 0 {
				firstCode = res.code
			}
			if firstCode == 0 && printErr != nil {
				firstCode = one
			}
			if cleanupErr := p.delReslt(res); firstCode == 0 && cleanupErr != nil {
				firstCode = one
			}
		case <-ctx.Done():
			firstCode = one
		}
	}
	return firstCode
}

func (p *Pssh) printResult(res *result, host string) error {
	var resultErr error
	if p.ShowHostName {
		var c prn
		if res.code != 0 || res.err != nil {
			c = p.boldRed
		} else {
			c = p.green
		}
		// nolint: errcheck,gosec
		_, resultErr = c.Printf("%s  result code %d\n", host, res.code)
	}
	if res.err != nil {
		// nolint: errcheck,gosec
		e := res.err.Error()
		if !strings.HasSuffix(e, "\n") {
			e += "\n"
		}
		_, _ = p.red.Printf("result err: %s", e)
	}
	if res.stdout.Size() > 0 {
		_, err := res.stdout.WriteTo(p.stdout)
		resultErr = errors.Join(resultErr, err)
	}
	if res.stderr.Size() > 0 {
		_, err := res.stderr.WriteTo(prnWriter{p.red})
		resultErr = errors.Join(resultErr, err)
	}
	if resultErr != nil {
		_, _ = p.red.Printf("local output err: %s\n", resultErr)
	}
	return resultErr
}

type client interface {
	NewSession() (*ssh.Session, error)
}

func readStream(out io.Writer, r io.Reader, errCh chan<- error) {
	buffer := copyBufferPool.Get().(*[]byte)
	_, err := io.CopyBuffer(out, r, *buffer)
	copyBufferPool.Put(buffer)
	errCh <- err
	close(errCh)
}

type prnWriter struct {
	prn
}

func (w prnWriter) Write(data []byte) (int, error) {
	_, err := w.Print(string(data))
	return len(data), err
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
