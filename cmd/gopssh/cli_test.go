package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/masahide/gopssh/pkg/pssh"
)

func executeForTest(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := executeModern(context.Background(), args, strings.NewReader("unrequested stdin"), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestModernDispatch(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{}, true},
		{[]string{"run"}, true},
		{[]string{"--json"}, true},
		{[]string{"--json", "doctor"}, true},
		{[]string{"--json", "hosst"}, true},
		{[]string{"--json", "completely-unrelated"}, true},
		{[]string{"--help"}, true},
		{[]string{"hosst"}, true},
		{[]string{"-h", "hosts", "run"}, false},
		{[]string{"--debug", "-h", "hosts", "doctor"}, false},
		{[]string{"--version"}, false},
	}
	for _, test := range tests {
		if got := isModern(test.args); got != test.want {
			t.Errorf("isModern(%q)=%t, want %t", test.args, got, test.want)
		}
	}
}

func TestTopHelpDiscoversCommands(t *testing.T) {
	code, stdout, stderr := executeForTest(t, "--help")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	for _, command := range []string{"run", "doctor", "hosts", "config", "version", "completion", "help"} {
		if !strings.Contains(stdout, command) {
			t.Errorf("help missing %q", command)
		}
	}
	for _, legacyHelp := range []string{
		"Legacy syntax remains supported; in legacy mode -h means hosts file:",
		"gopssh -h hosts.txt -u root -p 10 -d uptime",
		"Run 'gopssh help legacy' for full legacy help.",
	} {
		if !strings.Contains(stdout, legacyHelp) {
			t.Errorf("help missing %q", legacyHelp)
		}
	}
	if strings.Contains(stdout, "Help topics:") {
		t.Errorf("top help uses ambiguous help topic list: %q", stdout)
	}
}

func TestNoArgumentsRenderTopLevelUsageError(t *testing.T) {
	code, stdout, stderr := executeForTest(t)
	if code != paramErrCode {
		t.Fatalf("code=%d", code)
	}
	if stdout != "" {
		t.Fatalf("stdout=%q, want empty", stdout)
	}
	if !strings.Contains(stderr, "Error: command is required") ||
		!strings.Contains(stderr, topHelp()) {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestTopLevelJSONCommandErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--json"},
		{"--json", "hosst"},
		{"--json", "completely-unrelated"},
	} {
		code, stdout, stderr := executeForTest(t, args...)
		if code != paramErrCode {
			t.Fatalf("args=%v code=%d", args, code)
		}
		var envelope usageEnvelope
		if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
			t.Fatalf("args=%v invalid JSON %q: %v", args, stdout, err)
		}
		if envelope.Error.HelpCommand != "gopssh --help" ||
			!strings.Contains(stderr, topHelp()) {
			t.Fatalf("args=%v error=%+v stderr=%q", args, envelope.Error, stderr)
		}
		if len(args) == 1 && envelope.Error.Code != "missing_argument" {
			t.Errorf("args=%v error code=%q", args, envelope.Error.Code)
		}
		if len(args) > 1 && envelope.Error.Code != "unknown_command" {
			t.Errorf("args=%v error code=%q", args, envelope.Error.Code)
		}
	}
}

