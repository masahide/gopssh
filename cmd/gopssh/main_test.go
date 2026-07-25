package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/masahide/gopssh/pkg/pssh"
)

func TestDefaultConfig(t *testing.T) {
	c := defaultConfig()
	if c.Concurrency != pssh.DefaultConcurrency {
		t.Errorf("c.Concurrency=%d, want %d", c.Concurrency, pssh.DefaultConcurrency)
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
	if c.Timeout != 15*time.Second {
		t.Error("c.Timeout != 5*time.Second")
	}
	if c.IdentityFileOnly {
		t.Error("identity files must not disable SSH Agent by default")
	}
	if c.MaxBufferMemory != pssh.DefaultMaxBufferMemory {
		t.Errorf("MaxBufferMemory=%d, want %d", c.MaxBufferMemory, pssh.DefaultMaxBufferMemory)
	}
	if c.MaxSpoolSize != pssh.DefaultMaxSpoolSize {
		t.Errorf("MaxSpoolSize=%d, want %d", c.MaxSpoolSize, pssh.DefaultMaxSpoolSize)
	}
	if len(c.Kex) != 0 || len(c.Ciphers) != 0 || len(c.Macs) != 0 {
		t.Error("secure SSH defaults must be used unless algorithms are explicitly configured")
	}
}

func TestByteSizeValue(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1", 1},
		{"32KiB", 32 << 10},
		{"128MiB", 128 << 20},
		{"10GiB", 10 << 30},
	}
	for _, test := range tests {
		got, err := parseByteSize(test.input)
		if err != nil {
			t.Fatalf("parseByteSize(%q): %v", test.input, err)
		}
		if got != test.want {
			t.Errorf("parseByteSize(%q)=%d, want %d", test.input, got, test.want)
		}
		if formatted := formatByteSize(got); test.input != "1" && formatted != test.input {
			t.Errorf("formatByteSize(%d)=%q, want %q", got, formatted, test.input)
		}
	}
	for _, input := range []string{"", "0", "-1", "1MB", "abc"} {
		if _, err := parseByteSize(input); err == nil {
			t.Errorf("parseByteSize(%q) error=nil", input)
		}
	}
}

func TestConfigureCrypto(t *testing.T) {
	t.Run("secure defaults", func(t *testing.T) {
		c := &pssh.Config{}
		configureCrypto(c, false, "", "", "")
		if c.Kex != nil || c.Ciphers != nil || c.Macs != nil {
			t.Fatalf("got Kex=%v Ciphers=%v Macs=%v, want nil defaults", c.Kex, c.Ciphers, c.Macs)
		}
	})
	t.Run("legacy order", func(t *testing.T) {
		c := &pssh.Config{}
		configureCrypto(c, true, "", "", "")
		if !reflect.DeepEqual(c.Kex, legacyKexAlgos) {
			t.Errorf("Kex=%v, want %v", c.Kex, legacyKexAlgos)
		}
		if !reflect.DeepEqual(c.Ciphers, legacyCiphers) {
			t.Errorf("Ciphers=%v, want %v", c.Ciphers, legacyCiphers)
		}
		if !reflect.DeepEqual(c.Macs, legacyMacs) {
			t.Errorf("MACs=%v, want %v", c.Macs, legacyMacs)
		}
	})
	t.Run("explicit override", func(t *testing.T) {
		c := &pssh.Config{}
		configureCrypto(c, true, "kex-a", "cipher-a", "mac-a")
		if !reflect.DeepEqual(c.Kex, []string{"kex-a"}) ||
			!reflect.DeepEqual(c.Ciphers, []string{"cipher-a"}) ||
			!reflect.DeepEqual(c.Macs, []string{"mac-a"}) {
			t.Fatalf("explicit algorithms were not preserved: %+v", c)
		}
	})
}

func TestCheckFlag(t *testing.T) {
	var tests = []struct {
		args     []string
		showVer  bool
		wantRet  int
		wantExit bool
	}{
		{args: []string{"1", "2"}, showVer: false, wantRet: 0, wantExit: false},
		{args: []string{"1"}, showVer: false, wantRet: 2, wantExit: true},
		{args: []string{"1"}, showVer: true, wantRet: 0, wantExit: true},
	}
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	oldShowVer := showVer
	t.Cleanup(func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
		showVer = oldShowVer
	})
	for i, test := range tests {
		flag.CommandLine = flag.NewFlagSet("gopssh-test", flag.ContinueOnError)
		os.Args = test.args
		showVer = flag.Bool("version", test.showVer, "Show version")
		if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
			t.Fatal(err)
		}
		buf := bytes.NewBuffer(nil)
		ret, exit := checkFlag(buf)
		if ret != test.wantRet || exit != test.wantExit {
			t.Errorf("%d ret=%d exit=%t, want ret=%d exit=%t", i, ret, exit, test.wantRet, test.wantExit)
		}
	}
}

func TestSignalExitCode(t *testing.T) {
	if got := signalExitCode(0, nil); got != 0 {
		t.Errorf("no signal code=%d", got)
	}
	if got := signalExitCode(1, errors.New("received signal: interrupt")); got != 130 {
		t.Errorf("SIGINT code=%d", got)
	}
	if got := signalExitCode(1, errors.New("received signal: terminated")); got != 143 {
		t.Errorf("SIGTERM code=%d", got)
	}
}

func TestLegacyParserStopsAtCommandAndPreservesBooleanValues(t *testing.T) {
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	t.Cleanup(func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	})
	flag.CommandLine = flag.NewFlagSet("gopssh", flag.ContinueOnError)
	os.Args = []string{
		"gopssh", "-h", "hosts.txt", "-c=false", "-s=false", "--debug",
		"run", "--something",
	}
	config := newConfig()
	if config.Hostsfile != "hosts.txt" || config.ColorMode || config.SortPrint || !config.Debug {
		t.Fatalf("legacy config=%+v", config)
	}
	if got, want := flag.Args(), []string{"run", "--something"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy command args=%v, want %v", got, want)
	}
}
