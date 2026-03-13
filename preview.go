package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func runPreview(key string) {
	if key == separator || key == "" {
		return
	}

	path := resolveKeyToPath(key)
	if path == "" {
		fmt.Println(key)
		return
	}

	r := scanRepo(path, false)

	if r.Error != "" {
		fmt.Println(r.Error)
		return
	}

	fmt.Printf("branch  %s\n", r.Branch)

	if !r.LastCommit.IsZero() {
		fmt.Printf("commit  %s — %s\n", r.AgeStr(), r.LastMsg)
	}

	if r.Ahead > 0 || r.Behind > 0 {
		fmt.Println()
		if r.Behind > 0 {
			fmt.Printf("↓ %d behind upstream\n", r.Behind)
		}
		if r.Ahead > 0 {
			fmt.Printf("↑ %d ahead (unpushed)\n", r.Ahead)
		}
	}

	if r.DirtyCount() > 0 {
		fmt.Printf("\n%d dirty files:\n", r.DirtyCount())
		out := run(path, "git", "status", "--porcelain")
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line != "" {
				fmt.Printf("  %s\n", line)
			}
		}
	}
}

func runAction(verb, key string) {
	if key == separator || key == "" {
		return
	}

	path := resolveKeyToPath(key)

	switch verb {
	case "fetch":
		if path != "" {
			run(path, "git", "fetch", "--all", "--prune")
		}
	case "pull":
		if path != "" {
			run(path, "git", "pull")
		}
	case "kill":
		exec.Command("tmux", "kill-session", "-t", key).Run()
	case "stash":
		if path != "" {
			run(path, "git", "stash")
		}
	}

	// refresh cache to reflect the change
	runCache()
}

// resolveKeyToPath turns a cache key (session name or path) into a filesystem path
func resolveKeyToPath(key string) string {
	if strings.HasPrefix(key, "/") {
		return key
	}

	// check if it's a tmux session and get its working directory
	out, err := exec.Command("tmux", "display-message", "-t", key, "-p", "#{pane_current_path}").Output()
	if err != nil {
		// try session_path as fallback
		out, err = exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{session_path}", "-f", "#{==:#{session_name},"+key+"}").Output()
		if err != nil {
			return ""
		}
		parts := strings.SplitN(strings.TrimSpace(string(out)), "\t", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return ""
	}

	return strings.TrimSpace(string(out))
}
