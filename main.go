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
	cw := &ConWork{
		Host:     flag.Arg(0),
		Port:     *port,
		Commands: make(chan Input),
		Err:      make(chan Result),
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
	results := make(chan Result)
	cw.Commands <- Input{
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

type Type int

const (
	EOF Type = iota
	StdErr
	SessionErr
	ReadErr
)

type Result struct {
	ServerID  int
	SessionID int
	Type      Type
	Data      string
}
type Input struct {
	Command string
	Stdin   string
	Results chan Result
}

func (i Input) SessionErr(ctx context.Context, m string) {
	select {
	case <-ctx.Done():
	case i.Results <- Result{Type: SessionErr, Data: m}:
		close(i.Results)
	}
	return
}

type ConWork struct {
	Id       int
	Host     string
	Port     string
	Commands chan Input
	Err      chan Result
}

func (c *ConWork) conWorker(ctx context.Context, config *ssh.ClientConfig) {
	hostport := fmt.Sprintf("%s:%s", c.Host, c.Port)
	conn, err := ssh.Dial("tcp", hostport, config)
	if err != nil {
		select {
		case <-ctx.Done():
		case c.Err <- fmt.Errorf("cannot connect %v: %v", hostport, err):
		}
	}
	defer conn.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-c.Commands:
			c.conSession(ctx, conn, cmd)
		}
	}
}

func (c *ConWork) conSession(ctx context.Context, conn *ssh.Client, cmd Input) {
	session, err := conn.NewSession()
	if err != nil {
		cmd.SessionErr(ctx, fmt.Sprintf("cannot open new session: %v", err))
		return
	}
	defer session.Close()
	defer close(cmd.Results)
	stdout, err := session.StdoutPipe()
	if err != nil {
		cmd.SessionErr(ctx, fmt.Sprintf("cannot open stdoutPipe: %v", err))
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		cmd.SessionErr(ctx, fmt.Sprintf("cannot open stderrPipe: %v", err))
		return
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go readLineWorker(ctx, cmd.Results, bufio.NewReader(stdout), StdOut, wg)
	go readLineWorker(ctx, cmd.Results, bufio.NewReader(stderr), StdErr, wg)
	session.Stdin = strings.NewReader(cmd.Stdin)
	err = session.Run(cmd.Command)
	if err != nil {
		if ee, ok := err.(*ssh.ExitError); ok {
			select {
			case <-ctx.Done():
			case cmd.Results <- Result{Type: SessionErr, Data: fmt.Sprintf("session Run: %v", ee)}:
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case cmd.Results <- Result{Type: SessionErr, Data: fmt.Sprintf("session Wait: %v", err)}:
		}
	}
	wg.Wait()
}

func readLineWorker(ctx context.Context, out chan Result, r bufio.Reader, t Type, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		line, err := r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			select {
			case <-ctx.Done():
			case out <- Result{Type: ReadErr, Data: err.Error()}:
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case out <- Result{Type: t, Data: string(line)}:
		}
		if err == io.EOF {
			select {
			case <-ctx.Done():
			case out <- Result{Type: EOF, Data: ""}:
			}
			return
		}
	}
}

func main() {
	os.Exit(run())
}
