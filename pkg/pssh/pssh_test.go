package pssh

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
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
		p.print = newPrint(os.Stdout, false)
		p.conInstances = make(chan conInstance, len(tc.ins))
		for _, c := range tc.ins {
			p.conInstances <- conInstance{
				err:     errors.New("hoge"),
				conWork: &conWork{id: c.id, host: c.host},
			}
		}
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
					stdout: &bytes.Buffer{},
					stderr: &bytes.Buffer{},
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
		p.print = newPrint(os.Stdout, false)
		p.conInstances = make(chan conInstance, len(tc.ins))
		for _, c := range tc.ins {
			p.conInstances <- conInstance{
				err:     errors.New("hoge"),
				conWork: &conWork{id: c.id, host: c.host},
			}
		}
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
					stdout: &bytes.Buffer{},
					stderr: &bytes.Buffer{},
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
		os.Setenv("HOME", test.home)
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
