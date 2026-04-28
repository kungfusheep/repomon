package main

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed all:templates
var defaultTemplates embed.FS

type langInfo struct {
	pathDefault string   // path under $REPOMON_CODE_ROOT
	pathEnv     string   // env var name that overrides pathDefault
	verify      []string // command run in target dir to confirm it builds+runs
}

var languages = map[string]langInfo{
	"go":    {"go/src", "REPOMON_LANG_GO", []string{"go", "run", "."}},
	"ts":    {"ts", "REPOMON_LANG_TS", []string{"bun", "run", "index.ts"}},
	"js":    {"js", "REPOMON_LANG_JS", []string{"node", "index.js"}},
	"py":    {"py", "REPOMON_LANG_PY", []string{"python3", "main.py"}},
	"rust":  {"rust", "REPOMON_LANG_RUST", []string{"cargo", "run", "--quiet"}},
	"swift": {"swift", "REPOMON_LANG_SWIFT", []string{"swift", "run"}},
	"zig":   {"zig", "REPOMON_LANG_ZIG", []string{"zig", "build", "run"}},
	"lua":   {"lua", "REPOMON_LANG_LUA", []string{"lua", "main.lua"}},
	"nvim":  {"lua", "REPOMON_LANG_LUA", nil}, // plugin layout — nothing standalone to run
}

func runNew(args []string) {
	vals := flagValues(args, "--new")
	if len(vals) < 2 {
		fmt.Fprintln(os.Stderr, "usage: repomon --new <lang> <name> [--no-switch]")
		fmt.Fprintf(os.Stderr, "languages: %s\n", strings.Join(supportedLangs(), ", "))
		os.Exit(1)
	}
	if err := executeNew(vals[0], vals[1], !contains(args, "--no-switch"), os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// executeNew scaffolds a new project, writing progress and command output to
// out. Used by both the CLI flag (out=os.Stderr) and the dashboard view
// (out=pipe writer feeding a Log component).
func executeNew(lang, name string, autoSwitch bool, out io.Writer) error {
	info, ok := languages[lang]
	if !ok {
		return fmt.Errorf("unknown language %q. supported: %s", lang, strings.Join(supportedLangs(), ", "))
	}
	if name == "" {
		return fmt.Errorf("name is required")
	}

	target := resolveTargetDir(name, info)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("target already exists: %s", target)
	}

	fmt.Fprintf(out, "→ scaffolding %s project at %s\n", lang, target)

	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("creating dir: %w", err)
	}

	subs := substitutions(lang, name)
	if err := scaffoldTo(lang, target, subs, out); err != nil {
		return fmt.Errorf("scaffolding: %w", err)
	}

	// if the template included a _init.sh, run it once then remove it
	initPath := filepath.Join(target, "_init.sh")
	if _, err := os.Stat(initPath); err == nil {
		_ = os.Chmod(initPath, 0755)
		cmd := exec.Command("bash", initPath)
		cmd.Dir = target
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("_init.sh failed: %w", err)
		}
		_ = os.Remove(initPath)
	}

	if err := gitInit(target, out); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	if info.verify != nil {
		fmt.Fprintf(out, "→ verify: %s\n", strings.Join(info.verify, " "))
		cmd := exec.Command(info.verify[0], info.verify[1:]...)
		cmd.Dir = target
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("verify failed: %w", err)
		}
	}

	addRepoToConfig(target)

	fmt.Fprintf(out, "✓ ready: %s\n", target)

	if autoSwitch {
		openTmuxSession(target)
	}
	return nil
}

