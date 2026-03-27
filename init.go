package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const statusCmd = "mkdir -p /tmp/claude-status && echo '{\"status\":\"%s\",\"cwd\":\"'\"$(pwd)\"'\",\"ts\":'$(date +%%s)'}' > /tmp/claude-status/$(echo \"$(pwd)\" | shasum | cut -c1-12).json"
const cleanupCmd = "rm -f /tmp/claude-status/$(echo \"$(pwd)\" | shasum | cut -c1-12).json"

var repomonHooks = map[string]string{
	"UserPromptSubmit": fmt.Sprintf(statusCmd, "working"),
	"Stop":             fmt.Sprintf(statusCmd, "idle"),
	"StopFailure":      fmt.Sprintf(statusCmd, "idle"),
	"Notification":     fmt.Sprintf(statusCmd, "idle"),
	"SessionEnd":       cleanupCmd,
}

func claudeSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func runInitClaude() {
	path := claudeSettingsPath()

	// read existing settings or start fresh
	settings := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &settings) != nil {
			settings = map[string]any{}
		}
	}

	// get or create hooks map
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	installed := 0
	for event, cmd := range repomonHooks {
		if hasRepomonHook(hooks, event) {
			fmt.Fprintf(os.Stderr, "  %s: already installed\n", event)
			continue
		}

		hook := map[string]any{
			"type":    "command",
			"command": cmd,
		}
		entry := map[string]any{
			"matcher": "",
			"hooks":   []any{hook},
		}

		existing, _ := hooks[event].([]any)
		hooks[event] = append(existing, entry)
		installed++
		fmt.Fprintf(os.Stderr, "  %s: installed\n", event)
	}

	settings["hooks"] = hooks

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "installed %d hooks to %s\n", installed, path)
}

// hasRepomonHook checks if a hook event already has a repomon status command
func hasRepomonHook(hooks map[string]any, event string) bool {
	entries, ok := hooks[event].([]any)
	if !ok {
		return false
	}
	for _, entry := range entries {
		e, _ := entry.(map[string]any)
		hookList, _ := e["hooks"].([]any)
		for _, h := range hookList {
			hm, _ := h.(map[string]any)
			cmd, _ := hm["command"].(string)
			if strings.Contains(cmd, "claude-status") {
				return true
			}
		}
	}
	return false
}
