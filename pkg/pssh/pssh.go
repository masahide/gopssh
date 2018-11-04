package pssh

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type prn interface {
	Print(a ...interface{}) (n int, err error)
	Printf(format string, a ...interface{}) (n int, err error)
}

type print struct {
	colorMode bool
	output    io.Writer
	red       prn
	boldRed   prn
	green     prn
}

// ToSlice comma separated to slice
func ToSlice(s string) []string {
	return strings.Split(s, ",")
}

func newPrint(output io.Writer, colorMode bool) *print {
	p := &print{
		output:    output,
		colorMode: colorMode,
	}
	p.init()
	return p
}

func (p *print) init() {
	if p.colorMode {
		p.red = color.New(color.FgRed)
		p.boldRed = color.New(color.FgRed).Add(color.Bold)
		p.green = color.New(color.FgGreen)
		return
	}
	p.red = &print{}
	p.boldRed = &print{}
	p.green = &print{}
}

func (p *print) Print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(p.output, a...)
}
func (p *print) Printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(p.output, format, a...)
}

type dialIface interface {
	Dial(network, address string) (net.Conn, error)
}
type netDial struct{}

func (n netDial) Dial(network, address string) (net.Conn, error) { return net.Dial(network, address) }

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
	netDialer     dialIface
	sshDialer     sshDialIface
	conInstances  chan conInstance
	cws           []*conWork
	clientConf    ssh.ClientConfig
	identFileData [][]byte
}

// Config pssh config
type Config struct {
	Concurrency      int
	User             string
	Hostsfile        string
	ShowHostName     bool
	ColorMode        bool
	IgnoreHostKey    bool
	Debug            bool
	StdinFlag        bool
	IdentityFileOnly bool
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
	p.concurrentGoroutines = make(chan struct{}, p.Concurrency)
	p.print = newPrint(os.Stdout, p.ColorMode)
	p.stdoutPool = sync.Pool{New: newBytesBuf}
	p.stderrPool = sync.Pool{New: newBytesBuf}
	p.netDialer = netDial{}
	p.sshDialer = sshDial{}
	p.identFileData = p.readIdentFiles()
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
	stdout    *bytes.Buffer
	stderr    *bytes.Buffer
}

