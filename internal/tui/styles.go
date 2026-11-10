package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"renamer/internal/engine"
	"renamer/internal/match"
)

var (
	cOK     = lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#3fb950"}
	cWarn   = lipgloss.AdaptiveColor{Light: "#9a6700", Dark: "#d29922"}
	cBad    = lipgloss.AdaptiveColor{Light: "#cf222e", Dark: "#f85149"}
	cDim    = lipgloss.AdaptiveColor{Light: "#6e7781", Dark: "#8b949e"}
	cAccent = lipgloss.AdaptiveColor{Light: "#0969da", Dark: "#58a6ff"}
	cOrange = lipgloss.AdaptiveColor{Light: "#bc4c00", Dark: "#f0883e"}

	sTitle   = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sDim     = lipgloss.NewStyle().Foreground(cDim)
	sOK      = lipgloss.NewStyle().Foreground(cOK)
	sWarn    = lipgloss.NewStyle().Foreground(cWarn)
	sBad     = lipgloss.NewStyle().Foreground(cBad)
	sAccent  = lipgloss.NewStyle().Foreground(cAccent)
	sOrange  = lipgloss.NewStyle().Foreground(cOrange)
	sBold    = lipgloss.NewStyle().Bold(true)
	sGroup   = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sCursor  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sHelp    = lipgloss.NewStyle().Foreground(cDim)
	sPanel   = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, false, false).BorderForeground(cDim)
	sErrLine = lipgloss.NewStyle().Foreground(cBad)
)

func statusIcon(it *engine.Item) string {
	switch it.Status {
	case engine.StatusOK:
		if it.Group.Confidence == match.Manual {
			return sAccent.Render("✓")
		}
		return sOK.Render("✓")
	case engine.StatusReview:
		return sWarn.Render("~")
	case engine.StatusLow:
		return sOrange.Render("?")
	case engine.StatusUnmatched:
		return sBad.Render("✗")
	case engine.StatusConflict:
		return sBad.Render("!")
	case engine.StatusInPlace:
		return sDim.Render("=")
	}
	return " "
}

func confStyle(c match.Confidence) lipgloss.Style {
	switch c {
	case match.High:
		return sOK
	case match.Manual:
		return sAccent
	case match.Medium:
		return sWarn
	case match.Low:
		return sOrange
	}
	return sBad
}

// truncRight cuts s to width w with an ellipsis.
func truncRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.Truncate(s, w, "…")
}

// truncLeft keeps the end of s (useful for paths).
func truncLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	rs := []rune(s)
	width := 1 // ellipsis
	i := len(rs)
	for i > 0 {
		rw := runewidth.RuneWidth(rs[i-1])
		if width+rw > w {
			break
		}
		width += rw
		i--
	}
	return "…" + string(rs[i:])
}

func padRight(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
