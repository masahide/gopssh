package pssh

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/testdata"
)

/*
type testWriter struct{ result []byte }

func newTestWriter() *testWriter { return &testWriter{[]byte{}} }

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.result = append(w.result, p...)
	return len(w.result), nil
}
*/

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
	/*
		tw := newTestWriter()
		log.SetOutput(tw)
		defer log.SetOutput(os.Stderr)
		log.SetFlags(0)
		defer log.SetFlags(log.LstdFlags)
	*/
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
		{false, &print{}},
		{true, color.New()},
	}
	for _, test := range tests {
		p := &Pssh{
			Config: &Config{ColorMode: test.colorMode},
		}
		p.Init()
		if _, ok := test.want.(*print); ok {
			if _, ok := p.red.(*print); !ok {
				t.Errorf("res type :%T, want %T", p.red, test.want)
			}
		}
		if _, ok := test.want.(*color.Color); ok {
			if _, ok := p.red.(*color.Color); !ok {
				t.Errorf("res type :%T, want %T", p.red, test.want)
			}
		}
		if p.stdoutPool.Get().(*bytes.Buffer).Len() != 0 {
			t.Errorf("len:%d,want:0", p.stdoutPool.Get().(*bytes.Buffer).Len())
		}
		if p.stderrPool.Get().(*bytes.Buffer).Len() != 0 {
			t.Errorf("len:%d,want:0", p.stderrPool.Get().(*bytes.Buffer).Len())
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
			if err.Error() != test.err.Error() {
				t.Errorf("err:%s,want:%s", err.Error(), test.err.Error())
			}
		}
		if !sliceEq(r, test.want) {
			t.Errorf("r:%v, want:%v", r, test.want)
		}
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
	os.Setenv("HOME", "./test")
	r, err = getHostKeyCallback(false)
	if err != nil {
		t.Error(err)
	}
	if r == nil {
		t.Error("r:nil, want:not nil")
	}
	os.Setenv("HOME", "/dev/null")
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
		output: buf,
	}
	p.Print("hoge")
	if buf.String() != "hoge" {
		t.Errorf("buf:%s, want:hoge", buf.String())
	}
	buf.Reset()
	p.Printf("fuga%s", "hoge")
	if buf.String() != "fugahoge" {
		t.Errorf("buf:%s, want:fugahoge", buf.String())
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
	if i != 1 {
		t.Error("i!=1")
	}

}

func TestGetConInstanceErrs(t *testing.T) {
	p := &Pssh{}
	p.conInstances = make(chan conInstance, 1)
	p.conInstances <- conInstance{
		err:     errors.New("hoge"),
		conWork: &conWork{host: "host1"},
	}
	close(p.conInstances)
	p.cws = []*conWork{{}}
	err := p.getConInstanceErrs()
	if err.Error() != "host:host1 err:hoge" {
		t.Errorf("err=%s,want:host:host1 err:hoge", err.Error())
	}
	p.conInstances = make(chan conInstance, 1)
	p.conInstances <- conInstance{
		conWork: &conWork{host: ""},
		err:     nil,
	}
	close(p.conInstances)
	p.cws = []*conWork{{}}
	err = p.getConInstanceErrs()
	if err != nil {
		t.Error("err != nil")
	}
}

type mockPrin struct {
	buf bytes.Buffer
}

func (p *mockPrin) Print(a ...interface{}) (n int, err error) {
	fmt.Fprint(&p.buf, a...)
	return 0, nil
}
func (p *mockPrin) Printf(format string, a ...interface{}) (n int, err error) {
	fmt.Fprintf(&p.buf, format, a...)
	return 0, nil
}

func TestPrintResults(t *testing.T) {
	p := &Pssh{
		Config: &Config{
			ShowHostName: true,
		},
	}
	p.print = newPrint(os.Stdout, false)
	p.conInstances = make(chan conInstance, 1)
	p.conInstances <- conInstance{
		err:     errors.New("hoge"),
		conWork: &conWork{host: "host1"},
	}
	mock := mockPrin{}
	p.red = &mock
	p.boldRed = &mock
	p.green = &mock
	results := make(chan *result)
	cws := []*conWork{
		{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		results <- &result{
			conID:  0,
			stdout: &bytes.Buffer{},
			stderr: &bytes.Buffer{},
		}
	}()
	p.printResults(ctx, results, cws)
	if !strings.HasPrefix(mock.buf.String(), "  reslut code 0") {
		t.Errorf("buf=%s, want:'  reslut code 0'", mock.buf.String())
	}
}

func TestPsshRun(t *testing.T) {
	p := &Pssh{Config: &Config{}}
	p.Hostsfile = "test/null"
	b := bytes.Buffer{}
	log.SetFlags(0)
	log.SetOutput(&b)
	p.Init()
	p.Run()
	if !strings.HasPrefix(b.String(), "read hosts") {
		t.Errorf("b=%s,want:read hosts..", b.String())
	}
	p.IgnoreHostKey = true
	b.Reset()
	p.Run()
	if b.String() != "" {
		t.Errorf("b=%s,want:''", b.String())
	}
}

func TestDialSocket(t *testing.T) {
	p := &Pssh{Config: &Config{ColorMode: true}}
	p.Init()
	p.netDialer = mockNetDial{}
	var authConn net.Conn
	err := p.dialSocket(&authConn, "")
	if err != nil {
		t.Error(err)
	}
}
func TestSshKeyAgentCallback(t *testing.T) {
	p := &Pssh{Config: &Config{ColorMode: true}}
	p.Init()
	p.netDialer = mockNetDial{}
	p.SSHAuthSocket = "/dev/null"
	f := p.sshKeyAgentCallback()
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
func TestMerageAuthMethods(t *testing.T) {
	p := &Pssh{Config: &Config{ColorMode: true}}
	p.Init()
	p.netDialer = mockNetDial{}
	p.IdentityFileOnly = false
	identMethods := p.getIdentFileAuthMethods([][]byte{testdata.PEMBytes["dsa"]})
	k, f := p.merageAuthMethods(identMethods)
	if len(f) != 1 {
		t.Errorf("len(f)==%d,want=1", len(f))
	}
	if k != nil {
		t.Error("k!=nil")
	}
	p.IdentityFileOnly = true
	k, f = p.merageAuthMethods([]ssh.AuthMethod{})
	if len(f) != 0 {
		t.Errorf("len(f)==%d,want=0", len(f))
	}
	if k != nil {
		t.Error("k!=nil")
	}

}
