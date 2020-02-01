package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/masahide/gopssh/pkg/pssh"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	defaultKexFlags     = "diffie-hellman-group1-sha1,diffie-hellman-group14-sha1,ecdh-sha2-nistp256,ecdh-sha2-nistp384,ecdh-sha2-nistp521,curve25519-sha256@libssh.org"
	defaultCiphersFlags = "arcfour256,aes128-gcm@openssh.com,chacha20-poly1305@openssh.com,aes128-ctr,aes192-ctr,aes256-ctr"
	defaultMacsFlags    = "hmac-sha1-96,hmac-sha1,hmac-sha2-256,hmac-sha2-256-etm@openssh.com"
	// https://man.openbsd.org/ssh_config#IdentityFile
	defaultIdentityFiles = "~/.ssh/id_dsa,~/.ssh/id_ecdsa,~/.ssh/id_ed25519,~/.ssh/id_rsa"
	defaultMaxAgent      = 50
	defaultTimeout       = 15 * time.Second
	paramErrCode         = 2
)

// nolint: gochecknoglobals
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	showVer = flag.Bool("version", false, "Show version")
)

func newConfig() *pssh.Config {
	kexFlag := defaultKexFlags
	ciphersFlag := defaultCiphersFlags
	macsFlag := defaultMacsFlags
	identityFiles := defaultIdentityFiles
	c := pssh.Config{
		Concurrency:   0,
		MaxAgentConns: defaultMaxAgent,
		User:          os.Getenv("USER"),
		Hostsfile:     "",
		ShowHostName:  false,
		ColorMode:     true,
		IgnoreHostKey: false,
		Debug:         false,
		SortPrint:     true,
		Timeout:       defaultTimeout,
		SSHAuthSocket: os.Getenv("SSH_AUTH_SOCK"),
	}
	flag.IntVar(&c.Concurrency, "p", c.Concurrency, "concurrency (defalut \"0\" is unlimit)")
	flag.IntVar(&c.MaxAgentConns, "a", c.MaxAgentConns, "Max ssh agent unix socket connections")
	flag.StringVar(&c.User, "u", c.User, "username")
	flag.StringVar(&c.Hostsfile, "h", c.Hostsfile, "host file")
	flag.BoolVar(&c.SortPrint, "s", c.SortPrint, "sort the results and output")
	flag.BoolVar(&c.ShowHostName, "d", c.ShowHostName, "show hostname")
	flag.BoolVar(&c.ColorMode, "c", c.ColorMode, "colorized outputs")
	flag.BoolVar(&c.IgnoreHostKey, "k", c.IgnoreHostKey, "Do not check the host key")
	flag.BoolVar(&c.Debug, "debug", c.Debug, "debug outputs")
	flag.DurationVar(&c.Timeout, "timeout", c.Timeout, "maximum amount of time for the TCP connection to establish.")
	flag.StringVar(&kexFlag, "kex", kexFlag, "allowed key exchanges algorithms")
	flag.StringVar(&ciphersFlag, "ciphers", ciphersFlag, "allowed cipher algorithms")
	flag.StringVar(&macsFlag, "macs", macsFlag, "allowed MAC algorithms")
	flag.StringVar(&identityFiles, "i", identityFiles, "identity files")
	flag.Parse()
	c.Kex = pssh.ToSlice(kexFlag)
	c.Ciphers = pssh.ToSlice(ciphersFlag)
	c.Macs = pssh.ToSlice(macsFlag)
	c.IdentityFileOnly = identityFiles != defaultIdentityFiles
	c.IdentFiles = pssh.ToSlice(identityFiles)

	// see: https://qiita.com/tanksuzuki/items/e712717675faf4efb07a#パイプで渡された時だけ処理する
	c.StdinFlag = !terminal.IsTerminal(0)
	return &c
}

func checkFlag(w io.Writer) (ret int, exit bool) {
	flag.CommandLine.SetOutput(w)
	if *showVer {
		// nolint: errcheck
		fmt.Fprintf(w, "%v, commit %v, built at %v\n", version, commit, date)
		return 0, true
	}
	if flag.NArg() == 0 {
		flag.Usage()
		// nolint: errcheck
		fmt.Fprintf(w, "example:\n$ ./gopssh -h <(echo host1 host2) ls -la /etc/\n")
		return paramErrCode, true
	}
	return 0, false
}

func main() {
	p := &pssh.Pssh{Config: newConfig()}
	if ret, exit := checkFlag(os.Stdout); exit {
		os.Exit(ret)
	}
	p.Init()
	os.Exit(p.Run())
}
