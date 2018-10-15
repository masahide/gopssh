package main

import (
	"bytes"
	"flag"
	"os"
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

func TestCheckFlag(t *testing.T) {
	var tests = []struct {
		args     []string
		showVer  bool
		wantRet  int
		wantExit bool
	}{
		{args: []string{"1", "2"}, showVer: false, wantRet: 0, wantExit: false},
		{args: []string{"1"}, showVer: false, wantRet: 2, wantExit: false},
		{args: []string{"1"}, showVer: true, wantRet: 0, wantExit: true},
	}
	for i, test := range tests {
		os.Args = test.args
		flag.Parse()
		showVer = &test.showVer
		b := []byte{}
		buf := bytes.NewBuffer(b)
		if ret, exit := checkFlag(buf); exit {
			if ret != test.wantRet {
				t.Errorf("%d ret=%d,wantRet:%d", i, ret, test.wantRet)
			}
		}
	}
}
