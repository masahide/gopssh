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
	defaultTimeout       = 15 * time.Second
	paramErrCode         = 2
)

// nolint: gochecknoglobals
var (
	legacyMacs = []string{
		ssh.InsecureHMACSHA196,
		ssh.HMACSHA1,
		ssh.HMACSHA256ETM,
		ssh.HMACSHA512ETM,
		ssh.HMACSHA256,
		ssh.HMACSHA512,
	}
	legacyKexAlgos = []string{
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
	legacyCiphers = []string{
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
	kexAlgos := ""
	ciphersFlag := ""
	macsFlag := ""
	identityFiles := defaultIdentityFiles
	legacyCrypto := false
	c := defaultConfig()
	flag.IntVar(&c.Concurrency, "p", c.Concurrency, "maximum concurrent SSH connections")
	flag.IntVar(&c.MaxAgentConns, "a", c.MaxAgentConns, "Max ssh agent unix socket connections")
	flag.Int64Var(&c.MaxOutputBytes, "max-output-bytes", c.MaxOutputBytes, "maximum buffered bytes per stdout/stderr stream and host")
	flag.StringVar(&c.User, "u", c.User, "username")
	flag.StringVar(&c.Hostsfile, "h", c.Hostsfile, "host file")
	flag.BoolVar(&c.SortPrint, "s", c.SortPrint, "sort the results and output")
	flag.BoolVar(&c.ShowHostName, "d", c.ShowHostName, "show hostname")
	flag.BoolVar(&c.ColorMode, "c", c.ColorMode, "colorized outputs")
	flag.BoolVar(&c.IgnoreHostKey, "k", c.IgnoreHostKey, "Do not check the host key")
	flag.BoolVar(&c.Debug, "debug", c.Debug, "debug outputs")
	flag.DurationVar(&c.Timeout, "timeout", c.Timeout, "maximum amount of time for the TCP connection to establish.")
	flag.BoolVar(&legacyCrypto, "legacy-crypto", legacyCrypto, "use the legacy SSH algorithms and priority order")
	flag.StringVar(&kexAlgos, "kex", kexAlgos, "comma-separated key exchange algorithms (default: secure SSH defaults)")
	flag.StringVar(&ciphersFlag, "ciphers", ciphersFlag, "comma-separated cipher algorithms (default: secure SSH defaults)")
	flag.StringVar(&macsFlag, "macs", macsFlag, "comma-separated MAC algorithms (default: secure SSH defaults)")
	flag.BoolVar(&c.IdentityFileOnly, "identities-only", false, "use identity files only and disable SSH Agent authentication")
	flag.StringVar(&identityFiles, "i", identityFiles, "identity files")
	flag.Parse()
	configureCrypto(&c, legacyCrypto, kexAlgos, ciphersFlag, macsFlag)
	c.IdentFiles = pssh.ToSlice(identityFiles)

	// see: https://qiita.com/tanksuzuki/items/e712717675faf4efb07a#パイプで渡された時だけ処理する
	c.StdinFlag = !term.IsTerminal(0)
	return &c
}

func defaultConfig() pssh.Config {
	return pssh.Config{
		Concurrency:    pssh.DefaultConcurrency,
		MaxAgentConns:  pssh.DefaultMaxAgentConns,
		MaxOutputBytes: pssh.DefaultMaxOutputBytes,
		User:           os.Getenv("USER"),
		Hostsfile:      "",
		ShowHostName:   false,
		ColorMode:      true,
		IgnoreHostKey:  false,
		Debug:          false,
		SortPrint:      true,
		Timeout:        defaultTimeout,
		SSHAuthSocket:  os.Getenv("SSH_AUTH_SOCK"),
	}
}

func configureCrypto(c *pssh.Config, legacy bool, kexAlgos, ciphers, macs string) {
	if legacy {
		if kexAlgos == "" {
			kexAlgos = strings.Join(legacyKexAlgos, ",")
		}
		if ciphers == "" {
			ciphers = strings.Join(legacyCiphers, ",")
		}
		if macs == "" {
			macs = strings.Join(legacyMacs, ",")
		}
	}
	c.Kex = pssh.ToSlice(kexAlgos)
	c.Ciphers = pssh.ToSlice(ciphers)
	c.Macs = pssh.ToSlice(macs)
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
	if err := p.Validate(); err != nil {
		log.Print(err)
		os.Exit(paramErrCode)
	}
	p.Init()
	os.Exit(p.Run())
}
