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
	inputCh := make(chan Input)
	errCh := make(chan error)
	cw := &ConWork{
		Host:     flag.Arg(0),
		Port:     *port,
		Commands: inputCh,
		Err:      errCh,
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
	StdOut Type = iota
	StdErr
	SessionErr
	ReadErr
)

type Result struct {
	Type Type
	Data string
}
type Input struct {
	Command string
	Stdin   string
	Results chan Result
}

type ConWork struct {
	Host     string
	Port     string
	Commands chan Input
	Err      chan error
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
		select {
		case <-ctx.Done():
			return
		case cmd.Results <- Result{Type: SessionErr, Data: fmt.Sprintf("cannot open new session: %v", err)}:
		}
		close(cmd.Results)
		return
	}
	defer session.Close()
	defer close(cmd.Results)
	session.Stdout = readLineWorker(ctx, cmd.Results, StdOut)
	session.Stderr = readLineWorker(ctx, cmd.Results, StdErr)
	session.Stdin = strings.NewReader(cmd.Stdin)
	err = session.Run(cmd.Command)
	if err != nil {
		if ee, ok := err.(*ssh.ExitError); ok {
			select {
			case <-ctx.Done():
				return
			case cmd.Results <- Result{Type: SessionErr, Data: fmt.Sprintf("session Run: %v", ee)}:
			}
			return
		}
	}
	log.Print("wait")
	err = session.Wait()
	log.Print("end wait")
	if err != nil {
		select {
		case <-ctx.Done():
			return
		case cmd.Results <- Result{Type: SessionErr, Data: fmt.Sprintf("session Wait: %v", err)}:
		}
	}
}

func readLineWorker(ctx context.Context, out chan Result, t Type) io.Writer {
	pr, w := io.Pipe()
	r := bufio.NewReader(pr)
	go func() {
		defer w.Close()
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
				return
			}
		}
	}()
	return w
}

func main() {
	os.Exit(run())
}
