package pssh

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type mockSess struct {
	err    error
	res    *result
	stdout []byte
	stderr []byte
}

func (s *mockSess) StderrPipe() (io.Reader, error) { return bytes.NewReader(s.stderr), nil }
func (s *mockSess) StdoutPipe() (io.Reader, error) { return bytes.NewReader(s.stdout), nil }
func (s *mockSess) Start(cmd string) error         { return nil }
func (s *mockSess) Wait() error                    { return s.err }
func (s *mockSess) Close() error                   { return nil }

func (s *mockSess) runner(ctx context.Context, res *result, session sess) {
	s.res = res
}

type mockClient struct {
	err error
}

type endlessReader struct {
	done <-chan struct{}
}

func (r *endlessReader) Read(data []byte) (int, error) {
	select {
	case <-r.done:
		return 0, io.EOF
	default:
		for i := range data {
			data[i] = 'x'
		}
		return len(data), nil
	}
}

type blockingReader struct {
	done <-chan struct{}
}

func (r *blockingReader) Read([]byte) (int, error) {
	<-r.done
	return 0, io.EOF
}

type blockingSess struct {
	done       chan struct{}
	started    chan struct{}
	closed     chan struct{}
	closeOnce  sync.Once
	withOutput bool
}

func newBlockingSess(withOutput bool) *blockingSess {
	return &blockingSess{
		done:       make(chan struct{}),
		started:    make(chan struct{}),
		closed:     make(chan struct{}),
		withOutput: withOutput,
	}
}

func (s *blockingSess) StderrPipe() (io.Reader, error) {
	return bytes.NewReader(nil), nil
}

func (s *blockingSess) StdoutPipe() (io.Reader, error) {
	if s.withOutput {
		return &endlessReader{done: s.done}, nil
	}
	return &blockingReader{done: s.done}, nil
}

func (s *blockingSess) Start(string) error {
	close(s.started)
	return nil
}

func (s *blockingSess) Wait() error {
	<-s.done
	return errors.New("session closed")
}

func (s *blockingSess) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		close(s.closed)
	})
	return nil
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
	_ = s.con.delReslt(r)
}

