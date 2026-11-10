package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"renamer/internal/engine"
	"renamer/internal/fsops"
)

type applyState struct {
	ops       []engine.Op
	idx       int
	failed    map[*engine.Item]bool
	errs      []string
	movedVids int
	movedSubs int
	skipped   int
	srcDirs   map[string]bool
	stop      bool
	stopped   bool
}

type opDoneMsg struct {
	idx int
	err error
}

func (m *Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "y", "Y":
		m.app = applyState{ops: m.eng.Plan(), failed: map[*engine.Item]bool{}, srcDirs: map[string]bool{}}
		m.scr = scrApplying
		return m, tea.Batch(m.spin.Tick, m.runOp(0))
	case "n", "N", "esc", "q":
		m.scr = scrReview
	}
	return m, nil
}

func (m *Model) viewConfirm() string {
	ops := m.eng.Plan()
	vids, subs := 0, 0
	for _, op := range ops {
		if op.IsSub {
			subs++
		} else {
			vids++
		}
	}
	_, _, _, un, cf, _ := m.counts()
	desel := 0
	for _, it := range m.eng.Items {
		if !it.Selected && it.Target != "" && it.Status != engine.StatusConflict {
			desel++
		}
	}
	var b strings.Builder
	b.WriteString(sTitle.Render("Apply changes") + "\n\n")
	fmt.Fprintf(&b, "Move %s and %s\ninto %s\n\n", sBold.Render(plural(vids, "video")), sBold.Render(plural(subs, "subtitle")), sAccent.Render(m.opt.Out))
	fmt.Fprintf(&b, "%s\n", sDim.Render(fmt.Sprintf("Skipped: %d conflicts · %d unmatched · %d deselected", cf, un, desel)))
	b.WriteString(sDim.Render("Existing files are never overwritten. Source folders left empty will be removed.") + "\n\n")
	b.WriteString(sHelp.Render("y apply · n back"))
	return b.String()
}

func (m *Model) runOp(i int) tea.Cmd {
	if i >= len(m.app.ops) {
		return nil
	}
	op := m.app.ops[i]
	if op.IsSub && m.app.failed[op.Item] {
		return func() tea.Msg { return opDoneMsg{i, errSkipped} }
	}
	return func() tea.Msg {
		return opDoneMsg{i, fsops.Move(op.Src, op.Dst, nil)}
	}
}

type skipErr struct{}

func (skipErr) Error() string { return "skipped" }

var errSkipped error = skipErr{}

func (m *Model) updateApplying(msg tea.Msg) (tea.Model, tea.Cmd) {
	d, ok := msg.(opDoneMsg)
	if !ok {
		return m, nil
	}
	a := &m.app
	op := a.ops[d.idx]
	switch {
	case d.err == errSkipped:
		a.skipped++
	case d.err != nil:
		if !op.IsSub {
			a.failed[op.Item] = true
		}
		a.errs = append(a.errs, fmt.Sprintf("%s: %v", op.Item.Video.Rel, d.err))
	default:
		a.srcDirs[filepath.Dir(op.Src)] = true
		if op.IsSub {
			a.movedSubs++
		} else {
			a.movedVids++
		}
	}
	a.idx = d.idx + 1
	cmds := []tea.Cmd{m.prog.SetPercent(float64(a.idx) / float64(len(a.ops)))}
	if a.idx >= len(a.ops) || a.stop {
		a.stopped = a.stop && a.idx < len(a.ops)
		for dir := range a.srcDirs {
			fsops.RemoveEmptyDirs(dir, m.eng.Root)
		}
		m.scr = scrDone
		return m, tea.Batch(cmds...)
	}
	cmds = append(cmds, m.runOp(a.idx))
	return m, tea.Batch(cmds...)
}

func (m *Model) viewApplying() string {
	a := &m.app
	var b strings.Builder
	b.WriteString(sTitle.Render("Moving files") + "\n\n")
	b.WriteString(m.prog.View() + fmt.Sprintf("  %d/%d\n\n", a.idx, len(a.ops)))
	if a.idx < len(a.ops) {
		b.WriteString(m.spin.View() + " " + truncLeft(a.ops[a.idx].Item.Video.Rel, m.width-4) + "\n")
	}
	if m.flash != "" {
		b.WriteString("\n" + sWarn.Render(m.flash) + "\n")
	}
	return b.String()
}

func (m *Model) viewDone() string {
	a := &m.app
	var b strings.Builder
	title := "Done"
	if a.stopped {
		title = "Stopped"
	}
	b.WriteString(sTitle.Render(title) + "\n\n")
	fmt.Fprintf(&b, "%s moved %s and %s\n", sOK.Render("✓"), plural(a.movedVids, "video"), plural(a.movedSubs, "subtitle"))
	if a.stopped {
		fmt.Fprintf(&b, "%s stopped early: %d operations not run\n", sWarn.Render("~"), len(a.ops)-a.idx)
	}
	if a.skipped > 0 {
		fmt.Fprintf(&b, "%s %d subtitles skipped because their video failed\n", sWarn.Render("~"), a.skipped)
	}
	if len(a.errs) > 0 {
		fmt.Fprintf(&b, "%s %d errors:\n", sBad.Render("✗"), len(a.errs))
		h := max(3, m.height-10)
		for i, e := range a.errs {
			if i >= h {
				fmt.Fprintf(&b, "  … and %d more\n", len(a.errs)-h)
				break
			}
			b.WriteString("  " + sErrLine.Render(truncRight(e, m.width-4)) + "\n")
		}
	}
	b.WriteString("\n" + sHelp.Render("q quit"))
	return b.String()
}

// Summary returns a plain-text report printed after the TUI exits.
func (m *Model) Summary() string {
	if m.scr != scrDone {
		return ""
	}
	a := &m.app
	var b strings.Builder
	fmt.Fprintf(&b, "Moved %s and %s into %s\n", plural(a.movedVids, "video"), plural(a.movedSubs, "subtitle"), m.opt.Out)
	for _, e := range a.errs {
		fmt.Fprintf(&b, "error: %s\n", e)
	}
	return b.String()
}
