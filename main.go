package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var (
	user = flag.String("u", "", "user")
	port = flag.Int("P", 22, "port")
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

	hostport := fmt.Sprintf("%s:%d", flag.Arg(0), *port)
	conn, err := ssh.Dial("tcp", hostport, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot connect %v: %v", hostport, err)
		return 1
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open new session: %v", err)
		return 1
	}
	defer session.Close()

	go func() {
		time.Sleep(5 * time.Second)
		conn.Close()
	}()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin
	err = session.Run(strings.Join(flag.Args()[1:], " "))
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		if ee, ok := err.(*ssh.ExitError); ok {
			return ee.ExitStatus()
		}
		return 1
	}
	return 0
}

func main() {
	os.Exit(run())
}
