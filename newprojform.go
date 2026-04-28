package main

import (
	"io"
	"time"

	. "github.com/kungfusheep/glyph"
)

// registerNewProjectView wires the "new" named view: a form on top, a streaming
// Log beneath. Submission runs the scaffolder in a goroutine, piping its output
// into the Log. After success the user presses 'y' to switch into the new tmux
// session (or Esc to stay on the dashboard) — that keypress is dispatched on
// the input thread, so app.Stop unblocks the read loop cleanly.
func registerNewProjectView(app *App, d *dashboard) {
	langs := supportedLangs()

	var langIdx int
	var name string
	statusLine := ""
	hintLine := "enter to scaffold · esc to dashboard"
	spinFrame := 0

	state := "form" // form | running | success | failure
	showLog := false
	isRunning := false

	setState := func(s string) {
		state = s
		showLog = s != "form"
		isRunning = s == "running"
	}

	// single pipe lives for app lifetime; closing isn't necessary because
	// goroutines exit on success/failure and the OS reaps on process exit
	pr, pw := io.Pipe()

	form := Form.LabelFG(Cyan).OnSubmit(func(f *FormC) {
		if state != "form" && state != "failure" {
			return
		}
		if !f.ValidateAll() {
			return
		}
		if langIdx < 0 || langIdx >= len(langs) {
			return
		}

		// capture submission values — the form keeps accepting keystrokes
		// during scaffolding, so reading `name`/`langIdx` later would pick
		// up anything the user typed while we were busy.
		submittedLang := langs[langIdx]
		submittedName := name

		setState("running")
		statusLine = "scaffolding..."
		hintLine = ""
		app.RequestRender()

		go func() {
			err := executeNew(submittedLang, submittedName, false, pw)
			if err != nil {
				setState("failure")
				statusLine = "✗ " + err.Error()
				hintLine = "edit name and submit to retry · esc to dashboard"
				app.RequestRender()
				return
			}
			setState("success")
			statusLine = "✓ ready"
			hintLine = "y to switch into new session · esc to stay on dashboard"
			d.newProjectPath = resolveTargetDir(submittedName, languages[submittedLang])
			// blur the form so its field sub-router pops off the input
			// stack — otherwise the field's HandleUnmatched eats our 'y'
			// before it reaches the view-level handler.
			f.FocusManager().BlurCurrent()
			app.RequestRender()
		}()
	})(
		Field("lang", Radio(&langIdx, langs...)),
		Field("name", Input(&name).Placeholder("project name").Width(40).Validate(VRequired)),
	)

	// spinner ticker — only requests render while running, so it doesn't churn
	go func() {
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for range tick.C {
			if isRunning {
				spinFrame++
				app.RequestRender()
			}
		}
	}()

	dim := Style{FG: BrightBlack}
	cyan := Style{FG: Cyan}

	newView := app.View("new",
		VBox.MarginVH(1, 4).Grow(1)(
			VBox.Border(BorderRounded).Title("new project").MarginVH(0, 1)(
				form,
				HBox(
					If(&isRunning).Then(HBox(Spinner(&spinFrame).FG(Cyan), Text(" "))),
					Text(&statusLine).Style(cyan),
				),
				Text(&hintLine).Style(dim),
			),
			If(&showLog).Then(
				VBox.Grow(1).MarginVH(1, 0).Border(BorderSingle).Title("output")(
					Log(pr).Grow(1).MaxLines(1000),
				),
			),
		),
	)

	newView.Handle("<Escape>", func() {
		if state == "running" {
			return // can't bail mid-scaffold
		}
		// user said no to switching (or just navigating back); clear the path
		d.newProjectPath = ""
		app.PopView()
	})
	newView.Handle("y", func() {
		if state == "success" && d.newProjectPath != "" {
			app.Stop()
		}
	})
	newView.Handle("<C-c>", func() {
		if state == "running" {
			return
		}
		d.newProjectPath = ""
		app.Stop()
	})
}
