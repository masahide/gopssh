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

func (c *conWork) conWorker(ctx context.Context, config ssh.ClientConfig) {
	if c.Pssh == nil {
		return
	}
	if c.Concurrency > 0 {
		defer func() { <-c.concurrentGoroutines }()
	}
	authMethods := c.mergeAuthMethods(c.getIdentFileAuthMethods(c.identFileData))
	config.Auth = authMethods
	if c.Debug {
		log.Printf("start ssh.Dial : %s", c.host)
	}
	conn, err := c.sshDialer.Dial("tcp", c.host, &config)
	if err != nil {
		select {
		case <-ctx.Done():
		case cmd := <-c.command:
			res := c.newResult(c.id, cmd.id)
			res.code = connectFailureCode
			res.err = fmt.Errorf("cannot connect [%s]: %w", c.host, err)
			select {
			case <-ctx.Done():
				c.delReslt(res)
			case cmd.results <- res:
			}
		}
		return
	}
	// nolint: errcheck
	defer conn.Close()
	c.commandLoop(ctx, conn, false)
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
