package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
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
	c := defaultConfig()
	options := defaultLegacyFlagOptions()
	registerLegacyFlags(flag.CommandLine, &c, &options)
	flag.Parse()
	configureCrypto(&c, options.legacyCrypto, options.kexAlgos, options.ciphers, options.macs)
	c.IdentFiles = pssh.ToSlice(options.identityFiles)

	// see: https://qiita.com/tanksuzuki/items/e712717675faf4efb07a#パイプで渡された時だけ処理する
	c.StdinFlag = !term.IsTerminal(0)
	return &c
}

type legacyFlagOptions struct {
	kexAlgos      string
	ciphers       string
	macs          string
	identityFiles string
	legacyCrypto  bool
}

func defaultLegacyFlagOptions() legacyFlagOptions {
	return legacyFlagOptions{identityFiles: defaultIdentityFiles}
}

func registerLegacyFlags(fs *flag.FlagSet, c *pssh.Config, options *legacyFlagOptions) {
	fs.IntVar(&c.Concurrency, "p", c.Concurrency, "maximum concurrent SSH connections")
	fs.IntVar(&c.MaxAgentConns, "a", c.MaxAgentConns, "Max ssh agent unix socket connections")
	fs.Var((*byteSizeValue)(&c.MaxBufferMemory), "max-buffer-memory", "maximum total memory used for buffered remote output before spilling to disk")
	fs.Var((*byteSizeValue)(&c.MaxSpoolSize), "max-spool-size", "maximum total disk space used for spooled remote output")
	fs.StringVar(&c.SpoolDir, "spool-dir", c.SpoolDir, "parent directory for temporary output files (default: system temporary directory)")
	fs.StringVar(&c.User, "u", c.User, "username")
	fs.StringVar(&c.Hostsfile, "h", c.Hostsfile, "host file")
	fs.BoolVar(&c.SortPrint, "s", c.SortPrint, "sort the results and output")
	fs.BoolVar(&c.ShowHostName, "d", c.ShowHostName, "show hostname")
	fs.BoolVar(&c.ColorMode, "c", c.ColorMode, "colorized outputs")
	fs.BoolVar(&c.IgnoreHostKey, "k", c.IgnoreHostKey, "Do not check the host key")
	fs.BoolVar(&c.Debug, "debug", c.Debug, "debug outputs")
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, "maximum amount of time for the TCP connection to establish.")
	fs.BoolVar(&options.legacyCrypto, "legacy-crypto", options.legacyCrypto, "use the legacy SSH algorithms and priority order")
	fs.StringVar(&options.kexAlgos, "kex", options.kexAlgos, "comma-separated key exchange algorithms (default: secure SSH defaults)")
	fs.StringVar(&options.ciphers, "ciphers", options.ciphers, "comma-separated cipher algorithms (default: secure SSH defaults)")
	fs.StringVar(&options.macs, "macs", options.macs, "comma-separated MAC algorithms (default: secure SSH defaults)")
	fs.BoolVar(&c.IdentityFileOnly, "identities-only", false, "use identity files only and disable SSH Agent authentication")
	fs.StringVar(&options.identityFiles, "i", options.identityFiles, "identity files")
}

func defaultConfig() pssh.Config {
	return pssh.Config{
		Concurrency:     pssh.DefaultConcurrency,
		MaxAgentConns:   pssh.DefaultMaxAgentConns,
		MaxBufferMemory: pssh.DefaultMaxBufferMemory,
		MaxSpoolSize:    pssh.DefaultMaxSpoolSize,
		User:            os.Getenv("USER"),
		Hostsfile:       "",
		ShowHostName:    false,
		ColorMode:       true,
		IgnoreHostKey:   false,
		Debug:           false,
		SortPrint:       true,
		Timeout:         defaultTimeout,
		SSHAuthSocket:   os.Getenv("SSH_AUTH_SOCK"),
	}
}

type byteSizeValue int64

func (v *byteSizeValue) Set(input string) error {
	size, err := parseByteSize(input)
	if err != nil {
		return err
	}
	*v = byteSizeValue(size)
	return nil
}

func (v *byteSizeValue) String() string {
	if v == nil {
		return ""
	}
	return formatByteSize(int64(*v))
}

func (v *byteSizeValue) Get() any {
	return int64(*v)
}

func parseByteSize(input string) (int64, error) {
	value := strings.TrimSpace(input)
	upper := strings.ToUpper(value)
	multiplier := int64(1)
	for _, unit := range []struct {
		suffix     string
		multiplier int64
	}{
		{"GIB", 1 << 30},
		{"MIB", 1 << 20},
		{"KIB", 1 << 10},
		{"B", 1},
	} {
		if strings.HasSuffix(upper, unit.suffix) {
			multiplier = unit.multiplier
			value = strings.TrimSpace(value[:len(value)-len(unit.suffix)])
			break
		}
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number <= 0 || number > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("invalid byte size %q", input)
	}
	return number * multiplier, nil
}

func formatByteSize(size int64) string {
	for _, unit := range []struct {
		suffix string
		size   int64
	}{
		{"GiB", 1 << 30},
		{"MiB", 1 << 20},
		{"KiB", 1 << 10},
	} {
		if size >= unit.size && size%unit.size == 0 {
			return fmt.Sprintf("%d%s", size/unit.size, unit.suffix)
		}
	}
	return fmt.Sprintf("%dB", size)
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
	if isModern(os.Args[1:]) {
		log.SetFlags(0)
		ctx, cancel := context.WithCancelCause(context.Background())
		signalChannel := make(chan os.Signal, 1)
		signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(signalChannel)
		go func() {
			select {
			case received := <-signalChannel:
				cancel(fmt.Errorf("received signal: %s", received))
			case <-ctx.Done():
			}
		}()
		code := executeModern(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
		code = signalExitCode(code, context.Cause(ctx))
		cancel(nil)
		os.Exit(code)
	}
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := p.RunContext(ctx)
	stop()
	os.Exit(code)
}

func signalExitCode(code int, cause error) int {
	if cause == nil {
		return code
	}
	if strings.Contains(cause.Error(), syscall.SIGTERM.String()) {
		return 143
	}
	return 130
}
