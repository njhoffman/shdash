# shdash TODO

Open work on shdash.

Reviewed: 2026-11-26

## 6. Cache

- [x] **Two shdash builds sharing the cache delete each other's entries.**
  (p3) [bug] `cache.Clean` (internal/cache/cache.go) removes every
  `<name>.json` the running binary has no collector for. `bin/shdash` (dev)
  and `~/go/bin/shdash` (installed) use the same cache directory, so after a
  panel is added or renamed, whichever is older deletes the newer one's
  entry on each background refresh and the newer one collects it again.
  Only remove entries untouched for longer than the longest TTL (e.g. 7
  days), or record the collector set's version and skip removal when an
  entry is newer than the binary.
- [ ] **`-i` decodes every cache entry every 2 seconds.** (p3) [perf] The
  tick calls `reload` (internal/tui/tui.go), which runs `dash.Load` and
  `rebuild` each time, so "renders on change" (0.12.1) only covers key
  presses: idle, it still reads and renders all panels and has Shell read
  the startup-times file every 2 s. Stat the entries and rebuild only when
  an mtime changed, plus once a minute for the ages.
- [x] **`cache.Save` renames without syncing.** (p3) [bug] The temp file is
  closed and renamed but never `Sync`ed, so a crash or power loss soon after
  can leave an empty `<name>.json`; `Load` then reports a decode error and
  the panel loses its data until the next refresh. Call `f.Sync()` before
  `Close` (a few ms per panel, in the detached refresh only).

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

### 3. Packages (completed 2026-11-26)

- [x] **Read audit's compact per-manager counts once it exports them.** (p3)
  [perf] The Packages panel parses audit's `packages.json` (1.9 MB, every
  package's registry metadata) for a few counts. audit's TODO has a
  `packages` block for `summary.json` ("Per-manager package counts in the
  export's summary.json"); when it lands, read that and keep `packages.json`
  as the fallback for older exports.

### 4. Robustness (completed 2026-11-26)

- [x] **A panicking collector takes the whole refresh down.** (p2) [bug]
  `dash.RefreshAll` runs each collector in a goroutine with no `recover`, so
  a parser panic on unexpected tool output kills the background refresh
  (the panels not yet saved keep their old data) and, in `-i`, the program
  with the terminal still in the alt screen. Recover in `dash.Refresh` and
  record `panic: ...` as that panel's error, like any other failure.
- [x] **The Git panel caches a cut-short list as a success.** (p3) [bug]
  `lastCommit` drops every error, including the context's deadline, so when
  the 1-minute timeout hits, the commits read so far are saved with no error
  and the panel looks complete. Return them with `ctx.Err()` so the panel
  says the refresh failed and it is retried after the TTL.
- [x] **`--refresh` waits silently behind a running refresh.** (p3) [feature]
  It blocks on the lock with no output, and Repos can hold it for up to its
  3-minute timeout. Try the lock without waiting first and, when it is
  held, print "waiting for the running refresh" to stderr before blocking.
- [x] **Nothing cleans the cache directory.** (p3) [chore] A `Save` killed
  between create and rename leaves a `.<panel>-*.tmp` behind for good, and a
  renamed or removed panel's `<name>.json` stays forever. Have the
  background refresh remove temp files older than a day and entries no
  collector owns.
- [x] **No CI.** (p3) [chore] The repo is public but `just check` only runs
  locally. Add a GitHub Actions workflow that runs it with Go 1.25 and
  golangci-lint, so a push that breaks the build or the race tests shows up
  on GitHub.

### 5. Display (completed 2026-11-26)

- [x] **Packages rows are cut at 80 columns.** (p2) [bug] Two columns of 38
  leave no room for the full row: `pnpm 98 · 15 updates (3 major) ·…`,
  `uv 85 · 14 updates · 2 uncheck…`, `checked 20d ago (audit packages
  outda…`. Needs a decision on the shorter form: for example shorter words
  ("15 behind", "11 unchecked" dropped to the detail view), and the command
  hint only when the data is stale.
- [x] **"checked 20d ago" reads as the last scan.** (p2) [bug] The footer
  is the oldest registry lookup, but a plain `audit packages` rescan
  refreshes only apt, snap and flatpak, so right after one the panel still
  says 20 days (seen 2026-11-26) and looks wrong. Say what it is:
  `registry checked 20d ago`, with `audit packages outdated` as the hint,
  and the scan's own age next to it when they differ.
- [x] **Audit and Packages lag a new export by up to 10 minutes.** (p3)
  [feature] They are only collected again when their TTL runs out, so a
  rescan just done is not on screen. Both only read files: collect them
  again whenever `summary.json` is newer than the cached entry (one stat
  per run), and keep the TTL as the fallback.
- [x] **Shell's startup row is cut at 80 columns.** (p3) [bug]
  `startup 0.86s · median 0.38s (32) · 2…` loses the startup file and line
  counts. Put them on the `repo` row, or a row of their own, so the times
  are what the row holds.
- [x] **The tmux row lists every session and is cut.** (p3) [feature] With
  nine sessions, `tmux main-audit* main-harmonic main-rofi mai…` shows three.
  Show the count and the attached ones in the snapshot and every name in the
  detail view.
- [x] **The rest of the Shell panel is cut at 80 columns.** (p3) [bug]
  `live 618 aliases · 8036 functions · 1413 widgets` and `repo 547 aliases ·
  399 functions · 203 scripts` are about 50 columns in a 38-column panel.
  Needs a decision on the layout: one row per kind with live and repo side
  by side (`aliases 618 live · 547 repo`, `funcs 8036 live · 399 repo`), or
  shorter words.
- [x] **Packages has no detail view.** (p3) [feature] `summary.json` now
  carries each manager's gaps (current, patch, minor, major, update, ahead,
  unknown) and `carried_from`; `Enter` in `-i` could show them, with the
  unknown counts the snapshot leaves out. Needs the gaps stored (schema
  bump) and a `Detail` renderer.
- [x] **System's `pkgs` row repeats the Packages panel.** (p3) [cleanup]
  fastfetch's package total is the sum the Packages panel already breaks
  down by manager. Drop the row and the `Packages` fastfetch module, or keep
  it as a cross-check when audit's export is missing: a decision.
- [x] **`-i` says "shown on the next run".** (p3) [bug] A panel with no
  cache yet reads "collecting, shown on the next run", but interactive mode
  shows it as soon as its refresh ends. Say "collecting" there.
- [x] **`-i` renders every panel again for each key.** (p3) [perf] `View`,
  `keepFocusVisible` and the focus moves each build the arrangement, which
  decodes every cache entry and has the Shell panel read the startup-times
  file, two or three times per key press. Build the panels once per reload,
  refresh or resize and keep them in the model.
