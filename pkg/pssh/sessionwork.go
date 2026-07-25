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
	Start(cmd string) error
	Wait() error
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
	if res.code == 0 {
		res.code = one
	}
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

	errs := []sessErr{
		{name: "stdoutStream err:", err: nil}, // 0
		{name: "stderrStream err:", err: nil}, // 1
		{name: "", err: nil},                  // 2
		{name: "I/O err:", err: nil},          // 3
	}
	if err = ctx.Err(); err != nil {
		errs[3].err = err
	} else if err = session.Start(s.command); err == nil {
		errChs := []chan error{make(chan error, 1), make(chan error, 1)}
		go readStream(res.stdout, stdout, errChs[0])
		go readStream(res.stderr, stderr, errChs[1])
		waitCh := make(chan error, 1)
		go func() {
			waitCh <- session.Wait()
		}()

		var waitErr error
		var interrupted bool
		select {
		case waitErr = <-waitCh:
		case outputErr := <-res.stdout.Fatal():
			interrupted = true
			errs[3].err = outputErr
			if closeErr := session.Close(); closeErr != nil {
				errs[3].err = errors.Join(errs[3].err, closeErr)
			}
			<-waitCh
		case outputErr := <-res.stderr.Fatal():
			interrupted = true
			errs[3].err = outputErr
			if closeErr := session.Close(); closeErr != nil {
				errs[3].err = errors.Join(errs[3].err, closeErr)
			}
			<-waitCh
		case <-ctx.Done():
			interrupted = true
			errs[3].err = ctx.Err()
			if closeErr := session.Close(); closeErr != nil {
				errs[3].err = errors.Join(errs[3].err, closeErr)
			}
			<-waitCh
		}
		if !interrupted && waitErr != nil {
			if ee, ok := waitErr.(*ssh.ExitError); ok {
				errs[2].err = ee
				res.code = ee.ExitStatus()
			} else {
				errs[3].err = waitErr
			}
		}
		for i := 0; i < len(errChs); i++ {
			errs[i].err = <-errChs[i]
		}
	} else {
		errs[3].err = err
	}
	stdoutOutputErr := res.stdout.Finalize()
	stderrOutputErr := res.stderr.Finalize()
	errs = append(errs,
		sessErr{name: "stdout output err:", err: stdoutOutputErr},
		sessErr{name: "stderr output err:", err: stderrOutputErr},
	)
	if res.code == 0 && (stdoutOutputErr != nil || stderrOutputErr != nil) {
		res.code = one
	}
	res.err = getAllError(errs)
	if res.code == 0 && res.err != nil {
		res.code = one
	}
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
	if ctx.Err() != nil {
		_ = s.con.delReslt(res)
		return
	}
	select {
	case <-ctx.Done():
		_ = s.con.delReslt(res)
	case s.results <- res:
	}
}
