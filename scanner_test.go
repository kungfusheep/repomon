package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}

	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("setup %v: %v", c, err)
		}
	}

	// create a file and commit it
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0644)

	cmds = [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("setup %v: %v", c, err)
		}
	}

	return dir
}

func TestScanRepo_CleanGo(t *testing.T) {
	dir := setupTestRepo(t)
	r := scanRepo(dir, false)

	if r.Error != "" {
		t.Fatalf("unexpected error: %s", r.Error)
	}
	if r.Branch != "main" && r.Branch != "master" {
		t.Errorf("expected main or master, got %q", r.Branch)
	}
	if r.Modified != 0 {
		t.Errorf("expected 0 modified, got %d", r.Modified)
	}
	if r.Untracked != 0 {
		t.Errorf("expected 0 untracked, got %d", r.Untracked)
	}
	if r.Language != "Go" {
		t.Errorf("expected Go language, got %q", r.Language)
	}
	if r.LastCommit.IsZero() {
		t.Error("expected non-zero last commit time")
	}
	if r.LastMsg != "init" {
		t.Errorf("expected commit msg 'init', got %q", r.LastMsg)
	}
	if r.Health != HealthGreen {
		t.Errorf("expected GREEN health, got %s", r.Health)
	}
}

func TestScanRepo_DirtyFiles(t *testing.T) {
	dir := setupTestRepo(t)

	// add untracked files
	for i := range 5 {
		name := filepath.Join(dir, "file"+string(rune('a'+i))+".txt")
		os.WriteFile(name, []byte("test"), 0644)
	}

	// modify existing
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){ println() }\n"), 0644)

	r := scanRepo(dir, false)

	if r.Modified != 1 {
		t.Errorf("expected 1 modified, got %d", r.Modified)
	}
	if r.Untracked != 5 {
		t.Errorf("expected 5 untracked, got %d", r.Untracked)
	}
	if r.DirtyCount() != 6 {
		t.Errorf("expected 6 dirty, got %d", r.DirtyCount())
	}
}

func TestScanRepo_NotARepo(t *testing.T) {
	dir := t.TempDir()
	r := scanRepo(dir, false)

	if r.Error == "" {
		t.Error("expected error for non-git dir")
	}
	if r.Health != HealthRed {
		t.Errorf("expected RED health, got %s", r.Health)
	}
}

func TestScanRepo_NotFound(t *testing.T) {
	r := scanRepo("/nonexistent/path/that/does/not/exist", false)

	if r.Error == "" {
		t.Error("expected error for missing dir")
	}
}

func TestCalcHealth_Green(t *testing.T) {
	r := Repo{
		LastCommit: time.Now().Add(-24 * time.Hour),
		Modified:   0,
		Untracked:  0,
	}
	if h := calcHealth(r); h != HealthGreen {
		t.Errorf("expected GREEN, got %s", h)
	}
}

func TestCalcHealth_Amber_SomeDirty(t *testing.T) {
	r := Repo{
		LastCommit: time.Now().Add(-24 * time.Hour),
		Modified:   2,
		Untracked:  2,
	}
	if h := calcHealth(r); h != HealthAmber {
		t.Errorf("expected AMBER, got %s", h)
	}
}

func TestCalcHealth_Red_ManyDirty(t *testing.T) {
	r := Repo{
		LastCommit: time.Now().Add(-24 * time.Hour),
		Modified:   5,
		Untracked:  7,
	}
	if h := calcHealth(r); h != HealthRed {
		t.Errorf("expected RED, got %s", h)
	}
}

func TestCalcHealth_Red_NeverCommitted(t *testing.T) {
	r := Repo{}
	if h := calcHealth(r); h != HealthRed {
		t.Errorf("expected RED, got %s", h)
	}
}

func TestCalcHealth_Red_BuildFail(t *testing.T) {
	b := false
	r := Repo{
		LastCommit: time.Now(),
		Builds:     &b,
	}
	if h := calcHealth(r); h != HealthRed {
		t.Errorf("expected RED, got %s", h)
	}
}

func TestCalcHealth_Red_TestFail(t *testing.T) {
	b := true
	tf := false
	r := Repo{
		LastCommit: time.Now(),
		Builds:     &b,
		TestsPass:  &tf,
	}
	if h := calcHealth(r); h != HealthRed {
		t.Errorf("expected RED, got %s", h)
	}
}