func TestRun(t *testing.T) {
	p := &Pssh{Config: &Config{
		ColorMode:       true,
		MaxBufferMemory: DefaultMaxBufferMemory,
		MaxSpoolSize:    DefaultMaxSpoolSize,
	}}
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

func TestOutputLimitReturnsNonzero(t *testing.T) {
	p := &Pssh{Config: &Config{
		ColorMode:       false,
		MaxBufferMemory: outputChunkSize,
		MaxSpoolSize:    1,
	}}
	p.Init()
	t.Cleanup(func() {
		_ = p.cleanupOutputStorage()
	})
	results := make(chan *result, 1)
	s := &sessionWork{
		con: &conWork{Pssh: p},
		input: &input{
			results: results,
		},
	}
	res := s.newResult()
	s.run(context.Background(), res, &mockSess{stdout: bytes.Repeat([]byte("x"), int(outputChunkSize+1))})
	got := <-results
	if got.code != one {
		t.Errorf("code=%d, want %d", got.code, one)
	}
	if got.err == nil || !strings.Contains(got.err.Error(), "maximum spool size") {
		t.Fatalf("err=%v, want maximum spool size error", got.err)
	}
	outputs := make(chan *result, 1)
	outputs <- got
	p.print = newPrint(io.Discard, io.Discard, false)
	if code := p.printSortResults(context.Background(), outputs, []*conWork{{host: "host"}}); code != one {
		t.Errorf("printSortResults()=%d, want %d", code, one)
	}
}

func TestSpoolCreationFailureReturnsNonzero(t *testing.T) {
	parent := t.TempDir()
	invalidSpoolDir := parent + "/not-a-directory"
	if err := os.WriteFile(invalidSpoolDir, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &Pssh{Config: &Config{
		ColorMode:       false,
		MaxBufferMemory: outputChunkSize,
		MaxSpoolSize:    1 << 20,
		SpoolDir:        invalidSpoolDir,
	}}
	p.Init()
	results := make(chan *result, 1)
	s := &sessionWork{
		con: &conWork{Pssh: p},
		input: &input{
			results: results,
		},
	}
	res := s.newResult()
	s.run(context.Background(), res, &mockSess{stdout: bytes.Repeat([]byte("x"), int(outputChunkSize+1))})
	got := <-results
	if got.code != one {
		t.Errorf("code=%d, want %d", got.code, one)
	}
	if got.err == nil || !strings.Contains(got.err.Error(), "create output spool file") {
		t.Fatalf("err=%v, want spool creation error", got.err)
	}
	_ = p.delReslt(got)
}

func TestSpoolWriteFailureReturnsNonzero(t *testing.T) {
	tempDir := t.TempDir()
	memory := newMemoryBudget(outputChunkSize)
	spool := newMemoryBudget(1 << 20)
	stdout := newSpillBuffer(memory, spool, func() (*os.File, error) {
		file, err := os.CreateTemp(tempDir, "closed-output-*")
		if err != nil {
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
		return file, nil
	})
	stderr := newSpillBuffer(memory, spool, func() (*os.File, error) {
		return os.CreateTemp(tempDir, "stderr-*")
	})
	p := &Pssh{Config: &Config{ColorMode: false}}
	p.print = newPrint(io.Discard, io.Discard, false)
	results := make(chan *result, 1)
	s := &sessionWork{
		con: &conWork{Pssh: p},
		input: &input{
			results: results,
		},
	}
	res := &result{stdout: stdout, stderr: stderr}
	s.run(context.Background(), res, &mockSess{stdout: bytes.Repeat([]byte("x"), int(outputChunkSize+1))})
	got := <-results
	if got.code != one {
		t.Errorf("code=%d, want %d", got.code, one)
	}
	if got.err == nil || !strings.Contains(got.err.Error(), "write output spool file") {
		t.Fatalf("err=%v, want spool write error", got.err)
	}
	_ = p.delReslt(got)
}

func TestOutputFatalClosesRunningSession(t *testing.T) {
	p := &Pssh{Config: &Config{
		ColorMode:       false,
		MaxBufferMemory: outputChunkSize,
		MaxSpoolSize:    1,
		SpoolDir:        t.TempDir(),
	}}
	p.Init()
	t.Cleanup(func() {
		_ = p.cleanupOutputStorage()
	})
	results := make(chan *result, 1)
	s := &sessionWork{
		con: &conWork{Pssh: p},
		input: &input{
			results: results,
		},
	}
	session := newBlockingSess(true)
	go s.run(context.Background(), s.newResult(), session)

	select {
	case <-session.closed:
	case <-time.After(time.Second):
		t.Fatal("session was not closed after output spool failure")
	}
	select {
	case got := <-results:
		if got.code != one {
			t.Errorf("code=%d, want %d", got.code, one)
		}
		if got.err == nil || !strings.Contains(got.err.Error(), "maximum spool size") {
			t.Fatalf("err=%v, want maximum spool size error", got.err)
		}
		_ = p.delReslt(got)
	case <-time.After(time.Second):
		t.Fatal("run did not return after output spool failure")
	}
}

func TestContextCancellationClosesRunningSession(t *testing.T) {
	p := &Pssh{Config: &Config{
		ColorMode:       false,
		MaxBufferMemory: DefaultMaxBufferMemory,
		MaxSpoolSize:    DefaultMaxSpoolSize,
	}}
	p.Init()
	results := make(chan *result, 1)
	s := &sessionWork{
		con: &conWork{Pssh: p},
		input: &input{
			results: results,
		},
	}
	session := newBlockingSess(false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.run(ctx, s.newResult(), session)
		close(done)
	}()
	<-session.started
	cancel()

	select {
	case <-session.closed:
	case <-time.After(time.Second):
		t.Fatal("session was not closed after context cancellation")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run did not return after context cancellation")
	}
}
