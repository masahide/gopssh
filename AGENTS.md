# Repository Guidelines

## Project Structure & Module Organization
- `cmd/gopssh`: CLI entrypoint (`main.go`, tests).
- `pkg/pssh`: core concurrency/SSH logic and tests; fixtures in `pkg/pssh/test/`.
- `pack/`: packaging helpers (`debpack`, `rpmpack`, Homebrew formula template).
- `.github/workflows/`: CI for build/test/release.
- Top-level files: `Makefile`, `Dockerfile`, `go.mod`, `go.sum`, `README.md`.

## Build, Test, and Development Commands
- `make` or `make build`: build `gopssh` from `cmd/gopssh/main.go`.
- `make test`: race-enabled tests + coverage for `cmd/gopssh` and `pkg/pssh`.
- `make cover`: open combined coverage report.
- `make lint`: run `golangci-lint` (all linters, except `lll`, `wsl`).
- `make fmt`: apply `gofmt` and `goimports` across the repo.
- Release-style local build (example):
  `go build -v -ldflags "-X main.version=0.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date --iso-8601=seconds)" -o .bin/gopssh cmd/gopssh/main.go`
- Packaging (after building `.bin/gopssh`): `go run pack/debpack/main.go` or `go run pack/rpmpack/main.go`.

## Coding Style & Naming Conventions
- Go formatting is mandatory: run `make fmt` (tabs by default; see `.editorconfig`).
- Packages: short, lower-case names; exported identifiers use `CamelCase`.
- Errors: wrap with context via `fmt.Errorf("...: %w", err)`.
- Keep CLI flags and defaults documented in `README.md` when changed.

## Testing Guidelines
- Tests live alongside code as `*_test.go` with `TestXxx` functions.
- Run all tests: `make test`; filter: `make test TEST_PATTERN=SessionWork`.
- Aim to cover concurrency paths and SSH edge cases; avoid external network.
- Add deterministic fixtures under `pkg/pssh/test/` when needed.

## Commit & Pull Request Guidelines
- Commits: imperative mood; prefer conventional prefixes (e.g., `feat:`, `fix:`, `chore:`). Reference issues (`#123`) when applicable.
- PRs: include summary, rationale, test plan/output, and any CLI/flag changes. Update docs and add tests for new behavior.

## Security & Configuration Tips
- Do not commit secrets or private keys. Test keys belong only under `pkg/pssh/test/`.
- Be cautious with `-k` (skip host key check): for testing only.
- SSH agent usage respects `SSH_AUTH_SOCK`; document non-default expectations in PRs.

## Agent-Specific Instructions
- 日本語で応答してください（ユーザー向けメッセージ、PR説明、追補ドキュメント）。
- コード、識別子、コマンド、ログは英語のままで構いません。
- 英語表現を引用する場合は必要に応じて日本語を併記してください。