func TestAgeStr(t *testing.T) {
	tests := []struct {
		age    time.Duration
		expect string
	}{
		{0, "today"},
		{12 * time.Hour, "today"},
		{36 * time.Hour, "yesterday"},
		{5 * 24 * time.Hour, "5d ago"},
		{14 * 24 * time.Hour, "2w ago"},
		{60 * 24 * time.Hour, "2mo ago"},
		{400 * 24 * time.Hour, "1y ago"},
	}

	for _, tt := range tests {
		r := Repo{LastCommit: time.Now().Add(-tt.age)}
		got := r.AgeStr()
		if got != tt.expect {
			t.Errorf("age %v: expected %q, got %q", tt.age, tt.expect, got)
		}
	}
}

func TestAgeStr_Never(t *testing.T) {
	r := Repo{}
	if r.AgeStr() != "never" {
		t.Errorf("expected 'never', got %q", r.AgeStr())
	}
}

func TestHealthString(t *testing.T) {
	if HealthGreen.String() != "GREEN" {
		t.Error("GREEN string mismatch")
	}
	if HealthAmber.String() != "AMBER" {
		t.Error("AMBER string mismatch")
	}
	if HealthRed.String() != "RED" {
		t.Error("RED string mismatch")
	}
	if HealthParked.String() != "PARKED" {
		t.Error("PARKED string mismatch")
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandHome("~/test")
	expect := filepath.Join(home, "test")
	if got != expect {
		t.Errorf("expected %q, got %q", expect, got)
	}

	got = expandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expected absolute path unchanged, got %q", got)
	}
}

func TestDiscoverRepos(t *testing.T) {
	dir := t.TempDir()

	// create git repos at the top level
	repo1 := filepath.Join(dir, "project1")
	repo2 := filepath.Join(dir, "project2")
	os.MkdirAll(filepath.Join(repo1, ".git"), 0755)
	os.MkdirAll(filepath.Join(repo2, ".git"), 0755)

	// non-repo directory (should be skipped)
	os.MkdirAll(filepath.Join(dir, "notarepo"), 0755)

	// worktree-style dir (should be skipped)
	os.MkdirAll(filepath.Join(dir, "project1@feature", ".git"), 0755)

	repos := discoverRepos(dir)
	if len(repos) != 2 {
		t.Errorf("expected 2 repos, got %d: %v", len(repos), repos)
	}
}

func TestExportMarkdown(t *testing.T) {
	repos := []Repo{
		{
			Name:       "test-project",
			Path:       "/tmp/test",
			Branch:     "main",
			LastCommit: time.Now(),
			Language:   "Go",
			Health:     HealthGreen,
		},
		{
			Name:      "broken",
			Path:      "/tmp/broken",
			Branch:    "master",
			Modified:  15,
			Untracked: 5,
			Language:  "Go",
			Health:    HealthRed,
		},
	}

	md := exportMarkdown(repos)

	if len(md) == 0 {
		t.Fatal("expected non-empty markdown")
	}
	if !strings.Contains(md, "Project Control Centre") {
		t.Error("expected header in export")
	}
}

func TestScanRepos_Parallel(t *testing.T) {
	dir1 := setupTestRepo(t)
	dir2 := setupTestRepo(t)

	repos := scanRepos([]string{dir1, dir2}, false)
	if len(repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(repos))
	}
	for _, r := range repos {
		if r.Error != "" {
			t.Errorf("unexpected error scanning %s: %s", r.Path, r.Error)
		}
	}
}

func TestDetectLanguage(t *testing.T) {
	dir := t.TempDir()

	// go project
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	if lang := detectLanguage(dir); lang != "Go" {
		t.Errorf("expected Go, got %q", lang)
	}

	// typescript project
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, "package.json"), []byte("{}"), 0644)
	if lang := detectLanguage(dir2); lang != "TypeScript" {
		t.Errorf("expected TypeScript, got %q", lang)
	}

	// unknown
	dir3 := t.TempDir()
	if lang := detectLanguage(dir3); lang != "Unknown" {
		t.Errorf("expected Unknown, got %q", lang)
	}
}
