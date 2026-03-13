package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/kungfusheep/glyph"
)

type entry struct {
	Key       string
	Name      string
	DimName   string // non-empty for non-session entries (dimmed)
	BrightName string // non-empty for session entries (normal)
	Branch    string
	AmberInfo string // shown in amber when dirty or behind
	DimInfo   string // shown dimmed when clean
	Dirty     int
	Ahead     int
	Behind    int
	IsSession bool
	Path      string
	LastMsg   string
	Age       string
	files     string
	filesInit bool
}

type pvLine struct {
	Normal string
	Dimmed string
}

type dashboard struct {
	entries []entry
	result  *entry
	fl      *FilterListC[entry]

	preview []pvLine
	status  string
}

func runDashboard() {
	d := &dashboard{}
	d.loadTmuxSessions()

	if err := d.run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	d.switchTo()
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
	if len(cfg.Repos) == 0 {
		return
	}

	paths := make([]string, len(cfg.Repos))
	for i, r := range cfg.Repos {
		paths[i] = r.Path
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
	app, err := NewApp()
	if err != nil {
		return err
	}

	dim := Style{FG: BrightBlack}
	amber := Style{FG: RGB(180, 150, 80)}
	selStyle := Style{BG: RGB(40, 40, 40)}

	d.fl = FilterList(&d.entries, func(e *entry) string { return e.Name }).
		Placeholder("search...").
		MaxVisible(40).
		SelectedStyle(selStyle).
		Render(func(e *entry) any {
			return HBox(
				Text(&e.BrightName),
				Text(&e.DimName).Style(dim),
				Text(&e.AmberInfo).Style(amber),
				Text(&e.DimInfo).Style(dim),
			)
		}).
		Handle("<Enter>", func(e *entry) {
			d.result = e
			app.Stop()
		}).
		HandleClear("<Escape>", app.Stop)

	app.OnBeforeRender(func() {
		d.updatePreview()
	})

	app.SetView(
		VBox.Grow(1)(
			HBox.MarginVH(0, 1)(
				Text("repomon").Bold(),
				Space(),
				Text("ctrl-f:fetch  ctrl-r:pull  ctrl-x:kill").Style(dim),
			),
			HBox.Grow(1)(
				VBox.WidthPct(0.55)(d.fl),
				VBox.WidthPct(0.45).MarginVH(0, 2)(
					ForEach(&d.preview, func(l *pvLine) any {
						return Textf(&l.Normal, Dim(&l.Dimmed))
					}),
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

	return app.Run()
}

func (d *dashboard) updatePreview() {
	sel := d.fl.Selected()
	if sel == nil {
		d.preview = d.preview[:0]
		return
	}

	d.preview = d.preview[:0]

	if sel.Branch != "" {
		d.preview = append(d.preview, pvLine{Normal: "branch  " + sel.Branch})
	} else {
		d.preview = append(d.preview, pvLine{Normal: sel.Path})
	}

	if sel.LastMsg != "" {
		d.preview = append(d.preview, pvLine{Dimmed: "commit  " + sel.Age + " - " + sel.LastMsg})
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
			sel.files = run(sel.Path, "git", "status", "--porcelain")
			sel.filesInit = true
		}
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

func (d *dashboard) doAction(verb string, app *App) {
	sel := d.fl.Selected()
	if sel == nil {
		return
	}

	d.status = verb + "..."

	go func() {
		path := sel.Path
		if path == "" && sel.IsSession {
			path = resolveKeyToPath(sel.Key)
		}

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
			exec.Command("tmux", "kill-session", "-t", sel.Key).Run()
		}

		d.enrichWithRepos()
		d.fl.Refresh()
		d.status = verb + " done"
		app.RequestRender()
	}()
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
