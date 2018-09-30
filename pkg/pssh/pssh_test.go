package pssh

import (
	"testing"

	"github.com/fatih/color"
	"github.com/pkg/errors"
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
	}

}

func TestNewResult(t *testing.T) {
	r := newResult(1, 2)
	if r.conID != 1 {
		t.Errorf("conID:%d, want %d", r.conID, 1)
	}
	if r.sessionID != 2 {
		t.Errorf("sessionID:%d, want %d", r.sessionID, 2)
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
