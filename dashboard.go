package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	. "github.com/kungfusheep/forme"
)

// display row for AutoTable -- exported fields become columns
type RepoRow struct {
	Health string
	Name   string
	Branch string
	Age    string
	Dirty  string
	Build  string
	Tests  string
	Sync   string
	Lang   string
}

type dashboard struct {
	app   *App
	repos []Repo

	// reactive state -- all pointer-backed, mutate and render
	rows       []RepoRow
	statusLine string
	filterTab  int
	tabs       []string
	greenCount string
	amberCount string
	redCount   string
	totalCount string
	dirtyCount string
	scanAge    string
}

func newDashboard(repos []Repo) *dashboard {
	d := &dashboard{
		repos: repos,
		tabs:  []string{"All", "RED", "AMBER", "GREEN"},
	}
	d.updateCounts()
	d.applyFilter()
	return d
}

func (d *dashboard) updateCounts() {
	counts := map[Health]int{}
	totalDirty := 0
	for _, r := range d.repos {
		counts[r.Health]++
		totalDirty += r.DirtyCount()
	}
	d.totalCount = fmt.Sprintf("%d repos", len(d.repos))
	d.greenCount = fmt.Sprintf("%d", counts[HealthGreen])
	d.amberCount = fmt.Sprintf("%d", counts[HealthAmber])
	d.redCount = fmt.Sprintf("%d", counts[HealthRed])
	d.dirtyCount = fmt.Sprintf("%d uncommitted files", totalDirty)
	d.scanAge = time.Now().Format("15:04:05")
}

func (d *dashboard) applyFilter() {
	var filtered []Repo
	switch d.filterTab {
	case 0:
		filtered = append(filtered, d.repos...)
	case 1:
		for _, r := range d.repos {
			if r.Health == HealthRed {
				filtered = append(filtered, r)
			}
		}
	case 2:
		for _, r := range d.repos {
			if r.Health == HealthAmber {
				filtered = append(filtered, r)
			}
		}
	case 3:
		for _, r := range d.repos {
			if r.Health == HealthGreen {
				filtered = append(filtered, r)
			}
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Health != filtered[j].Health {
			return filtered[i].Health > filtered[j].Health
		}
		return filtered[i].DirtyCount() > filtered[j].DirtyCount()
	})

	d.rows = make([]RepoRow, len(filtered))
	for i, r := range filtered {
		d.rows[i] = toRow(r)
	}
	d.statusLine = fmt.Sprintf(" %d repos shown", len(filtered))
}

func toRow(r Repo) RepoRow {
	dirty := fmt.Sprintf("%d", r.DirtyCount())
	if r.DirtyCount() == 0 {
		dirty = "-"
	}

	sync := ""
	if r.Ahead > 0 {
		sync += fmt.Sprintf("+%d", r.Ahead)
	}
	if r.Behind > 0 {
		if sync != "" {
			sync += "/"
		}
		sync += fmt.Sprintf("-%d", r.Behind)
	}
	if sync == "" {
		sync = "-"
	}

	return RepoRow{
		Health: r.HealthStr(),
		Name:   r.Name,
		Branch: r.Branch,
		Age:    r.AgeStr(),
		Dirty:  dirty,
		Build:  r.BuildStr(),
		Tests:  r.TestStr(),
		Sync:   sync,
		Lang:   r.Language,
	}
}

func (d *dashboard) run() error {
	app, err := NewApp()
	if err != nil {
		return err
	}
	d.app = app
	app.JumpKey("g")

	headerStyle := Style{FG: Cyan, Attr: AttrBold}.Uppercase()
	altStyle := Style{BG: PaletteColor(235)}
	dimStyle := Style{FG: BrightBlack}

	app.SetView(VBox(
		HBox.MarginXY(0, 1).Gap(2)(
			Text("repomon").Bold().FG(Cyan),
			Space(),
			Text(&d.scanAge).Style(dimStyle),
		),

		HBox.Margin(1).Gap(3)(
			Leader("repos", &d.totalCount).Width(18),
			Leader("green", &d.greenCount).Width(12),
			Leader("amber", &d.amberCount).Width(12),
			Leader("red", &d.redCount).Width(10),
			Leader("dirty", &d.dirtyCount).Width(28),
		),

		SpaceH(1),

		Tabs(d.tabs, &d.filterTab).
			Style(TabsStyleBox).
			Gap(1).
			ActiveStyle(Style{FG: Cyan, Attr: AttrBold}).
			InactiveStyle(Style{FG: BrightBlack}),

		SpaceH(1),

		VBox.Grow(1)(
			AutoTable(&d.rows).
				HeaderStyle(headerStyle).
				AltRowStyle(altStyle).
				Gap(2).
				Sortable(),
		),

		HBox(
			Text(&d.statusLine).Dim(),
			Space(),
			Text("1-4:filter  g:jump/sort  r:rescan  e:export  q:quit").Dim(),
		),
	)).
		Handle("q", app.Stop).
		Handle("<Esc>", app.Stop).
		Handle("1", func() { d.filterTab = 0; d.applyFilter() }).
		Handle("2", func() { d.filterTab = 1; d.applyFilter() }).
		Handle("3", func() { d.filterTab = 2; d.applyFilter() }).
		Handle("4", func() { d.filterTab = 3; d.applyFilter() }).
		Handle("<Tab>", func() {
			d.filterTab = (d.filterTab + 1) % len(d.tabs)
			d.applyFilter()
		}).
		Handle("<S-Tab>", func() {
			d.filterTab = (d.filterTab - 1 + len(d.tabs)) % len(d.tabs)
			d.applyFilter()
		}).
		Handle("r", func() { d.rescan() }).
		Handle("e", func() { d.export() })

	return app.Run()
}

func (d *dashboard) rescan() {
	d.statusLine = " scanning..."
	d.app.RequestRender()

	go func() {
		paths := make([]string, len(d.repos))
		for i, r := range d.repos {
			paths[i] = r.Path
		}
		d.repos = scanRepos(paths, false)
		d.updateCounts()
		d.applyFilter()
		d.statusLine = fmt.Sprintf(" rescanned at %s", time.Now().Format("15:04:05"))
		d.app.RequestRender()
	}()
}

func (d *dashboard) export() {
	md := exportMarkdown(d.repos)
	home := expandHome("~")
	path := home + "/.config/repomon/export.md"
	if err := writeExportFile(path, md); err != nil {
		d.statusLine = fmt.Sprintf(" export failed: %s", err)
	} else {
		d.statusLine = fmt.Sprintf(" exported to %s", path)
	}
}

func writeExportFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func buildRepoSummary(r Repo) string {
	var parts []string

	if r.Error != "" {
		return r.Error
	}
	if r.DirtyCount() > 0 {
		parts = append(parts, fmt.Sprintf("%d dirty", r.DirtyCount()))
	}
	if r.Builds != nil && !*r.Builds {
		parts = append(parts, "build failing")
	}
	if r.TestsPass != nil && !*r.TestsPass {
		parts = append(parts, "tests failing")
	}
	if r.LastCommit.IsZero() {
		parts = append(parts, "never committed")
	}
	if len(parts) == 0 {
		return "clean"
	}
	return strings.Join(parts, ", ")
}
