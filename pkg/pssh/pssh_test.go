package pssh

import (
	"bytes"
	"net"
	"os"
	"testing"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/agent"
)

type testWriter struct{ result []byte }

func newTestWriter() *testWriter { return &testWriter{[]byte{}} }

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.result = append(w.result, p...)
	return len(w.result), nil
}

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
		res := toSlice(test.s)
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
		{false, print{}},
		{true, color.New()},
	}
	for _, test := range tests {
		p := &Pssh{
			Config: &Config{ColorMode: test.colorMode},
		}
		p.Init()
		if _, ok := test.want.(print); ok {
			if _, ok := p.red.(print); !ok {
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

func TestNewResult(t *testing.T) {
	s := &sessionWork{
		id: 2,
		con: &conWork{
			id: 1,
		},
	}
	r := s.newResult()
	if r.conID != 1 {
		t.Errorf("conID:%d, want %d", r.conID, 1)
	}
	if r.sessionID != 2 {
		t.Errorf("sessionID:%d, want %d", r.sessionID, 2)
	}
	delReslt(r)
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
}
