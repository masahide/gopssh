package pssh

import (
	"context"
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

type mockNetDial struct{}

func (n mockNetDial) Dial(network, address string) (net.Conn, error) { return &conMock{}, nil }

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

type mockSSHDial struct{}

func (n mockSSHDial) Dial(network, addr string, config *ssh.ClientConfig) (sshClientIface, error) {
	return &conSSHMock{}, nil
}

func mockStartSessionWorker(ctx context.Context, conn sshClientIface, cmd input) {
	cmd.results <- &result{}
}

func TestConWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &Pssh{Config: &Config{ColorMode: true}}
	p.Init()
	p.netDialer = mockNetDial{}
	p.sshDialer = mockSSHDial{}
	c := &conWork{Pssh: p, id: 1, host: "host1", command: make(chan input, 1)}
	conInstances := make(chan conInstance, 1)
	go c.conWorker(ctx, ssh.ClientConfig{}, "", conInstances)
	results := make(chan *result, 1)
	c.command <- input{command: "", stdin: "", results: results}
	c.startSession = mockStartSessionWorker
	res := <-results
	if res.err != nil {
		t.Error(res)
	}
}
