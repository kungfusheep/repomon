package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	. "github.com/kungfusheep/glyph"
)

type entry struct {
	Key        string
	Name       string
	DimName    string // non-empty for non-session entries (dimmed)
	BrightName string // non-empty for session entries (normal)
	Branch     string
	AmberInfo  string // shown in amber when dirty or behind
	DimInfo    string // shown dimmed when clean
	Dirty      int
	Ahead      int
	Behind     int
	IsSession  bool
	Path       string
	Remote     string
	LastMsg    string
	Age        string
	ClaudeWork bool   // true when claude is actively working
	ClaudeIdle string // non-empty when claude is idle
	files      string
	filesInit  bool
}

type pvLine struct {
	Normal string
	Dimmed string
}

type dashboard struct {
	entries []entry
	result  *entry
	fl      *FilterListC[entry]
	app     *App

	preview      []pvLine
	actionOutput []pvLine
	actionActive bool
	lastSelKey   string
	todoContent  string
	todoPath       string
	editTodo       bool
	spinFrame      int
	claudeLastPoll time.Time
	status         string
}

func runDashboard() {
	d := &dashboard{}
	d.loadTmuxSessions()

	if err := d.run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	d.switchTo()

	if d.editTodo && d.todoPath != "" {
		sel := d.result
		sessionKey := sel.Key
		if !sel.IsSession {
			sessionKey = strings.ReplaceAll(filepath.Base(sel.Key), ".", "_")
		}
		exec.Command("tmux", "new-window", "-t", sessionKey, "nvim", d.todoPath).Run()
	}
}

// loadTmuxSessions populates entries from tmux sessions only. Instant.
func (d *dashboard) loadTmuxSessions() {
	sessions := getTmuxSessions()
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastAttached > sessions[j].LastAttached
	})

	d.entries = nil
	for _, s := range sessions {
		d.entries = append(d.entries, entry{
			Key:        s.Name,
			Name:       s.Name,
			BrightName: s.Name,
			IsSession:  true,
			Path:       s.Path,
		})
	}

	d.status = fmt.Sprintf("%d sessions · loading...", len(sessions))
}

// loadConfigEntries adds repos and directories from config. Runs in background.
func (d *dashboard) loadConfigEntries() {
	cfg, _ := loadConfig()
	listedPaths := map[string]bool{}
	for _, e := range d.entries {
		if e.Path != "" {
			listedPaths[normPath(e.Path)] = true
		}
	}

	for _, r := range cfg.Repos {
		np := normPath(r.Path)
		if listedPaths[np] {
			continue
		}
		listedPaths[np] = true
		d.entries = append(d.entries, entry{
			Key:     r.Path,
			Name:    filepath.Base(r.Path),
			DimName: filepath.Base(r.Path),
			Path:    r.Path,
		})
	}

	for _, root := range cfg.Roots {
		for _, dir := range discoverDirs(root) {
			if listedPaths[normPath(dir)] {
				continue
			}
			listedPaths[normPath(dir)] = true
			d.entries = append(d.entries, entry{
				Key:     dir,
				Name:    filepath.Base(dir),
				DimName: filepath.Base(dir),
				Path:    dir,
			})
		}
	}
}

// applyRepos updates entries in place from a set of scanned repos.
func (d *dashboard) applyRepos(repos []Repo) {
	repoByPath := map[string]Repo{}
	for _, r := range repos {
		repoByPath[normPath(r.Path)] = r
	}

	totalDirty := 0
	sessionCount := 0
	for i := range d.entries {
		e := &d.entries[i]
		if e.IsSession {
			sessionCount++
		}
		np := normPath(e.Path)
		repo, ok := repoByPath[np]
		if !ok {
			continue
		}
		e.Remote = repo.Remote
		e.Branch = repo.Branch
		e.Dirty = repo.DirtyCount()
		e.Ahead = repo.Ahead
		e.Behind = repo.Behind
		e.LastMsg = repo.LastMsg
		e.Age = repo.AgeStr()
		info := buildInfoStr(repo)
		if e.Dirty > 0 || e.Behind > 0 {
			e.AmberInfo = "  " + info
			e.DimInfo = ""
		} else {
			e.AmberInfo = ""
			e.DimInfo = "  " + info
		}
		totalDirty += e.Dirty
	}

	d.status = fmt.Sprintf("%d sessions · %d repos · %d dirty files", sessionCount, len(repos), totalDirty)
}