func TestJSONAndHelpReturnMachineReadableError(t *testing.T) {
	tests := []struct {
		args     []string
		helpText string
	}{
		{args: []string{"--json", "--help"}, helpText: topHelp()},
		{args: []string{"--json", "help"}, helpText: topHelp()},
		{args: []string{"help", "--json"}, helpText: topHelp()},
		{args: []string{"--json", "run", "--help"}, helpText: runHelpText()},
		{args: []string{"run", "--json", "--help"}, helpText: runHelpText()},
		{args: []string{"--json", "doctor", "--help"}, helpText: doctorHelpText()},
		{args: []string{"--json", "hosts", "--help"}, helpText: hostsHelpText()},
		{args: []string{"--json", "hosts", "list", "--help"}, helpText: hostsListHelpText()},
		{args: []string{"--json", "config", "--help"}, helpText: configHelpText()},
		{args: []string{"--json", "config", "show", "--help"}, helpText: configShowHelpText()},
		{args: []string{"--json", "version", "--help"}, helpText: versionHelpText()},
		{args: []string{"--json", "completion", "--help"}, helpText: completionHelpText()},
	}
	for _, test := range tests {
		code, stdout, stderr := executeForTest(t, test.args...)
		if code != paramErrCode {
			t.Errorf("args=%v code=%d", test.args, code)
		}
		var envelope usageEnvelope
		if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
			t.Fatalf("args=%v invalid JSON %q: %v", test.args, stdout, err)
		}
		if envelope.Error.Code != "invalid_argument" ||
			!strings.Contains(stderr, test.helpText) {
			t.Errorf("args=%v error=%+v stderr=%q", test.args, envelope.Error, stderr)
		}
	}
}

func TestLegacyHelpIsSeparateTopic(t *testing.T) {
	code, stdout, stderr := executeForTest(t, "help", "legacy")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	for _, text := range []string{
		"gopssh [legacy options] command",
		"-h string",
		"host file",
		"-version",
		"gopssh run --hosts-file",
	} {
		if !strings.Contains(stdout, text) {
			t.Errorf("legacy help missing %q:\n%s", text, stdout)
		}
	}
}

