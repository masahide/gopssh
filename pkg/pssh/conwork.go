package pssh

import (
	"context"
	"fmt"
	"log"

	"golang.org/x/crypto/ssh"
)

type conWork struct {
	*Pssh
	id           int
	host         string
	command      chan input
	startSession func(ctx context.Context, conn sshClientIface, cmd input)
}

// TemporaryError is network error
type TemporaryError interface {
	Temporary() bool
}

/*
func (c *conWork) dialSocket(authConn *net.Conn, socket string) error {
	// https://stackoverflow.com/questions/30228482/golang-unix-socket-error-dial-resource-temporarily-unavailable
	return backoff.Retry(func() error {
		var err error
		*authConn, err = c.netDialer.Dial("unix", socket)
		if err != nil {
			if terr, ok := err.(TemporaryError); ok && terr.Temporary() {
				return err
			}
		}
		return nil
	}, backoff.NewExponentialBackOff())
}
*/

func (c *conWork) conWorker(ctx context.Context, config ssh.ClientConfig, socket string, instanceCh chan<- conInstance) {
	if c.Pssh == nil {
		return
	}
	if c.Concurrency > 0 {
		defer func() { <-c.concurrentGoroutines }()
	}
	sshKeyAgent, authMethods := c.merageAuthMethods(c.getIdentFileAuthMethods(c.identFileData))
	if sshKeyAgent != nil {
		// nolint: errcheck
		defer sshKeyAgent.close()
	}
	config.Auth = authMethods
	/*
		var authConn net.Conn
		if err := c.dialSocket(&authConn, socket); err != nil {
			log.Fatalf("net.Dial: %v", err)
		}
		// nolint: errcheck
		defer authConn.Close()
		agentClient := agent.NewClient(authConn)
		config.Auth = []ssh.AuthMethod{ssh.PublicKeysCallback(agentClient.Signers)}
	*/

	res := conInstance{conWork: c, err: nil}
	if c.Debug {
		log.Printf("start ssh.Dial : %s", c.host)
	}
	conn, err := c.sshDialer.Dial("tcp", c.host, &config)
	if err != nil {
		res.err = fmt.Errorf("cannot connect [%s] err:%s", c.host, err)
		select {
		case <-ctx.Done():
		case instanceCh <- res:
		}
		return
	}
	// nolint: errcheck
	defer conn.Close()
	c.commandLoop(ctx, conn, true)
}

func (c *conWork) commandLoop(ctx context.Context, conn sshClientIface, loop bool) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-c.command:
			c.startSession(ctx, conn, cmd)
		}
		if !loop {
			return
		}
	}
}

func (c *conWork) startSessionWorker(ctx context.Context, conn sshClientIface, cmd input) {
	s := &sessionWork{id: cmd.id, input: &cmd, con: c}
	s.runner = s.run
	s.worker(ctx, conn)
}
