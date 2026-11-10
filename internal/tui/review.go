package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"renamer/internal/engine"
	"renamer/internal/media"
	"renamer/internal/naming"
)

// refresh recomputes the visible item list after any change, keeping the
// cursor on the same item when possible.
func (m *Model) refresh() {
	var cur *engine.Item
	if m.cursor >= 0 && m.cursor < len(m.visible) {
		cur = m.visible[m.cursor]
	}
	q := strings.ToLower(m.filter)
	m.visible = m.visible[:0]
	for _, it := range m.eng.Items {
		if m.onlyAttn && !it.Status.NeedsAttention() {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Video.Rel+" "+m.relOut(it.Target)+" "+it.Group.Label()), q) {
			continue
		}
		m.visible = append(m.visible, it)
	}
	m.cursor = 0
	for i, it := range m.visible {
		if it == cur {
			m.cursor = i
			break
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

func (m *Model) relOut(p string) string {
	if p == "" {
		return ""
	}
	r, err := filepath.Rel(m.opt.Out, p)
	if err != nil {
		return p
	}
	return r
}

func (m *Model) current() *engine.Item {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return m.visible[m.cursor]
}

func (m *Model) rebuild() {
	m.eng.Rebuild()
	m.refresh()
}

func (m *Model) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.filtering {
		switch k.String() {
		case "enter", "esc":
			m.filtering = false
			m.filterInput.Blur()
			if k.String() == "esc" {
				m.filter = ""
				m.filterInput.SetValue("")
			}
			m.refresh()
			return m, nil
		}
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.filter = m.filterInput.Value()
		m.refresh()
		return m, cmd
	}
	m.flash = ""
	page := max(1, m.listHeight()/2)
	switch k.String() {
	case "q":
		m.cancel()
		return m, tea.Quit
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(len(m.visible)-1, m.cursor+1)
	case "pgup", "ctrl+u":
		m.cursor = max(0, m.cursor-page)
	case "pgdown", "ctrl+d":
		m.cursor = min(len(m.visible)-1, m.cursor+page)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.visible) - 1
	case " ":
		if it := m.current(); it != nil {
			if it.Target == "" {
				m.flash = "This file has no target yet: press enter to fix the match."
			} else {
				it.Selected = !it.Selected
				m.rebuild()
			}
		}
	case "x":
		if it := m.current(); it != nil {
			sel := false
			for _, o := range it.Group.Items {
				if !o.Selected && o.Target != "" {
					sel = true
				}
			}
			for _, o := range it.Group.Items {
				if o.Target != "" || !sel {
					o.Selected = sel
				}
			}
			m.rebuild()
		}
	case "enter":
		if it := m.current(); it != nil {
			return m, m.openFix(it, false)
		}
	case "e":
		if it := m.current(); it != nil {
			if it.Group.Kind != media.TV {
				m.flash = "Episode numbers only apply to TV. Use enter → t to switch this to a TV search."
			} else {
				return m, m.openFix(it, true)
			}
		}
	case "f":
		m.onlyAttn = !m.onlyAttn
		m.refresh()
	case "/":
		m.filtering = true
		m.filterInput.Focus()
		return m, nil
	case "i":
		m.scr = scrIgnored
		m.ignoredOff = 0
	case "a":
		if len(m.eng.Plan()) == 0 {
			m.flash = "Nothing selected to move."
		} else {
			m.scr = scrConfirm
		}
	}
	return m, nil
}

func (m *Model) counts() (ok, review, low, unmatched, conflict, selected int) {
	for _, it := range m.eng.Items {
		switch it.Status {
		case engine.StatusOK:
			ok++
		case engine.StatusReview:
			review++
		case engine.StatusLow:
			low++
		case engine.StatusUnmatched:
			unmatched++
		case engine.StatusConflict:
			conflict++
		}
		if it.Applicable() {
			selected++
		}
	}
	return
}

func (m *Model) listHeight() int { return max(3, m.height-9) }

