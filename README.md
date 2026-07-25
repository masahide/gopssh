# gopssh

[![Go Report Card](https://goreportcard.com/badge/github.com/masahide/gopssh)](https://goreportcard.com/report/github.com/masahide/gopssh)
[![Build status](https://github.com/masahide/gopssh/actions/workflows/buildpkg.yml/badge.svg)](https://github.com/masahide/gopssh/actions/workflows/buildpkg.yml)

`gopssh` is a CLI for running the same command on multiple SSH targets in
parallel. The canonical interface uses subcommands. The legacy flag-based
syntax remains available for compatibility.

## Quick start

```bash
printf '%s\n' host1 host2 > hosts.txt
gopssh hosts validate --file hosts.txt
gopssh run --dry-run --hosts-file hosts.txt -- uptime
gopssh run --hosts-file hosts.txt -- uptime
```

By default, `gopssh` verifies `~/.ssh/known_hosts` and uses the SSH Agent
first when available, followed by identity files.

## `run`

```text
gopssh run [options] -- command [arguments...]
gopssh run [options] --command '<shell command>'
```

```bash
gopssh run \
  --hosts-file hosts.txt \
  --user root \
  --parallel 10 \
  --show-host \
  -- uptime

gopssh run \
  --hosts-file hosts.txt \
  --host extra.example.com:2222 \
  --identity ~/.ssh/id_ed25519 \
  --order completion \
  -- printf '%s\n' 'hello world'
```

After `--`, each argument is safely single-quoted for a POSIX shell before
being passed to the remote shell, preserving argument boundaries. Use
`--command` when you need to pass a complex shell expression as-is. These two
forms are mutually exclusive. When `--command` is not used, the `--` delimiter
before the command is required.

You can combine `-H, --hosts-file` with the repeatable `--host` option. Targets
are processed in hosts-file order, followed by `--host` argument order.
Duplicates are preserved.

The new syntax does not read stdin by default. Specify `--stdin` to forward
the process stdin or `--stdin-file PATH` to forward a file. The same stdin
content is sent to every host, with a maximum size of 64 MiB.

### Dry-run

```bash
gopssh run --dry-run --hosts-file hosts.txt -- 'sudo systemctl restart app'
gopssh run --json --dry-run --host host1 -- uptime
```

`--dry-run` validates and displays the targets, command, authentication
candidates, known_hosts policy, concurrency, and output settings. It does not
open TCP connections, request SSH Agent signatures, create SSH sessions, or
run remote commands.

## `doctor`

```bash
gopssh doctor --hosts-file hosts.txt
gopssh --json doctor --hosts-file hosts.txt
```

This command locally diagnoses the version, OS, user, SSH Agent socket,
identity files, known_hosts, hosts file, concurrency, memory and spool limits,
TTY and color environment, and authentication candidates. It does not connect
to the network by default. Only when `--connect --limit N` is explicitly set
does it attempt TCP connections, SSH handshakes, host-key verification, and
authentication against the first N targets. It does not create remote sessions
or run commands. When using `--connect`, `--hosts-file` must contain at least
one target.

The exit code is 0 on success, 1 when a required check fails, and 2 for invalid
arguments. The SSH Agent and identity files are alternative authentication
paths, so the authentication check succeeds when either one is available. The
`required` field on each JSON check indicates whether that check can cause the
overall doctor command to fail.

## `hosts`

```bash
gopssh hosts list --file hosts.txt
gopssh --json hosts list --file hosts.txt
gopssh hosts validate --file hosts.txt
gopssh hosts validate --strict --file hosts.txt
```

`list` displays input order, original and normalized values, host, port,
IPv4/IPv6/DNS type, duplicate status, and line number. `validate` checks
comments and blank lines, host names and IP formats, ports from 1 to 65535,
empty files, and duplicates. Duplicates are warnings by default and errors
with `--strict`. Neither command performs DNS resolution or network access.

## `config` and `version`

```bash
gopssh config show
gopssh --json config show
gopssh version
gopssh --json version
```

`config show` displays effective values and their sources, such as `default`
or `environment`. It does not expose private-key contents or SSH Agent key
material. The legacy `gopssh -version` keeps its compatible three-line format,
while `gopssh version` uses the new one-line format.

## JSON / NDJSON

The schema major version is `1`. With `--json`, stdout contains only JSON;
diagnostics and contextual help are written separately to stderr. Combining
`--json` with explicit help returns a JSON `invalid_argument` error instead of
plain-text help on stdout. Color is always disabled.

`run --json` emits NDJSON with one `result` line per host and one final
`summary` line.

```json
{"schema_version":"1","type":"result","index":0,"target":"host1:22","status":"success","exit_code":0,"error":null,"duration_ms":1234,"stdout":"ok\n","stdout_encoding":"utf-8","stderr":"","stderr_encoding":"utf-8"}
{"schema_version":"1","type":"summary","total":1,"succeeded":1,"failed":0,"connection_failed":0,"canceled":0,"local_errors":0,"aggregate_exit_code":0}
```

- Valid UTF-8 is represented in `stdout` / `stderr` with
  `*_encoding: "utf-8"`.
- Output containing invalid UTF-8 is preserved without loss in
  `stdout_base64` / `stderr_base64` with `*_encoding: "base64"`.
- Empty output is an empty string. `error` is `null` when there is no error.
- `connection_failed` is used only when the execution engine classifies a
  failure as occurring during connection setup. A remote command that exits
  with 255 is treated as a normal `failed` result.
- `--order input` preserves input order; `--order completion` uses completion
  order.
- Adding fields is backward-compatible. Removing fields or changing their
  meaning requires a new schema major version.

With `--output-dir DIR`, the original stdout and stderr byte streams are saved
as `<index>-<sanitized-target>.stdout` and `.stderr`. Directories use mode 0700
and files use mode 0600. JSON results contain absolute paths and byte counts
instead of inline output. File writes complete before the result is written to
stdout. If a write fails, `gopssh` still emits a complete result with
`status: "output_failed"`, reports `output_io_failed` to stderr, and increments
`local_errors` in the summary.

A JSON error that occurs before execution begins is a single JSON object.

```json
{"schema_version":"1","ok":false,"error":{"code":"missing_argument","message":"remote command is required","command_path":["gopssh","run"],"suggestions":[],"usage":"gopssh run [options] -- command [arguments...]","help_command":"gopssh run --help"}}
```

## stdout, stderr, color, and exit codes

With the new syntax, remote stdout is written to stdout, while remote stderr,
connection errors, and diagnostics are written to stderr. `--color auto`
detects TTYs independently for each output stream and respects `NO_COLOR` and
`TERM=dumb`. `never` disables color, while `always` explicitly enables it even
for non-TTY output.

The `run` command supports these `--exit-policy` values:

- `first`: the first non-zero exit code in result-output order (connection
  failures use 255)
- `any`: 1 when at least one target fails
- `always-zero`: 0 for remote failures, but non-zero for syntax, local I/O, and
  internal errors

Common exit codes are 0 for success, 1 for local or aggregate errors, 2 for
syntax or configuration errors, 130 for SIGINT, and 143 for SIGTERM. The
`aggregate_exit_code` in the NDJSON summary matches the process exit code.

## Authentication and known_hosts

Authentication normally tries the SSH Agent first, then explicitly configured
identity files, then the default identity files when none were specified.
`--identities-only` disables the SSH Agent. known_hosts verification is enabled
by default. `--insecure-ignore-host-key` is dangerous because it permits
man-in-the-middle attacks; use it only when you can verify the target host key
through another trusted channel.

## Large output and spooling

Remote output shares a default process-wide memory budget of 128 MiB. Data
beyond that budget spills to mode-0600 files inside a mode-0700 temporary
directory. The default total spool limit is 10 GiB. JSON output is streamed
from the spool without loading the entire output back into memory. If the limit
is exceeded, a disk error occurs, or the operation is canceled, `gopssh` stops
the SSH session and removes temporary files.

```text
--max-buffer-memory 128MiB
--max-spool-size 10GiB
--spool-dir /path/to/private-parent
```

## Legacy syntax compatibility

Invocations beginning with a top-level flag are passed to the legacy parser.
In the legacy syntax, `-h` specifies the hosts file rather than help. Flag
parsing stops at the first non-flag argument, and the remaining arguments are
joined with spaces. Non-TTY stdin is also forwarded automatically.

Running `gopssh` without arguments reports `Error: command is required` and
the new top-level help to stderr, then returns the legacy-compatible exit code
2. Run `gopssh help legacy` to see all legacy options.

```bash
# Legacy syntax
gopssh -h hosts.txt -u root -p 10 -d uptime
# Canonical syntax
gopssh run --hosts-file hosts.txt --user root --parallel 10 --show-host -- uptime

# Legacy syntax
gopssh -h hosts.txt -c=false -s=false command
# Canonical syntax
gopssh run --hosts-file hosts.txt --color never --order completion -- command

# Legacy syntax
gopssh -h hosts.txt -i ~/.ssh/id_a,~/.ssh/id_b command
# Canonical syntax
gopssh run --hosts-file hosts.txt \
  --identity ~/.ssh/id_a \
  --identity ~/.ssh/id_b \
  -- command
```

Legacy flags, defaults, output format, stdin behavior, result aggregation, and
exit codes are preserved. No deprecation warning is emitted.

## Installation

macOS / Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/install.sh | sh
```

The default installation directory is `~/.local/bin/gopssh`. To select a
different version or destination:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/install.sh |
  GOPSSH_VERSION=v0.5.6 GOPSSH_INSTALL_DIR="$HOME/bin" sh
```

To uninstall:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/uninstall.sh | sh
```

## Development and build

```bash
make build
make test
make lint
```

Build with release metadata:

```bash
go build -ldflags \
  "-X main.version=0.6.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date --iso-8601=seconds)" \
  -o .bin/gopssh ./cmd/gopssh
```

RPM and DEB packaging can obtain metadata from the compatible three-line
output of `.bin/gopssh -version`.