func supportedLangs() []string {
	out := make([]string, 0, len(languages))
	for k := range languages {
		out = append(out, k)
	}
	// stable order so help text is consistent
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func resolveTargetDir(name string, info langInfo) string {
	root := os.Getenv("REPOMON_CODE_ROOT")
	if root == "" {
		root = "~/code"
	}
	sub := os.Getenv(info.pathEnv)
	if sub == "" {
		sub = info.pathDefault
	}
	return filepath.Join(expandHome(root), sub, name)
}

func substitutions(lang, name string) map[string]string {
	subs := map[string]string{
		"name":   name,
		"module": name,
		"cmd":    name,
	}

	switch lang {
	case "go":
		if prefix := os.Getenv("REPOMON_GO_MODULE_PREFIX"); prefix != "" {
			subs["module"] = strings.TrimRight(prefix, "/") + "/" + name
		}
	case "nvim":
		// neovim convention: plugin called "foo.nvim" exposes module "foo"
		mod := strings.TrimSuffix(name, ".nvim")
		subs["module"] = mod
		if mod != "" {
			// first letter capitalised feels right for a :Command
			subs["cmd"] = strings.ToUpper(mod[:1]) + mod[1:]
		}
	}

	return subs
}

// templateRoot returns the fs.FS rooted at the given language's template dir,
// preferring a disk override and falling back to the embedded copy.
func templateRoot(lang string) (fs.FS, string, error) {
	dir := os.Getenv("REPOMON_TEMPLATES_DIR")
	if dir == "" {
		dir = "~/.config/repomon/templates"
	}
	disk := filepath.Join(expandHome(dir), lang)
	if info, err := os.Stat(disk); err == nil && info.IsDir() {
		return os.DirFS(disk), "disk:" + disk, nil
	}

	sub, err := fs.Sub(defaultTemplates, "templates/"+lang)
	if err != nil {
		return nil, "", fmt.Errorf("no templates for %q: %w", lang, err)
	}
	// confirm the embedded subtree actually exists
	if _, err := fs.Stat(sub, "."); err != nil {
		return nil, "", fmt.Errorf("no templates for %q", lang)
	}
	return sub, "embedded", nil
}

func scaffoldTo(lang, target string, subs map[string]string, out io.Writer) error {
	root, source, err := templateRoot(lang)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  templates: %s\n", source)

	return fs.WalkDir(root, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}

		outRel := strings.TrimSuffix(substitute(path, subs), ".tmpl")
		outPath := filepath.Join(target, outRel)

		if d.IsDir() {
			return os.MkdirAll(outPath, 0755)
		}

		data, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		// only substitute inside *.tmpl; other files (e.g. _init.sh) copy verbatim
		if strings.HasSuffix(path, ".tmpl") {
			data = []byte(substitute(string(data), subs))
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if strings.HasSuffix(outRel, ".sh") {
			mode = 0755
		}
		return os.WriteFile(outPath, data, mode)
	})
}

func substitute(s string, subs map[string]string) string {
	for k, v := range subs {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

func gitInit(dir string, out io.Writer) error {
	steps := [][]string{
		{"git", "init", "--quiet"},
		{"git", "add", "."},
		{"git", "commit", "--quiet", "-m", "initial"},
	}
	for _, step := range steps {
		cmd := exec.Command(step[0], step[1:]...)
		cmd.Dir = dir
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(step, " "), err)
		}
	}
	return nil
}

func addRepoToConfig(path string) {
	cfg, err := loadConfig()
	if err != nil {
		cfg = Config{}
	}
	for _, r := range cfg.Repos {
		if normPath(r.Path) == normPath(path) {
			return
		}
	}
	cfg.Repos = append(cfg.Repos, RepoConfig{Path: path})
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warn: could not update repomon config: %v\n", err)
	}
}

// openTmuxSession mirrors dashboard.switchTo's path-side behaviour: create the
// session if missing, switch into it if we're already inside tmux.
func openTmuxSession(path string) {
	sessionKey := strings.ReplaceAll(filepath.Base(path), ".", "_")
	if exec.Command("tmux", "has-session", "-t", sessionKey).Run() != nil {
		exec.Command("tmux", "new-session", "-d", "-s", sessionKey, "-c", path).Run()
	}
	if os.Getenv("TMUX") != "" {
		exec.Command("tmux", "switch-client", "-t", sessionKey).Run()
	} else {
		fmt.Fprintf(os.Stderr, "tmux session ready: %s (attach with: tmux attach -t %s)\n", sessionKey, sessionKey)
	}
}

