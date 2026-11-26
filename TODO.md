# shdash TODO

Open work on shdash.

Reviewed: 2026-11-25

## 1. Dashboard

- [ ] **Interactive mode (`shdash -i`).** (p2) [feature] The same panels as a
  Bubble Tea TUI: resize-aware layout, panel focus, per-panel refresh, scrolling
  long panels (Git's local repos, Audit's tasks).
- [ ] **Startup time in the Shell panel.** (p2) [feature] zsh's startup time is
  only known inside a live shell (`zsh/zshrc.zsh` computes `DIFF`) and is not
  written anywhere. Needs ~/.shell to record it (e.g. append to a file under
  `$XDG_CACHE_HOME/shell/` without forking) before shdash can show it.
- [ ] **Recent directories in the Projects panel.** (p3) [feature] fasd is a
  lazy shell function in ~/.shell, not a binary, so shdash cannot run it; read
  its data file (`~/.fasd`) or z.lua's directly instead.
- [ ] **Install through ~/.shell's deploy.** (p2) [chore] A `packages.tsv` row
  (go or ghq) once the repo has a GitHub remote.
- [ ] **Git panel scans every repo every 15 minutes.** (p3) [perf]
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
