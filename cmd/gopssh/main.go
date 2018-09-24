package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var (
	user         = flag.String("u", os.Getenv("USER"), "user")
	hostsfile    = flag.String("h", "", "host file")
	stdinFlag    = flag.Bool("i", false, "read stdin")
	showHostNmae = flag.Bool("n", false, "show hostname")
	colorMode    = flag.Bool("c", true, "colorized outputs")

	stdoutPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
	stderrPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

	red     = color.New()
	boldRed = color.New()
	green   = color.New()
)

func init() {
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *colorMode {
		red = color.New(color.FgRed)
		boldRed = color.New(color.FgRed).Add(color.Bold)
		green = color.New(color.FgGreen)
	}
}

type resType int

const (
	eof resType = iota
	sessionErr
	readErr
)

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

func newReslt(conID, sessionID int) *result {
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

func (s *sessionWork) newReslt() *result {
	return newReslt(s.con.id, s.id)
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

func run() int {
	hosts, err := readHosts(*hostsfile)
	if err != nil {
		log.Fatalf("read hosts file err: %s", err)
	}
	socket := os.Getenv("SSH_AUTH_SOCK")
	authConn, err := net.Dial("unix", socket)
	if err != nil {
		log.Fatalf("net.Dial: %v", err)
	}
	agentClient := agent.NewClient(authConn)

	config := &ssh.ClientConfig{
		User: *user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		Timeout:         5 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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
		go cws[i].conWorker(ctx, config, conInstances)

	}
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
	if *stdinFlag {
		stdin, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
	}
	results := make(chan *result, len(hosts))
	in := input{
		command: strings.Join(flag.Args(), " "),
		stdin:   string(stdin),
		results: results,
	}
	for i := range cws {
		cws[i].command <- in
	}
	for i := 0; i < len(hosts); i++ {
		select {
		case res := <-results:
			if *showHostNmae {
				host := cws[res.conID].host
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
				fmt.Print(res.stdout.String())
			}
			if res.stderr.Len() > 0 {
				red.Print(res.stderr.String())
			}
			delReslt(res)
		case <-ctx.Done():
		}
	}

	cancel()

	return 0

}

/*
func {
	paramCh := make(chan conInstance, 1)
	in <- getInstances{1, paramCh}
	for i := range paramCh {
	}
}
*/

type getInstances struct {
	id  int
	res chan<- conInstance
}

func (c *conWork) conWorker(ctx context.Context, config *ssh.ClientConfig, instanceCh chan<- conInstance) {
	res := conInstance{conWork: c, err: nil}
	conn, err := ssh.Dial("tcp", c.host, config)
	if err != nil {
		res.err = fmt.Errorf("cannot connect [%s] err:%s", c.host, err)
		select {
		case <-ctx.Done():
		case instanceCh <- res:
		}
		return
	}
	defer conn.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-c.command:
			s := sessionWork{id: cmd.id, input: &cmd, con: c}
			go s.worker(ctx, conn)
		}
	}
}

func (s *sessionWork) worker(ctx context.Context, conn *ssh.Client) {
	res := s.newReslt()
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

func main() {
	os.Exit(run())
}
