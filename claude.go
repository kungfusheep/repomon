package main

import (
	"encoding/json"
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

// loadClaudeStatuses reads all status files and returns a map of cwd -> status.
// stale entries (> 2 hours) are ignored.
func loadClaudeStatuses() map[string]string {
	result := map[string]string{}

	entries, err := os.ReadDir(claudeStatusDir)
	if err != nil {
		return result
	}

	cutoff := time.Now().Unix() - 7200

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
		if s.Ts < cutoff {
			continue
		}
		result[normPath(s.Cwd)] = s.Status
	}

	return result
}

