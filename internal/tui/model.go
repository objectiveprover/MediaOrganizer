// Package tui is the Bubble Tea interface: scan → review → fix → apply.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"renamer/internal/engine"
	"renamer/internal/scan"
)

type screen int

const (
	scrLoading screen = iota
	scrReview
	scrFix
	scrConfirm
	scrApplying
	scrDone
	scrIgnored
)

// Options configure the TUI.
type Options struct {
	Root    string
	Out     string
	API     engine.API
	Workers int
}

// Model is the root Bubble Tea model.
type Model struct {
	opt    Options
	ctx    context.Context
	cancel context.CancelFunc
	eng    *engine.Engine
	scr    screen
	width  int
	height int
	err    error
	flash  string

	spin spinner.Model
	prog progress.Model

	// loading
	phase       string
	done, total int
	matchCh     chan tea.Msg

	// review
	visible     []*engine.Item
	cursor      int
	offset      int
	onlyAttn    bool
	filter      string
	filtering   bool
	filterInput textinput.Model
	ignoredOff  int

	fix fixState
	app applyState
}

// New creates the model.
func New(opt Options) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = sAccent
	fi := textinput.New()
	fi.Prompt = "/ "
	fi.Placeholder = "filter by path or title"
	if opt.Workers <= 0 {
		opt.Workers = 8
	}
	return &Model{
		opt: opt, ctx: ctx, cancel: cancel, spin: sp, filterInput: fi,
		prog:  progress.New(progress.WithDefaultGradient()),
		phase: "Scanning files",
	}
}

// ---- messages

type scanDoneMsg struct {
	eng *engine.Engine
	err error
}
type matchProgressMsg struct{ done, total int }
type matchDoneMsg struct{}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.scanCmd())
}

func (m *Model) scanCmd() tea.Cmd {
	return func() tea.Msg {
		sr, err := scan.Scan(m.opt.Root, m.opt.Out)
		if err != nil {
			return scanDoneMsg{err: err}
		}
		return scanDoneMsg{eng: engine.New(m.opt.API, sr, m.opt.Out)}
	}
}

func (m *Model) startMatch() tea.Cmd {
	ch := make(chan tea.Msg, 64)
	m.matchCh = ch
	eng := m.eng
	go func() {
		eng.MatchAll(m.ctx, m.opt.Workers, func(d, t int) {
			select {
			case ch <- matchProgressMsg{d, t}:
			default:
			}
		})
		ch <- matchDoneMsg{}
		close(ch)
	}()
	return waitFor(ch)
}

func waitFor(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.prog.Width = min(60, msg.Width-10)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		pm, cmd := m.prog.Update(msg)
		m.prog = pm.(progress.Model)
		return m, cmd
	case tea.KeyMsg:
		// Fast typing can deliver several runes in one message; outside text
		// inputs, handle them one key at a time.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !m.typing() {
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		if msg.String() == "ctrl+c" {
			if m.scr == scrApplying {
				m.app.stop = true
				m.flash = "Stopping after the current file…"
				return m, nil
			}
			m.cancel()
			return m, tea.Quit
		}
	}

	switch m.scr {
	case scrLoading:
		return m.updateLoading(msg)
	case scrReview:
		return m.updateReview(msg)
	case scrFix:
		return m.updateFix(msg)
	case scrConfirm:
		return m.updateConfirm(msg)
	case scrApplying:
		return m.updateApplying(msg)
	case scrDone:
		if k, ok := msg.(tea.KeyMsg); ok && (k.String() == "q" || k.String() == "enter" || k.String() == "esc") {
			return m, tea.Quit
		}
	case scrIgnored:
		return m.updateIgnored(msg)
	}
	return m, nil
}

func (m *Model) updateLoading(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case scanDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.eng = msg.eng
		if len(m.eng.Items) == 0 {
			m.err = fmt.Errorf("no video files found in %s", m.opt.Root)
			return m, nil
		}
		m.phase = "Matching against TMDB"
		m.total = len(m.eng.Groups)
		return m, m.startMatch()
	case matchProgressMsg:
		m.done, m.total = msg.done, msg.total
		return m, waitFor(m.matchCh)
	case matchDoneMsg:
		m.scr = scrReview
		m.refresh()
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "q" || (m.err != nil && msg.String() == "enter") {
			m.cancel()
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.scr {
	case scrLoading:
		return m.viewLoading()
	case scrReview:
		return m.viewReview()
	case scrFix:
		return m.viewFix()
	case scrConfirm:
		return m.viewConfirm()
	case scrApplying:
		return m.viewApplying()
	case scrDone:
		return m.viewDone()
	case scrIgnored:
		return m.viewIgnored()
	}
	return ""
}

func (m *Model) viewLoading() string {
	var b strings.Builder
	b.WriteString(sTitle.Render("renamer") + "\n\n")
	if m.err != nil {
		b.WriteString(sBad.Render("Error: "+m.err.Error()) + "\n\n" + sHelp.Render("q quit"))
		return b.String()
	}
	b.WriteString(m.spin.View() + " " + m.phase)
	if m.total > 0 {
		fmt.Fprintf(&b, "  %d/%d", m.done, m.total)
	}
	if m.eng != nil {
		fmt.Fprintf(&b, "\n\n%s", sDim.Render(fmt.Sprintf("%d videos in %d groups · %d files ignored",
			len(m.eng.Items), len(m.eng.Groups), len(m.eng.Ignored))))
	}
	b.WriteString("\n\n" + sHelp.Render("q quit"))
	return b.String()
}

// typing reports whether a text input currently has focus.
func (m *Model) typing() bool {
	return (m.scr == scrReview && m.filtering) || (m.scr == scrFix && m.fix.mode != inNone)
}
