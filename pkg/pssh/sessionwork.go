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

type sessErr struct {
	name string
	err  error
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

	errChs := []chan error{make(chan error, 1), make(chan error, 1)}
	errs := []sessErr{
		{name: "stdoutStream err:", err: nil}, // 0
		{name: "stderrStream err:", err: nil}, // 1
		{name: "", err: nil},                  // 2
		{name: "I/O err:", err: nil},          // 3
	}
	go readStream(ctx, res.stdout, stdout, errChs[0])
	go readStream(ctx, res.stderr, stderr, errChs[1])
	err = session.Run(s.command)
	if err != nil {
		if ee, ok := err.(*ssh.ExitError); ok {
			errs[2].err = ee
			res.code = ee.ExitStatus()
		} else {
			errs[3].err = err
		}
	}
	for i := 0; i < len(errChs); i++ {
		errs[i].err = getErr(ctx, errChs[i])
	}
	res.err = getAllError(errs)
	s.errResult(ctx, res)
}
func getAllError(errs []sessErr) error {
	s := make([]string, 0, len(errs))
	for _, e := range errs {
		if e.err != nil {
			s = append(s, e.name+e.err.Error())
		}
	}
	if len(s) > 0 {
		return errors.New(strings.Join(s, "\n"))
	}
	return nil
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
