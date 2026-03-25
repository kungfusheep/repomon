# repomon

git repo health monitor with a TUI dashboard, inline git actions, and tmux session switcher cache.

scans repos in parallel, tracks branch status, dirty files, upstream sync, build/test results, and assigns a health rating. search across repo names, branches, and status.

## install

```bash
go install github.com/kungfusheep/repomon@latest
```

## setup

```bash
repomon --discover ~/code        # find git repos, save config
repomon --cache                  # build tmux session cache
repomon --install-cron           # background refresh every 5 min (launchd)
```

## usage

```
repomon                          interactive TUI dashboard
repomon --cache                  scan repos + tmux sessions, write fzf cache
repomon --discover ~/code [...]  find git repos and save config
repomon --export                 markdown report to stdout
repomon --export --output FILE   markdown report to file
repomon --export --build         include go build/test checks
repomon --install-cron           install launchd job (5 min refresh)
repomon --uninstall-cron         remove launchd job
```

## dashboard

two-panel TUI powered by [glyph](https://github.com/kungfusheep/glyph). left panel lists tmux sessions and repos with fuzzy search across names and git status. right panel shows a live detail view — remote, branch, last commit, sync status, and dirty files.

git actions run inline with streamed output in the detail panel:

- `enter` — switch to session (creates one if needed)
- `ctrl-f` — fetch (streams output live)
- `ctrl-r` — pull (streams output live)
- `ctrl-x` — kill session
- `esc` — quit

loads instantly from scan cache, then refreshes in the background. action output auto-clears after 3 seconds.

## health

| status | meaning |
|--------|---------|
| GREEN | clean, recent commits, builds pass |
| AMBER | a few dirty files, or stale with uncommitted work |
| RED | 10+ dirty files, build/test failures, never committed, or missing |

## tmux integration

the `--cache` flag writes a tab-delimited, ANSI-colored file to `~/.config/repomon/cache` — pipe it to `fzf --ansi` for an instant session switcher.

optional tmux hook for refresh on session switch:

```
set-hook -g client-session-changed 'run-shell -b "repomon --cache"'
```

## languages

auto-detected: Go, TypeScript, Rust, Swift, Lua.

## files

| path | purpose |
|------|---------|
| `~/.config/repomon/config.json` | repo paths and roots |
| `~/.config/repomon/cache` | pre-built fzf input |
| `~/.config/repomon/scan-cache.json` | last scan results for instant TUI |
| `~/.config/repomon/repomon.log` | launchd output |
