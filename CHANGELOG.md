# Changelog

All notable changes to shdash are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
uses [semantic versioning](https://semver.org/).

## [Unreleased]

## [0.9.0] - 2026-11-26

### Added

- A Packages panel, after Repos: per package manager audit tracks (apt, snap,
  flatpak, uv, pnpm, cargo, go, gem, gh), how many packages are installed, how
  many can be updated (with the majors in bold) and how many audit could not
  check, read from audit's `packages.json` export. A last line says how old
  the export is, bold past 7 days.

### Changed

- Audit and Shell now come after Packages in the priority order.

## [0.8.4] - 2026-11-26

### Fixed

- `shdash -i` on a terminal narrower than 20 columns shows "too narrow"
  instead of lines that wrap and garble the screen.

## [0.8.3] - 2026-11-26

### Fixed

- The Shell panel's startup time is read when the panel is drawn, so it
  includes the newest shell instead of lagging up to the panel's 30-minute
  TTL.

## [0.8.2] - 2026-11-26

### Fixed

- The Projects panel's check that recent directories still exist takes at
  most a second, within the panel's timeout: a directory on a network mount
  whose server is down (an automount waits for its mount timeout) no longer
  stalls the refresh. Directories not checked in time are listed.

## [0.8.1] - 2026-11-26

### Fixed

- In `shdash -i`, a refresh asked for while another one in the same process
  runs (such as the Repos scan auto-refresh starts at launch) now waits for
  it and then runs; it was dropped with "a background refresh is running".

## [0.8.0] - 2026-11-26

### Added

- In `shdash -i`, `Enter` shows the focused panel with its full lists: every
  Claude project and recent directory (up to 50 each), the last 50 commits,
  every repo with local-only work, and up to 100 open high and medium audit
  tasks. The snapshot shows what it did before.

## [0.7.0] - 2026-11-26

### Added

- `shdash -i`, an interactive mode: the same panels full-screen, laid out
  again on resize and scrolled instead of dropped. Focus moves with
  `h`/`j`/`k`/`l`, the arrows and `Tab`; `Enter` opens the focused panel
  full-screen; `r` / `R` refresh it / every panel; `q` quits. Panels past
  their TTL refresh on their own, and results from a background refresh
  show within 2 s.

## [0.6.0] - 2026-11-26

### Added

- The Shell panel's `startup` row shows zsh's startup time: the newest one and
  the median of the last 50, with the median in bold past 0.5 s. It reads
  `$XDG_CACHE_HOME/shell/startup-times`, which ~/.shell 0.136.0's zshrc
  writes.

## [0.5.0] - 2026-11-26

### Added

- Nerd Font icons before the panel titles, with `--icons` or `SHDASH_ICONS=1`
  (`--no-icons` overrides both). Off by default.

## [0.4.0] - 2026-11-26

### Added

- The Projects panel lists the five most recently visited directories, with
  their age, from z.lua's history (`$_ZL_DATA`, else `~/.zlua`). `$HOME`, `/`,
  directories that no longer exist, and directories inside a more recent one
  already listed are left out.

### Changed

- The Projects panel keeps its tmux sessions and directories when
  `claude-projects` fails, and says the refresh failed.

## [0.3.1] - 2026-11-26

### Fixed

- `shdash --version` reports the real version, commit and date for builds
  made with `go install …@version` or a plain `go build`, which do not set
  the version through `-ldflags`; they printed `dev (unknown, unknown)`.

## [0.3.0] - 2026-11-26

### Added

- A Repos panel, after Git, with repo-triage's repos that have local-only
  work. It refreshes hourly instead of every 15 minutes, so the ~35 s scan
  over every repo runs a quarter as often.

### Changed

- The Git panel shows only recent commits across your repos; the local-work
  summary moved to Repos. Audit and Shell now come after Repos, so they are
  dropped first on a short terminal.

## [0.2.0] - 2026-11-26

### Changed

- A panel that has never collected anything because its tool fails is now
  shown with the error instead of being left out; so is one whose cached data
  cannot be read.
- `refresh.log` keeps earlier runs: each background refresh appends a line
  with its time and panels, then each panel's error. Past 256 KB the log is
  moved to `refresh.log.1`.

### Fixed

- A run no longer starts a background refresh while one is already running,
  which truncated the running refresh's log and forked a process that exited
  at once.

## [0.1.0] - 2026-11-25

### Added

- `shdash` prints a snapshot of five panels, System, Projects, Git, Audit and
  Shell, in one to three columns by terminal width, dropping the
  lowest-priority panels to fit the terminal's height.
- Each panel is cached as JSON under `$XDG_CACHE_HOME/shdash` with its own TTL.
  Stale panels are shown with their age and refreshed by a detached background
  run; `--refresh` collects everything in the foreground.
- Panels come from the existing tools: fastfetch, claude-projects, tmux, ghq and
  git, repo-triage, audit's JSON export, shell-stats and shell-env-stats' live
  cache.
- `--width`, `--height`, `--no-color`, `--version`.
