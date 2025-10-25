package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/masahide/gopssh/pkg/pssh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const (
	// https://man.openbsd.org/ssh_config#IdentityFile
	defaultIdentityFiles = "~/.ssh/id_dsa,~/.ssh/id_ecdsa,~/.ssh/id_ed25519,~/.ssh/id_rsa"
	defaultMaxAgent      = 50
	defaultTimeout       = 15 * time.Second
	paramErrCode         = 2
)

// nolint: gochecknoglobals
var (
	defaultMacsFlags = []string{
		ssh.InsecureHMACSHA196,
		ssh.HMACSHA1,
		ssh.HMACSHA256ETM,
		ssh.HMACSHA512ETM,
		ssh.HMACSHA256,
		ssh.HMACSHA512,
	}
	defaultKexAlgos = []string{
		ssh.InsecureKeyExchangeDH1SHA1,
		ssh.InsecureKeyExchangeDH14SHA1,
		ssh.InsecureKeyExchangeDHGEXSHA1,
		ssh.KeyExchangeMLKEM768X25519,
		ssh.KeyExchangeCurve25519,
		ssh.KeyExchangeECDHP256,
		ssh.KeyExchangeECDHP384,
		ssh.KeyExchangeECDHP521,
		ssh.KeyExchangeDH14SHA256,
	}
	defaultCiphersFlags = []string{
		ssh.InsecureCipherRC4256,
		ssh.CipherAES128GCM,
		ssh.CipherAES256GCM,
		ssh.CipherChaCha20Poly1305,
		ssh.CipherAES128CTR,
		ssh.CipherAES192CTR,
		ssh.CipherAES256CTR,
	}
	version = "dev"
	commit  = "none"
	date    = "unknown"
	showVer = flag.Bool("version", false, "Show version")
)

func newConfig() *pssh.Config {
	kexAlgos := strings.Join(defaultKexAlgos, ",")
	ciphersFlag := strings.Join(defaultCiphersFlags, ",")
	macsFlag := strings.Join(defaultMacsFlags, ",")
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
	flag.StringVar(&kexAlgos, "kex", kexAlgos, "allowed key exchanges algorithms")
	flag.StringVar(&ciphersFlag, "ciphers", ciphersFlag, "allowed cipher algorithms")
	flag.StringVar(&macsFlag, "macs", macsFlag, "allowed MAC algorithms")
	flag.StringVar(&identityFiles, "i", identityFiles, "identity files")
	flag.Parse()
	c.Kex = pssh.ToSlice(kexAlgos)
	c.Ciphers = pssh.ToSlice(ciphersFlag)
	c.Macs = pssh.ToSlice(macsFlag)
	c.IdentityFileOnly = identityFiles != defaultIdentityFiles
	c.IdentFiles = pssh.ToSlice(identityFiles)

	// see: https://qiita.com/tanksuzuki/items/e712717675faf4efb07a#パイプで渡された時だけ処理する
	c.StdinFlag = !term.IsTerminal(0)
	return &c
}

func checkFlag(w io.Writer) (ret int, exit bool) {
	flag.CommandLine.SetOutput(w)
	if *showVer {
		// nolint: errcheck
		fmt.Fprintf(w, "version: %v\ncommit: %v\nbuilt_at: %v\n", version, commit, date)
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
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	p := &pssh.Pssh{Config: newConfig()}
	if ret, exit := checkFlag(os.Stdout); exit {
		os.Exit(ret)
	}
	p.Init()
	os.Exit(p.Run())
}