func TestExplicitAndErrorHelpDestinations(t *testing.T) {
	code, stdout, stderr := executeForTest(t, "hosts", "--help")
	if code != 0 || stdout == "" || stderr != "" {
		t.Fatalf("explicit help code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = executeForTest(t, "hosts")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "gopssh hosts <command>") {
		t.Fatalf("error help code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.Count(stderr, "Run 'gopssh hosts --help'") != 1 {
		t.Fatalf("error help rendered more than once: %q", stderr)
	}
}

func TestUnknownCommandSuggestionAndJSONError(t *testing.T) {
	code, stdout, stderr := executeForTest(t, "--json", "hosts", "lsit")
	if code != 2 || !strings.Contains(stderr, `Did you mean "list"`) {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var envelope usageEnvelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout is not one JSON object: %q: %v", stdout, err)
	}
	if envelope.Error.Code != "unknown_subcommand" ||
		envelope.Error.HelpCommand != "gopssh hosts --help" ||
		!reflect.DeepEqual(envelope.Error.Suggestions, []string{"list"}) {
		t.Fatalf("error=%+v", envelope.Error)
	}
}

func TestLowConfidenceUnknownCommandHasNoSuggestion(t *testing.T) {
	code, _, stderr := executeForTest(t, "completely-unrelated")
	if code != 2 {
		t.Fatalf("code=%d", code)
	}
	if strings.Contains(stderr, "Did you mean") {
		t.Fatalf("unexpected suggestion: %q", stderr)
	}
}

func TestRunDryRunPreservesTargetOrderAndArgumentBoundaries(t *testing.T) {
	hostsFile := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hostsFile, []byte("host1\n[::1]:2200\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := executeForTest(t,
		"run", "--json", "--dry-run", "--hosts-file", hostsFile, "--host", "host2",
		"--", "printf", "%s\n", "hello world", "a'b",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var plan struct {
		Targets []string `json:"targets"`
		Command string   `json:"command"`
	}
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatal(err)
	}
	wantTargets := []string{"host1:22", "[::1]:2200", "host2:22"}
	if !reflect.DeepEqual(plan.Targets, wantTargets) {
		t.Errorf("targets=%v, want %v", plan.Targets, wantTargets)
	}
	if want := "'printf' '%s\n' 'hello world' 'a'\"'\"'b'"; plan.Command != want {
		t.Errorf("command=%q, want %q", plan.Command, want)
	}
}

func TestRunCommandSourcesAreExclusive(t *testing.T) {
	code, stdout, stderr := executeForTest(t,
		"run", "--json", "--host", "host1", "--command", "uptime", "--", "date",
	)
	if code != 2 || !strings.Contains(stderr, "mutually exclusive") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(stdout), &value); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestControlFlagNamesCanBeFlagValues(t *testing.T) {
	t.Run("help is command value in JSON mode", func(t *testing.T) {
		code, stdout, stderr := executeForTest(t,
			"run", "--json", "--dry-run", "--host", "host1",
			"--command", "--help",
		)
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		var plan struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
			t.Fatalf("invalid JSON %q: %v", stdout, err)
		}
		if plan.Command != "--help" {
			t.Fatalf("command=%q, want --help", plan.Command)
		}
	})

	t.Run("json is command value in text mode", func(t *testing.T) {
		code, stdout, stderr := executeForTest(t,
			"run", "--dry-run", "--host", "host1",
			"--command", "--json",
		)
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "Command: --json") || json.Valid([]byte(stdout)) {
			t.Fatalf("unexpected output: %q", stdout)
		}
	})

	for _, args := range [][]string{
		{"run", "--dry-run", "--host", "host1", "ls", "--help"},
		{"run", "--dry-run", "--host", "host1", "echo", "--json"},
	} {
		code, stdout, stderr := executeForTest(t, args...)
		if code != paramErrCode || stdout != "" ||
			!strings.Contains(stderr, "command arguments must follow --") {
			t.Errorf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestControlFlagScannerSkipsOptionValues(t *testing.T) {
	for _, args := range [][]string{
		{"--identity", "--help"},
		{"--file", "--json"},
		{"--limit=10", "--help"},
		{"--command=--help", "--json"},
	} {
		scan := scanControlFlags(args, false)
		switch {
		case reflect.DeepEqual(args, []string{"--limit=10", "--help"}):
			if !scan.help {
				t.Errorf("args=%v help=false", args)
			}
		case reflect.DeepEqual(args, []string{"--command=--help", "--json"}):
			if !scan.json {
				t.Errorf("args=%v json=false", args)
			}
		case scan.help || scan.json:
			t.Errorf("args=%v scan=%+v", args, scan)
		}
	}
	if hasArgumentDelimiter([]string{"--identity", "--", "uptime"}) {
		t.Error("flag value was treated as the command delimiter")
	}
	if !hasArgumentDelimiter([]string{"--identity", "key", "--", "uptime"}) {
		t.Error("command delimiter was not detected")
	}
}

type trackingReader struct {
	reads int
}

func (r *trackingReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.EOF
}

func TestModernRunDoesNotReadStdinUnlessRequested(t *testing.T) {
	reader := &trackingReader{}
	var stdout, stderr bytes.Buffer
	code := executeModern(context.Background(),
		[]string{"run", "--dry-run", "--host", "host1", "--", "uptime"},
		reader, &stdout, &stderr,
	)
	if code != 0 || reader.reads != 0 {
		t.Fatalf("code=%d stdin reads=%d stderr=%q", code, reader.reads, stderr.String())
	}
}

func TestRunRequiresTargetsAndCommand(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--", "uptime"},
		{"run", "--host", "host1"},
		{"run", "--host", "host1", "uptime"},
	} {
		code, _, stderr := executeForTest(t, args...)
		if code != 2 || !strings.Contains(stderr, "Usage:") {
			t.Errorf("args=%v code=%d stderr=%q", args, code, stderr)
		}
	}
}

func TestRunJSONConnectionFailureAndExitPolicies(t *testing.T) {
	target := "127.0.0.1:1"
	for _, test := range []struct {
		policy string
		code   int
	}{
		{"first", 255},
		{"any", 1},
		{"always-zero", 0},
	} {
		t.Run(test.policy, func(t *testing.T) {
			code, stdout, _ := executeForTest(t,
				"run", "--json", "--insecure-ignore-host-key", "--exit-policy", test.policy,
				"--host", target, "--", "uptime",
			)
			if code != test.code {
				t.Fatalf("code=%d, want %d; stdout=%q", code, test.code, stdout)
			}
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) != 2 {
				t.Fatalf("NDJSON lines=%d, want 2: %q", len(lines), stdout)
			}
			var result, summary map[string]any
			if err := json.Unmarshal([]byte(lines[0]), &result); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(lines[1]), &summary); err != nil {
				t.Fatal(err)
			}
			if result["type"] != "result" || result["status"] != "connection_failed" ||
				summary["type"] != "summary" || int(summary["aggregate_exit_code"].(float64)) != test.code {
				t.Fatalf("result=%v summary=%v", result, summary)
			}
		})
	}
}

