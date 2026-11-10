package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"renamer/internal/engine"
	"renamer/internal/match"
	"renamer/internal/media"
	"renamer/internal/naming"
)

type inputMode int

const (
	inNone inputMode = iota
	inSearch
	inID
	inEpisode
)

type fixState struct {
	item    *engine.Item
	group   *engine.Group
	kind    media.Kind
	cands   []match.Candidate
	cursor  int
	mode    inputMode
	input   textinput.Model
	loading bool
	msg     string
	errMsg  string
}

type searchResultMsg struct {
	cands []match.Candidate
	err   error
}
type chooseResultMsg struct{ res engine.GroupResult }
type byIDMsg struct {
	cand match.Candidate
	err  error
}
type seasonsMsg struct {
	g       *engine.Group
	seasons map[int]*engine.SeasonInfo
}

func (m *Model) openFix(it *engine.Item, episode bool) tea.Cmd {
	g := it.Group
	m.fix = fixState{item: it, group: g, kind: g.Kind, cands: g.Candidates}
	if g.Kind == media.Unknown {
		m.fix.kind = media.Movie
	}
	if g.Chosen != nil {
		for i, c := range m.fix.cands {
			if c.ID == g.Chosen.ID && c.Kind == g.Chosen.Kind {
				m.fix.cursor = i
			}
		}
	}
	m.scr = scrFix
	if episode {
		return m.startInput(inEpisode)
	}
	if len(m.fix.cands) == 0 {
		return m.startInput(inSearch)
	}
	return nil
}

func (m *Model) startInput(mode inputMode) tea.Cmd {
	ti := textinput.New()
	ti.CharLimit = 200
	ti.Width = max(20, m.width-20)
	switch mode {
	case inSearch:
		ti.Prompt = "Search: "
		ti.Placeholder = "title, optionally followed by a year"
		q := m.fix.group.Query
		if m.fix.group.Year > 0 {
			q += " " + strconv.Itoa(m.fix.group.Year)
		}
		ti.SetValue(q)
	case inID:
		ti.Prompt = "TMDB ID: "
		ti.Placeholder = "e.g. 603 (from themoviedb.org/movie/603)"
	case inEpisode:
		ti.Prompt = "Episode: "
		ti.Placeholder = "S01E02, 1x02, S01E02-E03, or just 2 (keeps season)"
		if len(m.fix.item.Episodes) > 0 {
			ti.SetValue(naming.EpisodeCode(m.fix.item.Season, m.fix.item.Episodes))
		}
	}
	ti.CursorEnd()
	m.fix.input = ti
	m.fix.mode = mode
	m.fix.errMsg = ""
	return m.fix.input.Focus()
}

var (
	reQueryYear = regexp.MustCompile(`^(.*?)\s*\(?((?:19|20)\d\d)\)?$`)
	reEpInput   = regexp.MustCompile(`(?i)^\s*s?(\d{1,3})\s*[ex]\s*(\d{1,4})(?:\s*-\s*e?(\d{1,4}))?\s*$`)
	reEpOnlyIn  = regexp.MustCompile(`^\s*e?(\d{1,4})(?:\s*-\s*e?(\d{1,4}))?\s*$`)
)

func parseEpisodeInput(s string, curSeason int) (int, []int, bool) {
	rng := func(a, b string) []int {
		x, _ := strconv.Atoi(a)
		if b == "" {
			return []int{x}
		}
		y, _ := strconv.Atoi(b)
		if y < x || y-x > 50 {
			return []int{x}
		}
		var out []int
		for i := x; i <= y; i++ {
			out = append(out, i)
		}
		return out
	}
	if m := reEpInput.FindStringSubmatch(s); m != nil {
		season, _ := strconv.Atoi(m[1])
		return season, rng(m[2], m[3]), true
	}
	if m := reEpOnlyIn.FindStringSubmatch(strings.ToLower(s)); m != nil {
		if curSeason < 0 {
			curSeason = 1
		}
		return curSeason, rng(m[1], m[2]), true
	}
	return 0, nil, false
}

