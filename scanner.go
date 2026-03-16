package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Health int

const (
	HealthGreen  Health = iota // clean, builds, tests pass
	HealthAmber                // builds but dirty/no tests/stale
	HealthRed                  // significant uncommitted work or problems
	HealthParked               // intentionally shelved
)

func (h Health) String() string {
	switch h {
	case HealthGreen:
		return "GREEN"
	case HealthAmber:
		return "AMBER"
	case HealthRed:
		return "RED"
	case HealthParked:
		return "PARKED"
	}
	return "UNKNOWN"
}

type Repo struct {
	Name       string
	Path       string
	Branch     string
	LastCommit time.Time
	LastMsg    string
	Modified   int
	Untracked  int
	Ahead      int
	Behind     int
	Language   string
	Builds     *bool // nil = not checked, true/false = result
	TestsPass  *bool
	TestCount  int
	Health     Health
	Remote     string
	Worktrees  int
	Error      string
}

func (r *Repo) DirtyCount() int {
	return r.Modified + r.Untracked
}

func (r *Repo) AgeStr() string {
	d := time.Since(r.LastCommit)
	switch {
	case r.LastCommit.IsZero():
		return "never"
	case d < 24*time.Hour:
		return "today"
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

func (r *Repo) HealthStr() string { return r.Health.String() }

func (r *Repo) BuildStr() string {
	if r.Builds == nil {
		return "-"
	}
	if *r.Builds {
		return "OK"
	}
	return "FAIL"
}

func (r *Repo) TestStr() string {
	if r.TestsPass == nil {
		return "-"
	}
	if *r.TestsPass {
		if r.TestCount > 0 {
			return fmt.Sprintf("PASS (%d)", r.TestCount)
		}
		return "PASS"
	}
	return "FAIL"
}

func scanRepos(paths []string, runBuild bool) []Repo {
	var mu sync.Mutex
	var repos []Repo
	var wg sync.WaitGroup

	sem := make(chan struct{}, 16)

	for _, p := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			sem <- struct{}{}
			r := scanRepo(path, runBuild)
			<-sem
			mu.Lock()
			repos = append(repos, r)
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	return repos
}

func scanRepo(path string, runBuild bool) Repo {
	path = expandHome(path)

	r := Repo{
		Name: filepath.Base(path),
		Path: path,
	}

	if _, err := os.Stat(path); err != nil {
		r.Error = "directory not found"
		r.Health = HealthRed
		return r
	}

	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		r.Error = "not a git repo"
		r.Health = HealthRed
		return r
	}

	r.Language = detectLanguage(path)
	gitStatus(&r)
	gitLog(&r)
	gitAheadBehind(&r)
	gitRemote(&r)
	gitWorktrees(&r)

	if runBuild && r.Language == "Go" {
		checkBuild(&r)
		checkTests(&r)
	}

	r.Health = calcHealth(r)
	return r
}

func gitStatus(r *Repo) {
	out := run(r.Path, "git", "branch", "--show-current")
	r.Branch = strings.TrimSpace(out)

	out = run(r.Path, "git", "status", "--porcelain")
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		xy := line[:2]
		if xy == "??" {
			r.Untracked++
		} else {
			r.Modified++
		}
	}
}

func gitLog(r *Repo) {
	out := run(r.Path, "git", "log", "-1", "--format=%aI\t%s")
	parts := strings.SplitN(strings.TrimSpace(out), "\t", 2)
	if len(parts) == 2 {
		if t, err := time.Parse(time.RFC3339, parts[0]); err == nil {
			r.LastCommit = t
		}
		r.LastMsg = parts[1]
	}
}

func gitAheadBehind(r *Repo) {
	out := run(r.Path, "git", "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	out = strings.TrimSpace(out)
	parts := strings.Fields(out)
	if len(parts) == 2 {
		r.Ahead, _ = strconv.Atoi(parts[0])
		r.Behind, _ = strconv.Atoi(parts[1])
	}
}

func gitRemote(r *Repo) {
	// get the first remote, whatever it's called
	remotes := strings.TrimSpace(run(r.Path, "git", "remote"))
	if remotes == "" {
		return
	}
	name := strings.SplitN(remotes, "\n", 2)[0]
	out := strings.TrimSpace(run(r.Path, "git", "remote", "get-url", name))
	if out == "" {
		return
	}
	// ssh: git@github.com:owner/repo.git
	if strings.HasPrefix(out, "git@") {
		out = strings.TrimPrefix(out, "git@")
		out = strings.Replace(out, ":", "/", 1)
	}
	// https: https://github.com/owner/repo.git
	out = strings.TrimPrefix(out, "https://")
	out = strings.TrimPrefix(out, "http://")
	out = strings.TrimSuffix(out, ".git")
	r.Remote = out
}

func gitWorktrees(r *Repo) {
	out := run(r.Path, "git", "worktree", "list", "--porcelain")
	r.Worktrees = strings.Count(out, "worktree ")
}

func checkBuild(r *Repo) {
	_, err := runWithErr(r.Path, "go", "build", "./...")
	b := err == nil
	r.Builds = &b
}

func checkTests(r *Repo) {
	out, err := runWithErr(r.Path, "go", "test", "./...")
	pass := err == nil
	r.TestsPass = &pass

	// count test packages that actually ran
	count := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ok") {
			count++
		}
	}
	r.TestCount = count
}

func detectLanguage(path string) string {
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		return "Go"
	}
	if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
		return "TypeScript"
	}
	if _, err := os.Stat(filepath.Join(path, "Cargo.toml")); err == nil {
		return "Rust"
	}

	// check for swift project markers
	entries, _ := filepath.Glob(filepath.Join(path, "*.xcodeproj"))
	if len(entries) > 0 {
		return "Swift"
	}
	entries, _ = filepath.Glob(filepath.Join(path, "*.swift"))
	if len(entries) > 0 {
		return "Swift"
	}
	if _, err := os.Stat(filepath.Join(path, "Package.swift")); err == nil {
		return "Swift"
	}

	// lua plugin detection
	if _, err := os.Stat(filepath.Join(path, "lua")); err == nil {
		entries, _ := filepath.Glob(filepath.Join(path, "lua", "*", "*.lua"))
		if len(entries) > 0 {
			return "Lua"
		}
	}
	if _, err := os.Stat(filepath.Join(path, "colors")); err == nil {
		return "Lua"
	}

	return "Unknown"
}

func calcHealth(r Repo) Health {
	if r.Error != "" {
		return HealthRed
	}

	dirty := r.DirtyCount()
	daysSince := time.Since(r.LastCommit).Hours() / 24

	// red: significant uncommitted work, or never committed, or build fails
	if r.LastCommit.IsZero() {
		return HealthRed
	}
	if r.Builds != nil && !*r.Builds {
		return HealthRed
	}
	if r.TestsPass != nil && !*r.TestsPass {
		return HealthRed
	}
	if dirty >= 10 {
		return HealthRed
	}

	// amber: some uncommitted work, or stale, or no tests
	if dirty > 2 {
		return HealthAmber
	}
	if daysSince > 90 && dirty > 0 {
		return HealthAmber
	}
	if r.TestsPass != nil && *r.TestsPass && r.TestCount == 0 {
		return HealthAmber
	}

	return HealthGreen
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func run(dir string, name string, args ...string) string {
	out, _ := runWithErr(dir, name, args...)
	return out
}

func runWithErr(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String() + stderr.String(), err
	}
	return stdout.String(), nil
}
