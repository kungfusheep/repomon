# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
go build ./...              # Build all packages
go test ./...               # Run all tests
go test -v ./...            # Run tests with verbose output
go test -run TestFuncName   # Run a specific test by name
go install github.com/kungfusheep/repomon@latest  # Install binary
```

There is no linter or formatter configured beyond standard `gofmt`.

## Architecture

Repomon is a Go CLI tool that scans git repositories in parallel, calculates health status, and presents results via a terminal UI (TUI) or as fzf-compatible cache for tmux session switching.

### Core Flow

`main.go` routes CLI flags to these modes:
- **Cache mode** (`--cache`): Scans repos, writes ANSI-colored fzf-compatible cache to `~/.config/repomon/cache`
- **Dashboard mode** (default): Interactive TUI built on the `glyph` library (local dependency at `../tui`)
- **Discover mode** (`--discover`): Walks directory trees to find git repos, saves to config
- **Export mode** (`--export`): Generates markdown report of repo health
- **Preview/Action mode**: Used by fzf for repo details and git operations

### Key Modules

- **scanner.go** — `scanRepo()` / `scanRepos()`: Parallel git inspection (branch, dirty files, ahead/behind, last commit, language detection). Health calculation: GREEN (clean+recent), AMBER (few dirty or stale), RED (10+ dirty, build/test failures, missing)
- **config.go** — JSON config at `~/.config/repomon/config.json` with schema `{roots: [...], repos: [{path, name?, group?}]}`
- **cache.go** — Two-tier caching: `scan-cache.json` (structured data for instant TUI load) + `cache` (fzf-ready ANSI output). `getTmuxSessions()` merges active tmux sessions with repo data
- **dashboard.go** — TUI using `glyph` library with search filtering, preview pane, and actions (Ctrl-F fetch, Ctrl-R pull, Ctrl-X kill session)
- **preview.go** — Renders repo detail view for fzf preview and resolves cache keys back to paths
- **launchd.go** — macOS-only launchd integration for periodic background cache refresh

### Dependencies

The `glyph` TUI framework (`github.com/kungfusheep/glyph`) is a local replace directive pointing to `../tui`. It must be present at that path for builds to work. The `fzf` library is used for fuzzy matching in the TUI filter.

### Patterns

- Parallel scanning uses `sync.WaitGroup` across all repos
- Paths are normalized via symlink resolution and home directory expansion throughout
- Two-phase TUI loading: cached results shown instantly, then background rescan updates the view
