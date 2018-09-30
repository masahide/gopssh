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

type print struct{}

func (p print) Print(a ...interface{}) (n int, err error) {
	return fmt.Print(a)
}
func (p print) Printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Printf(format, a)
}

// Pssh pssh struct
type Pssh struct {
	*Config
	red                  prn
	boldRed              prn
	green                prn
	concurrentGoroutines chan struct{}
	stdoutPool           sync.Pool
	stderrPool           sync.Pool
}

// Config pssh config
type Config struct {
	Concurrency   int
	User          string
	Hostsfile     string
	ShowHostNmae  bool
	ColorMode     bool
	IgnoreHostKey bool
	Debug         bool
	Timeout       time.Duration
	KexFlag       string

	// ciphers
	kex     []string
	ciphers []string
	macs    []string
}

var (
	concurrency   = flag.Int("p", 0, "concurrency (defalut \"0\" is unlimit)")
	user          = flag.String("u", os.Getenv("USER"), "user")
	hostsfile     = flag.String("h", "", "host file")
	showHostNmae  = flag.Bool("d", false, "show hostname")
	colorMode     = flag.Bool("c", false, "colorized outputs")
	ignoreHostKey = flag.Bool("k", false, "Do not check the host key")
	debug         = flag.Bool("debug", false, "debug outputs")
	timeout       = flag.Duration("timeout", 5*time.Second, "maximum amount of time for the TCP connection to establish.")
	kexFlag       = flag.String("kex",
		"diffie-hellman-group1-sha1,diffie-hellman-group14-sha1,ecdh-sha2-nistp256,ecdh-sha2-nistp384,ecdh-sha2-nistp521,curve25519-sha256@libssh.org",
		"allowed key exchanges algorithms",
	)
	ciphersFlag = flag.String("ciphers",
		"arcfour256,aes128-gcm@openssh.com,chacha20-poly1305@openssh.com,aes128-ctr,aes192-ctr,aes256-ctr",
		"allowed cipher algorithms")
	macsFlag = flag.String("macs",
		"hmac-sha1-96,hmac-sha1,hmac-sha2-256,hmac-sha2-256-etm@openssh.com",
		"allowed MAC algorithms")
	// "ssh-rsa,ssh-dss,ecdsa-sha2-nistp256,ecdsa-sha2-nistp384,ecdsa-sha2-nistp521,ssh-ed25519"

	stdoutPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
	stderrPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

	sshAuthSocket = os.Getenv("SSH_AUTH_SOCK")

	red                  = color.New()
	boldRed              = color.New()
	green                = color.New()
	concurrentGoroutines chan struct{}
)

func toSlice(s string) []string {
	return strings.Split(s, ",")
}

