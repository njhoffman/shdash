# Changelog

All notable changes to shdash are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
uses [semantic versioning](https://semver.org/).

## [Unreleased]

## [0.13.3] - 2026-11-26

### Fixed

- A panel's cache entry is synced to disk before it replaces the old one,
  so a crash right after a refresh can no longer leave it empty.

## [0.13.2] - 2026-11-26

### Fixed

- The cache cleanup leaves another build's panels alone: an entry no panel
  of the running shdash owns is removed only once it has not been written
  for a week, so a dev build and the installed one sharing the cache no
  longer delete each other's new panels on every refresh.

## [0.13.1] - 2026-11-26

### Fixed

- The Shell panel fits 80 columns: aliases, functions, widgets and scripts
  get a row each, with the live shell's count beside the repo's, numbers
  aligned. The repo's widget count, collected but never shown, is on its
  row.

## [0.13.0] - 2026-11-26

### Changed

- Packages right-aligns the update counts and lines up the "(N major)"
  notes, now in a muted shade of the accent instead of bold.
- Projects lines its rows up in columns (version, branch, todos, overdue,
  changed, last session), with the todos before the sparser columns so they
  stay on screen at 80 columns.
- Repos lines up the repo names and branches.

## [0.12.1] - 2026-11-26

### Fixed

- The background refresh cleans the cache directory: it removes the entries
  of panels shdash no longer has and, a day on, the temporary files of a
  write that was killed before it finished, and logs what it removed.
- `-i` renders the panels only when their data changes and on each 2-second
  tick, instead of two or three times on every key press.

### Added

- A GitHub Actions workflow runs `just check`'s steps (gofmt, golangci-lint,
  the tests with `-race`, the build) on every push to `main` and every pull
  request.

## [0.12.0] - 2026-11-26

### Added

- The Packages panel's full-screen view in `-i` lists, under each package
  manager, its packages by gap (current, patch, minor, major, update, ahead,
  unknown) and how many audit never looked up, and says when audit could not
  read a manager and kept an earlier scan of it.

## [0.11.1] - 2026-11-26

### Changed

- The Projects panel's tmux row is the session count and the attached
  sessions; it listed every name, which was cut at 80 columns past three.
  The full-screen panel in `-i` still lists every session.

### Fixed

- The Shell panel's startup file and line counts have their own `init` row:
  next to the startup times they were cut at 80 columns.

## [0.11.0] - 2026-11-26

### Changed

- The Packages panel fits 80 columns: each manager's row is its installed
  and updatable counts, and the footer says when audit last scanned, how
  many packages it never looked up, and, on its own line, when the registry
  was last checked, with `audit packages outdated` under it once that is
  over 7 days old. It read "checked 20d ago" right after a rescan, because a
  plain rescan does not look the registries up again.
- Audit and Packages are collected again as soon as audit writes a new
  `summary.json`, instead of up to 10 minutes later.
- The System panel no longer has a `pkgs` row: the Packages panel breaks the
  same count down by manager.

### Added

- `--refresh` says "waiting for the running refresh" on stderr when another
  refresh holds the lock, instead of waiting silently.

## [0.10.1] - 2026-11-26

### Fixed

- A collector that crashes on output it does not expect no longer ends the
  whole refresh, or `-i` with the terminal left on its screen: that panel
  keeps its last data and says the refresh failed with the panic's message.
  A panel whose cached data crashes its renderer says it cannot show it.
- The Git panel no longer saves a list cut short by its 1-minute timeout as
  a complete one: it keeps the commits read and says the refresh failed
  after how many repos, so it is retried after the TTL.
- In `-i`, a panel with nothing cached says "collecting", not "shown on the
  next run": interactive mode shows it as soon as its refresh ends.

## [0.10.0] - 2026-11-26

### Changed

- The Packages panel reads the per-manager counts in audit's `summary.json`
  (audit 0.29.0 and later), a few hundred bytes, instead of counting every
  item in the 1.9 MB `packages.json`; an export without them still falls back
  to `packages.json`.

### Fixed

- "unchecked" in the Packages panel is audit's own count of packages it never
  looked up in their registry. It counted every package without a gap, which
  also took in commits and local builds whose versions do not compare.
- The "checked ... ago" line is the age of the oldest registry lookup, not of
  the export, which audit rewrites after every run whether or not it looked
  anything up. With only system managers (no registry) it is the export's age.

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
