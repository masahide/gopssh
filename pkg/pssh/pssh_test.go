package pssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/testdata"
)

func sliceEq(a, b []string) bool {

	// If one is nil, the other must also be nil.
	if (a == nil) != (b == nil) {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestToSlice(t *testing.T) {
	var tests = []struct {
		s    string
		want []string
	}{
		{"hoge,fuga,uho", []string{"hoge", "fuga", "uho"}},
	}
	for _, test := range tests {
		res := ToSlice(test.s)
		if !sliceEq(res, test.want) {
			t.Errorf("res %v,want %v", res, test.want)
		}
	}
}

func TestInit(t *testing.T) {
	var tests = []struct {
		colorMode bool
		want      prn
	}{
		{false, writerPrinter{}},
		{true, color.New()},
	}
	for _, test := range tests {
		p := &Pssh{
			Config: &Config{ColorMode: test.colorMode},
		}
		p.Init()
		if _, ok := test.want.(writerPrinter); ok {
			if _, ok := p.red.(writerPrinter); !ok {
				t.Errorf("res type :%T, want %T", p.red, test.want)
			}
		}
		if _, ok := test.want.(*color.Color); ok {
			if _, ok := p.red.(*color.Color); !ok {
				t.Errorf("res type :%T, want %T", p.red, test.want)
			}
		}
		if p.outputMemory.Used() != 0 || p.outputSpool.Used() != 0 {
			t.Errorf("output budgets are not empty: memory=%d spool=%d", p.outputMemory.Used(), p.outputSpool.Used())
		}
	}

}

func TestReadHosts(t *testing.T) {
	var tests = []struct {
		file string
		want []string
		err  error
	}{
		{"test/hosts1", []string{"abc:22", "abc:24", "bbb:1", "ddd:22"}, nil},
		{"a", nil, errors.New("open a: no such file or directory")},
	}
	for _, test := range tests {
		r, err := readHosts(test.file)
		if test.err != nil {
			if err == nil {
				t.Fatalf("err=nil, want:%s", test.err)
			}
			if err.Error() != test.err.Error() {
				t.Errorf("err:%s,want:%s", err.Error(), test.err.Error())
			}
		}
		if !sliceEq(r, test.want) {
			t.Errorf("r:%v, want:%v", r, test.want)
		}
	}

}

func TestReadHostsIPv6AndComments(t *testing.T) {
	hostsFile := filepath.Join(t.TempDir(), "hosts")
	data := "host1 # production\n::1\n[2001:db8::1]:2222\nhost2:2200 host3\n\n# ignored\n"
	if err := os.WriteFile(hostsFile, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readHosts(hostsFile)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"host1:22", "[::1]:22", "[2001:db8::1]:2222", "host2:2200", "host3:22"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hosts=%v, want %v", got, want)
	}
}

func TestReadHostsRejectsMissingPort(t *testing.T) {
	hostsFile := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hostsFile, []byte("host:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readHosts(hostsFile); err == nil {
		t.Fatal("readHosts() error=nil, want invalid host error")
	}
}

func TestNormalizeHostRejectsInvalidPorts(t *testing.T) {
	for _, host := range []string{"host:0", "host:65536", "host:70000", "host:-1", "host:abc"} {
		t.Run(host, func(t *testing.T) {
			if _, err := normalizeHost(host); err == nil {
				t.Fatalf("normalizeHost(%q) error=nil", host)
			}
		})
	}
	for _, host := range []string{"host:1", "host:65535"} {
		t.Run(host, func(t *testing.T) {
			if _, err := normalizeHost(host); err != nil {
				t.Fatalf("normalizeHost(%q) error=%v", host, err)
			}
		})
	}
}
func TestGetHostKeyCallback(t *testing.T) {
	r, err := getHostKeyCallback(true)
	if err != nil {
		t.Error(err)
	}
	if r("", &net.IPAddr{}, &agent.Key{}) != nil {
		t.Errorf("r:%v, want:nil", r)
	}
	t.Setenv("HOME", "./test")
	r, err = getHostKeyCallback(false)
	if err != nil {
		t.Error(err)
	}
	if r == nil {
		t.Error("r:nil, want:not nil")
	}
	t.Setenv("HOME", "/dev/null")
	r, err = getHostKeyCallback(false)
	if err == nil {
		t.Error(err)
	}
	if r != nil {
		t.Error("r:not nil, want: nil")
	}
}
func TestPrint(t *testing.T) {
	b := []byte{}
	buf := bytes.NewBuffer(b)
	p := &print{
		stdout: buf,
		stderr: buf,
	}
	if _, err := p.Print("hoge"); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hoge" {
		t.Errorf("buf:%s, want:hoge", buf.String())
	}
	buf.Reset()
	if _, err := p.Printf("fuga%s", "hoge"); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "fugahoge" {
		t.Errorf("buf:%s, want:fugahoge", buf.String())
	}
}

func TestPrintResultSeparatesStdoutAndStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := &Pssh{
		Config: &Config{ShowHostName: true},
		print:  newPrint(&stdout, &stderr, false),
	}
	res := &result{
		code:   1,
		err:    errors.New("remote failed"),
		stdout: newTestResultOutput("normal output\n"),
		stderr: newTestResultOutput("error output\n"),
	}
	if err := p.printResult(res, "host1:22"); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "normal output\n" {
		t.Errorf("stdout=%q", stdout.String())
	}
	for _, want := range []string{"host1:22  result code 1", "result err: remote failed", "error output"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr=%q, missing %q", stderr.String(), want)
		}
	}
}