// applyClaudeStatus reads claude status files and updates entries.
// throttled to once every 500ms.
func (d *dashboard) applyClaudeStatus() {
	now := time.Now()
	if now.Sub(d.claudeLastPoll) < 500*time.Millisecond {
		return
	}
	d.claudeLastPoll = now

	statuses := loadClaudeStatuses()
	for i := range d.entries {
		e := &d.entries[i]
		if e.Path == "" {
			continue
		}
		info, ok := statuses[normPath(e.Path)]
		if !ok {
			e.ClaudeWork = false
			e.ClaudeIdle = ""
			continue
		}
		e.ClaudeWork = info.Status == "working"
		if info.Status == "idle" {
			e.ClaudeIdle = "  ◆ claude " + formatIdleDuration(time.Since(info.Ts))
		} else {
			e.ClaudeIdle = ""
		}
	}
}

// enrichFromCache loads cached scan results for instant display.
func (d *dashboard) enrichFromCache() bool {
	repos, ok := loadScanCache()
	if !ok {
		return false
	}
	d.applyRepos(repos)
	return true
}

// enrichWithRepos scans repos live and updates entries in place.
func (d *dashboard) enrichWithRepos() {
	cfg, _ := loadConfig()

	seen := map[string]bool{}
	var paths []string
	for _, r := range cfg.Repos {
		np := normPath(r.Path)
		if !seen[np] {
			paths = append(paths, r.Path)
			seen[np] = true
		}
	}

	// include tmux session paths not already in config
	for _, e := range d.entries {
		if e.Path == "" {
			continue
		}
		np := normPath(e.Path)
		if !seen[np] {
			paths = append(paths, e.Path)
			seen[np] = true
		}
	}

	if len(paths) == 0 {
		return
	}

	repos := scanRepos(paths, false)
	saveScanCache(repos)
	d.applyRepos(repos)
}

func buildInfoStr(r Repo) string {
	var parts []string
	parts = append(parts, r.Branch)
	if r.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", r.Behind))
	}
	if r.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", r.Ahead))
	}
	dirty := r.DirtyCount()
	if dirty > 0 {
		parts = append(parts, fmt.Sprintf("%d dirty", dirty))
	}
	return strings.Join(parts, " · ")
}

