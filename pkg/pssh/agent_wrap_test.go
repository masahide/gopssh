package pssh

import (
	"crypto"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/testdata"
)

var (
	testPrivateKeys map[string]interface{}
	testSigners     map[string]ssh.Signer
	testPublicKeys  map[string]ssh.PublicKey
)

func init() {
	var err error

	n := len(testdata.PEMBytes)
	testPrivateKeys = make(map[string]interface{}, n)
	testSigners = make(map[string]ssh.Signer, n)
	testPublicKeys = make(map[string]ssh.PublicKey, n)
	for t, k := range testdata.PEMBytes {
		testPrivateKeys[t], err = ssh.ParseRawPrivateKey(k)
		if err != nil {
			panic(fmt.Sprintf("Unable to parse test key %s: %v", t, err))
		}
		testSigners[t], err = ssh.NewSignerFromKey(testPrivateKeys[t])
		if err != nil {
			panic(fmt.Sprintf("Unable to create signer for test key %s: %v", t, err))
		}
		testPublicKeys[t] = testSigners[t].PublicKey()
	}
}

type mockNetDial struct{}

func (n mockNetDial) Dial(network, address string) (net.Conn, error) { return &conMock{}, nil }

func TestGetPut(t *testing.T) {
	cp := newConnPools("", 1)
	a := cp.Get()
	if a == nil {
		t.Error("cp.Get()==nil")
	}
	cp.Put(a)
}

func TestDialSocket(t *testing.T) {
	cp := &connPools{netDialer: mockNetDial{}}
	con, err := cp.dialSocket("")
	if err != nil {
		t.Error(err)
	}
	if con == nil {
		t.Error("con==nil")
	}
}

type mockAgentClient struct {
	*connPools

	signers []ssh.Signer
}

func newMoockAgentClient(cp *connPools) agent.ExtendedAgent {
	return &mockAgentClient{connPools: cp}
}

func (c *mockAgentClient) RemoveAll() error {
	return errors.New("not implemented mockAgentClient RemoveAll")
}

func (c *mockAgentClient) Remove(key ssh.PublicKey) error {
	return errors.New("not implemented mockAgentClient Remove")
}

func (c *mockAgentClient) Lock(passphrase []byte) error {
	return errors.New("not implemented mockAgentClient Lock")
}

func (c *mockAgentClient) Unlock(passphrase []byte) error {
	return errors.New("not implemented mockAgentClient Unlock")
}

// List returns the identities known to the agent.
func (c *mockAgentClient) List() ([]*agent.Key, error) {
	return nil, nil
}

// Sign has the agent sign the data using a protocol 2 key as defined
// in [PROTOCOL.agent] section 2.6.2.
func (c *mockAgentClient) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return nil, nil
}

func (c *mockAgentClient) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	return nil, nil
}

// Add adds a private key to the agent. If a certificate is given,
// that certificate is added instead as public key.
func (c *mockAgentClient) Add(key agent.AddedKey) error {
	return errors.New("not implemented mockAgentClient Add")
}

// Signers provides a callback for mockAgentClient authentication.
func (c *mockAgentClient) Signers() ([]ssh.Signer, error) {
	keys := []*agent.Key{{}}
	result := make([]ssh.Signer, len(keys))
	for i, k := range keys {
		result[i] = &mockAgentKeyringSigner{c, k}
	}
	c.signers = result
	return result, nil
}

func (c *mockAgentClient) Extension(extensionType string, contents []byte) ([]byte, error) {
	return []byte("hoge"), nil
}

type mockAgentKeyringSigner struct {
	agent *mockAgentClient
	pub   ssh.PublicKey
}

func (s *mockAgentKeyringSigner) PublicKey() ssh.PublicKey {
	return s.pub
}

func (s *mockAgentKeyringSigner) Sign(rand io.Reader, data []byte) (*ssh.Signature, error) {
	// The agent has its own entropy source, so the rand argument is ignored.
	return nil, nil
}

