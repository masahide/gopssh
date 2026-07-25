# Repository Guidelines

## Project Structure & Module Organization
- `cmd/gopssh/main.go` provides the CLI entrypoint, with supporting helpers under `cmd/gopssh/gopssh/`.
- Core parallel SSH logic, worker pools, and agent wrappers live in `pkg/pssh/`, which also houses unit tests and fixtures in `pkg/pssh/test/`.
- RPM and DEB packaging utilities are grouped under `pack/`; the curl-based installer is `install.sh`, and release metadata lives in `releasenote.template.md`.
- The top-level `Makefile` orchestrates builds, tests, linting, and coverage aggregation; keep changes aligned with its targets.

## Build, Test, and Development Commands
- `make build` compiles `cmd/gopssh` into a local `gopssh` binary; it is the default target and cleans stale artifacts.
- `make test` runs race-enabled tests for all packages and writes coverage to `coverage.txt`.
- `make lint` invokes the pinned `golangci-lint` binary from `./bin`; run `make setup` once to install it (requires network access).
- For release builds, mirror the README example: `go build -ldflags "-X main.version=$(TAG) ..."` so version metadata stays accurate.

## Coding Style & Naming Conventions
- Follow Go defaults: tabs for indentation, `gofmt` + `goimports` formatting, exported symbols in PascalCase, internals in camelCase.
- Keep files self-contained and small; place shared concurrency helpers in `pkg/pssh` rather than duplicating logic in `cmd/`.
- Run `make fmt` (or its underlying `gofmt`/`goimports` loop) before committing to avoid lint noise.

## Testing Guidelines
- Name tests `*_test.go` with functions `TestXxx`; table-driven cases fit well for SSH session permutations.
- Use `go test ./cmd/gopssh ./pkg/pssh -race` for quick local runs; rely on `make test` when you need coverage artifacts.
- Place reusable fixtures under `pkg/pssh/test` and keep them deterministic to prevent race test flakiness.

## Commit & Pull Request Guidelines
- Recent history mixes concise imperative commits (`fix worker hang`) with Conventional Commits (`fix: update dependency`); prefer the latter style for clarity.
- Ensure commits stay focused and include context for packaging or release changes (e.g., note RPM spec updates).
- Before opening a PR, run `make build test lint`, attach the resulting summary, link related issues, and describe any manual SSH verification performed.

## Release & Packaging Notes
- Build artifacts land in `.bin/`; confirm the binary version with `./gopssh -version` before running `pack/debpack` or `pack/rpmpack`.
- Keep `install.sh` aligned with release archive names and the generated `checksums.txt` file.
- Update `releasenote.template.md` alongside packaging changes so CI-driven releases stay coherent.
