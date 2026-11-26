# shdash

A snapshot of this system and shell environment, laid out to fit the
terminal: System, Projects, Git, Repos, Audit and Shell panels.

```
shdash [--refresh] [--width N] [--height N] [--no-color] [--version]
```

Every panel is read from a cache, so a run takes a few milliseconds. A panel
older than its TTL is still shown, with its age in bold, and refreshed by a
detached `shdash --background-refresh` that the next run picks up. `--refresh`
collects every panel in the foreground first.

## Panels

| Panel    | Source                                                                 | TTL  | First run |
|----------|------------------------------------------------------------------------|------|-----------|
| System   | `fastfetch --format json`: host, OS, kernel, uptime, CPU, memory, disk, battery, packages | 2m   | inline |
| Projects | `claude-projects --json`: the five most recent; `tmux list-sessions`; the five most recently visited directories from z.lua's data file (`$_ZL_DATA`, else `~/.zlua`), without `$HOME`, missing directories, or ones inside a directory already listed | 5m   | background |
| Git      | last commit of each of your repos (`ghq list -p` under `$SHDASH_OWNER`, plus `~/.shell`) | 15m  | background |
| Repos    | `repo-triage --tsv`: repos with local-only work                       | 1h   | background |
| Audit    | audit's JSON export (`summary.json`, `tasks.json`): report age, open tasks, stale commands, top high tasks | 10m  | inline |
| Shell    | `shell-env-stats`' live-count cache (`$XDG_CACHE_HOME/shell/env-stats-live`) and `shell-stats --json` | 30m  | inline |

shdash runs these tools instead of reimplementing them, so it expects the
`~/.shell` checkout's scripts on `PATH`. A panel whose tool fails and has
never produced data shows the error in place of its data; one that worked
before keeps its last data and says the refresh failed. A failing panel is
retried once per TTL. No background refresh is started while one is running.

"Inline" panels are cheap enough to collect in the foreground when nothing is
cached yet; the others show "collecting" until the background refresh is done.

## Layout

One column under 80 columns, two under 140, three from 140. Panels go to the
shortest column in priority order (System, Projects, Git, Repos, Audit,
Shell). On a terminal, the lowest-priority panels are dropped until the output
fits the height less one line; piped output has no height limit. Width:
`--width`, else the terminal's, else `$COLUMNS`, else 80. One accent color, no
boxes; `--no-color` or `NO_COLOR` turn colors off.

## Files and variables

- `$SHDASH_CACHE_DIR`, else `$XDG_CACHE_HOME/shdash`: one `<panel>.json` per
  panel, `.refresh.lock`, and `refresh.log`: one line per background refresh
  (time and panels) followed by any panel's error, moved to `refresh.log.1`
  once it passes 256 KB.
- `$SHDASH_AUDIT_EXPORT`: audit's export directory (default
  `$XDG_DATA_HOME/audit/export`).
- `$SHDASH_OWNER`: whose ghq repos count as yours (default `njhoffman`).

## Development

`just check` runs gofmt, golangci-lint, the tests (`-race`) and the build;
`just install` puts the binary in `$(go env GOPATH)/bin`. The tests use
fixtures and fake runners, never the host's tools, except one that builds the
binary and runs it against stub tools on `PATH` (`just test -short` skips it).