func (m *Model) viewReview() string {
	w := m.width
	var b strings.Builder

	ok, rv, low, un, cf, sel := m.counts()
	head := sTitle.Render("renamer") + "  " + sDim.Render(truncLeft(m.opt.Root, w/3)+" → "+truncLeft(m.opt.Out, w/3))
	stats := fmt.Sprintf("%s %d  %s %d  %s %d  %s %d  %s %d   %s",
		sOK.Render("✓"), ok, sWarn.Render("~"), rv, sOrange.Render("?"), low, sBad.Render("✗"), un, sBad.Render("!"), cf,
		sBold.Render(fmt.Sprintf("%d/%d selected", sel, len(m.eng.Items))))
	b.WriteString(head + "\n" + stats)
	if m.onlyAttn {
		b.WriteString(sWarn.Render("  [needs attention only]"))
	}
	if m.filter != "" && !m.filtering {
		b.WriteString(sAccent.Render("  [filter: " + m.filter + "]"))
	}
	b.WriteString("\n")

	// Build list lines.
	type line struct {
		text string
		item int // index into visible, -1 for headers
	}
	var lines []line
	start := make([]int, len(m.visible))
	var lastGroup *engine.Group
	for i, it := range m.visible {
		g := it.Group
		if g != lastGroup && (g.Kind == media.TV || len(g.Items) > 1) {
			hdr := sGroup.Render("── "+g.Label()) + sDim.Render(fmt.Sprintf(" · %s · %s", g.Kind, plural(len(g.Items), "file")))
			if g.Chosen != nil {
				hdr += sDim.Render(fmt.Sprintf(" · tmdb %d · ", g.Chosen.ID)) + confStyle(g.Confidence).Render(g.Confidence.String())
			}
			lines = append(lines, line{hdr, -1})
		}
		lastGroup = g
		start[i] = len(lines)

		cur := "  "
		if i == m.cursor {
			cur = sCursor.Render("▌ ")
		}
		box := "[ ]"
		if it.Selected {
			box = "[x]"
		}
		if it.Target == "" {
			box = "   "
		}
		src := truncLeft(it.Video.Rel, w-12)
		l1 := cur + box + " " + statusIcon(it) + " " + sDim.Render(it.Group.Kind.Short()) + "  " + src
		var l2 string
		if it.Target != "" {
			t := "→ " + truncLeft(m.relOut(it.Target), w-12)
			if it.Selected {
				l2 = "          " + t
			} else {
				l2 = "          " + sDim.Render(t)
			}
		} else {
			note := "no target"
			if len(it.Notes) > 0 {
				note = it.Notes[0]
			}
			l2 = "          " + sBad.Render(truncRight(note, w-12))
		}
		lines = append(lines, line{l1, i}, line{l2, i})
	}

	h := m.listHeight()
	if len(m.visible) > 0 {
		s := start[m.cursor]
		top := s
		if s > 0 && lines[s-1].item == -1 {
			top = s - 1
		}
		if top < m.offset {
			m.offset = top
		}
		if s+2 > m.offset+h {
			m.offset = s + 2 - h
		}
	}
	m.offset = max(0, min(m.offset, max(0, len(lines)-h)))
	b.WriteString("\n")
	for i := m.offset; i < min(len(lines), m.offset+h); i++ {
		b.WriteString(lines[i].text + "\n")
	}
	for i := len(lines) - m.offset; i < h; i++ {
		b.WriteString("\n")
	}
	if len(m.visible) == 0 {
		b.WriteString(sDim.Render("  nothing to show") + "\n")
	}

	// Details panel.
	b.WriteString(sPanel.Width(w).Render(m.details(w)) + "\n")

	if m.filtering {
		b.WriteString(m.filterInput.View())
	} else if m.flash != "" {
		b.WriteString(sWarn.Render(m.flash))
	} else {
		b.WriteString(sHelp.Render(truncRight("↑↓ move · space select · x group · enter fix · e episode · f attention · / filter · i ignored · a apply · q quit", w)))
	}
	return b.String()
}

func (m *Model) details(w int) string {
	it := m.current()
	if it == nil {
		return "\n\n"
	}
	p := it.Video.Parsed
	parsed := fmt.Sprintf("Parsed: %s %q", p.Kind, p.Title)
	if p.Year > 0 {
		parsed += fmt.Sprintf(" %d", p.Year)
	}
	if p.Kind == media.TV {
		parsed += " " + naming.EpisodeCode(it.Season, it.Episodes)
	}
	if len(p.Notes) > 0 {
		parsed += sDim.Render(" · " + strings.Join(p.Notes, " · "))
	}
	g := it.Group
	matchLine := sBad.Render("Match: none")
	if g.Chosen != nil {
		c := g.Chosen
		matchLine = fmt.Sprintf("Match: %s · tmdb %d · %s", naming.WithYear(c.Name(), c.Year), c.ID, confStyle(g.Confidence).Render(g.Confidence.String()))
		if c.Name() != c.Title {
			matchLine += sDim.Render(" · " + c.Title)
		}
	}
	notes := ""
	if len(it.Notes) > 0 {
		notes = sWarn.Render("Notes: " + strings.Join(it.Notes, "; "))
	}
	subs := ""
	if len(it.Subs) > 0 {
		var tags []string
		for _, s := range it.Subs {
			tags = append(tags, strings.TrimPrefix(filepath.Ext(strings.TrimSuffix(s.Dst, filepath.Ext(s.Dst))), "."))
		}
		subs = fmt.Sprintf("Subtitles: %d (%s)", len(it.Subs), strings.Join(tags, ", "))
	}
	return truncRight(parsed, w) + "\n" + truncRight(matchLine, w) + "\n" + truncRight(notes, w) + "\n" + truncRight(subs, w)
}

func (m *Model) updateIgnored(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	h := max(1, m.height-4)
	switch k.String() {
	case "esc", "i", "q":
		m.scr = scrReview
	case "down", "j":
		m.ignoredOff = min(max(0, len(m.eng.Ignored)-h), m.ignoredOff+1)
	case "up", "k":
		m.ignoredOff = max(0, m.ignoredOff-1)
	case "pgdown", "ctrl+d":
		m.ignoredOff = min(max(0, len(m.eng.Ignored)-h), m.ignoredOff+h)
	case "pgup", "ctrl+u":
		m.ignoredOff = max(0, m.ignoredOff-h)
	}
	return m, nil
}

func (m *Model) viewIgnored() string {
	var b strings.Builder
	b.WriteString(sTitle.Render("Ignored files") + sDim.Render(fmt.Sprintf("  %d", len(m.eng.Ignored))) + "\n\n")
	h := max(1, m.height-4)
	for i := m.ignoredOff; i < min(len(m.eng.Ignored), m.ignoredOff+h); i++ {
		ig := m.eng.Ignored[i]
		reason := sDim.Render(" (" + ig.Reason + ")")
		b.WriteString(truncLeft(ig.Rel, m.width-len(ig.Reason)-4) + reason + "\n")
	}
	if len(m.eng.Ignored) == 0 {
		b.WriteString(sDim.Render("none") + "\n")
	}
	b.WriteString("\n" + sHelp.Render("↑↓ scroll · esc back"))
	return b.String()
}
