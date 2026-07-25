package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/masahide/gopssh/pkg/pssh"
	"golang.org/x/term"
)

const (
	schemaVersion = "1"
	maxStdinSize  = 64 << 20
)

var modernCommands = []string{"run", "doctor", "hosts", "config", "version", "completion", "help"}

type usageError struct {
	Code         string   `json:"code"`
	Message      string   `json:"message"`
	CommandPath  []string `json:"command_path"`
	InvalidToken string   `json:"invalid_token,omitempty"`
	Suggestions  []string `json:"suggestions"`
	Usage        string   `json:"usage"`
	HelpCommand  string   `json:"help_command"`
}

type usageEnvelope struct {
	SchemaVersion string     `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         usageError `json:"error"`
}

type commandError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type stringList []string

func (v *stringList) String() string { return strings.Join(*v, ",") }
func (v *stringList) Set(value string) error {
	*v = append(*v, value)
	return nil
}

type runOptions struct {
	config       pssh.Config
	hostsFile    string
	hosts        stringList
	identities   stringList
	command      string
	stdin        bool
	stdinFile    string
	dryRun       bool
	json         bool
	order        string
	color        string
	outputDir    string
	exitPolicy   string
	legacyCrypto bool
	identitySet  bool
	kex          string
	ciphers      string
	macs         string
}

func isModern(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "--help" {
		return true
	}
	if args[0] == "--json" {
		return len(args) > 1 && contains(modernCommands, args[1])
	}
	return contains(modernCommands, args[0]) || !strings.HasPrefix(args[0], "-")
}

func executeModern(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	jsonMode := false
	if len(args) > 0 && args[0] == "--json" {
		jsonMode = true
		args = args[1:]
	}
	if len(args) == 0 || args[0] == "--help" {
		if _, err := fmt.Fprint(stdout, topHelp()); err != nil {
			return 1
		}
		return 0
	}
	command := args[0]
	if !contains(modernCommands, command) {
		return renderUsageError(stdout, stderr, jsonMode, newUsageError(
			"unknown_command", fmt.Sprintf("unknown command %q for %q", command, "gopssh"),
			[]string{"gopssh"}, command, suggest(command, modernCommands), topUsage(),
		))
	}
	args = args[1:]
	switch command {
	case "help":
		return runHelp(args, stdout, stderr, jsonMode)
	case "run":
		return runModern(ctx, args, stdin, stdout, stderr, jsonMode)
	case "doctor":
		return runDoctor(ctx, args, stdout, stderr, jsonMode)
	case "hosts":
		return runHosts(args, stdout, stderr, jsonMode)
	case "config":
		return runConfig(args, stdout, stderr, jsonMode)
	case "version":
		return runVersion(args, stdout, stderr, jsonMode)
	case "completion":
		return runCompletion(args, stdout, stderr, jsonMode)
	default:
		panic("unreachable")
	}
}

func newUsageError(code, message string, path []string, token string, suggestions []string, usage string) *usageError {
	if suggestions == nil {
		suggestions = []string{}
	}
	return &usageError{
		Code: code, Message: message, CommandPath: path, InvalidToken: token,
		Suggestions: suggestions, Usage: usage, HelpCommand: strings.Join(path, " ") + " --help",
	}
}

func renderUsageError(stdout, stderr io.Writer, jsonMode bool, usageErr *usageError) int {
	if jsonMode {
		_ = json.NewEncoder(stdout).Encode(usageEnvelope{SchemaVersion: schemaVersion, OK: false, Error: *usageErr})
	}
	_, _ = fmt.Fprintf(stderr, "Error: %s\n", usageErr.Message)
	if len(usageErr.Suggestions) == 1 {
		_, _ = fmt.Fprintf(stderr, "Did you mean %q?\n", usageErr.Suggestions[0])
	} else if len(usageErr.Suggestions) > 1 {
		_, _ = fmt.Fprintf(stderr, "Did you mean one of: %s?\n", strings.Join(usageErr.Suggestions, ", "))
	}
	_, _ = fmt.Fprintf(stderr, "\n%s\n\nRun '%s' for full help.\n", helpForPath(usageErr.CommandPath), usageErr.HelpCommand)
	return paramErrCode
}

func parseFlagError(err error, path []string, known []string, usage string) *usageError {
	message := err.Error()
	code := "invalid_argument"
	token := ""
	suggestions := []string{}
	if strings.HasPrefix(message, "flag provided but not defined: ") {
		code = "unknown_option"
		token = strings.TrimPrefix(message, "flag provided but not defined: ")
		suggestions = suggest(token, known)
	}
	return newUsageError(code, message, path, token, suggestions, usage)
}

func runHelp(args []string, stdout, stderr io.Writer, jsonMode bool) int {
	if jsonMode {
		return renderUsageError(stdout, stderr, true, newUsageError(
			"invalid_argument", "--json is not supported for help", []string{"gopssh", "help"}, "", nil, "gopssh help [command [subcommand]]",
		))
	}
	path := []string{"gopssh"}
	for _, arg := range args {
		path = append(path, arg)
		if helpForPath(path) == "" {
			return renderUsageError(stdout, stderr, false, newUsageError(
				"unknown_command", fmt.Sprintf("unknown help topic %q", strings.Join(args, " ")),
				path[:len(path)-1], arg, nil, strings.Join(path[:len(path)-1], " ")+" <command>",
			))
		}
	}
	if _, err := fmt.Fprint(stdout, helpForPath(path)); err != nil {
		return 1
	}
	return 0
}

func defaultRunOptions() runOptions {
	c := defaultConfig()
	c.StdinFlag = false
	return runOptions{
		config: c, order: "input", color: "auto", exitPolicy: "first",
	}
}

func runFlagSet(options *runOptions) (*flag.FlagSet, []string) {
	fs := flag.NewFlagSet("gopssh run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&options.hostsFile, "hosts-file", "", "hosts file")
	fs.StringVar(&options.hostsFile, "H", "", "hosts file")
	fs.Var(&options.hosts, "host", "target")
	fs.StringVar(&options.config.User, "user", options.config.User, "SSH user")
	fs.StringVar(&options.config.User, "u", options.config.User, "SSH user")
	fs.IntVar(&options.config.Concurrency, "parallel", options.config.Concurrency, "parallel connections")
	fs.IntVar(&options.config.Concurrency, "p", options.config.Concurrency, "parallel connections")
	fs.IntVar(&options.config.MaxAgentConns, "max-agent-connections", options.config.MaxAgentConns, "agent connections")
	fs.Var(&options.identities, "identity", "identity file")
	fs.Var(&options.identities, "i", "identity file")
	fs.BoolVar(&options.config.IdentityFileOnly, "identities-only", false, "disable SSH Agent")
	fs.DurationVar(&options.config.Timeout, "connect-timeout", options.config.Timeout, "connect timeout")
	fs.BoolVar(&options.config.ShowHostName, "show-host", false, "show target")
	fs.StringVar(&options.order, "order", options.order, "input or completion")
	fs.StringVar(&options.color, "color", options.color, "auto, always, or never")
	fs.BoolVar(&options.config.IgnoreHostKey, "insecure-ignore-host-key", false, "skip known_hosts verification")
	fs.BoolVar(&options.legacyCrypto, "legacy-crypto", false, "use legacy SSH algorithms")
	fs.StringVar(&options.kex, "kex", "", "key exchange algorithms")
	fs.StringVar(&options.ciphers, "ciphers", "", "ciphers")
	fs.StringVar(&options.macs, "macs", "", "MACs")
	fs.Var((*byteSizeValue)(&options.config.MaxBufferMemory), "max-buffer-memory", "output memory limit")
	fs.Var((*byteSizeValue)(&options.config.MaxSpoolSize), "max-spool-size", "output spool limit")
	fs.StringVar(&options.config.SpoolDir, "spool-dir", "", "spool parent")
	fs.BoolVar(&options.config.Debug, "debug", false, "debug diagnostics")
	fs.BoolVar(&options.dryRun, "dry-run", false, "print plan without connecting")
	fs.BoolVar(&options.json, "json", options.json, "emit JSON or NDJSON")
	fs.StringVar(&options.outputDir, "output-dir", "", "save per-target output")
	fs.StringVar(&options.exitPolicy, "exit-policy", options.exitPolicy, "first, any, or always-zero")
	fs.StringVar(&options.command, "command", "", "literal remote shell command")
	fs.BoolVar(&options.stdin, "stdin", false, "forward process stdin")
	fs.StringVar(&options.stdinFile, "stdin-file", "", "forward file")
	known := []string{
		"--hosts-file", "-H", "--host", "--user", "-u", "--parallel", "-p",
		"--max-agent-connections", "--identity", "-i", "--identities-only",
		"--connect-timeout", "--show-host", "--order", "--color",
		"--insecure-ignore-host-key", "--legacy-crypto", "--kex", "--ciphers",
		"--macs", "--max-buffer-memory", "--max-spool-size", "--spool-dir",
		"--debug", "--dry-run", "--json", "--output-dir", "--exit-policy",
		"--command", "--stdin", "--stdin-file",
	}
	return fs, known
}

func runModern(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, globalJSON bool) int {
	if hasHelp(args) {
		if _, err := fmt.Fprint(stdout, runHelpText()); err != nil {
			return 1
		}
		return 0
	}
	options := defaultRunOptions()
	options.json = globalJSON || requestedJSON(args)
	fs, known := runFlagSet(&options)
	if err := fs.Parse(args); err != nil {
		return renderUsageError(stdout, stderr, globalJSON || options.json, parseFlagError(err, []string{"gopssh", "run"}, known, runUsage()))
	}
	options.json = options.json || globalJSON
	if len(options.identities) == 0 {
		options.identities = pssh.ToSlice(defaultIdentityFiles)
	}
	commandArgs := fs.Args()
	if options.command != "" && len(commandArgs) > 0 {
		return renderUsageError(stdout, stderr, options.json, newUsageError(
			"conflicting_options", "--command and command arguments after -- are mutually exclusive",
			[]string{"gopssh", "run"}, "--command", nil, runUsage(),
		))
	}
	if options.command == "" && len(commandArgs) == 0 {
		return renderUsageError(stdout, stderr, options.json, newUsageError(
			"missing_argument", "remote command is required", []string{"gopssh", "run"}, "", nil, runUsage(),
		))
	}
	if options.command == "" {
		options.command = shellJoin(commandArgs)
	}
	if err := validateRunOptions(&options); err != nil {
		return renderUsageError(stdout, stderr, options.json, newUsageError(
			"invalid_argument", err.Error(), []string{"gopssh", "run"}, "", nil, runUsage(),
		))
	}
	targets, err := loadTargets(options.hostsFile, options.hosts)
	if err != nil {
		return renderUsageError(stdout, stderr, options.json, newUsageError(
			"hosts_file_invalid", err.Error(), []string{"gopssh", "run"}, options.hostsFile, nil, runUsage(),
		))
	}
	if len(targets) == 0 {
		return renderUsageError(stdout, stderr, options.json, newUsageError(
			"missing_argument", "at least one --hosts-file or --host target is required",
			[]string{"gopssh", "run"}, "", nil, runUsage(),
		))
	}
	stdinData, err := readStdin(options, stdin)
	if err != nil {
		return renderUsageError(stdout, stderr, options.json, newUsageError(
			"invalid_argument", err.Error(), []string{"gopssh", "run"}, "", nil, runUsage(),
		))
	}
	options.config.Targets = targets
	options.config.Command = options.command
	options.config.Stdin = stdinData
	options.config.IdentFiles = options.identities
	options.config.SortPrint = options.order == "input"
	options.config.ColorMode = options.color != "never" && !options.json
	options.config.ColorAlways = options.color == "always" && !options.json
	options.config.Stdout = stdout
	options.config.Stderr = stderr
	options.config.ExitPolicy = options.exitPolicy
	configureCrypto(&options.config, options.legacyCrypto, options.kex, options.ciphers, options.macs)
	if options.dryRun {
		return printDryRun(options, targets, stdout)
	}
	if err := preflightRun(options); err != nil {
		return renderCommandError(stdout, stderr, options.json, err)
	}
	return executeRun(ctx, options, targets, stdout, stderr)
}

func preflightRun(options runOptions) *commandError {
	if !options.config.IgnoreHostKey {
		knownHosts := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
		if err := pssh.ValidateHostKeyPolicy(false); err != nil {
			return &commandError{
				Code: "known_hosts_unavailable", Message: err.Error(),
				Details: map[string]any{"path": knownHosts},
			}
		}
	}
	if options.outputDir != "" {
		if _, err := prepareOutputDirectory(options.outputDir); err != nil {
			return &commandError{
				Code: "output_io_failed", Message: err.Error(),
				Details: map[string]any{"path": options.outputDir},
			}
		}
	}
	return nil
}

func renderCommandError(stdout, stderr io.Writer, jsonMode bool, commandErr *commandError) int {
	if jsonMode {
		_ = json.NewEncoder(stdout).Encode(map[string]any{
			"schema_version": schemaVersion,
			"ok":             false,
			"error":          commandErr,
		})
	} else {
		_, _ = fmt.Fprintf(stderr, "Error: %s\n", commandErr.Message)
	}
	return 1
}

func validateRunOptions(options *runOptions) error {
	switch options.order {
	case "input", "completion":
	default:
		return fmt.Errorf("--order must be input or completion")
	}
	switch options.color {
	case "auto", "always", "never":
	default:
		return fmt.Errorf("--color must be auto, always, or never")
	}
	switch options.exitPolicy {
	case "first", "any", "always-zero":
	default:
		return fmt.Errorf("--exit-policy must be first, any, or always-zero")
	}
	if options.config.Concurrency <= 0 || options.config.MaxAgentConns <= 0 {
		return fmt.Errorf("parallel limits must be greater than zero")
	}
	if options.stdin && options.stdinFile != "" {
		return fmt.Errorf("--stdin and --stdin-file are mutually exclusive")
	}
	return nil
}

func loadTargets(hostsFile string, inline []string) ([]string, error) {
	var targets []string
	if hostsFile != "" {
		if hostsFile == "-" {
			return nil, errors.New("--hosts-file - is not supported; use a named file")
		}
		entries, err := parseHostEntries(hostsFile)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Error != "" {
				return nil, fmt.Errorf("%s:%d: %s", hostsFile, entry.Line, entry.Error)
			}
			targets = append(targets, entry.Normalized)
		}
	}
	for _, value := range inline {
		target, err := normalizeModernHost(value)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

var dnsLabelPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func normalizeModernHost(value string) (string, error) {
	normalized, err := pssh.NormalizeHost(value)
	if err != nil {
		return "", err
	}
	host, _, err := net.SplitHostPort(normalized)
	if err != nil {
		return "", err
	}
	if net.ParseIP(host) != nil {
		return normalized, nil
	}
	parts := strings.Split(host, ".")
	if len(parts) == 4 {
		allNumeric := true
		for _, part := range parts {
			if _, err := strconv.Atoi(part); err != nil {
				allNumeric = false
				break
			}
		}
		if allNumeric {
			return "", fmt.Errorf("invalid IPv4 address %q", host)
		}
	}
	if len(host) > 253 {
		return "", fmt.Errorf("invalid DNS name %q", host)
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if !dnsLabelPattern.MatchString(label) {
			return "", fmt.Errorf("invalid DNS name %q", host)
		}
	}
	return normalized, nil
}

func readStdin(options runOptions, stdin io.Reader) ([]byte, error) {
	if !options.stdin && options.stdinFile == "" {
		return []byte{}, nil
	}
	reader := stdin
	var file *os.File
	if options.stdinFile != "" {
		var err error
		file, err = os.Open(options.stdinFile)
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxStdinSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxStdinSize {
		return nil, fmt.Errorf("stdin exceeds the 64MiB limit")
	}
	return data, nil
}

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " ")
}

func printDryRun(options runOptions, targets []string, stdout io.Writer) int {
	auth := []string{"identity-files"}
	if !options.config.IdentityFileOnly && options.config.SSHAuthSocket != "" {
		auth = append([]string{"ssh-agent"}, auth...)
	}
	plan := map[string]any{
		"schema_version":        schemaVersion,
		"type":                  "dry_run",
		"targets":               targets,
		"user":                  options.config.User,
		"parallel":              options.config.Concurrency,
		"max_agent_connections": options.config.MaxAgentConns,
		"authentication":        auth,
		"host_key_policy":       map[bool]string{true: "insecure-ignore", false: "known-hosts"}[options.config.IgnoreHostKey],
		"connect_timeout":       options.config.Timeout.String(),
		"order":                 options.order,
		"color":                 options.color,
		"max_buffer_memory":     options.config.MaxBufferMemory,
		"max_spool_size":        options.config.MaxSpoolSize,
		"spool_dir":             options.config.SpoolDir,
		"command":               options.command,
		"stdin_bytes":           len(options.config.Stdin),
		"output_dir":            options.outputDir,
		"exit_policy":           options.exitPolicy,
	}
	if options.json {
		if err := json.NewEncoder(stdout).Encode(plan); err != nil {
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(stdout, "Dry run (no network connection)\nTargets: %d\n", len(targets)); err != nil {
		return 1
	}
	for _, target := range targets {
		if _, err := fmt.Fprintf(stdout, "  %s\n", target); err != nil {
			return 1
		}
	}
	if _, err := fmt.Fprintf(stdout, "User: %s\nParallel: %d\nAuthentication: %s\nHost key policy: %s\n",
		options.config.User, options.config.Concurrency, strings.Join(auth, ", "), plan["host_key_policy"]); err != nil {
		return 1
	}
	if _, err := fmt.Fprintf(stdout, "Command: %s\nOrder: %s\nColor: %s\nExit policy: %s\n",
		options.command, options.order, options.color, options.exitPolicy); err != nil {
		return 1
	}
	return 0
}

type runStats struct {
	total, succeeded, failed, connectionFailed, canceled int
}

func executeRun(ctx context.Context, options runOptions, targets []string, stdout, stderr io.Writer) int {
	stats := &runStats{total: len(targets)}
	seen := make(map[int]bool, len(targets))
	handler := func(result *pssh.Result) error {
		seen[result.Index] = true
		switch {
		case errors.Is(result.Err, context.Canceled):
			stats.canceled++
		case result.ExitCode == 255:
			stats.connectionFailed++
			stats.failed++
		case result.ExitCode != 0 || result.Err != nil:
			stats.failed++
		default:
			stats.succeeded++
		}
		if options.json {
			return writeJSONResult(stdout, result, options.outputDir)
		}
		if options.outputDir != "" {
			stdoutPath, stderrPath, err := writeOutputFiles(options.outputDir, result)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "%s stdout=%s stderr=%s exit_code=%d\n", result.Target, stdoutPath, stderrPath, result.ExitCode)
			return err
		}
		return writeTextResult(stdout, stderr, result, options.config.ShowHostName)
	}
	if options.json || options.outputDir != "" {
		options.config.ResultHandler = handler
	}
	engine := &pssh.Pssh{Config: &options.config}
	if err := engine.Validate(); err != nil {
		return 2
	}
	engine.Init()
	code := engine.RunContext(ctx)
	if options.json {
		if ctx.Err() != nil {
			for index, target := range targets {
				if seen[index] {
					continue
				}
				stats.canceled++
				canceledResult := &pssh.Result{
					Index: index, Target: target, ExitCode: 1, Err: context.Canceled,
					Stdout: emptyResultOutput{}, Stderr: emptyResultOutput{},
				}
				if err := writeJSONResult(stdout, canceledResult, options.outputDir); err != nil {
					return 1
				}
			}
		}
		code = signalExitCode(code, context.Cause(ctx))
		summary := map[string]any{
			"schema_version": schemaVersion, "type": "summary", "total": stats.total,
			"succeeded": stats.succeeded, "failed": stats.failed,
			"connection_failed": stats.connectionFailed, "canceled": stats.canceled,
			"aggregate_exit_code": code,
		}
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			return 1
		}
	}
	return code
}

type emptyResultOutput struct{}

func (emptyResultOutput) WriteTo(io.Writer) (int64, error) { return 0, nil }
func (emptyResultOutput) Size() int64                      { return 0 }

func writeTextResult(stdout, stderr io.Writer, result *pssh.Result, showHost bool) error {
	var headingErr, resultErr error
	if showHost {
		_, headingErr = fmt.Fprintf(stderr, "%s  result code %d\n", result.Target, result.ExitCode)
	}
	if result.Err != nil {
		_, resultErr = fmt.Fprintf(stderr, "result err: %s\n", result.Err)
	}
	_, stdoutErr := result.Stdout.WriteTo(stdout)
	_, stderrErr := result.Stderr.WriteTo(stderr)
	return errors.Join(headingErr, resultErr, stdoutErr, stderrErr)
}

func writeOutputFiles(directory string, result *pssh.Result) (string, string, error) {
	absolute, err := prepareOutputDirectory(directory)
	if err != nil {
		return "", "", err
	}
	base := fmt.Sprintf("%d-%s", result.Index, sanitizeTarget(result.Target))
	stdoutPath := filepath.Join(absolute, base+".stdout")
	stderrPath := filepath.Join(absolute, base+".stderr")
	if err := writeResultFile(stdoutPath, result.Stdout); err != nil {
		return "", "", err
	}
	if err := writeResultFile(stderrPath, result.Stderr); err != nil {
		return "", "", err
	}
	return stdoutPath, stderrPath, nil
}

func prepareOutputDirectory(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	switch {
	case err == nil && !info.IsDir():
		return "", fmt.Errorf("%s is not a directory", absolute)
	case err == nil:
		return absolute, nil
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return "", err
	}
	return absolute, nil
}

func writeResultFile(path string, output pssh.ResultOutput) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to overwrite symbolic link %s", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	_, copyErr := output.WriteTo(file)
	return errors.Join(copyErr, file.Close())
}

func sanitizeTarget(target string) string {
	var builder strings.Builder
	for _, r := range target {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func writeJSONResult(writer io.Writer, result *pssh.Result, outputDir string) error {
	status := "success"
	if errors.Is(result.Err, context.Canceled) {
		status = "canceled"
	} else if result.ExitCode == 255 {
		status = "connection_failed"
	} else if result.ExitCode != 0 || result.Err != nil {
		status = "failed"
	}
	errorMessage := any(nil)
	if result.Err != nil {
		errorMessage = result.Err.Error()
	}
	prefix := map[string]any{
		"schema_version": schemaVersion, "type": "result", "index": result.Index,
		"target": result.Target, "status": status, "exit_code": result.ExitCode,
		"error": errorMessage, "duration_ms": result.Duration.Milliseconds(),
	}
	data, err := json.Marshal(prefix)
	if err != nil {
		return err
	}
	if _, err := writer.Write(data[:len(data)-1]); err != nil {
		return err
	}
	if outputDir != "" {
		stdoutPath, stderrPath, err := writeOutputFiles(outputDir, result)
		if err != nil {
			return err
		}
		extra, _ := json.Marshal(map[string]any{
			"stdout_path": stdoutPath, "stderr_path": stderrPath,
			"stdout_bytes": result.Stdout.Size(), "stderr_bytes": result.Stderr.Size(),
		})
		if _, err := fmt.Fprintf(writer, ",%s\n", extra[1:]); err != nil {
			return err
		}
		return nil
	}
	if err := writeJSONOutputField(writer, "stdout", result.Stdout); err != nil {
		return err
	}
	if err := writeJSONOutputField(writer, "stderr", result.Stderr); err != nil {
		return err
	}
	_, err = io.WriteString(writer, "}\n")
	return err
}

type utf8Validator struct {
	tail  []byte
	valid bool
}

func (v *utf8Validator) Write(data []byte) (int, error) {
	original := len(data)
	if !v.valid {
		return original, nil
	}
	data = append(v.tail, data...)
	v.tail = v.tail[:0]
	for len(data) > 0 {
		if !utf8.FullRune(data) {
			v.tail = append(v.tail, data...)
			break
		}
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			v.valid = false
			break
		}
		data = data[size:]
	}
	return original, nil
}

func outputIsUTF8(output pssh.ResultOutput) (bool, error) {
	validator := &utf8Validator{valid: true}
	if _, err := output.WriteTo(validator); err != nil {
		return false, err
	}
	return validator.valid && len(validator.tail) == 0, nil
}

func writeJSONOutputField(writer io.Writer, name string, output pssh.ResultOutput) error {
	valid, err := outputIsUTF8(output)
	if err != nil {
		return err
	}
	if valid {
		if _, err := fmt.Fprintf(writer, ",%q:", name); err != nil {
			return err
		}
		stringWriter := &jsonStringWriter{writer: writer}
		if err := stringWriter.begin(); err != nil {
			return err
		}
		if _, err := output.WriteTo(stringWriter); err != nil {
			return err
		}
		if err := stringWriter.end(); err != nil {
			return err
		}
		_, err = fmt.Fprintf(writer, ",%q:%q", name+"_encoding", "utf-8")
		return err
	}
	if _, err := fmt.Fprintf(writer, ",%q:", name+"_base64"); err != nil {
		return err
	}
	if _, err := io.WriteString(writer, `"`); err != nil {
		return err
	}
	encoder := base64.NewEncoder(base64.StdEncoding, writer)
	if _, err := output.WriteTo(encoder); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, `",%q:%q`, name+"_encoding", "base64")
	return err
}

