package pssh

import (
	"crypto"
	"errors"
	"io"
	"log"
	"net"
	"sync"

	"github.com/cenkalti/backoff"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type keyAgent struct {
	agent.ExtendedAgent
	authConn net.Conn
}

type connPools struct {
	limit     chan struct{}
	connPool  sync.Pool
	netDialer dialIface
}

type dialIface interface {
	Dial(network, address string) (net.Conn, error)
}
type netDial struct{}

func (n netDial) Dial(network, address string) (net.Conn, error) { return net.Dial(network, address) }

func newConnPools(socket string, n int) *connPools {
	cp := &connPools{
		netDialer: netDial{},
	}
	cp.limit = make(chan struct{}, n)
	cp.connPool = sync.Pool{New: func() interface{} {
		var ka keyAgent
		var err error
		ka.authConn, err = cp.dialSocket(socket)
		if err != nil {
			log.Fatalf("Failed dial socket: %s", socket)
		}
		ka.ExtendedAgent = agent.NewClient(ka.authConn)
		return &ka
	}}
	return cp
}

func (cp *connPools) Get() *keyAgent {
	cp.limit <- struct{}{} // 空くまで待つ
	return cp.connPool.Get().(*keyAgent)
}

func (cp *connPools) Put(ka *keyAgent) {
	cp.connPool.Put(ka)
	<-cp.limit // 解放
}

func (cp *connPools) dialSocket(socket string) (net.Conn, error) {
	// https://stackoverflow.com/questions/30228482/golang-unix-socket-error-dial-resource-temporarily-unavailable
	var authConn net.Conn
	err := backoff.Retry(func() error {
		var err error
		authConn, err = cp.netDialer.Dial("unix", socket)
		if err != nil {
			if terr, ok := err.(TemporaryError); ok && terr.Temporary() {
				return err
			}
		}
		return nil
	}, backoff.NewExponentialBackOff())
	return authConn, err
}

type agentClient struct {
	*connPools

	signers []ssh.Signer
	mu      sync.RWMutex
}

func newAgentClient(cp *connPools) agent.ExtendedAgent {
	return &agentClient{connPools: cp}
}

func (c *agentClient) RemoveAll() error {
	return errors.New("not implemented agentClient RemoveAll")
}

func (c *agentClient) Remove(key ssh.PublicKey) error {
	return errors.New("not implemented agentClient Remove")
}

func (c *agentClient) Lock(passphrase []byte) error {
	return errors.New("not implemented agentClient Lock")
}

func (c *agentClient) Unlock(passphrase []byte) error {
	return errors.New("not implemented agentClient Unlock")
}

// List returns the identities known to the agent.
func (c *agentClient) List() ([]*agent.Key, error) {
	ka := c.Get()
	defer c.Put(ka)
	return ka.List()
}

// Sign has the agent sign the data using a protocol 2 key as defined
// in [PROTOCOL.agent] section 2.6.2.
func (c *agentClient) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	ka := c.Get()
	defer c.Put(ka)
	return ka.Sign(key, data)
}

func (c *agentClient) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	ka := c.Get()
	defer c.Put(ka)
	return ka.SignWithFlags(key, data, flags)
}

// Add adds a private key to the agent. If a certificate is given,
// that certificate is added instead as public key.
func (c *agentClient) Add(key agent.AddedKey) error {
	return errors.New("not implemented agentClient Add")
}

// Signers provides a callback for agentClient authentication.
func (c *agentClient) Signers() ([]ssh.Signer, error) {
	c.mu.RLock()
	res := c.signers
	c.mu.RUnlock()
	if res != nil {
		return res, nil
	}
	keys, err := c.List()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]ssh.Signer, len(keys))
	for i, k := range keys {
		result[i] = &agentKeyringSigner{c, k}
	}
	c.signers = result
	return result, nil
}

func (c *agentClient) Extension(extensionType string, contents []byte) ([]byte, error) {
	ka := c.Get()
	defer c.Put(ka)
	return ka.Extension(extensionType, contents)
}

type agentKeyringSigner struct {
	agent *agentClient
	pub   ssh.PublicKey
}

func (s *agentKeyringSigner) PublicKey() ssh.PublicKey {
	return s.pub
}

func (s *agentKeyringSigner) Sign(rand io.Reader, data []byte) (*ssh.Signature, error) {
	// The agent has its own entropy source, so the rand argument is ignored.
	return s.agent.Sign(s.pub, data)
}

func (s *agentKeyringSigner) SignWithOpts(rand io.Reader, data []byte, opts crypto.SignerOpts) (*ssh.Signature, error) {
	return nil, errors.New("not implemented agentKeyringSigner SignWithOpts")
}