func (d *dashboard) run() error {
	app := NewApp()

	d.app = app

	dim := Style{FG: BrightBlack}
	amber := Style{FG: RGB(180, 150, 80)}
	claude := Style{FG: RGB(200, 150, 50)}
	selStyle := Style{BG: RGB(40, 40, 40)}

	d.fl = FilterList(&d.entries, func(e *entry) string { return e.Name + " " + e.AmberInfo + " " + e.DimInfo + " " + e.ClaudeIdle }).
		Placeholder("search...").
		MaxVisible(40).
		SelectedStyle(selStyle).
		Render(func(e *entry) any {
			return HBox(
				Text(&e.BrightName),
				Text(&e.DimName).Style(dim),
				Text(&e.AmberInfo).Style(amber),
				Text(&e.DimInfo).Style(dim),
				If(&e.ClaudeWork).Then(HBox(Text("  "), Spinner(&d.spinFrame).FG(claude.FG), Text(" claude").Style(claude))),
				Text(&e.ClaudeIdle).Style(dim),
			)
		}).
		Handle("<Enter>", func(e *entry) {
			d.result = e
			app.Stop()
		}).
		HandleClear("<Escape>", app.Stop)

	app.OnBeforeRender(func() {
		d.applyClaudeStatus()
		d.updatePreview()
	})

	app.SetView(
		VBox.Grow(1)(
			HBox.MarginVH(0, 1)(
				Text("repomon").Bold(),
				Space(),
				Text("ctrl-f:fetch  ctrl-r:pull  ctrl-x:kill  ctrl-t:todo").Style(dim),
			),
			HBox.Grow(1)(
				VBox.WidthPct(0.55)(d.fl),
				VBox.WidthPct(0.45).MarginVH(0, 2)(
					ForEach(&d.preview, func(l *pvLine) any {
						return Textf(&l.Normal, Dim(&l.Dimmed))
					}),
					If(&d.todoContent).Then(
						VBox.Grow(1).MarginVH(1, 0)(
							TextView(&d.todoContent).Grow(1),
						),
					),
				),
			),
			HBox.MarginVH(0, 1)(
				Text(&d.status).Style(dim),
			),
		),
	)

	app.Handle("<A-p>", app.Stop)
	app.Handle("<C-c>", app.Stop)
	app.Handle("<C-f>", func() { d.doAction("fetch", app) })
	app.Handle("<C-r>", func() { d.doAction("pull", app) })
	app.Handle("<C-x>", func() { d.doAction("kill", app) })
	app.Handle("<C-t>", func() {
		sel := d.fl.Selected()
		if sel != nil && d.todoPath != "" {
			d.result = sel
			d.editTodo = true
			app.Stop()
		}
	})

	go func() {
		// phase 1: add config entries + enrich from cache (fast)
		d.loadConfigEntries()
		d.enrichFromCache()
		d.fl.Refresh()
		app.RequestRender()

		// phase 2: fresh scan (slow, updates cache)
		d.enrichWithRepos()
		d.fl.Refresh()
		app.RequestRender()
	}()

	// spinner ticker for claude working animation
	go func() {
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for range tick.C {
			d.spinFrame++
			app.RequestRender()
		}
	}()

	return app.Run()
}

func (d *dashboard) updatePreview() {
	sel := d.fl.Selected()
	if sel == nil {
		d.preview = d.preview[:0]
		d.todoContent = ""
		return
	}

	// clear action output and reload todo when selection changes
	if sel.Key != d.lastSelKey {
		d.actionActive = false
		d.actionOutput = d.actionOutput[:0]
		d.lastSelKey = sel.Key
		d.loadTodo(sel.Path)
	}

	// show action output instead of repo info when active
	if d.actionActive {
		d.preview = append(d.preview[:0], d.actionOutput...)
		return
	}

	d.preview = d.preview[:0]

	if sel.Remote != "" {
		d.preview = append(d.preview, pvLine{Dimmed: sel.Remote})
	}

	if sel.Branch != "" {
		d.preview = append(d.preview, pvLine{Normal: "branch  " + sel.Branch})
	} else {
		d.preview = append(d.preview, pvLine{Normal: sel.Path})
	}

	if sel.LastMsg != "" {
		d.preview = append(d.preview, pvLine{Dimmed: "commit  " + sel.Age + " - " + sel.LastMsg})
	}

	if sel.ClaudeWork {
		d.preview = append(d.preview, pvLine{Normal: "claude  working"})
	} else if sel.ClaudeIdle != "" {
		d.preview = append(d.preview, pvLine{Dimmed: "claude  idle"})
	}

	d.preview = append(d.preview, pvLine{}) // spacer

	if sel.Behind > 0 {
		d.preview = append(d.preview, pvLine{Normal: fmt.Sprintf("↓ %d behind upstream", sel.Behind)})
	}
	if sel.Ahead > 0 {
		d.preview = append(d.preview, pvLine{Normal: fmt.Sprintf("↑ %d ahead (unpushed)", sel.Ahead)})
	}

	if sel.Dirty > 0 {
		d.preview = append(d.preview, pvLine{}) // spacer
		d.preview = append(d.preview, pvLine{Normal: fmt.Sprintf("%d dirty files:", sel.Dirty)})

		if !sel.filesInit && sel.Path != "" {
			sel.filesInit = true
			path := sel.Path
			go func() {
				sel.files = run(path, "git", "status", "--porcelain")
				if d.app != nil {
					d.app.RequestRender()
				}
			}()
		}
		if sel.files != "" {
			shown := 0
			for _, line := range strings.Split(strings.TrimSpace(sel.files), "\n") {
				if len(line) < 3 {
					continue
				}
				status := strings.TrimRight(line[:2], " ")
				name := strings.TrimSpace(line[2:])
				d.preview = append(d.preview, pvLine{Dimmed: "  " + status + " " + name})
				shown++
				if shown >= 12 {
					d.preview = append(d.preview, pvLine{Dimmed: fmt.Sprintf("  +%d more", sel.Dirty-shown)})
					break
				}
			}
		}
	}
}

