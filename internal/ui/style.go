// Package ui is yardo's terminal UI: the home board, its menus, and the
// prompts.
package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// Tab is one board tab. A tab with Sections lists their items (whose Tab
// is a section's ID) in order, each section under its Title.
type Tab struct {
	ID, Title string
	Sections  []Tab
}

// ExtraTab holds the extra mise tools: ids "mise:<spec>".
const ExtraTab = "__extra__"

var extraRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@:/+-]*$`)

// ValidExtra reports whether q can be an extra mise tool spec ("pulumi",
// "pipx:pytest", "github:owner/repo@1.2").
func ValidExtra(q string) bool { return len(q) <= 200 && extraRe.MatchString(q) }

var (
	accent   = lipgloss.NewStyle().Foreground(tui.Accent)
	bold     = lipgloss.NewStyle().Bold(true)
	muted    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8A8A"))
	green    = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	tabOn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1A1A1A")).Background(tui.Accent)
	accentBd = accent.Bold(true)
)

// wrap splits s into lines at most w cells wide, at spaces where it can.
func wrap(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		for runewidth.StringWidth(word) > w {
			cut := runewidth.Truncate(word, w, "")
			if cut == "" {
				// A rune wider than w: it gets a line of its own.
				cut = string([]rune(word)[0])
			}
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			lines = append(lines, cut)
			word = word[len(cut):]
		}
		switch {
		case line == "":
			line = word
		case runewidth.StringWidth(line)+1+runewidth.StringWidth(word) <= w:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" || len(lines) == 0 {
		lines = append(lines, line)
	}
	return lines
}