func (s *mockAgentKeyringSigner) SignWithOpts(rand io.Reader, data []byte, opts crypto.SignerOpts) (*ssh.Signature, error) {
	return nil, errors.New("not implemented mockAgentKeyringSigner SignWithOpts")
}

func TestNewAgentClient(t *testing.T) {
	res := newAgentClient(nil)
	if res == nil {
		t.Error("res==nil")
	}
}

func TestRemoveAll(t *testing.T) {
	cp := newConnPools("", 1)
	cp.netDialer = mockNetDial{}
	ac := newAgentClient(cp)
	err := ac.RemoveAll()
	if err == nil {
		t.Error("err==nil")
	}
}

func TestRemove(t *testing.T) {
	cp := newConnPools("", 1)
	cp.netDialer = mockNetDial{}
	ac := newAgentClient(cp)
	err := ac.Remove(testPublicKeys["rsa"])
	if err == nil {
		t.Error("err==nil")
	}
}
func TestLock(t *testing.T) {
	cp := newConnPools("", 1)
	cp.netDialer = mockNetDial{}
	ac := newAgentClient(cp)
	err := ac.Lock([]byte{})
	if err == nil {
		t.Error("err==nil")
	}
}

func TestUnlock(t *testing.T) {
	cp := newConnPools("", 1)
	cp.netDialer = mockNetDial{}
	ac := newAgentClient(cp)
	err := ac.Unlock([]byte{})
	if err == nil {
		t.Error("err==nil")
	}
}

func TestList(t *testing.T) {
	cp := newConnPools("", 1)
	cp.netDialer = mockNetDial{}
	cp.connPool = sync.Pool{New: func() interface{} {
		var ka keyAgent
		ka.ExtendedAgent = newMoockAgentClient(cp)
		return &ka
	}}
	ac := newAgentClient(cp)
	list, err := ac.List()
	if err != nil {
		t.Error("err!=nil")
	}
	if len(list) != 0 {
		t.Errorf("list=%q", list)
	}
}

func TestSign(t *testing.T) {
	cp := newConnPools("", 1)
	cp.netDialer = mockNetDial{}
	cp.connPool = sync.Pool{New: func() interface{} {
		var ka keyAgent
		ka.ExtendedAgent = newMoockAgentClient(cp)
		return &ka
	}}
	ac := newAgentClient(cp)
	sig, err := ac.Sign(testPublicKeys["rsa"], []byte{})
	if err != nil {
		t.Error("err!=nil")
	}
	if sig != nil {
		t.Errorf("sig=%q", sig)
	}
}

func TestSignWithFlags(t *testing.T) {
	cp := newConnPools("", 1)
	cp.netDialer = mockNetDial{}
	cp.connPool = sync.Pool{New: func() interface{} {
		var ka keyAgent
		ka.ExtendedAgent = newMoockAgentClient(cp)
		return &ka
	}}
	ac := newAgentClient(cp)
	sig, err := ac.SignWithFlags(testPublicKeys["rsa"], []byte{}, agent.SignatureFlagReserved)
	if err != nil {
		t.Error("err!=nil")
	}
	if sig != nil {
		t.Errorf("sig=%q", sig)
	}
}

func TestAdd(t *testing.T) {
	cp := newConnPools("", 1)
	cp.netDialer = mockNetDial{}
	cp.connPool = sync.Pool{New: func() interface{} {
		var ka keyAgent
		ka.ExtendedAgent = newMoockAgentClient(cp)
		return &ka
	}}
	ac := newAgentClient(cp)
	err := ac.Add(agent.AddedKey{})
	if err == nil {
		t.Error("err==nil")
	}
}

func TestSigners(t *testing.T) {
	cp := newConnPools("", 1)
	cp.netDialer = mockNetDial{}
	cp.connPool = sync.Pool{New: func() interface{} {
		var ka keyAgent
		ka.ExtendedAgent = newMoockAgentClient(cp)
		return &ka
	}}
	ac := newAgentClient(cp)
	list, err := ac.Signers()
	if err != nil {
		t.Error(err)
	}
	if len(list) != 0 {
		t.Errorf("list:%q", list)
	}
}
