package pssh

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"golang.org/x/crypto/ssh"
)

type mockSess struct {
	err error
	res *result
}

func (s *mockSess) StderrPipe() (io.Reader, error) { return bytes.NewReader([]byte{}), nil }
func (s *mockSess) StdoutPipe() (io.Reader, error) { return bytes.NewReader([]byte{}), nil }
func (s *mockSess) Run(cmd string) error           { return s.err }
func (s *mockSess) Close() error                   { return nil }

func (s *mockSess) runner(ctx context.Context, res *result, session sess) {
	s.res = res
}

type mockClient struct {
	err error
}

func (c *mockClient) NewSession() (*ssh.Session, error) {
	return &ssh.Session{}, c.err
}

func TestWorker(t *testing.T) {
	p := &Pssh{Config: &Config{ColorMode: true}}
	p.Init()
	s := &sessionWork{
		id: 2,
		con: &conWork{
			Pssh:    p,
			id:      1,
			host:    "host1",
			command: make(chan input, 1),
		},
		input: &input{
			stdin:   "",
			results: make(chan *result, 10),
		},
	}
	m := &mockSess{}
	s.runner = m.runner
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.worker(ctx, &mockClient{err: errors.New("")})
	if m.res != nil {
		t.Error("m.res==nil, want:not nil")
	}
	s.worker(ctx, &mockClient{})
	if m.res == nil {
		t.Error("m.res!=nil, want:nil")
	}
}

func TestNewResult(t *testing.T) {
	s := &sessionWork{
		id: 2,
		con: &conWork{
			Pssh: &Pssh{Config: &Config{ColorMode: true}},
			id:   1,
		},
	}
	s.con.Init()
	r := s.newResult()
	if r.conID != 1 {
		t.Errorf("conID:%d, want %d", r.conID, 1)
	}
	if r.sessionID != 2 {
		t.Errorf("sessionID:%d, want %d", r.sessionID, 2)
	}
	s.con.delReslt(r)
}

func TestRun(t *testing.T) {
	p := &Pssh{Config: &Config{ColorMode: true}}
	p.Init()
	results := make(chan *result, 10)
	s := &sessionWork{
		id: 2,
		con: &conWork{
			Pssh:    p,
			id:      1,
			host:    "host1",
			command: make(chan input, 1),
		},
		input: &input{
			stdin:   "",
			results: results,
		},
	}
	res := s.newResult()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.run(ctx, res, &mockSess{})
	r := <-results
	if r.err != nil {
		t.Errorf("r.res:%s, want: nil", r.err)
	}
	s.run(ctx, res, &mockSess{err: errors.New("")})
	r = <-results
	if r.err == nil {
		t.Error("r.res!=nil, want:nil")
	}
	s.run(ctx, res, &mockSess{err: &ssh.ExitError{}})
	r = <-results
	if r.err == nil {
		t.Error("r.res!=nil, want:nil")
	}
}
