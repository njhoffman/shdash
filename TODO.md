# shdash TODO

Open work on shdash.

Reviewed: 2026-11-26

## Completed

### 1. Dashboard (completed 2026-11-26)

- [x] **Interactive mode (`shdash -i`).** (p2) [feature] The same panels as a
  Bubble Tea TUI: resize-aware layout, panel focus, per-panel refresh, scrolling
  long panels (Git's local repos, Audit's tasks).
- [x] **Startup time in the Shell panel.** (p2) [feature] zsh's startup time is
  only known inside a live shell (`zsh/zshrc.zsh` computes `DIFF`) and is not
  written anywhere. Needs ~/.shell to record it (e.g. append to a file under
  `$XDG_CACHE_HOME/shell/` without forking) before shdash can show it.
- [x] **Recent directories in the Projects panel.** (p3) [feature] fasd is a
  lazy shell function in ~/.shell, not a binary, so shdash cannot run it; read
  its data file (`~/.fasd`) or z.lua's directly instead.
- [x] **`go install` builds report version "dev".** (p2) [bug]
  `internal/version` only gets its values from `-ldflags`, which `just
  install` sets but `go install github.com/njhoffman/shdash/cmd/shdash@latest`
  (deploy's `go` method, gup) does not, so `shdash --version` prints
  `dev (unknown, unknown)`. Fall back to `debug.ReadBuildInfo()`: the module
  version and `vcs.revision`/`vcs.time` settings when ldflags left the
  defaults.
- [x] **Install through ~/.shell's deploy.** (p2) [chore] A `packages.tsv` row
  (go or ghq) once the repo has a GitHub remote.
- [x] **Git panel scans every repo every 15 minutes.** (p3) [perf]
  `repo-triage --tsv` takes ~35 s over ~300 repos; it runs detached, but it is
  still CPU and disk work while the machine is in use. Consider a longer TTL
  for the triage half, or only repos changed since the last run.
- [x] **Background refresh log keeps only the last run.** (p3) [chore]
  `refresh.log` is truncated on every spawn, so an error from a run that
  failed is lost by the next one; keep the last N runs or append with a cap.
- [x] **Spawning a refresh truncates a running child's log.** (p2) [bug]
  `spawnRefresh` (cmd/shdash/main.go) opens `refresh.log` with `O_TRUNC` and
  starts a child on every run with due panels, even while an earlier child
  still holds the lock and is writing that log. The new child exits at once
  (lock busy), but the truncate has already wiped the running child's output.
  Fix: try the lock non-blocking before spawning and skip the spawn when held
  (also saves a fork per shell start during a 35 s Git refresh).
- [x] **Panels that only ever failed vanish silently.** (p3) [feature]
  `dash.Panels` drops a panel whose entry has an error but no data, and one
  whose `Lines` fails to decode, with no hint why it is missing. Show the
  panel with a dim one-line error instead.
- [x] **`--refresh` loads the cache twice.** (p3) [refactor] `run` loads
  states, then the `--refresh` branch reloads them after taking the lock and
  discards the first load; load once, after the lock in that branch.
- [x] **Full lists in interactive mode's panel view.** (p3) [feature]
  `Enter` in `shdash -i` shows the focused panel's lines full-screen, but
  collectors keep only what the snapshot shows (5 projects, 4 repos plus "N
  more", 5 dirs, Audit's top tasks), so there is nothing more to scroll to.
  Store the full lists in the cache (schema bumps) and cap them in `Lines`
  for the snapshot only, with a `Detail` renderer for the full view.

### 2. Interactive mode and collectors (completed 2026-11-26)

- [x] **In-process refreshes in `-i` block each other.** (p2) [bug] Each
  `refreshCmd` (internal/tui/tui.go) takes the flock with a new open of the
  lock file, so while one refresh holds it (Repos' ~35 s repo-triage, started
  by auto-refresh at launch) every other `r`/`R` gets "busy", is dropped, and
  says "a background refresh is running", which is wrong. Fix: one refresh
  worker per process that holds the flock and takes a queue of panel names,
  so a second request waits for the first instead of failing.
- [x] **The recent-dirs check can hang on a network mount.** (p3) [bug]
  `recentDirs` (internal/collect/projects.go) checks each z.lua path with
  `os.Stat` outside the collector's context, so a path under the `/mnt/nfs`
  automount (10 s mount timeout) stalls the Projects refresh while the NAS is
  down. Check the paths under the context's deadline (stat in a goroutine and
  stop at the deadline), or skip the check for paths outside `$HOME`.
- [x] **The Shell panel's startup time can be 30 minutes old.** (p3) [feature]
  The Shell TTL is 30 min, but `startup-times` changes with every new shell.
  Read that file when the panel is drawn, not only when it is collected, or
  give it a short TTL of its own.
- [x] **`-i` under 20 columns overflows the screen.** (p3) [bug]
  `render.Arrange` lays out at no less than 20 columns, so on a narrower
  terminal the lines wrap and the screen garbles. Show a "too narrow" line
  instead below 20 columns.
