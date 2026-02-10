package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Config struct {
	Repos []RepoConfig `json:"repos"`
}

type RepoConfig struct {
	Path  string `json:"path"`
	Name  string `json:"name,omitempty"`  // override display name
	Group string `json:"group,omitempty"` // grouping label
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "repomon", "config.json")
}

func loadConfig() (Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return Config{}, err
	}
	var c Config
	err = json.Unmarshal(data, &c)
	return c, err
}

func saveConfig(c Config) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

// skip directories that are clearly not user projects
var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".cache": true,
	"dist": true, "build": true, ".git": true,
	"pkg": true, "bin": true,
}

// discoverRepos walks a root directory looking for git repos (1 level deep)
func discoverRepos(root string) []string {
	root = expandHome(root)
	var repos []string
	seen := map[string]bool{}

	entries, err := os.ReadDir(root)
	if err != nil {
		return repos
	}

	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' || skipDirs[e.Name()] {
			continue
		}

		// skip worktree copies (name@branch pattern) -- we'll find them via git
		name := e.Name()
		if strings.Contains(name, "@") {
			continue
		}

		p := filepath.Join(root, name)

		if isGitRepo(p) {
			if !seen[p] {
				repos = append(repos, p)
				seen[p] = true
			}
		}
	}

	sort.Strings(repos)
	return repos
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	// .git can be a file (worktree) or directory
	return info.IsDir() || info.Mode().IsRegular()
}
