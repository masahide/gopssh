package pssh

import (
	"testing"
)

func TestDialSocket(t *testing.T) {
	cp := &connPools{netDialer: mockNetDial{}}
	_, err := cp.netDialer.Dial("unix", "")
	if err != nil {
		t.Error(err)
	}
}