func TestRunJSONPreflightErrorIsSingleObject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	code, stdout, stderr := executeForTest(t, "run", "--json", "--host", "host1", "--", "uptime")
	if code != 1 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var payload struct {
		OK    bool         `json:"ok"`
		Error commandError `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if payload.OK || payload.Error.Code != "known_hosts_unavailable" {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestRunJSONCancellationEmitsResultsAndSignalSummary(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("received signal: terminated"))
	var stdout, stderr bytes.Buffer
	code := executeModern(ctx, []string{
		"run", "--json", "--insecure-ignore-host-key", "--host", "host1", "--", "uptime",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != 143 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines=%d, want result and summary: %q", len(lines), stdout.String())
	}
	var result, summary map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &result); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &summary); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "canceled" || int(summary["aggregate_exit_code"].(float64)) != 143 {
		t.Fatalf("result=%v summary=%v", result, summary)
	}
}

func TestUnknownOptionWithTrailingJSONKeepsStdoutMachineReadable(t *testing.T) {
	code, stdout, _ := executeForTest(t, "run", "--paralel", "2", "--json")
	if code != 2 {
		t.Fatalf("code=%d", code)
	}
	var payload usageEnvelope
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if payload.Error.Code != "unknown_option" {
		t.Fatalf("error=%+v", payload.Error)
	}
}

func TestHostsListAndValidate(t *testing.T) {
	hostsFile := filepath.Join(t.TempDir(), "hosts")
	data := "# comment\nhost1 127.0.0.1\n::1\nhost1\nhost:65536\nbad!\n"
	if err := os.WriteFile(hostsFile, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := executeForTest(t, "--json", "hosts", "list", "--file", hostsFile)
	if code != 1 {
		t.Fatalf("list code=%d stdout=%q", code, stdout)
	}
	var payload struct {
		Entries []hostEntry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Entries) != 6 || !payload.Entries[3].Duplicate ||
		payload.Entries[4].Error == "" || payload.Entries[5].Error == "" {
		t.Fatalf("entries=%+v", payload.Entries)
	}

	validFile := filepath.Join(t.TempDir(), "valid-hosts")
	if err := os.WriteFile(validFile, []byte("host1\nhost1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, _ = executeForTest(t, "hosts", "validate", "--file", validFile)
	if code != 0 {
		t.Errorf("duplicate warning code=%d, want 0", code)
	}
	code, _, _ = executeForTest(t, "hosts", "validate", "--strict", "--file", validFile)
	if code != 1 {
		t.Errorf("strict duplicate code=%d, want 1", code)
	}
}

func TestHostsListDoesNotSuggestValidateOnlyOption(t *testing.T) {
	code, _, stderr := executeForTest(t, "hosts", "list", "--strct")
	if code != paramErrCode {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, "--strict") {
		t.Fatalf("list suggested validate-only option: %q", stderr)
	}
}

func TestDoctorJSONIsProducedOnFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "")
	code, stdout, _ := executeForTest(t, "--json", "doctor", "--identities-only")
	if code != 1 {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
	var payload struct {
		SchemaVersion string        `json:"schema_version"`
		OK            bool          `json:"ok"`
		Checks        []doctorCheck `json:"checks"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != schemaVersion || payload.OK || len(payload.Checks) == 0 {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDoctorConnectRequiresTargets(t *testing.T) {
	code, stdout, stderr := executeForTest(t, "--json", "doctor", "--connect")
	if code != paramErrCode {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var envelope usageEnvelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if envelope.Error.Code != "missing_argument" ||
		envelope.Error.HelpCommand != "gopssh doctor --help" ||
		!strings.Contains(stderr, doctorHelpText()) {
		t.Fatalf("error=%+v stderr=%q", envelope.Error, stderr)
	}
}

func TestConfigShowDoesNotExposeIdentityContents(t *testing.T) {
	secret := "PRIVATE-KEY-CONTENT-MUST-NOT-APPEAR"
	identity := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(identity, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := executeForTest(t, "--json", "config", "show")
	if code != 0 || stderr != "" || strings.Contains(stdout, secret) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

type bytesResultOutput []byte

func (output bytesResultOutput) WriteTo(writer io.Writer) (int64, error) {
	n, err := writer.Write(output)
	return int64(n), err
}

func (output bytesResultOutput) Size() int64 { return int64(len(output)) }

func TestJSONResultUTF8AndBase64(t *testing.T) {
	tests := []struct {
		name   string
		stdout []byte
		field  string
	}{
		{"utf8", []byte("hello\n世界"), "stdout"},
		{"base64", []byte{0xff, 0x00}, "stdout_base64"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			result := &pssh.Result{
				Index: 0, Target: "host:22", Stdout: bytesResultOutput(test.stdout),
				Stderr: bytesResultOutput{}, ExitCode: 0,
			}
			if err := writeJSONResult(&output, result, ""); err != nil {
				t.Fatal(err)
			}
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatalf("invalid JSON %q: %v", output.String(), err)
			}
			if _, ok := record[test.field]; !ok {
				t.Fatalf("record=%v, missing %s", record, test.field)
			}
		})
	}
}

func TestJSONResultEscapesAllControlCharacters(t *testing.T) {
	controlOutput := []byte{
		'a', 0x00, 0x07, 0x08, 0x09, 0x0a, 0x0c, 0x0d,
		0x1b, 0x1f, 0x7f, '"', '\\', 'z',
	}
	var output bytes.Buffer
	result := &pssh.Result{
		Index: 0, Target: "host:22", Kind: pssh.ResultSuccess,
		Stdout: bytesResultOutput(controlOutput), Stderr: bytesResultOutput{},
	}
	if err := writeJSONResult(&output, result, ""); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(output.Bytes()) {
		t.Fatalf("invalid JSON: %q", output.String())
	}
	var record struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(record.Stdout), controlOutput) {
		t.Fatalf("stdout=%v, want %v", []byte(record.Stdout), controlOutput)
	}
}

func TestRemoteExit255IsNotConnectionFailure(t *testing.T) {
	result := &pssh.Result{
		Index: 0, Target: "host:22", Kind: pssh.ResultRemoteExit, ExitCode: 255,
		Stdout: bytesResultOutput{}, Stderr: bytesResultOutput{},
	}
	var output bytes.Buffer
	if err := writeJSONResult(&output, result, ""); err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["status"] != "failed" {
		t.Fatalf("status=%v, want failed", record["status"])
	}
	stats := &runStats{}
	updateRunStats(stats, result)
	if stats.connectionFailed != 0 || stats.failed != 1 {
		t.Fatalf("stats=%+v", stats)
	}

	result.Kind = pssh.ResultConnectionFailed
	output.Reset()
	if err := writeJSONResult(&output, result, ""); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["status"] != "connection_failed" {
		t.Fatalf("status=%v, want connection_failed", record["status"])
	}
}

func TestOutputDirectoryFailureKeepsNDJSONValid(t *testing.T) {
	directory := t.TempDir()
	result := &pssh.Result{
		Index: 0, Target: "host:22", Kind: pssh.ResultSuccess,
		Stdout: bytesResultOutput("stdout"), Stderr: bytesResultOutput("stderr"),
	}
	stdoutPath := filepath.Join(directory, "0-host_22.stdout")
	if err := os.Symlink(filepath.Join(directory, "elsewhere"), stdoutPath); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	stats := &runStats{total: 1}
	options := runOptions{json: true, outputDir: directory}
	if err := handleRunResult(options, stats, &stdout, &stderr, result); err == nil {
		t.Fatal("handleRunResult() error=nil, want output failure")
	}
	if err := writeJSONSummary(&stdout, stats, 1); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines=%d, want 2: %q", len(lines), stdout.String())
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("invalid NDJSON line: %q", line)
		}
	}
	var failure, summary map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &failure); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &summary); err != nil {
		t.Fatal(err)
	}
	if failure["status"] != "output_failed" || failure["error_code"] != "output_io_failed" {
		t.Fatalf("failure=%v", failure)
	}
	if summary["local_errors"] != float64(1) || stats.failed != 1 {
		t.Fatalf("summary=%v stats=%+v", summary, stats)
	}
	if !strings.Contains(stderr.String(), "output_io_failed") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestDoctorAuthenticationAlternatives(t *testing.T) {
	identity := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(identity, []byte("readable"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := defaultRunOptions()
	base.config.User = "user"
	base.config.IgnoreHostKey = true
	base.config.SSHAuthSocket = ""
	base.identities = nil

	tests := []struct {
		name   string
		mutate func(*runOptions)
		wantOK bool
	}{
		{
			name: "identity only available",
			mutate: func(options *runOptions) {
				options.identities = []string{identity}
				options.identitySet = true
			},
			wantOK: true,
		},
		{
			name: "agent only available",
			mutate: func(options *runOptions) {
				options.config.SSHAuthSocket = "/agent"
				options.agentProbe = func(string) error { return nil }
			},
			wantOK: true,
		},
		{
			name:   "no authentication",
			mutate: func(*runOptions) {},
			wantOK: false,
		},
		{
			name: "identities-only with identity",
			mutate: func(options *runOptions) {
				options.config.IdentityFileOnly = true
				options.identities = []string{identity}
				options.identitySet = true
			},
			wantOK: true,
		},
		{
			name: "identities-only without identity",
			mutate: func(options *runOptions) {
				options.config.IdentityFileOnly = true
			},
			wantOK: false,
		},
		{
			name: "explicit unreadable identity",
			mutate: func(options *runOptions) {
				options.identities = []string{filepath.Join(t.TempDir(), "missing")}
				options.identitySet = true
			},
			wantOK: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := base
			test.mutate(&options)
			checks := doctorChecks(options, &bytes.Buffer{}, &bytes.Buffer{})
			if got := doctorChecksOK(checks); got != test.wantOK {
				t.Fatalf("doctorChecksOK()=%t, want %t; checks=%+v", got, test.wantOK, checks)
			}
		})
	}
}

func TestOutputDirectoryPermissionsAndBytes(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "results")
	result := &pssh.Result{
		Index: 2, Target: "[::1]:22", ExitCode: 0,
		Stdout: bytesResultOutput("stdout"), Stderr: bytesResultOutput("stderr"),
	}
	var output bytes.Buffer
	if err := writeJSONResult(&output, result, directory); err != nil {
		t.Fatal(err)
	}
	var record struct {
		StdoutPath  string `json:"stdout_path"`
		StderrPath  string `json:"stderr_path"`
		StdoutBytes int64  `json:"stdout_bytes"`
		StderrBytes int64  `json:"stderr_bytes"`
	}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.StdoutBytes != 6 || record.StderrBytes != 6 {
		t.Fatalf("record=%+v", record)
	}
	for _, path := range []string{record.StdoutPath, record.StderrPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode=%o", path, info.Mode().Perm())
		}
	}
	if err := os.Chmod(record.StdoutPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writeOutputFiles(directory, result); err != nil {
		t.Fatal(err)
	}
	stdoutInfo, err := os.Stat(record.StdoutPath)
	if err != nil {
		t.Fatal(err)
	}
	if stdoutInfo.Mode().Perm() != 0o600 {
		t.Errorf("overwritten stdout mode=%o", stdoutInfo.Mode().Perm())
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("directory mode=%o", info.Mode().Perm())
	}
}
