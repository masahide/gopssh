package pssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/ssh"
)

type sess interface {
	StderrPipe() (io.Reader, error)
	StdoutPipe() (io.Reader, error)
	Run(cmd string) error
	Close() error
}

type sessionWork struct {
	id int
	*input
	con    *conWork
	runner func(ctx context.Context, res *result, session sess)
}

func (s *sessionWork) newResult() *result {
	return s.con.newResult(s.con.id, s.id)
}

func (s *sessionWork) getPipe(ctx context.Context, pipe func() (io.Reader, error), res *result, name string) (io.Reader, error) {
	out, err := pipe()
	if err != nil {
		s.result(ctx, fmt.Errorf("cannot open %sPipe: %v", name, err), res)
	}
	return out, err
}

func (s *sessionWork) result(ctx context.Context, err error, res *result) {
	res.err = err
	s.errResult(ctx, res)
}

func (s *sessionWork) run(ctx context.Context, res *result, session sess) {
	// nolint: errcheck,gosec
	defer session.Close()
	stdout, err := s.getPipe(ctx, session.StdoutPipe, res, "stdout")
	if err != nil {
		return
	}
	stderr, err := s.getPipe(ctx, session.StderrPipe, res, "stderr")
	if err != nil {
		return
	}

	errCh := make(chan error)
	go readStream(ctx, res.stdout, stdout, errCh)
	go readStream(ctx, res.stderr, stderr, errCh)
	err = session.Run(s.command)
	if err != nil {
		if ee, ok := err.(*ssh.ExitError); ok {
			res.err = errors.New(ee.Msg())
			res.code = ee.ExitStatus()
			s.errResult(ctx, res)
			return
		}
		res.err = fmt.Errorf("session Wait: %v", err)
		s.errResult(ctx, res)
		return
	}
	res.err = getFristErr(getErrs(ctx, errCh))
	s.errResult(ctx, res)
}

func (s *sessionWork) worker(ctx context.Context, conn client) {
	res := s.newResult()
	session, err := conn.NewSession()
	if err != nil {
		s.result(ctx, fmt.Errorf("cannot open new session: %v", err), res)
		return
	}
	// nolint: errcheck
	session.Stdin = strings.NewReader(s.stdin)
	s.runner(ctx, res, session)
}

func (s *sessionWork) errResult(ctx context.Context, res *result) {
	select {
	case <-ctx.Done():
	case s.results <- res:
	}
}
