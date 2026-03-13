package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	ansiDim      = "\033[38;2;100;100;100m"
	ansiDimAmber = "\033[38;2;140;120;60m"
	ansiReset    = "\033[0m"
	separator    = "───"
)

type tmuxSession struct {
	Name         string
	Path         string
	LastAttached int64
}

func runCache() {
	sessions := getTmuxSessions()

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastAttached > sessions[j].LastAttached
	})

	var repos []Repo
	cfg, err := loadConfig()
	hasConfig := err == nil
	if hasConfig {
		paths := make([]string, len(cfg.Repos))
		for i, r := range cfg.Repos {
			paths[i] = r.Path
		}
		repos = scanRepos(paths, false)
		saveScanCache(repos)
	}

	repoByPath := map[string]Repo{}
	for _, r := range repos {
		repoByPath[normPath(r.Path)] = r
	}

	var lines []string
	matchedPaths := map[string]bool{}

	// active sessions: white, only show dirt
	for _, s := range sessions {
		np := normPath(s.Path)
		if repo, ok := repoByPath[np]; ok {
			matchedPaths[np] = true
			lines = append(lines, fmt.Sprintf("%s\t%s", s.Name, formatSessionLine(s.Name, repo)))
		} else {
			lines = append(lines, fmt.Sprintf("%s\t%s", s.Name, s.Name))
		}
	}

	// separator
	lines = append(lines, fmt.Sprintf("%s\t%s%s%s", separator, ansiDim, separator, ansiReset))

	// unmatched repos: dimmed, sorted by most recent commit
	var unmatched []Repo
	for _, r := range repos {
		if !matchedPaths[normPath(r.Path)] {
			unmatched = append(unmatched, r)
		}
	}
	sort.Slice(unmatched, func(i, j int) bool {
		return unmatched[i].LastCommit.After(unmatched[j].LastCommit)
	})

	// track all paths we've already listed (sessions + repos)
	listedPaths := map[string]bool{}
	for k := range matchedPaths {
		listedPaths[k] = true
	}
	for _, r := range unmatched {
		np := normPath(r.Path)
		listedPaths[np] = true
		lines = append(lines, fmt.Sprintf("%s\t%s", r.Path, formatDimLine(r.Name, r)))
	}

	// discover all directories from config roots so non-git folders are included
	if hasConfig {
		var extraDirs []string
		for _, root := range cfg.Roots {
			extraDirs = append(extraDirs, discoverDirs(root)...)
		}
		sort.Strings(extraDirs)
		for _, d := range extraDirs {
			if listedPaths[normPath(d)] {
				continue
			}
			name := filepath.Base(d)
			lines = append(lines, fmt.Sprintf("%s\t%s%s%s", d, ansiDim, name, ansiReset))
		}
	}

	output := strings.Join(lines, "\n")
	if len(lines) > 0 {
		output += "\n"
	}

	cachePath := CacheFilePath()
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(cachePath, []byte(output), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing cache: %s\n", err)
		os.Exit(1)
	}
}

// sessions: normal text, dirty count as a subtle amber suffix
func formatSessionLine(name string, r Repo) string {
	dirty := r.DirtyCount()
	if dirty > 0 {
		return fmt.Sprintf("%s  %s%s· %d%s", name, ansiDimAmber, r.Branch, dirty, ansiReset)
	}
	return name
}

// non-session repos: all dimmed, dirty count inline
func formatDimLine(name string, r Repo) string {
	dirty := r.DirtyCount()
	if dirty > 0 {
		return fmt.Sprintf("%s%s  %s · %d%s", ansiDim, name, r.Branch, dirty, ansiReset)
	}
	return fmt.Sprintf("%s%s%s", ansiDim, name, ansiReset)
}

func getTmuxSessions() []tmuxSession {
	out, err := exec.Command("tmux", "list-sessions", "-F",
		"#{session_name}\t#{session_path}\t#{session_last_attached}").Output()
	if err != nil {
		return nil
	}

	var sessions []tmuxSession
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}

		attached, _ := strconv.ParseInt(parts[2], 10, 64)
		sessions = append(sessions, tmuxSession{
			Name:         parts[0],
			Path:         parts[1],
			LastAttached: attached,
		})
	}

	return sessions
}

func normPath(p string) string {
	p = filepath.Clean(expandHome(p))
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return p
}

func CacheFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "repomon", "cache")
}

func ScanCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "repomon", "scan-cache.json")
}

func saveScanCache(repos []Repo) {
	data, err := json.Marshal(repos)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(ScanCachePath()), 0755)
	_ = os.WriteFile(ScanCachePath(), data, 0644)
}

func loadScanCache() ([]Repo, bool) {
	data, err := os.ReadFile(ScanCachePath())
	if err != nil {
		return nil, false
	}
	var repos []Repo
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, false
	}
	return repos, true
}
