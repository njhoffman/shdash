# CLAUDE.md

shdash: a Go snapshot dashboard of this system and shell environment
(see README.md).

## Commands

- `just check` — gofmt check, golangci-lint, tests with `-race`, build. Run before committing.
- `just test [-short]` — tests; `-short` skips the one that builds the binary.
- `just install` — `go install` with version ldflags.
- `GOTOOLCHAIN=local` is exported by the justfile: never let `go mod tidy` pull a
  dependency that needs a newer Go than the installed one (x/term and x/sys are
  pinned to versions that build with go 1.25).

## Layout

- `cmd/shdash` — flags, the foreground run, the detached `--background-refresh`.
- `internal/collect` — one file per panel: `Collect` (runs the tools, returns a
  JSON-serializable struct) and `Lines` (renders it). Bump a collector's
  `Schema` when its data's shape changes.
- `internal/cache` — per-panel JSON entries (atomic writes) and the flock.
- `internal/dash` — due/refresh/panel logic between collectors and the cache.
- `internal/render` — column layout, priority drop, truncation.
- `internal/format` — the styles (one accent) and number/age helpers.

## Rules

- Collectors shell out to the tools that own the data (`claude-projects`,
  `repo-triage`, `shell-stats`, audit's export); do not reimplement them.
- Tests use `fakeEnv` (collect) or fake collectors (cmd); no test may depend on
  the host's tools or data.
