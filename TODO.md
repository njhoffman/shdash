# shdash TODO

Open work on shdash. Reviewed: 2026-11-25

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
- [ ] **Background refresh log keeps only the last run.** (p3) [chore]
  `refresh.log` is truncated on every spawn, so an error from a run that
  failed is lost by the next one; keep the last N runs or append with a cap.