type jsonStringWriter struct {
	writer io.Writer
	tail   []byte
}

func (w *jsonStringWriter) begin() error {
	_, err := io.WriteString(w.writer, `"`)
	return err
}

func (w *jsonStringWriter) Write(data []byte) (int, error) {
	original := len(data)
	data = append(w.tail, data...)
	w.tail = w.tail[:0]
	for len(data) > 0 {
		if !utf8.FullRune(data) {
			w.tail = append(w.tail, data...)
			break
		}
		r, size := utf8.DecodeRune(data)
		quoted := strconv.QuoteRune(r)
		if r == '\'' {
			quoted = "'"
		} else {
			quoted = quoted[1 : len(quoted)-1]
		}
		if _, err := io.WriteString(w.writer, quoted); err != nil {
			return 0, err
		}
		data = data[size:]
	}
	return original, nil
}

func (w *jsonStringWriter) end() error {
	if len(w.tail) != 0 {
		return errors.New("invalid UTF-8 tail")
	}
	_, err := io.WriteString(w.writer, `"`)
	return err
}

type hostEntry struct {
	Index      int    `json:"index"`
	Original   string `json:"original"`
	Normalized string `json:"normalized,omitempty"`
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Duplicate  bool   `json:"duplicate"`
	Line       int    `json:"line"`
	Error      string `json:"error,omitempty"`
}

