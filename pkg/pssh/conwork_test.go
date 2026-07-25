package pssh

import (
	"context"
	"errors"
	"net"
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
}

func (c *conSSHMock) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	return true, nil, nil
}
func (c *conSSHMock) OpenChannel(name string, data []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, nil
}
func (c *conSSHMock) Close() error                                                  { return nil }
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

func (n mockSSHDial) Dial(network, addr string, config *ssh.ClientConfig) (sshClientIface, error) {
	return &conSSHMock{}, n.err
}

type hostSSHDial struct{}

func (hostSSHDial) Dial(network, addr string, config *ssh.ClientConfig) (sshClientIface, error) {
	if addr == "bad:22" {
		return nil, errors.New("connection refused")
	}
	return &conSSHMock{}, nil
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
				if res.err == nil || res.code != connectFailureCode {
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
			_ = p.delReslt(result)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for all host results")
		}
	}
	if got[0] != connectFailureCode || got[1] != 0 {
		t.Fatalf("result codes=%v, want map[0:%d 1:0]", got, connectFailureCode)
	}
}