func (m *Model) updateFix(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := &m.fix
	switch msg := msg.(type) {
	case searchResultMsg:
		f.loading = false
		if msg.err != nil {
			f.errMsg = msg.err.Error()
			return m, nil
		}
		f.cands = msg.cands
		f.cursor = 0
		if len(f.cands) == 0 {
			f.errMsg = "No results. Try another title (s), the other type (t), or a TMDB ID (i)."
		}
		return m, nil
	case byIDMsg:
		f.loading = false
		if msg.err != nil {
			f.errMsg = "TMDB ID lookup failed: " + msg.err.Error()
			return m, nil
		}
		f.cands = []match.Candidate{msg.cand}
		f.cursor = 0
		return m, m.chooseCmd(msg.cand)
	case chooseResultMsg:
		f.loading = false
		m.eng.ApplyResult(msg.res)
		m.eng.Sort()
		m.rebuild()
		m.scr = scrReview
		m.flash = fmt.Sprintf("Matched %s to %s", plural(len(msg.res.Group.Items), "file"), msg.res.Group.Label())
		return m, nil
	case seasonsMsg:
		f.loading = false
		for s, si := range msg.seasons {
			msg.g.Seasons[s] = si
		}
		m.rebuild()
		m.scr = scrReview
		return m, nil
	case tea.KeyMsg:
		if f.loading {
			if msg.String() == "esc" {
				f.loading = false
			}
			return m, nil
		}
		if f.mode != inNone {
			return m.updateFixInput(msg)
		}
		switch msg.String() {
		case "esc", "q":
			m.scr = scrReview
		case "up", "k":
			f.cursor = max(0, f.cursor-1)
		case "down", "j":
			f.cursor = min(len(f.cands)-1, f.cursor+1)
		case "enter":
			if f.cursor >= 0 && f.cursor < len(f.cands) {
				return m, m.chooseCmd(f.cands[f.cursor])
			}
		case "s", "/":
			return m, m.startInput(inSearch)
		case "i":
			return m, m.startInput(inID)
		case "e":
			if f.group.Kind == media.TV || (f.group.Chosen != nil && f.group.Chosen.Kind == media.TV) {
				return m, m.startInput(inEpisode)
			}
			f.errMsg = "Choose a TV show first (t switches to TV search)."
		case "t":
			if f.kind == media.TV {
				f.kind = media.Movie
			} else {
				f.kind = media.TV
			}
			f.cands = nil
			return m, m.searchCmd(append([]string{f.group.Query}, f.group.Alts...), f.group.Year)
		case "u":
			// Unmatch: deselect group.
			m.eng.ApplyResult(engine.GroupResult{Group: f.group})
			m.rebuild()
			m.scr = scrReview
		}
	}
	return m, nil
}

func (m *Model) updateFixInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.fix
	switch k.String() {
	case "esc":
		f.mode = inNone
		f.input.Blur()
		return m, nil
	case "enter":
		val := strings.TrimSpace(f.input.Value())
		mode := f.mode
		f.mode = inNone
		f.input.Blur()
		switch mode {
		case inSearch:
			if val == "" {
				return m, nil
			}
			q, y := val, 0
			if mm := reQueryYear.FindStringSubmatch(val); mm != nil && strings.TrimSpace(mm[1]) != "" {
				q = strings.TrimSpace(mm[1])
				y, _ = strconv.Atoi(mm[2])
			}
			return m, m.searchCmd([]string{q}, y)
		case inID:
			id, err := strconv.Atoi(val)
			if err != nil || id <= 0 {
				f.errMsg = "Not a valid ID."
				return m, nil
			}
			f.loading = true
			kind, eng, ctx := f.kind, m.eng, m.ctx
			return m, func() tea.Msg {
				c, err := eng.ByID(ctx, kind, id)
				return byIDMsg{c, err}
			}
		case inEpisode:
			season, eps, ok := parseEpisodeInput(val, f.item.Season)
			if !ok {
				f.errMsg = "Couldn't read that. Examples: S01E02, 1x02, S01E02-E03, 5"
				f.mode = inEpisode
				return m, f.input.Focus()
			}
			m.eng.SetEpisode(f.item, season, eps)
			g := f.group
			if g.Chosen != nil && g.Chosen.Kind == media.TV {
				if missing := g.MissingSeasons(); len(missing) > 0 {
					f.loading = true
					eng, ctx, show := m.eng, m.ctx, *g.Chosen
					return m, func() tea.Msg {
						return seasonsMsg{g, eng.LoadSeasons(ctx, show, missing)}
					}
				}
			}
			m.rebuild()
			m.scr = scrReview
			return m, nil
		}
		return m, nil
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(k)
	return m, cmd
}

