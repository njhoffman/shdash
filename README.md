# shdash

A snapshot of this system and shell environment, laid out to fit the
terminal: System, Projects, Git, Repos, Audit and Shell panels.

```
shdash [-i] [--refresh] [--width N] [--height N] [--no-color] [--[no-]icons] [--version]
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
| Shell    | `shell-env-stats`' live-count cache (`$XDG_CACHE_HOME/shell/env-stats-live`), `shell-stats --json`, and zsh's startup times (`$XDG_CACHE_HOME/shell/startup-times`, written by ~/.shell's zshrc): the last and the median of the last 50, bold past 0.5 s | 30m  | inline |

shdash runs these tools instead of reimplementing them, so it expects the
`~/.shell` checkout's scripts on `PATH`. A panel whose tool fails and has
never produced data shows the error in place of its data; one that worked
before keeps its last data and says the refresh failed. A failing panel is
retried once per TTL. No background refresh is started while one is running.

"Inline" panels are cheap enough to collect in the foreground when nothing is
cached yet; the others show "collecting" until the background refresh is done.

## Interactive mode

`shdash -i` shows the same panels full-screen, laid out again on every
resize, without dropping any: the screen scrolls instead.

| Key | Action |
|-----|--------|
| `h` `j` `k` `l`, arrows | focus the panel left, below, above, right |
| `Tab`, `Shift-Tab` | next / previous panel in priority order |
| `Enter` | the focused panel full-screen with its full lists: every Claude project and recent directory (up to 50), the last 50 commits, every repo with local work, up to 100 open high and medium audit tasks (`j`/`k`, `PgUp`/`PgDn`, `g`/`G` scroll; `Esc` back) |
| `r`, `R` | refresh the focused panel / every panel now |
| `Ctrl-D`, `Ctrl-U`, mouse wheel | scroll |
| `q` | quit |

Panels past their TTL refresh on their own, in this process and under the
same lock as the background refresh; while another refresh holds it, the
cache is read again every 2 s and its results show when it ends.

## Layout

One column under 80 columns, two under 140, three from 140. Panels go to the
shortest column in priority order (System, Projects, Git, Repos, Audit,
Shell). On a terminal, the lowest-priority panels are dropped until the output
fits the height less one line; piped output has no height limit. Width:
`--width`, else the terminal's, else `$COLUMNS`, else 80. One accent color, no
boxes; `--no-color` or `NO_COLOR` turn colors off.

Panel titles get a Nerd Font icon with `--icons` or `SHDASH_ICONS=1` (also
`true`, `yes`, `on`); `--no-icons` turns them off whatever the variable says.
They are off by default: only the terminal knows whether its font has the
glyphs, so set the variable where it does. The icons: System 󰍹 (md-monitor),
Projects 󰉋 (md-folder), Git  (dev-git), Repos  (oct-repo), Audit 󰕥
(md-shield_check), Shell  (dev-terminal).

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
