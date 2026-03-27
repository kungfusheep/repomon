package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type claudeStatus struct {
	Status string `json:"status"`
	Cwd    string `json:"cwd"`
	Ts     int64  `json:"ts"`
}

const claudeStatusDir = "/tmp/claude-status"

type claudeInfo struct {
	Status string
	Ts     time.Time
}

// loadClaudeStatuses reads all status files and returns a map of cwd -> info.
func loadClaudeStatuses() map[string]claudeInfo {
	result := map[string]claudeInfo{}

	entries, err := os.ReadDir(claudeStatusDir)
	if err != nil {
		return result
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(claudeStatusDir, e.Name()))
		if err != nil {
			continue
		}
		var s claudeStatus
		if json.Unmarshal(data, &s) != nil || s.Cwd == "" {
			continue
		}
		result[normPath(s.Cwd)] = claudeInfo{
			Status: s.Status,
			Ts:     time.Unix(s.Ts, 0),
		}
	}

	return result
}

func formatIdleDuration(since time.Duration) string {
	switch {
	case since < time.Minute:
		return fmt.Sprintf("%ds", int(since.Seconds()))
	case since < time.Hour:
		return fmt.Sprintf("%dm", int(since.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(since.Hours()))
	}
}