func TestValidate(t *testing.T) {
	valid := Config{
		Concurrency:     DefaultConcurrency,
		MaxAgentConns:   DefaultMaxAgentConns,
		MaxBufferMemory: DefaultMaxBufferMemory,
		MaxSpoolSize:    DefaultMaxSpoolSize,
	}
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"concurrency", func(c *Config) { c.Concurrency = 0 }},
		{"max agent connections", func(c *Config) { c.MaxAgentConns = 0 }},
		{"max buffer memory", func(c *Config) { c.MaxBufferMemory = 0 }},
		{"max spool size", func(c *Config) { c.MaxSpoolSize = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := valid
			test.mutate(&c)
			if err := (&Pssh{Config: &c}).Validate(); err == nil {
				t.Fatal("Validate() error=nil")
			}
		})
	}
	if err := (&Pssh{Config: &valid}).Validate(); err != nil {
		t.Fatalf("Validate() error=%v", err)
	}
}

func TestRunConWorkers(t *testing.T) {
	p := &Pssh{
		concurrentGoroutines: make(chan struct{}, 1),
		Config:               &Config{Concurrency: 1},
	}
	p.cws = []*conWork{{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	i := p.runConWorkers(ctx)
	p.workerWG.Wait()
	if i != 1 {
		t.Error("i!=1")
	}

}

type mockPrin struct {
	buf bytes.Buffer
}

type testResultOutput struct {
	bytes.Buffer
	fatal chan error
}

func newTestResultOutput(value string) *testResultOutput {
	result := &testResultOutput{fatal: make(chan error)}
	_, _ = result.WriteString(value)
	return result
}

func (o *testResultOutput) WriteTo(dst io.Writer) (int64, error) {
	return o.Buffer.WriteTo(dst)
}

func (o *testResultOutput) Finalize() error { return nil }
func (o *testResultOutput) Close() error    { return nil }
func (o *testResultOutput) Err() error      { return nil }
func (o *testResultOutput) Fatal() <-chan error {
	return o.fatal
}
func (o *testResultOutput) Size() int64 { return int64(o.Len()) }

func (p *mockPrin) Print(a ...interface{}) (n int, err error) {
	fmt.Fprint(&p.buf, a...)
	return 0, nil
}
func (p *mockPrin) Printf(format string, a ...interface{}) (n int, err error) {
	fmt.Fprintf(&p.buf, format, a...)
	return 0, nil
}

func TestPrintResults(t *testing.T) {
	var tests = []struct {
		id       int
		ins      []idHost
		want     string
		wantCode int
	}{
		{id: 0, ins: []idHost{{0, "host0", 0}, {3, "host3", 0}, {1, "host1", 0}, {4, "host4", 0}, {2, "host2", 0}},
			want:     `host0  result code 0`,
			wantCode: 0,
		},
		{id: 0, ins: []idHost{{0, "host0", 1}, {3, "host3", 0}, {1, "host1", 0}, {4, "host4", 0}, {2, "host2", 0}},
			want:     `host0  result code 1`,
			wantCode: 1,
		},
		{id: 0, ins: []idHost{{0, "host0", 0}, {3, "host3", 0}, {1, "host1", 0}, {4, "host4", 4}, {2, "host2", 0}},
			want:     `host4  result code 4`,
			wantCode: 4,
		},
	}
	for id, tc := range tests {
		p := &Pssh{
			Config: &Config{
				ShowHostName: true,
			},
		}
		p.print = newPrint(os.Stdout, os.Stderr, false)
		mock := mockPrin{}
		p.red = &mock
		p.boldRed = &mock
		p.green = &mock
		results := make(chan *result)
		cws := make([]*conWork, len(tc.ins))
		for _, c := range tc.ins {
			cws[c.id] = &conWork{
				id: c.id, host: c.host,
			}
			//log.Printf("%v", *cws[i])
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			for _, c := range tc.ins {
				results <- &result{
					conID:  c.id,
					code:   c.code,
					stdout: newTestResultOutput(""),
					stderr: newTestResultOutput(""),
				}
			}
		}()
		code := p.printResults(ctx, results, cws)
		if !strings.Contains(mock.buf.String(), tc.want) {
			t.Errorf("id=%d buf=%s, want:'%s'", id, mock.buf.String(), tc.want)
		}
		if code != tc.wantCode {
			t.Errorf("id=%d code=%d, want:'%d'", id, code, tc.wantCode)
		}
	}
}

type idHost struct {
	id   int
	host string
	code int
}

func TestPrintSortResults(t *testing.T) {
	var tests = []struct {
		id       int
		ins      []idHost
		want     string
		wantCode int
	}{
		{id: 0, ins: []idHost{{0, "host0", 0}, {3, "host3", 0}, {1, "host1", 0}, {4, "host4", 0}, {2, "host2", 0}},
			want: `host0  result code 0
host1  result code 0
host2  result code 0
host3  result code 0
host4  result code 0
`,
			wantCode: 0,
		},
		{id: 1, ins: []idHost{{0, "host0", 0}, {1, "host1", 0}},
			want: `host0  result code 0
host1  result code 0
`,
			wantCode: 0,
		},
		{id: 2, ins: []idHost{{5, "host5", 0}, {3, "host3", 0}, {1, "host1", 0}, {4, "host4", 0}, {2, "host2", 0}, {0, "host0", 0}},
			want: `host0  result code 0
host1  result code 0
host2  result code 0
host3  result code 0
host4  result code 0
host5  result code 0
`,
			wantCode: 0,
		},
		{id: 3, ins: []idHost{{0, "host0", 0}, {1, "host1", 0}, {2, "host2", 0}, {3, "host3", 0}, {4, "host4", 0}, {5, "host5", 0}},
			want: `host0  result code 0
host1  result code 0
host2  result code 0
host3  result code 0
host4  result code 0
host5  result code 0
`,
			wantCode: 0,
		},
		{id: 4, ins: []idHost{{0, "host0", 0}, {1, "host1", 0}, {2, "host2", 0}, {3, "host3", 0}, {4, "host4", 0}, {5, "host5", 7}},
			want: `host0  result code 0
host1  result code 0
host2  result code 0
host3  result code 0
host4  result code 0
host5  result code 7
`,
			wantCode: 7,
		},
		{id: 5, ins: []idHost{{0, "host0", 2}, {1, "host1", 0}, {2, "host2", 0}, {3, "host3", 0}, {4, "host4", 0}, {5, "host5", 0}},
			want: `host0  result code 2
host1  result code 0
host2  result code 0
host3  result code 0
host4  result code 0
host5  result code 0
`,
			wantCode: 2,
		},
		{id: 6, ins: []idHost{{0, "host0", 0}, {1, "host1", 0}, {2, "host2", 0}, {3, "host3", 4}, {4, "host4", 3}, {5, "host5", 0}},
			want: `host0  result code 0
host1  result code 0
host2  result code 0
host3  result code 4
host4  result code 3
host5  result code 0
`,
			wantCode: 4,
		},
	}
	for id, tc := range tests {
		p := &Pssh{
			Config: &Config{
				ShowHostName: true,
			},
		}
		p.print = newPrint(os.Stdout, os.Stderr, false)
		mock := mockPrin{}
		p.red = &mock
		p.boldRed = &mock
		p.green = &mock
		results := make(chan *result)
		cws := make([]*conWork, len(tc.ins))
		for _, c := range tc.ins {
			cws[c.id] = &conWork{
				id:   c.id,
				host: c.host,
			}
			//log.Printf("%v", *cws[i])
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			for _, c := range tc.ins {
				results <- &result{
					conID:  c.id,
					code:   c.code,
					stdout: newTestResultOutput(""),
					stderr: newTestResultOutput(""),
				}
			}
		}()
		code := p.printSortResults(ctx, results, cws)
		if mock.buf.String() != tc.want {
			t.Errorf("id=%d buf=%s, want:'%s'", id, mock.buf.String(), tc.want)
		}
		if code != tc.wantCode {
			t.Errorf("id=%d code=%d, want:'%d'", id, code, tc.wantCode)
		}
	}
}

func TestPsshRun(t *testing.T) {
	p := &Pssh{Config: &Config{
		Concurrency:     DefaultConcurrency,
		MaxAgentConns:   DefaultMaxAgentConns,
		MaxBufferMemory: DefaultMaxBufferMemory,
		MaxSpoolSize:    DefaultMaxSpoolSize,
	}}
	p.Hostsfile = "test/missing"
	b := bytes.Buffer{}
	oldFlags := log.Flags()
	oldOutput := log.Writer()
	t.Cleanup(func() {
		log.SetFlags(oldFlags)
		log.SetOutput(oldOutput)
	})
	log.SetFlags(0)
	log.SetOutput(&b)
	p.Init()
	p.Run()
	if !strings.HasPrefix(b.String(), "read hosts") {
		t.Errorf("b=%s,want:read hosts..", b.String())
	}
	p.IgnoreHostKey = true
	p.Hostsfile = "test/null"
	b.Reset()
	p.Run()
	if b.String() != "" {
		t.Errorf("b=%s,want:''", b.String())
	}
}

func TestSshKeyAgentCallback(t *testing.T) {
	p := &Pssh{Config: &Config{ColorMode: true}}
	p.Init()
	p.SSHAuthSocket = "/dev/null"
	p.conns = nil
	f := p.sshKeyAgentCallback()
	if f != nil {
		t.Error("f!=nil")
	}
	p.conns = newConnPools(p.SSHAuthSocket, p.MaxAgentConns)
	f = p.sshKeyAgentCallback()
	if f == nil {
		t.Error("f==nil")
	}

}
func TestGetIdentFilesAuthMethods(t *testing.T) {
	p := &Pssh{Config: &Config{ColorMode: true}}
	p.Init()
	p.SSHAuthSocket = "/dev/null"
	f := p.getIdentFileAuthMethods([][]byte{{}})
	if len(f) != 0 {
		t.Error("len(f)!=0")
	}
	f = p.getIdentFileAuthMethods([][]byte{testdata.PEMBytes["dsa"]})
	if len(f) != 1 {
		t.Errorf("len(f)==%d,want=1", len(f))
	}

}
func TestMergeAuthMethods(t *testing.T) {
	p := &Pssh{Config: &Config{ColorMode: true}}
	p.Init()
	p.IdentityFileOnly = false
	identMethods := p.getIdentFileAuthMethods([][]byte{testdata.PEMBytes["dsa"]})
	f := p.mergeAuthMethods(identMethods)
	if len(f) != 1 {
		t.Errorf("len(f)==%d,want=1", len(f))
	}
	p.IdentityFileOnly = true
	f = p.mergeAuthMethods([]ssh.AuthMethod{})
	if len(f) != 0 {
		t.Errorf("len(f)==%d,want=0", len(f))
	}
}

func TestNewConWork(t *testing.T) {
	var tests = []struct {
		id   int
		host string
	}{
		{1, "1"},
		{2, ""},
	}
	for _, test := range tests {
		p := &Pssh{
			Config: &Config{},
		}
		c := p.newConWork(test.id, test.host)
		if c.id != test.id {
			t.Errorf("c.id=%d,test.id=%d", c.id, test.id)
		}
		if c.host != test.host {
			t.Errorf("c.host=%s,test.host=%s", c.host, test.host)
		}
	}
}

func TestReadIdentFiles(t *testing.T) {
	var tests = []struct {
		home       string
		identFiles []string
		want       [][]byte
	}{
		{"./test", []string{"~/ident"}, [][]byte{[]byte("abc\n")}},
		{"./test", []string{"~/hoge"}, [][]byte{}},
	}
	for _, test := range tests {
		t.Setenv("HOME", test.home)
		p := &Pssh{Config: &Config{IdentFiles: test.identFiles}}
		res := p.readIdentFiles()
		if len(res) != len(test.want) {
			t.Errorf("res:%v,want:%v", res, test.want)
		}
		if len(test.want) > 0 {
			if !bytes.Equal(res[0], test.want[0]) {
				t.Errorf("res:%v,want:%v", res, test.want)
			}
		}
	}
}

func TestOutputFunc(t *testing.T) {
	p := &Pssh{Config: &Config{}}
	var tests = []struct {
		flag bool
		want func(ctx context.Context, results chan *result, cws []*conWork) int
	}{
		{true, p.printSortResults},
		{false, p.printResults},
	}
	for _, test := range tests {
		p.SortPrint = test.flag
		if reflect.ValueOf(p.outputFunc()).Pointer() != reflect.ValueOf(test.want).Pointer() {
			t.Error("outputFunc != test.want")
		}
	}
}
