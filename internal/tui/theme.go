package tui

import (
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// yardo palette — a Claude-style warm coral accent on neutral text.
var (
	Accent = lipgloss.Color("#D97757") // coral
	subtle = lipgloss.Color("#6C6C6C")
	invert = lipgloss.Color("#1A1A1A")
	warn   = lipgloss.Color("#E5484D") // warm red — errors/warnings on stderr
)

// A Look is one of yardo's themes: its colours, its marks, and how its
// name and titles read. A terminal's background and plain text stay its
// own, so a look is the colours yardo paints with.
type Look struct {
	Key, Name, About string
	// Accent is "" for the user's accent colour (Classic); the others
	// bring their own. Ink is text on an accent background.
	Accent, Muted, OK, Warn, Bad, Info, Ink lipgloss.Color
	// Brand is the name in the header; Caps puts titles in capitals; Sep
	// goes between a title's parts (Settings › Look).
	Brand string
	Caps  bool
	Sep   string
	Mark  Marks
}

// Marks are a row's health marks.
type Marks struct{ OK, Down, Partial, Need, Update, Unchecked string }

// Looks are the themes, Classic first (the default).
var Looks = []Look{
	{Key: "classic", Name: "Classic", About: "calm; your accent colour",
		Muted: "#8A8A8A", OK: "2", Warn: "3", Bad: "1", Info: "12", Ink: "#1A1A1A",
		Brand: "yardo", Sep: "›", Mark: Marks{"✓", "✗", "~", "○", "↑", "·"}},
	{Key: "bridge", Name: "Bridge", About: "a starship console: navy, cyan, amber",
		Accent: "#4FE3F0", Muted: "#5F7E95", OK: "#5BE3A6", Warn: "#FFB547", Bad: "#FF4D6A", Info: "#8FA8FF", Ink: "#070D1A",
		Brand: "◢◤ YARDO ◥◣", Caps: true, Sep: "▸", Mark: Marks{"●", "✕", "◧", "◇", "↑", "·"}},
	{Key: "covert", Name: "Covert", About: "a night-ops dossier: phosphor green, signal orange",
		Accent: "#FF9F1C", Muted: "#5A8A63", OK: "#6DFF8C", Warn: "#FFD84D", Bad: "#FF3B3B", Info: "#7FD4FF", Ink: "#050B07",
		Brand: "⌖ YARDO //", Caps: true, Sep: "//", Mark: Marks{"■", "!", "▣", "?", "↑", "□"}},
	{Key: "tactical", Name: "Tactical", About: "a shooter HUD: graphite, white, orange",
		Accent: "#FF8A00", Muted: "#7D858F", OK: "#8BD450", Warn: "#FFD000", Bad: "#FF3B30", Info: "#4FA3FF", Ink: "#101215",
		Brand: "▌YARDO▐", Caps: true, Sep: "▸", Mark: Marks{"●", "✕", "◧", "◇", "↑", "·"}},
}

// Cur is the look in use.
var Cur = Looks[0]

// LookOf is the look key names, and whether there's one.
func LookOf(key string) (Look, bool) {
	for _, l := range Looks {
		if l.Key == key {
			return l, true
		}
	}
	return Looks[0], false
}

// Use makes key's look the one in use ("" or unknown: Classic), with
// accent as Classic's accent colour.
func Use(key string, accent lipgloss.Color) {
	Cur, _ = LookOf(key)
	Accent = Cur.Accent
	if Accent == "" {
		Accent = accent
	}
	subtle, invert, warn = lipgloss.Color("#6C6C6C"), Cur.Ink, lipgloss.Color("#E5484D")
	if Cur.Key != "classic" {
		subtle, warn = Cur.Muted, Cur.Bad
	}
}

// Title is a screen title as the look writes it: "Settings › Look", in
// its capitals and with its separator.
func Title(s string) string {
	if Cur.Sep != "›" {
		s = strings.ReplaceAll(s, "›", Cur.Sep)
	}
	if Cur.Caps {
		s = strings.ToUpper(s)
	}
	return s
}

// Theme returns the shared yardo huh theme so every prompt/form/picker
// renders with a consistent coral accent.
func Theme() *huh.Theme {
	t := huh.ThemeBase()

	f := &t.Focused
	f.Base = f.Base.BorderForeground(Accent)
	f.Title = f.Title.Foreground(Accent).Bold(true)
	f.SelectSelector = f.SelectSelector.Foreground(Accent)
	f.SelectedOption = f.SelectedOption.Foreground(Accent)
	f.MultiSelectSelector = f.MultiSelectSelector.Foreground(Accent)
	f.FocusedButton = f.FocusedButton.Background(Accent).Foreground(invert)
	f.TextInput.Cursor = f.TextInput.Cursor.Foreground(Accent)
	f.TextInput.Prompt = f.TextInput.Prompt.Foreground(Accent)

	t.Blurred.Title = t.Blurred.Title.Foreground(subtle)
	return t
}

// Steps renders a tab bar of wizard phases with the active one highlighted,
// e.g. Connection › Defaults › Access › Review.
func Steps(active string, steps ...string) string {
	activeStyle := lipgloss.NewStyle().Bold(true).Foreground(invert).Background(Accent).Padding(0, 1)
	idleStyle := lipgloss.NewStyle().Foreground(subtle).Padding(0, 1)
	sep := lipgloss.NewStyle().Foreground(subtle).Render("›")

	parts := make([]string, 0, len(steps))
	for _, s := range steps {
		if s == active {
			parts = append(parts, activeStyle.Render(s))
		} else {
			parts = append(parts, idleStyle.Render(s))
		}
	}
	return strings.Join(parts, sep)
}

// Subtitle renders a short line of muted text, e.g. a one-line description
// under the wizard's phase tabs.
func Subtitle(text string) string {
	return renderLines(lipgloss.NewStyle().Foreground(subtle), text)
}

// Muted dims text with the same subtle color Subtitle uses — for a
// picker option that's present but has nothing to act on (still
// listed, just visually deprioritized against options that do).
func Muted(text string) string {
	return renderLines(lipgloss.NewStyle().Foreground(subtle), text)
}

// Warnf renders a warning/error line (e.g. stderr output) in a color that
// stands out from normal text, so it isn't missed among plain prompt output.
func Warnf(text string) string {
	return renderLines(lipgloss.NewStyle().Foreground(warn), text)
}

// renderLines styles each line of text separately. lipgloss's Render pads
// every line of a multi-line string to the longest one, which turns a
// long error followed by a newline into a screen-wide run of spaces.
func renderLines(st lipgloss.Style, text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = st.Render(l)
		}
	}
	return strings.Join(lines, "\n")
}
