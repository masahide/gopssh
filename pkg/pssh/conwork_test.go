package pssh

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type conMock struct {
	laddr net.Addr
}

func (c *conMock) Read(b []byte) (n int, err error)   { return 0, nil }
func (c *conMock) Write(b []byte) (n int, err error)  { return 0, nil }
func (c *conMock) Close() error                       { return nil }
func (c *conMock) LocalAddr() net.Addr                { return c.laddr }
func (c *conMock) RemoteAddr() net.Addr               { return c.laddr }
func (c *conMock) SetDeadline(t time.Time) error      { return nil }
func (c *conMock) SetWriteDeadline(t time.Time) error { return nil }
func (c *conMock) SetReadDeadline(t time.Time) error  { return nil }

type conSSHMock struct {
	closeFunc func()
}

func (c *conSSHMock) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	return true, nil, nil
}
func (c *conSSHMock) OpenChannel(name string, data []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, nil
}
func (c *conSSHMock) Close() error {
	if c.closeFunc != nil {
		c.closeFunc()
	}
	return nil
}
func (c *conSSHMock) Wait() error                                                   { return nil }
func (c *conSSHMock) User() string                                                  { return "" }
func (c *conSSHMock) SessionID() []byte                                             { return nil }
func (c *conSSHMock) ClientVersion() []byte                                         { return nil }
func (c *conSSHMock) ServerVersion() []byte                                         { return nil }
func (c *conSSHMock) RemoteAddr() net.Addr                                          { return nil }
func (c *conSSHMock) LocalAddr() net.Addr                                           { return nil }
func (c *conSSHMock) Dial(n, addr string) (net.Conn, error)                         { return nil, nil }
func (c *conSSHMock) DialTCP(n string, laddr, raddr *net.TCPAddr) (net.Conn, error) { return nil, nil }
func (c *conSSHMock) HandleChannelOpen(channelType string) <-chan ssh.NewChannel    { return nil }
func (c *conSSHMock) Listen(n, addr string) (net.Listener, error)                   { return nil, nil }
func (c *conSSHMock) ListenTCP(laddr *net.TCPAddr) (net.Listener, error)            { return nil, nil }
func (c *conSSHMock) ListenUnix(socketPath string) (net.Listener, error)            { return nil, nil }
func (c *conSSHMock) NewSession() (*ssh.Session, error)                             { return nil, nil }

type mockSSHDial struct {
	err error
}

func (n mockSSHDial) DialContext(
	context.Context,
	string,
	string,
	*ssh.ClientConfig,
) (sshClientIface, error) {
	return &conSSHMock{}, n.err
}

type hostSSHDial struct{}

func (hostSSHDial) DialContext(
	_ context.Context,
	_ string,
	addr string,
	_ *ssh.ClientConfig,
) (sshClientIface, error) {
	if addr == "bad:22" {
		return nil, errors.New("connection refused")
	}
	return &conSSHMock{}, nil
}

type countingSSHDial struct {
	mu          sync.Mutex
	calls       []string
	blockAddr   string
	dialStarted chan struct{}
	releaseDial chan struct{}
	closed      atomic.Bool
}

type pipeDialer struct {
	conn    net.Conn
	started chan struct{}
}

func (d *pipeDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	close(d.started)
	return d.conn, nil
}

func (d *countingSSHDial) DialContext(
	_ context.Context,
	_ string,
	addr string,
	_ *ssh.ClientConfig,
) (sshClientIface, error) {
	d.mu.Lock()
	d.calls = append(d.calls, addr)
	d.mu.Unlock()
	if addr == d.blockAddr {
		close(d.dialStarted)
		<-d.releaseDial
	}
	return &conSSHMock{closeFunc: func() { d.closed.Store(true) }}, nil
}

func (d *countingSSHDial) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

func mockStartSessionWorker(ctx context.Context, conn sshClientIface, cmd input) {
	cmd.results <- &result{}
}