// Init Pssh
func (p *Pssh) Init() {
	p.concurrentGoroutines = make(chan struct{}, p.Concurrency)
	p.red = print{}
	p.boldRed = print{}
	p.green = print{}
	if p.ColorMode {
		p.red = color.New(color.FgRed)
		p.boldRed = color.New(color.FgRed).Add(color.Bold)
		p.green = color.New(color.FgGreen)
	}
	p.stdoutPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
	p.stderrPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
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

type sessionWork struct {
	id int
	*input
	con *conWork
}
type conWork struct {
	id      int
	host    string
	command chan input
}

type conInstance struct {
	*conWork
	err error
}

func newResult(conID, sessionID int) *result {
	r := &result{
		conID:     conID,
		sessionID: sessionID,
		stdout:    stdoutPool.Get().(*bytes.Buffer),
		stderr:    stderrPool.Get().(*bytes.Buffer),
	}
	r.stdout.Reset()
	r.stderr.Reset()
	return r
}
func delReslt(r *result) {
	stdoutPool.Put(r.stdout)
	stderrPool.Put(r.stderr)
}

func (s *sessionWork) newResult() *result {
	return newResult(s.con.id, s.id)
}

var re = regexp.MustCompile(":.+")

func readHosts(fileName string) ([]string, error) {
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

func getHostKeyCallback(insecure bool) ssh.HostKeyCallback {
	if insecure {
		return ssh.InsecureIgnoreHostKey()
	}
	file := path.Join(os.Getenv("HOME"), ".ssh/.ssh/known_hosts")
	cb, err := knownhosts.New(file)
	if err != nil {
		errors.Wrap(err, "knownhosts.New")
	}
	return cb
}

func (p *Pssh) run() int {
	hosts, err := readHosts(*hostsfile)
	if err != nil {
		log.Fatalf("read hosts file err: %s", err)
	}
	config := ssh.ClientConfig{
		User: *user,
		//Auth: []ssh.AuthMethod{ ssh.PublicKeysCallback(agentClient.Signers), },
		Timeout:         *timeout,
		HostKeyCallback: getHostKeyCallback(*ignoreHostKey),
		Config:          ssh.Config{KeyExchanges: p.kex, Ciphers: p.ciphers, MACs: p.macs},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conInstances := make(chan conInstance, len(hosts))
	cws := make([]*conWork, len(hosts))
	for i, host := range hosts {
		cws[i] = &conWork{
			id:      i,
			host:    host,
			command: make(chan input, 1),
		}
	}
	go func() {
		for i := range cws {
			if *concurrency > 0 {
				concurrentGoroutines <- struct{}{}
			}
			go cws[i].conWorker(ctx, config, sshAuthSocket, conInstances)
		}
	}()

	go func() {
		for con := range conInstances {
			if con.err != nil {
				log.Printf("host:%s err:%s", con.host, con.err)
				cancel()
				break
			}
		}
	}()

	stdin := []byte{}
	//if *stdinFlag {
	stdin, err = ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	//}
	results := make(chan *result, len(hosts))
	in := input{
		command: strings.Join(flag.Args(), " "),
		stdin:   string(stdin),
		results: results,
	}
	for i := range cws {
		cws[i].command <- in
	}
	printResults(ctx, results, cws)
	cancel()

	return 0

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

func printResults(ctx context.Context, results chan *result, cws []*conWork) {
	for i := 0; i < len(cws); i++ {
		select {
		case res := <-results:
			printResult(res, cws[res.conID].host)
			delReslt(res)
		case <-ctx.Done():
		}
	}
}

func printResult(res *result, host string) {
	if *showHostNmae {
		var c *color.Color
		if res.code != 0 || res.err != nil {
			c = boldRed
		} else {
			c = green
		}
		c.Printf("%s  reslut code %d\n", host, res.code)
	}
	if res.err != nil {
		red.Printf("result err: %s", res.err)
	}
	if res.stdout.Len() > 0 {
		res.stdout.WriteTo(os.Stdout)
	}
	if res.stderr.Len() > 0 {
		red.Print(res.stderr.String())
	}
}

type getInstances struct {
	id  int
	res chan<- conInstance
}

// TemporaryError is network error
type TemporaryError interface {
	Temporary() bool
}

func (c *conWork) conWorker(ctx context.Context, config ssh.ClientConfig, socket string, instanceCh chan<- conInstance) {
	if *concurrency > 0 {
		defer func() { <-concurrentGoroutines }()
	}
	// https://stackoverflow.com/questions/30228482/golang-unix-socket-error-dial-resource-temporarily-unavailable
	var authConn net.Conn
	err := backoff.Retry(func() error {
		var err error
		authConn, err = net.Dial("unix", socket)
		if err != nil {
			if terr, ok := err.(TemporaryError); ok && terr.Temporary() {
				return err
			}
		}
		return nil
	}, backoff.NewExponentialBackOff())
	if err != nil {
		log.Fatalf("net.Dial: %v", err)
	}
	defer authConn.Close()
	agentClient := agent.NewClient(authConn)
	config.Auth = []ssh.AuthMethod{ssh.PublicKeysCallback(agentClient.Signers)}

	res := conInstance{conWork: c, err: nil}
	if *debug {
		log.Printf("start ssh.Dial : %s", c.host)
	}
	conn, err := ssh.Dial("tcp", c.host, &config)
	if err != nil {
		res.err = fmt.Errorf("cannot connect [%s] err:%s", c.host, err)
		select {
		case <-ctx.Done():
		case instanceCh <- res:
		}
		return
	}
	if *debug {
		log.Printf("done ssh.Dial : %s", c.host)
	}
	defer conn.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-c.command:
			s := sessionWork{id: cmd.id, input: &cmd, con: c}
			s.worker(ctx, conn)
			return
		}
	}
}

func (s *sessionWork) worker(ctx context.Context, conn *ssh.Client) {
	res := s.newResult()
	session, err := conn.NewSession()
	if err != nil {
		res.err = fmt.Errorf("cannot open new session: %v", err)
		s.errResult(ctx, res)
		return
	}
	defer session.Close()
	stdout, err := session.StdoutPipe()
	if err != nil {
		res.err = fmt.Errorf("cannot open stdoutPipe: %v", err)
		s.errResult(ctx, res)
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		res.err = fmt.Errorf("cannot open stderrPipe: %v", err)
		s.errResult(ctx, res)
		return
	}

	errCh := make(chan error)
	go readStream(ctx, res.stdout, stdout, errCh)
	go readStream(ctx, res.stderr, stderr, errCh)
	session.Stdin = strings.NewReader(s.stdin)
	err = session.Run(s.command)
	if err != nil {
		if ee, ok := err.(*ssh.ExitError); ok {
			res.err = errors.New(ee.Msg())
			res.code = ee.ExitStatus()
			s.errResult(ctx, res)
			return
		}
		res.err = fmt.Errorf("session Wait: %v", err)
	}
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		errs[i] = nil
		select {
		case errs[i] = <-errCh:
		case <-ctx.Done():
			return
		}
	}
	for _, err := range errs {
		if err != nil {
			res.err = err
			break
		}
	}
	s.errResult(ctx, res)
	return

}
func (s *sessionWork) errResult(ctx context.Context, res *result) {
	select {
	case <-ctx.Done():
	case s.results <- res:
	}
	return
}

func readStream(ctx context.Context, out io.Writer, r io.Reader, errCh chan<- error) {
	_, err := io.Copy(out, r)
	select {
	case errCh <- err:
	case <-ctx.Done():
	}
}
