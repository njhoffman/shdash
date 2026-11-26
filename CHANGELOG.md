# Changelog

All notable changes to shdash are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
uses [semantic versioning](https://semver.org/).

## [Unreleased]

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