func TestConWorker(t *testing.T) {

	var tests = []struct {
		err error
	}{
		{nil},
		{errors.New("hoge")},
	}
	for _, test := range tests {
		ctx, cancel := context.WithCancel(context.Background())
		p := &Pssh{Config: &Config{ColorMode: true}}
		p.Concurrency = 1
		p.Init()
		//p.netDialer = mockNetDial{}
		p.sshDialer = mockSSHDial{err: test.err}
		c := &conWork{
			Pssh:         p,
			id:           1,
			host:         "host1",
			command:      make(chan input, 1),
			startSession: mockStartSessionWorker,
		}
		results := make(chan *result, 1)
		c.command <- input{command: "", stdin: "", results: results}
		go c.conWorker(ctx, ssh.ClientConfig{})
		select {
		case res := <-results:
			if test.err == nil && res.err != nil {
				t.Error(res.err)
			}
			if test.err != nil {
				if res.err == nil || res.code != connectFailureCode || res.kind != ResultConnectionFailed {
					t.Errorf("res=%+v, want connection failure", res)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for result")
		}
		cancel()
	}
}

func TestConnectionFailureDoesNotCancelOtherHosts(t *testing.T) {
	p := &Pssh{Config: &Config{
		Concurrency:     2,
		MaxAgentConns:   DefaultMaxAgentConns,
		MaxBufferMemory: DefaultMaxBufferMemory,
		MaxSpoolSize:    DefaultMaxSpoolSize,
	}}
	p.Init()
	p.sshDialer = hostSSHDial{}
	p.cws = []*conWork{
		p.newConWork(0, "bad:22"),
		p.newConWork(1, "good:22"),
	}
	p.cws[1].startSession = func(ctx context.Context, conn sshClientIface, cmd input) {
		cmd.results <- p.newResult(1, cmd.id)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan *result, len(p.cws))
	for _, worker := range p.cws {
		worker.command <- input{results: results}
	}
	p.runConWorkers(ctx)

	got := make(map[int]int)
	for range p.cws {
		select {
		case result := <-results:
			got[result.conID] = result.code
			if result.conID == 0 && result.kind != ResultConnectionFailed {
				t.Errorf("bad host kind=%q, want %q", result.kind, ResultConnectionFailed)
			}
			_ = p.delReslt(result)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for all host results")
		}
	}
	if got[0] != connectFailureCode || got[1] != 0 {
		t.Fatalf("result codes=%v, want map[0:%d 1:0]", got, connectFailureCode)
	}
}

func TestCancellationDoesNotLaunchQueuedHosts(t *testing.T) {
	const hostCount = 100
	p := &Pssh{Config: &Config{
		Concurrency:     1,
		MaxAgentConns:   DefaultMaxAgentConns,
		MaxBufferMemory: DefaultMaxBufferMemory,
		MaxSpoolSize:    DefaultMaxSpoolSize,
	}}
	p.Init()
	dialer := &countingSSHDial{}
	p.sshDialer = dialer
	p.cws = make([]*conWork, hostCount)
	started := make(chan struct{})
	for i := range p.cws {
		p.cws[i] = p.newConWork(i, "host")
		p.cws[i].startSession = func(ctx context.Context, _ sshClientIface, _ input) {
			select {
			case <-started:
			default:
				close(started)
			}
			<-ctx.Done()
		}
		p.cws[i].command <- input{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	launcherDone := make(chan struct{})
	go func() {
		p.runConWorkers(ctx)
		close(launcherDone)
	}()
	<-started
	cancel()

	select {
	case <-launcherDone:
	case <-time.After(time.Second):
		t.Fatal("worker launcher did not stop after cancellation")
	}
	p.workerWG.Wait()
	if got := dialer.count(); got != 1 {
		t.Fatalf("DialContext calls=%d, want 1", got)
	}
}

func TestDialCompletingAfterCancellationDoesNotStartCommand(t *testing.T) {
	p := &Pssh{Config: &Config{
		Concurrency:     1,
		MaxAgentConns:   DefaultMaxAgentConns,
		MaxBufferMemory: DefaultMaxBufferMemory,
		MaxSpoolSize:    DefaultMaxSpoolSize,
	}}
	p.Init()
	dialer := &countingSSHDial{
		blockAddr:   "host",
		dialStarted: make(chan struct{}),
		releaseDial: make(chan struct{}),
	}
	p.sshDialer = dialer
	commandStarted := make(chan struct{}, 1)
	c := p.newConWork(0, "host")
	c.startSession = func(context.Context, sshClientIface, input) {
		commandStarted <- struct{}{}
	}
	c.command <- input{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.conWorker(ctx, ssh.ClientConfig{})
		close(done)
	}()
	<-dialer.dialStarted
	cancel()
	close(dialer.releaseDial)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not return after canceled DialContext completed")
	}
	select {
	case <-commandStarted:
		t.Fatal("command started after cancellation")
	default:
	}
	if !dialer.closed.Load() {
		t.Fatal("connection was not closed after cancellation")
	}
}

func TestCanceledCommandLoopDoesNotStartSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := false
	c := &conWork{
		command: make(chan input, 1),
		startSession: func(context.Context, sshClientIface, input) {
			started = true
		},
	}
	c.command <- input{}
	c.commandLoop(ctx, &conSSHMock{}, false)
	if started {
		t.Fatal("session started with canceled context")
	}
}

func TestSSHDialContextCancelsHandshake(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer func() {
		_ = serverConn.Close()
	}()
	dialer := &pipeDialer{conn: clientConn, started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, dialErr := (sshDial{netDialer: dialer}).DialContext(ctx, "tcp", "host:22", &ssh.ClientConfig{
			HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		})
		done <- dialErr
	}()
	<-dialer.started
	cancel()
	select {
	case dialErr := <-done:
		if !errors.Is(dialErr, context.Canceled) {
			t.Fatalf("DialContext error=%v, want cancellation", dialErr)
		}
	case <-time.After(time.Second):
		t.Fatal("SSH handshake was not canceled")
	}
}
