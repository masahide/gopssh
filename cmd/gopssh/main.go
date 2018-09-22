package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var (
	user  = flag.String("u", "", "user")
	port  = flag.String("P", "22", "port")
	stdin = flag.Bool("i", false, "stdin")
)

func run() int {
	socket := os.Getenv("SSH_AUTH_SOCK")
	authConn, err := net.Dial("unix", socket)
	if err != nil {
		log.Fatalf("net.Dial: %v", err)
	}
	agentClient := agent.NewClient(authConn)
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		return 2
	}

	config := &ssh.ClientConfig{
		User: *user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		Timeout:         5 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	cw := &conWork{
		host:     flag.Arg(0),
		port:     *port,
		commands: make(chan input),
		err:      make(chan result),
	}

	ctx, cancel := context.WithCancel(context.Background())

	go cw.conWorker(ctx, config)
	in := []byte{}
	if *stdin {
		in, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
	}
	results := make(chan result)
	cw.commands <- input{
		Command: strings.Join(flag.Args()[1:], " "),
		Stdin:   string(in),
		Results: results,
	}
	for res := range results {
		log.Printf("%#v", res)
	}
	cancel()
	return 0
}

type resType int

const (
	eof resType = iota
	stdErr
	stdOut
	sessionErr
	readErr
)

type result struct {
	serverID  int
	sessionID int
	resType
	data string
}
type input struct {
	Command string
	Stdin   string
	Results chan result
}

func (i input) sessionErr(ctx context.Context, m string) {
	select {
	case <-ctx.Done():
	case i.Results <- result{resType: sessionErr, data: m}:
		close(i.Results)
	}
	return
}

var bufPool = sync.Pool{
	New: func() interface{} {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return new(bytes.Buffer)
	},
}


type sessionWork struct{
	con *conWork
	buf *bytes.Buffer
}



type conWork struct {
	id       int
	host     string
	port     string
	commands chan input
	err      chan result
}

func (c *conWork) conWorker(ctx context.Context, config *ssh.ClientConfig) {
	hostport := fmt.Sprintf("%s:%s", c.host, c.port)
	conn, err := ssh.Dial("tcp", hostport, config)
	if err != nil {
		select {
		case <-ctx.Done():
		case c.err <- result{resType: sessionErr, data: fmt.Sprintf("cannot connect %v: %v", hostport, err)}:
		}
	}
	defer conn.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-c.commands:
			c.conSession(ctx, conn, cmd)
		}
	}
}

func (c *conWork) conSession(ctx context.Context, conn *ssh.Client, cmd input) {
	session, err := conn.NewSession()
	if err != nil {
		cmd.sessionErr(ctx, fmt.Sprintf("cannot open new session: %v", err))
		return
	}
	defer session.Close()
	defer close(cmd.Results)
	stdout, err := session.StdoutPipe()
	if err != nil {
		cmd.sessionErr(ctx, fmt.Sprintf("cannot open stdoutPipe: %v", err))
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		cmd.sessionErr(ctx, fmt.Sprintf("cannot open stderrPipe: %v", err))
		return
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go readLineWorker(ctx, cmd.Results, bufio.NewReader(stdout), stdOut, wg)
	go readLineWorker(ctx, cmd.Results, bufio.NewReader(stderr), stdErr, wg)
	session.Stdin = strings.NewReader(cmd.Stdin)
	err = session.Run(cmd.Command)
	if err != nil {
		if ee, ok := err.(*ssh.ExitError); ok {
			select {
			case <-ctx.Done():
			case cmd.Results <- result{resType: sessionErr, data: fmt.Sprintf("session Run: %v", ee)}:
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case cmd.Results <- result{resType: sessionErr, data: fmt.Sprintf("session Wait: %v", err)}:
		}
	}
	wg.Wait()
}

func readLineWorker(ctx context.Context, out chan result, r *bufio.Reader, t resType, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		line, err := r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			select {
			case <-ctx.Done(): case out <- result{resType: readErr, data: err.Error()}:
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case out <- result{resType: t, data: string(line)}:
		}
		if err == io.EOF {
			select {
			case <-ctx.Done():
			case out <- result{resType: eof, data: ""}:
			}
			return
		}
	}
}

func main() {
	os.Exit(run())
}
