package main

import (
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	c := newConfig()
	if c.Concurrency != 0 {
		t.Error("c.Concurrency != 0")
	}
	if c.Hostsfile != "" {
		t.Error("c.Hostsfile != ''")
	}
	if c.ShowHostName {
		t.Error("c.ShowHostName != false")
	}
	if !c.ColorMode {
		t.Error("c.ColorMode != true")
	}
	if c.IgnoreHostKey {
		t.Error("c.IgnoreHostKey != false")
	}
	if c.Debug {
		t.Error("c.Debug !=false")
	}
	if c.Timeout != 5*time.Second {
		t.Error("c.Timeout != 5*time.Second")
	}
}
