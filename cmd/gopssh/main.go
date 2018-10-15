package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/masahide/gopssh/pkg/pssh"
)

var (
	// Version is version number
	Version = "dev"
	// Date is build date
	Date    = ""
	showVer = flag.Bool("version", false, "Show version")
)

func newConfig() *pssh.Config {
	kexFlag := "diffie-hellman-group1-sha1,diffie-hellman-group14-sha1,ecdh-sha2-nistp256,ecdh-sha2-nistp384,ecdh-sha2-nistp521,curve25519-sha256@libssh.org"
	ciphersFlag := "arcfour256,aes128-gcm@openssh.com,chacha20-poly1305@openssh.com,aes128-ctr,aes192-ctr,aes256-ctr"
	macsFlag := "hmac-sha1-96,hmac-sha1,hmac-sha2-256,hmac-sha2-256-etm@openssh.com"
	c := pssh.Config{
		Concurrency:   0,
		User:          os.Getenv("USER"),
		Hostsfile:     "",
		ShowHostName:  false,
		ColorMode:     true,
		IgnoreHostKey: false,
		Debug:         false,
		Timeout:       5 * time.Second,
		SSHAuthSocket: os.Getenv("SSH_AUTH_SOCK"),
	}
	//stdinFlag             = flag.Bool("i", false, "read stdin")
	flag.IntVar(&c.Concurrency, "p", c.Concurrency, "concurrency (defalut \"0\" is unlimit)")
	flag.StringVar(&c.User, "u", c.User, "username")
	flag.StringVar(&c.Hostsfile, "h", c.Hostsfile, "host file")
	flag.BoolVar(&c.ShowHostName, "d", c.ShowHostName, "show hostname")
	flag.BoolVar(&c.ColorMode, "c", c.ColorMode, "colorized outputs")
	flag.BoolVar(&c.IgnoreHostKey, "k", c.IgnoreHostKey, "Do not check the host key")
	flag.BoolVar(&c.Debug, "debug", c.Debug, "debug outputs")
	flag.DurationVar(&c.Timeout, "timeout", c.Timeout, "maximum amount of time for the TCP connection to establish.")
	flag.StringVar(&kexFlag, "kex", kexFlag, "allowed key exchanges algorithms")
	flag.StringVar(&ciphersFlag, "ciphers", ciphersFlag, "allowed cipher algorithms")
	flag.StringVar(&macsFlag, "macs", macsFlag, "allowed MAC algorithms")
	flag.Parse()
	c.Kex = pssh.ToSlice(kexFlag)
	c.Ciphers = pssh.ToSlice(ciphersFlag)
	c.Macs = pssh.ToSlice(macsFlag)
	return &c
}

func checkFlag(w io.Writer) (ret int, exit bool) {
	flag.CommandLine.SetOutput(w)
	if *showVer {
		fmt.Fprintf(w, "version: %s %s\n", Version, Date)
		return 0, true
	}
	if flag.NArg() == 0 {
		flag.Usage()
		return 2, true
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