// searchCmd tries each query in order (with, then without the year) and
// returns the first non-empty result list.
func (m *Model) searchCmd(queries []string, year int) tea.Cmd {
	f := &m.fix
	f.loading = true
	f.errMsg = ""
	kind, eng, ctx := f.kind, m.eng, m.ctx
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		var cands []match.Candidate
		var err error
		for _, q := range queries {
			if strings.TrimSpace(q) == "" {
				continue
			}
			cands, err = eng.Search(ctx, kind, q, year, nil)
			if err == nil && len(cands) == 0 && year > 0 {
				cands, err = eng.Search(ctx, kind, q, 0, nil)
			}
			if err != nil || len(cands) > 0 {
				break
			}
		}
		if len(cands) > 20 {
			cands = cands[:20]
		}
		return searchResultMsg{cands, err}
	})
}

func (m *Model) chooseCmd(c match.Candidate) tea.Cmd {
	m.fix.loading = true
	g, eng, ctx := m.fix.group, m.eng, m.ctx
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		return chooseResultMsg{eng.Choose(ctx, g, c)}
	})
}

func (m *Model) viewFix() string {
	f := &m.fix
	w := m.width
	var b strings.Builder
	g := f.group
	b.WriteString(sTitle.Render("Fix match") + sDim.Render(fmt.Sprintf("  %s in this group · searching %s", plural(len(g.Items), "file"), f.kind)) + "\n\n")

	p := f.item.Video.Parsed
	info := fmt.Sprintf("Parsed %s: %q", p.Kind, p.Title)
	if p.Year > 0 {
		info += fmt.Sprintf(" (%d)", p.Year)
	}
	if len(p.AltTitles) > 0 {
		info += sDim.Render("  alt: " + strings.Join(p.AltTitles, " | "))
	}
	b.WriteString(truncRight(info, w) + "\n")
	for i, it := range g.Items {
		if i == 3 {
			b.WriteString(sDim.Render(fmt.Sprintf("  … and %d more", len(g.Items)-3)) + "\n")
			break
		}
		mark := "  "
		if it == f.item {
			mark = sCursor.Render("▸ ")
		}
		b.WriteString(mark + sDim.Render(truncLeft(it.Video.Rel, w-4)) + "\n")
	}
	if g.Chosen != nil {
		b.WriteString("Current: " + naming.WithYear(g.Chosen.Name(), g.Chosen.Year) + sDim.Render(fmt.Sprintf(" · tmdb %d · ", g.Chosen.ID)) + confStyle(g.Confidence).Render(g.Confidence.String()) + "\n")
	}
	b.WriteString("\n")

	if f.loading {
		b.WriteString(m.spin.View() + " Asking TMDB…\n")
	} else if len(f.cands) == 0 {
		b.WriteString(sDim.Render("No candidates.") + "\n")
	}
	listH := max(3, m.height-16)
	top := 0
	perItem := 2
	if f.cursor*perItem >= listH {
		top = f.cursor - listH/perItem + 1
	}
	for i := top; i < len(f.cands) && (i-top)*perItem < listH; i++ {
		c := f.cands[i]
		cur := "  "
		if i == f.cursor {
			cur = sCursor.Render("▌ ")
		}
		title := naming.WithYear(c.Name(), c.Year)
		extra := ""
		if c.Name() != c.Title {
			extra += " · " + c.Title
		}
		if c.OriginalLanguage != "" {
			extra += " · " + c.OriginalLanguage
		}
		extra += fmt.Sprintf(" · tmdb %d · sim %.2f", c.ID, c.Sim)
		chosen := ""
		if g.Chosen != nil && g.Chosen.ID == c.ID && g.Chosen.Kind == c.Kind {
			chosen = sOK.Render(" ●")
		}
		b.WriteString(cur + sBold.Render(title) + chosen + sDim.Render(truncRight(extra, max(0, w-len(title)-6))) + "\n")
		ov := c.Overview
		if ov == "" {
			ov = "(no overview)"
		}
		b.WriteString("    " + sDim.Render(truncRight(ov, w-6)) + "\n")
	}
	b.WriteString("\n")
	if f.errMsg != "" {
		b.WriteString(sBad.Render(f.errMsg) + "\n")
	}
	if f.mode != inNone {
		b.WriteString(f.input.View() + "\n" + sHelp.Render("enter confirm · esc cancel"))
	} else {
		help := "↑↓ select · enter choose · s search · i TMDB id · t movie/tv · u unmatch"
		if g.Kind == media.TV {
			help += " · e set episode"
		}
		b.WriteString(sHelp.Render(truncRight(help+" · esc back", w)))
	}
	return b.String()
}
