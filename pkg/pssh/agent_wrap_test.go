package pssh

import (
	"net"
	"testing"
)

type mockNetDial struct{}

func (n mockNetDial) Dial(network, address string) (net.Conn, error) { return &conMock{}, nil }

func TestDialSocket(t *testing.T) {
	cp := &connPools{netDialer: mockNetDial{}}
	_, err := cp.netDialer.Dial("unix", "")
	if err != nil {
		t.Error(err)
	}
}
