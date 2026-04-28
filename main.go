package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]

	switch {
	case contains(args, "--cache"):
		runCache()
	case contains(args, "--preview"):
		key := flagValue(args, "--preview")
		runPreview(key)
	case contains(args, "--action"):
		// --action <verb> <key>
		vals := flagValues(args, "--action")
		if len(vals) >= 2 {
			runAction(vals[0], vals[1])
		}
	case contains(args, "--discover"):
		runDiscover(args)
	case contains(args, "--new"):
		runNew(args)
	case contains(args, "--export"):
		runExport(args)
	case contains(args, "--init-claude"):
		runInitClaude()
	case contains(args, "--install-cron"):
		installCron()
	case contains(args, "--uninstall-cron"):
		uninstallCron()
	case contains(args, "--help") || contains(args, "-h"):
		printUsage()
	default:
		runDashboard()
	}
}

func runDiscover(args []string) {
	roots := flagValues(args, "--discover")
	if len(roots) == 0 {
		roots = []string{"~/code"}
	}

	// expand ~ in roots so the config stores absolute paths
	expanded := make([]string, len(roots))
	for i, r := range roots {
		expanded[i] = expandHome(r)
	}

	var all []string
	for _, root := range roots {
		found := discoverRepos(root)
		all = append(all, found...)
	}

	fmt.Fprintf(os.Stderr, "found %d repos\n", len(all))

	var cfgRepos []RepoConfig
	for _, p := range all {
		cfgRepos = append(cfgRepos, RepoConfig{Path: p})
	}

	if err := saveConfig(Config{Roots: expanded, Repos: cfgRepos}); err != nil {
		fmt.Fprintf(os.Stderr, "error saving config: %s\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "saved to %s\n", configPath())
}

func runExport(args []string) {
	repos := loadAndScan(contains(args, "--build"))
	md := exportMarkdown(repos)

	outPath := flagValue(args, "--output")
	if outPath == "" {
		fmt.Print(md)
		return
	}

	outPath = expandHome(outPath)
	if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing: %s\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "exported to %s\n", outPath)
}

func loadAndScan(runBuild bool) []Repo {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "no config found -- run: repomon --discover ~/code\n")
		os.Exit(1)
	}

	paths := make([]string, len(cfg.Repos))
	for i, r := range cfg.Repos {
		paths[i] = r.Path
	}

	fmt.Fprintf(os.Stderr, "scanning %d repos...\n", len(paths))
	return scanRepos(paths, runBuild)
}

func printUsage() {
	fmt.Println(`repomon -- repo health monitor

usage:
  repomon --cache                  build tmux session cache (fast, for fzf switcher)
  repomon --export                 export markdown to stdout
  repomon --export --output FILE   export to file
  repomon --discover ~/code [...]  discover repos and save config
  repomon --new <lang> <name>      scaffold a new project, git init, verify, register
  repomon --install-cron           install launchd job for periodic cache refresh
  repomon --uninstall-cron         remove launchd job
  repomon --init-claude            install claude code hooks for status tracking

flags:
  --cache           scan repos + tmux sessions, write colored cache for fzf
  --build           run go build/test checks with --export (default: git-only, fast)
  --discover        scan directories for git repos and save config
  --export          output markdown report instead of TUI
  --output          file path for export (default: stdout)
  --install-cron    install a launchd plist that refreshes cache every 5 minutes
  --uninstall-cron  unload and remove the launchd plist
  --init-claude     install hooks into ~/.claude/settings.json for live status
  --new             scaffold a new project: <lang> <name> [--no-switch]
                    languages: go, ts, js, py, rust, swift, zig, lua, nvim
                    env: REPOMON_CODE_ROOT, REPOMON_LANG_<LANG>, REPOMON_TEMPLATES_DIR,
                         REPOMON_GO_MODULE_PREFIX`)
}

func contains(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			return args[i+1]
		}
	}
	return ""
}

func flagValues(args []string, flag string) []string {
	var vals []string
	for i, a := range args {
		if a == flag {
			for j := i + 1; j < len(args); j++ {
				if strings.HasPrefix(args[j], "--") {
					break
				}
				vals = append(vals, args[j])
			}
		}
	}
	return vals
}