func parseHostEntries(path string) ([]hostEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var entries []hostEntry
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := strings.SplitN(scanner.Text(), "#", 2)[0]
		for _, value := range strings.Fields(text) {
			entry := hostEntry{Index: len(entries), Original: value, Line: line}
			normalized, normalizeErr := normalizeModernHost(value)
			if normalizeErr != nil {
				entry.Error = normalizeErr.Error()
			} else {
				entry.Normalized = normalized
				entry.Duplicate = seen[normalized]
				seen[normalized] = true
				host, port, _ := net.SplitHostPort(normalized)
				entry.Host = host
				entry.Port, _ = strconv.Atoi(port)
				ip := net.ParseIP(host)
				switch {
				case ip == nil:
					entry.Kind = "dns"
				case strings.Contains(host, ":"):
					entry.Kind = "ipv6"
				default:
					entry.Kind = "ipv4"
				}
			}
			entries = append(entries, entry)
		}
	}
	return entries, scanner.Err()
}

func runHosts(args []string, stdout, stderr io.Writer, globalJSON bool) int {
	globalJSON = globalJSON || requestedJSON(args)
	if len(args) == 0 {
		return renderUsageError(stdout, stderr, globalJSON, newUsageError(
			"missing_argument", "hosts subcommand is required", []string{"gopssh", "hosts"}, "", nil, hostsUsage(),
		))
	}
	if hasHelp(args[:1]) {
		if _, err := fmt.Fprint(stdout, hostsHelpText()); err != nil {
			return 1
		}
		return 0
	}
	subcommand := args[0]
	if subcommand != "list" && subcommand != "validate" {
		return renderUsageError(stdout, stderr, globalJSON, newUsageError(
			"unknown_subcommand", fmt.Sprintf("unknown subcommand %q for %q", subcommand, "gopssh hosts"),
			[]string{"gopssh", "hosts"}, subcommand, suggest(subcommand, []string{"list", "validate"}), hostsUsage(),
		))
	}
	path := []string{"gopssh", "hosts", subcommand}
	if hasHelp(args[1:]) {
		if _, err := fmt.Fprint(stdout, helpForPath(path)); err != nil {
			return 1
		}
		return 0
	}
	fs := flag.NewFlagSet(strings.Join(path, " "), flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := ""
	jsonMode := globalJSON
	strict := false
	fs.StringVar(&file, "file", "", "hosts file")
	fs.BoolVar(&jsonMode, "json", jsonMode, "JSON output")
	if subcommand == "validate" {
		fs.BoolVar(&strict, "strict", false, "duplicates are errors")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return renderUsageError(stdout, stderr, jsonMode, parseFlagError(err, path, []string{"--file", "--json", "--strict"}, strings.Join(path, " ")+" --file <path>"))
	}
	if file == "" || fs.NArg() != 0 {
		return renderUsageError(stdout, stderr, jsonMode, newUsageError(
			"missing_argument", "--file is required and extra arguments are not allowed", path, "", nil, strings.Join(path, " ")+" --file <path>",
		))
	}
	entries, err := parseHostEntries(file)
	if err != nil {
		return renderUsageError(stdout, stderr, jsonMode, newUsageError(
			"hosts_file_not_found", err.Error(), path, file, nil, strings.Join(path, " ")+" --file <path>",
		))
	}
	errorsCount, warnings := 0, 0
	for _, entry := range entries {
		if entry.Error != "" {
			errorsCount++
		}
		if entry.Duplicate {
			if strict {
				errorsCount++
			} else {
				warnings++
			}
		}
	}
	if len(entries) == 0 {
		errorsCount++
	}
	if jsonMode {
		payload := map[string]any{"schema_version": schemaVersion, "command": "hosts " + subcommand, "entries": entries}
		if subcommand == "validate" {
			payload["valid"] = errorsCount == 0
			payload["errors"] = errorsCount
			payload["warnings"] = warnings
		}
		if err := json.NewEncoder(stdout).Encode(payload); err != nil {
			return 1
		}
	} else if subcommand == "list" {
		for _, entry := range entries {
			if _, err := fmt.Fprintf(stdout, "%d\t%s\t%s\t%s\t%d\t%s\tduplicate=%t\tline=%d\n",
				entry.Index, entry.Original, entry.Normalized, entry.Kind, entry.Port, entry.Error, entry.Duplicate, entry.Line); err != nil {
				return 1
			}
		}
	} else {
		if _, err := fmt.Fprintf(stdout, "targets=%d errors=%d warnings=%d valid=%t\n", len(entries), errorsCount, warnings, errorsCount == 0); err != nil {
			return 1
		}
		for _, entry := range entries {
			if entry.Error != "" {
				if _, err := fmt.Fprintf(stderr, "line %d: %s\n", entry.Line, entry.Error); err != nil {
					return 1
				}
			} else if entry.Duplicate {
				if _, err := fmt.Fprintf(stderr, "line %d: duplicate target %s\n", entry.Line, entry.Normalized); err != nil {
					return 1
				}
			}
		}
	}
	if errorsCount != 0 {
		return 1
	}
	return 0
}

type doctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func runDoctor(ctx context.Context, args []string, stdout, stderr io.Writer, globalJSON bool) int {
	if hasHelp(args) {
		if _, err := fmt.Fprint(stdout, doctorHelpText()); err != nil {
			return 1
		}
		return 0
	}
	options := defaultRunOptions()
	jsonMode := globalJSON || requestedJSON(args)
	connect := false
	limit := 10
	fs := flag.NewFlagSet("gopssh doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&options.hostsFile, "hosts-file", "", "hosts file")
	fs.StringVar(&options.hostsFile, "H", "", "hosts file")
	fs.StringVar(&options.config.User, "user", options.config.User, "SSH user")
	fs.Var(&options.identities, "identity", "identity file")
	fs.BoolVar(&options.config.IdentityFileOnly, "identities-only", false, "disable agent")
	fs.BoolVar(&options.config.IgnoreHostKey, "insecure-ignore-host-key", false, "skip known_hosts")
	fs.DurationVar(&options.config.Timeout, "connect-timeout", options.config.Timeout, "connect timeout")
	fs.IntVar(&options.config.Concurrency, "parallel", options.config.Concurrency, "parallel connections")
	fs.IntVar(&options.config.MaxAgentConns, "max-agent-connections", options.config.MaxAgentConns, "agent connections")
	fs.Var((*byteSizeValue)(&options.config.MaxBufferMemory), "max-buffer-memory", "memory limit")
	fs.Var((*byteSizeValue)(&options.config.MaxSpoolSize), "max-spool-size", "spool limit")
	fs.StringVar(&options.config.SpoolDir, "spool-dir", "", "spool parent")
	fs.BoolVar(&options.legacyCrypto, "legacy-crypto", false, "use legacy SSH algorithms")
	fs.StringVar(&options.kex, "kex", "", "key exchange algorithms")
	fs.StringVar(&options.ciphers, "ciphers", "", "ciphers")
	fs.StringVar(&options.macs, "macs", "", "MACs")
	fs.BoolVar(&connect, "connect", false, "perform network diagnostics")
	fs.IntVar(&limit, "limit", limit, "target limit")
	fs.BoolVar(&jsonMode, "json", jsonMode, "JSON output")
	if err := fs.Parse(args); err != nil {
		known := []string{
			"--hosts-file", "-H", "--user", "--identity", "--identities-only",
			"--insecure-ignore-host-key", "--connect-timeout", "--parallel",
			"--max-agent-connections", "--max-buffer-memory", "--max-spool-size",
			"--spool-dir", "--legacy-crypto", "--kex", "--ciphers", "--macs",
			"--connect", "--limit", "--json",
		}
		return renderUsageError(stdout, stderr, jsonMode, parseFlagError(err, []string{"gopssh", "doctor"}, known, "gopssh doctor [options]"))
	}
	if fs.NArg() != 0 || limit <= 0 {
		return renderUsageError(stdout, stderr, jsonMode, newUsageError(
			"invalid_argument", "extra arguments are not allowed and --limit must be greater than zero",
			[]string{"gopssh", "doctor"}, "", nil, "gopssh doctor [options]",
		))
	}
	fs.Visit(func(parsed *flag.Flag) {
		if parsed.Name == "identity" {
			options.identitySet = true
		}
	})
	if len(options.identities) == 0 {
		options.identities = pssh.ToSlice(defaultIdentityFiles)
	}
	configureCrypto(&options.config, options.legacyCrypto, options.kex, options.ciphers, options.macs)
	checks := doctorChecks(options, stdout, stderr)
	if connect {
		targets, err := loadTargets(options.hostsFile, nil)
		if err != nil {
			checks = append(checks, doctorCheck{"network", false, err.Error()})
		} else {
			options.config.IdentFiles = options.identities
			probe := &pssh.Pssh{Config: &options.config}
			for i, target := range targets {
				if i >= limit {
					break
				}
				probeErr := probe.ProbeContext(ctx, target)
				checks = append(checks, doctorCheck{"ssh:" + target, probeErr == nil, errorString(probeErr, "handshake and authentication succeeded")})
			}
		}
	}
	ok := true
	for _, check := range checks {
		ok = ok && check.OK
	}
	payload := map[string]any{
		"schema_version": schemaVersion, "ok": ok,
		"version": version, "commit": commit, "built_at": date,
		"os": runtime.GOOS, "arch": runtime.GOARCH, "checks": checks,
	}
	if jsonMode {
		_ = json.NewEncoder(stdout).Encode(payload)
	} else {
		for _, check := range checks {
			status := "ok"
			if !check.OK {
				status = "error"
			}
			if _, err := fmt.Fprintf(stdout, "%-6s %-24s %s\n", status, check.Name, check.Message); err != nil {
				return 1
			}
		}
	}
	if !ok {
		return 1
	}
	return 0
}

func doctorChecks(options runOptions, stdout, stderr io.Writer) []doctorCheck {
	checks := []doctorCheck{
		{"version", true, fmt.Sprintf("%s (%s, built %s)", version, commit, date)},
		{"platform", true, runtime.GOOS + "/" + runtime.GOARCH},
		{"user", options.config.User != "", valueOr(options.config.User, "not configured")},
		{"parallel", options.config.Concurrency > 0, strconv.Itoa(options.config.Concurrency)},
		{"max_agent_connections", options.config.MaxAgentConns > 0, strconv.Itoa(options.config.MaxAgentConns)},
		{"max_buffer_memory", options.config.MaxBufferMemory > 0, formatByteSize(options.config.MaxBufferMemory)},
		{"max_spool_size", options.config.MaxSpoolSize > 0, formatByteSize(options.config.MaxSpoolSize)},
		{"stdout_tty", true, strconv.FormatBool(isTerminalWriter(stdout))},
		{"stderr_tty", true, strconv.FormatBool(isTerminalWriter(stderr))},
		{"NO_COLOR", true, valueOr(os.Getenv("NO_COLOR"), "unset")},
		{"TERM", true, valueOr(os.Getenv("TERM"), "unset")},
	}
	socket := options.config.SSHAuthSocket
	socketOK := false
	socketMessage := "not configured"
	if socket != "" && !options.config.IdentityFileOnly {
		conn, err := net.DialTimeout("unix", socket, time.Second)
		socketOK = err == nil
		socketMessage = errorString(err, "available")
		if conn != nil {
			_ = conn.Close()
		}
	}
	checks = append(checks, doctorCheck{"ssh_agent", socketOK || options.config.IdentityFileOnly, socketMessage})
	readableIdentity := false
	for _, identity := range options.identities {
		expanded := expandHome(identity)
		file, err := os.Open(expanded)
		ok := err == nil
		if ok {
			readableIdentity = true
			_ = file.Close()
		}
		message := errorString(err, "readable")
		if err != nil && !options.identitySet {
			message = "optional default not found: " + expanded
		}
		checks = append(checks, doctorCheck{"identity:" + identity, ok || !options.identitySet, message})
	}
	knownHosts := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
	knownFile, knownErr := os.Open(knownHosts)
	if knownFile != nil {
		_ = knownFile.Close()
	}
	checks = append(checks, doctorCheck{"known_hosts", options.config.IgnoreHostKey || knownErr == nil, errorString(knownErr, knownHosts)})
	if options.hostsFile != "" {
		targets, err := pssh.ReadHosts(options.hostsFile)
		checks = append(checks, doctorCheck{"hosts_file", err == nil && len(targets) > 0, errorString(err, fmt.Sprintf("%d targets", len(targets)))})
	}
	parent := options.config.SpoolDir
	tempDir, err := os.MkdirTemp(parent, "gopssh-doctor-*")
	if err == nil {
		err = os.Remove(tempDir)
	}
	checks = append(checks, doctorCheck{"spool_directory", err == nil, errorString(err, valueOr(parent, os.TempDir()))})
	checks = append(checks, doctorCheck{"authentication", socketOK || readableIdentity, "at least one usable authentication source"})
	return checks
}

func runConfig(args []string, stdout, stderr io.Writer, globalJSON bool) int {
	globalJSON = globalJSON || requestedJSON(args)
	if len(args) == 0 {
		return renderUsageError(stdout, stderr, globalJSON, newUsageError(
			"missing_argument", "config subcommand is required", []string{"gopssh", "config"}, "", nil, configUsage(),
		))
	}
	if hasHelp(args[:1]) {
		if _, err := fmt.Fprint(stdout, configHelpText()); err != nil {
			return 1
		}
		return 0
	}
	if args[0] != "show" {
		return renderUsageError(stdout, stderr, globalJSON, newUsageError(
			"unknown_subcommand", fmt.Sprintf("unknown subcommand %q for %q", args[0], "gopssh config"),
			[]string{"gopssh", "config"}, args[0], suggest(args[0], []string{"show"}), configUsage(),
		))
	}
	if hasHelp(args[1:]) {
		if _, err := fmt.Fprint(stdout, configShowHelpText()); err != nil {
			return 1
		}
		return 0
	}
	jsonMode := globalJSON
	fs := flag.NewFlagSet("gopssh config show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&jsonMode, "json", jsonMode, "JSON output")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		if err == nil {
			err = errors.New("extra arguments are not allowed")
		}
		return renderUsageError(stdout, stderr, jsonMode, parseFlagError(err, []string{"gopssh", "config", "show"}, []string{"--json"}, "gopssh config show [--json]"))
	}
	options := defaultRunOptions()
	options.identities = pssh.ToSlice(defaultIdentityFiles)
	values := []map[string]any{
		{"name": "user", "value": options.config.User, "source": sourceForEnv("USER")},
		{"name": "parallel", "value": options.config.Concurrency, "source": "default"},
		{"name": "max_agent_connections", "value": options.config.MaxAgentConns, "source": "default"},
		{"name": "identity_files", "value": []string(options.identities), "source": "default"},
		{"name": "identities_only", "value": false, "source": "default"},
		{"name": "host_key_policy", "value": "known-hosts", "source": "default"},
		{"name": "connect_timeout", "value": options.config.Timeout.String(), "source": "default"},
		{"name": "output_order", "value": options.order, "source": "default"},
		{"name": "color", "value": options.color, "source": "default"},
		{"name": "max_buffer_memory", "value": options.config.MaxBufferMemory, "source": "default"},
		{"name": "max_spool_size", "value": options.config.MaxSpoolSize, "source": "default"},
		{"name": "spool_directory", "value": valueOr(options.config.SpoolDir, os.TempDir()), "source": "default"},
		{"name": "ssh_algorithms", "value": "secure defaults", "source": "default"},
		{"name": "ssh_agent_available", "value": options.config.SSHAuthSocket != "", "source": sourceForEnv("SSH_AUTH_SOCK")},
	}
	if jsonMode {
		if err := json.NewEncoder(stdout).Encode(map[string]any{"schema_version": schemaVersion, "settings": values}); err != nil {
			return 1
		}
	} else {
		for _, value := range values {
			encoded, _ := json.Marshal(value["value"])
			if _, err := fmt.Fprintf(stdout, "%-24s %-12s %s\n", value["name"], value["source"], encoded); err != nil {
				return 1
			}
		}
	}
	return 0
}

func runVersion(args []string, stdout, stderr io.Writer, globalJSON bool) int {
	if hasHelp(args) {
		if _, err := fmt.Fprint(stdout, versionHelpText()); err != nil {
			return 1
		}
		return 0
	}
	jsonMode := globalJSON || requestedJSON(args)
	fs := flag.NewFlagSet("gopssh version", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&jsonMode, "json", jsonMode, "JSON output")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		if err == nil {
			err = errors.New("extra arguments are not allowed")
		}
		return renderUsageError(stdout, stderr, jsonMode, parseFlagError(err, []string{"gopssh", "version"}, []string{"--json"}, "gopssh version [--json]"))
	}
	if jsonMode {
		if err := json.NewEncoder(stdout).Encode(map[string]any{
			"schema_version": schemaVersion, "version": version, "commit": commit, "built_at": date,
			"go_version": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		}); err != nil {
			return 1
		}
	} else {
		if _, err := fmt.Fprintf(stdout, "gopssh %s (%s, built %s)\n", version, commit, date); err != nil {
			return 1
		}
	}
	return 0
}