// resolveTodoPath returns the TODO file path for a repo, or empty if not found.
func resolveTodoPath(repoPath string) string {
	if repoPath == "" {
		return ""
	}

	pattern := os.Getenv("REPOMON_TODO_PATTERN")
	if pattern == "" {
		pattern = "{repo}/TODO.md"
	}

	home, _ := os.UserHomeDir()
	repoPath = expandHome(repoPath)

	relPath := repoPath
	if strings.HasPrefix(repoPath, home+"/") {
		relPath = repoPath[len(home)+1:]
	}

	todoPath := strings.ReplaceAll(pattern, "{path}", relPath)
	todoPath = strings.ReplaceAll(todoPath, "{repo}", repoPath)
	todoPath = expandHome(todoPath)

	if _, err := os.Stat(todoPath); err != nil {
		return ""
	}
	return todoPath
}

// loadTodo resolves the TODO file for a repo path and loads its content.
func (d *dashboard) loadTodo(repoPath string) {
	d.todoContent = ""
	d.todoPath = resolveTodoPath(repoPath)
	if d.todoPath == "" {
		return
	}

	data, err := os.ReadFile(d.todoPath)
	if err != nil {
		d.todoPath = ""
		return
	}
	d.todoContent = string(data)
}

func (d *dashboard) doAction(verb string, app *App) {
	sel := d.fl.Selected()
	if sel == nil {
		return
	}

	d.actionActive = true
	d.actionOutput = []pvLine{{Normal: verb + "..."}}
	d.status = verb + "..."
	app.RequestRender()

	go func() {
		path := sel.Path
		if path == "" && sel.IsSession {
			path = resolveKeyToPath(sel.Key)
		}

		switch verb {
		case "fetch":
			if path != "" {
				d.streamCmd(app, path, "git", "fetch", "--all", "--prune")
			}
		case "pull":
			if path != "" {
				d.streamCmd(app, path, "git", "pull")
			}
		case "kill":
			exec.Command("tmux", "kill-session", "-t", sel.Key).Run()
			d.actionOutput = []pvLine{{Normal: "session killed"}}
			app.RequestRender()
		}

		d.enrichWithRepos()
		d.fl.Refresh()
		d.status = verb + " done"
		app.RequestRender()

		time.Sleep(3 * time.Second)
		d.actionActive = false
		app.RequestRender()
	}()
}

func (d *dashboard) streamCmd(app *App, dir string, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	d.actionOutput = []pvLine{{Normal: strings.Join(append([]string{name}, args...), " ")}, {}}
	app.RequestRender()

	if err := cmd.Start(); err != nil {
		d.actionOutput = append(d.actionOutput, pvLine{Dimmed: "error: " + err.Error()})
		app.RequestRender()
		return
	}

	go func() {
		cmd.Wait()
		pw.Close()
	}()

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		d.actionOutput = append(d.actionOutput, pvLine{Dimmed: scanner.Text()})
		app.RequestRender()
	}

	if len(d.actionOutput) == 2 {
		d.actionOutput = append(d.actionOutput, pvLine{Dimmed: "done (no output)"})
	}
}

func (d *dashboard) switchTo() {
	if d.result == nil {
		return
	}

	key := d.result.Key

	if d.result.IsSession {
		exec.Command("tmux", "switch-client", "-t", key).Run()
		return
	}

	sessionKey := strings.ReplaceAll(filepath.Base(key), ".", "_")
	if exec.Command("tmux", "has-session", "-t", sessionKey).Run() != nil {
		exec.Command("tmux", "new-session", "-d", "-s", sessionKey, "-c", key).Run()
	}
	exec.Command("tmux", "switch-client", "-t", sessionKey).Run()
}
