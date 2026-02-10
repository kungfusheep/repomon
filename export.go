package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func exportMarkdown(repos []Repo) string {
	var b strings.Builder

	b.WriteString("# Project Control Centre\n\n")
	b.WriteString(fmt.Sprintf("Last scanned: %s\n\n", time.Now().Format("2006-01-02 15:04")))
	b.WriteString("---\n\n")

	writeAlerts(&b, repos)
	writeSummary(&b, repos)
	writeDashboard(&b, repos)

	return b.String()
}

func writeAlerts(b *strings.Builder, repos []Repo) {
	var alerts []Repo
	for _, r := range repos {
		if r.Health == HealthRed {
			alerts = append(alerts, r)
		}
	}

	if len(alerts) == 0 {
		return
	}

	b.WriteString("## Alerts & Risks\n\n")
	b.WriteString("| Project | Issue |\n")
	b.WriteString("|---------|-------|\n")
	for _, r := range alerts {
		issue := describeIssue(r)
		b.WriteString(fmt.Sprintf("| %s | %s |\n", r.Name, issue))
	}
	b.WriteString("\n---\n\n")
}

func describeIssue(r Repo) string {
	if r.Error != "" {
		return r.Error
	}
	if r.LastCommit.IsZero() {
		return fmt.Sprintf("Never committed (%d files on disk)", r.DirtyCount())
	}
	if r.Builds != nil && !*r.Builds {
		return "Build failing"
	}
	if r.TestsPass != nil && !*r.TestsPass {
		return "Tests failing"
	}
	if r.DirtyCount() >= 10 {
		return fmt.Sprintf("%d uncommitted files", r.DirtyCount())
	}
	return fmt.Sprintf("%d dirty files, last commit %s", r.DirtyCount(), r.AgeStr())
}

func writeSummary(b *strings.Builder, repos []Repo) {
	counts := map[Health]int{}
	totalDirty := 0
	for _, r := range repos {
		counts[r.Health]++
		totalDirty += r.DirtyCount()
	}

	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("**Total repos:** %d | ", len(repos)))
	b.WriteString(fmt.Sprintf("**GREEN:** %d | **AMBER:** %d | **RED:** %d | **PARKED:** %d\n",
		counts[HealthGreen], counts[HealthAmber], counts[HealthRed], counts[HealthParked]))
	b.WriteString(fmt.Sprintf("**Total uncommitted files:** %d\n\n", totalDirty))
	b.WriteString("---\n\n")
}

func writeDashboard(b *strings.Builder, repos []Repo) {
	// sort by health (red first), then by dirty count descending
	sorted := make([]Repo, len(repos))
	copy(sorted, repos)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Health != sorted[j].Health {
			return sorted[i].Health > sorted[j].Health // RED=2 > AMBER=1 > GREEN=0
		}
		return sorted[i].DirtyCount() > sorted[j].DirtyCount()
	})

	b.WriteString("## All Repos\n\n")
	b.WriteString("| Health | Project | Branch | Last Commit | Dirty | Build | Tests | Lang |\n")
	b.WriteString("|--------|---------|--------|-------------|-------|-------|-------|------|\n")
	for _, r := range sorted {
		b.WriteString(fmt.Sprintf("| %s | %s | `%s` | %s | %d | %s | %s | %s |\n",
			r.HealthStr(), r.Name, r.Branch, r.AgeStr(),
			r.DirtyCount(), r.BuildStr(), r.TestStr(), r.Language))
	}
	b.WriteString("\n")
}