func runCompletion(args []string, stdout, stderr io.Writer, jsonMode bool) int {
	if hasHelp(args) {
		if _, err := fmt.Fprint(stdout, completionHelpText()); err != nil {
			return 1
		}
		return 0
	}
	if jsonMode || len(args) != 1 || !contains([]string{"bash", "zsh", "fish", "powershell"}, args[0]) {
		token := ""
		if len(args) > 0 {
			token = args[0]
		}
		return renderUsageError(stdout, stderr, jsonMode, newUsageError(
			"invalid_argument", "completion shell must be one of bash, zsh, fish, or powershell",
			[]string{"gopssh", "completion"}, token, suggest(token, []string{"bash", "zsh", "fish", "powershell"}), "gopssh completion <bash|zsh|fish|powershell>",
		))
	}
	var err error
	switch args[0] {
	case "bash":
		_, err = fmt.Fprintln(stdout, `complete -W "run doctor hosts config version completion help" gopssh`)
	case "zsh":
		_, err = fmt.Fprintln(stdout, `compctl -k "(run doctor hosts config version completion help)" gopssh`)
	case "fish":
		_, err = fmt.Fprintln(stdout, `complete -c gopssh -f -a "run doctor hosts config version completion help"`)
	case "powershell":
		_, err = fmt.Fprintln(stdout, `Register-ArgumentCompleter -CommandName gopssh -ScriptBlock { param($w) "run","doctor","hosts","config","version","completion","help" | ? { $_ -like "$w*" } }`)
	}
	if err != nil {
		return 1
	}
	return 0
}

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func requestedJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" {
			return true
		}
	}
	return false
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func suggest(value string, candidates []string) []string {
	value = strings.TrimLeft(value, "-")
	var suggestions []string
	for _, candidate := range candidates {
		normalized := strings.TrimLeft(candidate, "-")
		distance := editDistance(value, normalized)
		limit := 2
		if len(normalized) > 8 {
			limit = 3
		}
		if distance <= limit || (len(value) >= 3 && strings.HasPrefix(normalized, value)) {
			suggestions = append(suggestions, candidate)
			if len(suggestions) == 3 {
				break
			}
		}
	}
	return suggestions
}