type conInstance struct {
	*conWork
	err error
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

var re = regexp.MustCompile(":.+")

func readHosts(fileName string) ([]string, error) {
	// nolint: gosec
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	lines := bytes.Fields(data)
	res := make([]string, len(lines))
	for i := range lines {
		res[i] = string(lines[i])
		if !re.MatchString(res[i]) {
			res[i] = res[i] + ":22"
		}

	}
	return res, nil
}

func getHostKeyCallback(insecure bool) (ssh.HostKeyCallback, error) {
	if insecure {
		// nolint: gosec
		return ssh.InsecureIgnoreHostKey(), nil
	}
	file := path.Join(os.Getenv("HOME"), ".ssh/known_hosts")
	cb, err := knownhosts.New(file)
	if err != nil {
		return nil, errors.Wrap(err, "knownhosts.New")
	}
	return cb, nil
}

func (p *Pssh) newConWork(id int, host string) *conWork {
	c := &conWork{Pssh: p, id: id, host: host, command: make(chan input, 1)}
	c.startSession = c.startSessionWorker
	return c
}

// Run main task
func (p *Pssh) Run() int {
	hosts, err := readHosts(p.Hostsfile)
	if err != nil {
		// nolint: errcheck,gosec
		log.Printf("read hosts file err: %s", err)
		return 1
	}
	hc, err := getHostKeyCallback(p.IgnoreHostKey)
	if err != nil {
		// nolint: errcheck,gosec
		log.Printf("read hosts file err: %s", err)
		return 1
	}
	p.clientConf = ssh.ClientConfig{
		User: p.User,
		//Auth:            p.getAuthMethods(),
		Timeout:         p.Timeout,
		HostKeyCallback: hc,
		Config:          ssh.Config{KeyExchanges: p.Config.Kex, Ciphers: p.Config.Ciphers, MACs: p.Config.Macs},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.conInstances = make(chan conInstance, len(hosts))
	p.cws = make([]*conWork, len(hosts))
	for i, host := range hosts {
		p.cws[i] = p.newConWork(i, host)
	}
	go p.runConWorkers(ctx)
	go func() {
		if err := p.getConInstanceErrs(); err != nil {
			log.Print(err)
			cancel()
		}
	}()

	stdin := []byte{}
	if p.StdinFlag {
		if stdin, err = ioutil.ReadAll(os.Stdin); err != nil {
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
	p.printResults(ctx, results, p.cws)
	cancel()

	return 0

}

func (p *Pssh) runConWorkers(ctx context.Context) int {
	for i := range p.cws {
		if p.Concurrency > 0 {
			p.concurrentGoroutines <- struct{}{}
		}
		go p.cws[i].conWorker(ctx, p.clientConf, p.conInstances)
	}
	return len(p.cws)
}

func (p *Pssh) getConInstanceErrs() error {
	for con := range p.conInstances {
		if con.err != nil {
			// nolint: errcheck,gosec
			return fmt.Errorf("host:%s err:%s", con.host, con.err)
		}
	}
	return nil
}

/*
func printSortResults(ctx context.Context, results chan *result, cws []*conWork) {
	resSlise := make([]*result, len(cws))
	cur := 0
L1:
	for i := 0; i < len(cws); i++ {
		select {
		case res := <-results:
		L2:
			for i = cur;i<=res.conID;i++ {
				cws[i] == nil{
					break L2
				}
			}
			if res.conID == cur {
				printResult(res, cws[res.conID].host)
				delReslt(res)
				cur++
				continue L1
			}
			resSlise[res.conID] = res
		case <-ctx.Done():
		}
	}
}
*/

func (p *Pssh) printResults(ctx context.Context, results chan *result, cws []*conWork) {
	for i := 0; i < len(cws); i++ {
		select {
		case res := <-results:
			p.printResult(res, cws[res.conID].host)
			p.delReslt(res)
		case <-ctx.Done():
		}
	}
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
		c.Printf("%s  reslut code %d\n", host, res.code)
	}
	if res.err != nil {
		// nolint: errcheck,gosec
		p.red.Printf("result err: %s", res.err)
	}
	if res.stdout.Len() > 0 {
		// nolint: errcheck,gosec
		res.stdout.WriteTo(os.Stdout)
	}
	if res.stderr.Len() > 0 {
		// nolint: errcheck,gosec
		p.red.Print(res.stderr.String())
	}
}

type client interface {
	NewSession() (*ssh.Session, error)
}

func getErrs(ctx context.Context, errCh <-chan error) []error {
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		errs[i] = nil
		select {
		case errs[i] = <-errCh:
		case <-ctx.Done():
			return errs
		}
	}
	return errs
}
func getFristErr(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func readStream(ctx context.Context, out io.Writer, r io.Reader, errCh chan<- error) {
	_, err := io.Copy(out, r)
	select {
	case errCh <- err:
	case <-ctx.Done():
	}
}

func (p *Pssh) dialSocket(authConn *net.Conn, socket string) error {
	// https://stackoverflow.com/questions/30228482/golang-unix-socket-error-dial-resource-temporarily-unavailable
	return backoff.Retry(func() error {
		var err error
		*authConn, err = p.netDialer.Dial("unix", socket)
		if err != nil {
			if terr, ok := err.(TemporaryError); ok && terr.Temporary() {
				return err
			}
		}
		return nil
	}, backoff.NewExponentialBackOff())
}

type sshKeyAgent struct {
	ssh.AuthMethod
	authConn net.Conn
}

func (s *sshKeyAgent) close() error {
	return s.authConn.Close()
}

func (p *Pssh) sshKeyAgentCallback() *sshKeyAgent {
	if len(p.SSHAuthSocket) == 0 {
		return nil
	}
	res := sshKeyAgent{}
	if err := p.dialSocket(&res.authConn, p.SSHAuthSocket); err != nil {
		// log.Fatalf("net.Dial: %v", err)
		return nil
	}
	// nolint: errcheck
	agentClient := agent.NewClient(res.authConn)
	res.AuthMethod = ssh.PublicKeysCallback(agentClient.Signers)
	return &res
}

func (p *Pssh) mergeAuthMethods(identMethods []ssh.AuthMethod) (*sshKeyAgent, []ssh.AuthMethod) {
	res := make([]ssh.AuthMethod, 0, len(identMethods)+1)
	var keyAgentMehod *sshKeyAgent
	if !p.IdentityFileOnly {
		if keyAgentMehod = p.sshKeyAgentCallback(); keyAgentMehod != nil {
			res = append(res, keyAgentMehod)
		}
	}
	return keyAgentMehod, append(res, identMethods...)
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
		filePath = strings.Replace(filePath, "~", home, 1)
		buffer, err := ioutil.ReadFile(filePath)
		if err != nil {
			continue
		}
		res = append(res, buffer)
	}
	return res
}