func editDistance(left, right string) int {
	previous := make([]int, len(right)+1)
	for i := range previous {
		previous[i] = i
	}
	for i, a := range left {
		current := []int{i + 1}
		for j, b := range right {
			cost := 0
			if a != b {
				cost = 1
			}
			current = append(current, min(current[j]+1, previous[j+1]+1, previous[j]+cost))
		}
		previous = current
	}
	return previous[len(right)]
}

func errorString(err error, success string) string {
	if err != nil {
		return err.Error()
	}
	return success
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}

func sourceForEnv(name string) string {
	if os.Getenv(name) != "" {
		return "environment"
	}
	return "default"
}

func isTerminalWriter(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func topUsage() string    { return "gopssh <command> [options]" }
func runUsage() string    { return "gopssh run [options] -- command [arguments...]" }
func hostsUsage() string  { return "gopssh hosts <list|validate> [options]" }
func configUsage() string { return "gopssh config <show> [options]" }

func helpForPath(path []string) string {
	switch strings.Join(path, " ") {
	case "gopssh":
		return topHelp()
	case "gopssh run":
		return runHelpText()
	case "gopssh doctor":
		return doctorHelpText()
	case "gopssh hosts":
		return hostsHelpText()
	case "gopssh hosts list":
		return hostsListHelpText()
	case "gopssh hosts validate":
		return hostsValidateHelpText()
	case "gopssh config":
		return configHelpText()
	case "gopssh config show":
		return configShowHelpText()
	case "gopssh version":
		return versionHelpText()
	case "gopssh completion":
		return completionHelpText()
	default:
		return ""
	}
}

func topHelp() string {
	return `gopssh runs commands on SSH targets in parallel.

Usage:
  gopssh <command> [options]

Commands:
  run          Run a remote command
  doctor       Diagnose local SSH configuration without connecting by default
  hosts        List or validate a hosts file without DNS or network access
  config       Show effective settings and their sources
  version      Show build information
  completion   Generate shell completion
  help         Show help for a command

Global options:
  --json       Emit stable JSON (run emits NDJSON)
  --help       Show this help

Examples:
  gopssh run --hosts-file hosts.txt --user root -- uptime
  gopssh --json doctor --hosts-file hosts.txt
  gopssh hosts validate --file hosts.txt

Legacy syntax remains supported; in legacy mode -h means hosts file:
  gopssh -h hosts.txt -u root -p 10 -d uptime
`
}

func runHelpText() string {
	return `Run one command on every target.

Usage:
  gopssh run [options] -- command [arguments...]
  gopssh run [options] --command '<shell command>'

Required:
  -H, --hosts-file PATH       Read legacy-format targets from PATH
      --host HOST[:PORT]      Add one target; repeatable
  A command and at least one target are required.

Options:
  -u, --user USER             SSH user (default: $USER)
  -p, --parallel N            Concurrent SSH connections (default: 32)
      --max-agent-connections N  Concurrent agent connections (default: 50)
  -i, --identity PATH         Identity file; repeatable
      --identities-only       Disable SSH Agent authentication
      --connect-timeout DURATION (default: 15s)
      --show-host             Print target and exit code to stderr
      --order input|completion (default: input)
      --color auto|always|never (default: auto)
      --insecure-ignore-host-key  Skip known_hosts verification; permits MITM attacks
      --stdin                 Forward process stdin (maximum: 64MiB)
      --stdin-file PATH       Forward a file (maximum: 64MiB)
      --dry-run               Validate and print the plan without connecting
      --json                  Emit one NDJSON result per target and a summary
      --output-dir DIR        Save raw stdout/stderr files with mode 0600
      --exit-policy first|any|always-zero (default: first)
      --max-buffer-memory SIZE (default: 128MiB)
      --max-spool-size SIZE   (default: 10GiB)
      --spool-dir DIR
      --legacy-crypto
      --kex LIST
      --ciphers LIST
      --macs LIST
      --debug
  -h, --help                  Show this help

Examples:
  gopssh run --hosts-file hosts.txt -- uptime
  gopssh run --host host1 --dry-run -- printf '%s\n' 'hello world'
  gopssh run --hosts-file hosts.txt --command 'sudo systemctl status app'
`
}

func doctorHelpText() string {
	return `Diagnose local SSH configuration. No network access occurs unless --connect is set.

Usage:
  gopssh doctor [options]

Options:
  -H, --hosts-file PATH
      --identity PATH         Repeatable
      --identities-only
      --insecure-ignore-host-key
      --connect              Opt in to SSH handshake and authentication checks
      --limit N              Maximum targets checked with --connect (default: 10)
      --json
  -h, --help

Example:
  gopssh --json doctor --hosts-file hosts.txt
`
}

func hostsHelpText() string {
	return `Inspect hosts files locally without DNS or network access.

Usage:
  gopssh hosts <command> [options]

Commands:
  list       List and normalize targets
  validate   Validate targets and report duplicates

Examples:
  gopssh hosts list --file hosts.txt
  gopssh hosts validate --file hosts.txt
`
}

func hostsListHelpText() string {
	return `Usage:
  gopssh hosts list --file PATH [--json]

Example:
  gopssh hosts list --file hosts.txt
`
}

func hostsValidateHelpText() string {
	return `Usage:
  gopssh hosts validate --file PATH [--strict] [--json]

Duplicates are warnings unless --strict is specified.

Example:
  gopssh hosts validate --file hosts.txt --strict
`
}

func configHelpText() string {
	return `Show effective configuration without exposing key material.

Usage:
  gopssh config <command>

Commands:
  show       Show settings and their sources

Example:
  gopssh config show
`
}

func configShowHelpText() string {
	return `Usage:
  gopssh config show [--json]

Example:
  gopssh --json config show
`
}

func versionHelpText() string {
	return `Usage:
  gopssh version [--json]

Example:
  gopssh version
`
}

func completionHelpText() string {
	return `Usage:
  gopssh completion <bash|zsh|fish|powershell>

Example:
  source <(gopssh completion bash)
`
}
