package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// HomeItem is one picked item on the home screen, with its status.
type HomeItem struct {
	ID, Label string
	// Group is where it's listed when it's fine (a picker tab or group).
	Group string
	// State is ok, down, partial, missing, secret or unchecked.
	State, Detail string
	// Inputs are the values left empty; Source how it's installed.
	Inputs []string
	Source string
}

// HomeAction is what the home screen was left with.
type HomeAction int

const (
	HomeQuit HomeAction = iota
	// HomeChange opens the picker, to add or change items.
	HomeChange
	// HomeRefresh checks everything again.
	HomeRefresh
	// HomeConnect shows how to reach this machine from a laptop.
	HomeConnect
	// HomeFix re-applies what needs fixing (Fix lists it).
	HomeFix
	// HomeEdit opens the selected item's values to fill in (Selected).
	HomeEdit
)

var (
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
)

// glyph is a state's mark and how it's said.
func glyph(state string) (string, string) {
	switch state {
	case "ok":
		return green.Render("●"), "ok"
	case "down":
		return red.Render("✖"), "down"
	case "partial":
		return yellow.Render("◐"), "partial"
	case "drifted":
		return yellow.Render("◆"), "drifted"
	case "missing":
		return red.Render("○"), "missing"
	case "secret":
		return accentBd.Render("✱"), "to fill in"
	}
	return muted.Render("·"), "not checked"
}

// problem reports whether an item belongs under "Needs attention".
func problem(state string) bool { return state != "ok" && state != "unchecked" }

type homeRow struct {
	head string    // a heading row, or
	item *HomeItem // an item row
}

// Home is the screen `sudo yardo` opens on once something is set up: what's
// installed and whether it's fine, problems first. It changes nothing; it
// hands the picker (a), a re-check (r) or quitting back to its caller.
type Home struct {
	title, sub string
	rows       []homeRow
	counts     map[string]int
	avail      int
	cursor     int // index into rows (an item row)
	top        int
	width      int
	height     int
	details    bool
	action     HomeAction
	out        bool
}

// NewHome builds the home screen: title and sub on the first line, items
// in groups (in the order of groups), avail items not installed yet.
func NewHome(title, sub string, items []*HomeItem, groups []string, avail int) *Home {
	h := &Home{title: title, sub: sub, counts: map[string]int{}, avail: avail, width: 100, height: 30}
	var attention []*HomeItem
	byGroup := map[string][]*HomeItem{}
	for _, it := range items {
		h.counts[it.State]++
		if problem(it.State) {
			attention = append(attention, it)
		} else {
			byGroup[it.Group] = append(byGroup[it.Group], it)
		}
	}
	if len(attention) > 0 {
		h.rows = append(h.rows, homeRow{head: "Needs attention"})
		for _, it := range attention {
			h.rows = append(h.rows, homeRow{item: it})
		}
	}
	for _, g := range groups {
		if len(byGroup[g]) == 0 {
			continue
		}
		h.rows = append(h.rows, homeRow{head: g})
		for _, it := range byGroup[g] {
			h.rows = append(h.rows, homeRow{item: it})
		}
	}
	h.cursor = h.next(-1, 1)
	return h
}

// next is the item row after (dir 1) or before (dir -1) i; i when none.
func (h *Home) next(i, dir int) int {
	for j := i + dir; j >= 0 && j < len(h.rows); j += dir {
		if h.rows[j].item != nil {
			return j
		}
	}
	if i < 0 {
		return 0
	}
	return i
}

func (h *Home) Init() tea.Cmd { return nil }

func (h *Home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = m.Width, m.Height
	case tea.KeyMsg:
		switch m.String() {
		case "up", "k":
			h.cursor = h.next(h.cursor, -1)
		case "down", "j":
			h.cursor = h.next(h.cursor, 1)
		case "pgdown":
			for i := 0; i < 10; i++ {
				h.cursor = h.next(h.cursor, 1)
			}
		case "pgup":
			for i := 0; i < 10; i++ {
				h.cursor = h.next(h.cursor, -1)
			}
		case "home":
			h.cursor = h.next(-1, 1)
		case "end":
			h.cursor = h.next(len(h.rows), -1)
		case "enter":
			h.details = !h.details
		case "a", "i", "c":
			h.action = HomeChange
			return h, tea.Quit
		case "r":
			h.action = HomeRefresh
			return h, tea.Quit
		case "x":
			h.action = HomeConnect
			return h, tea.Quit
		case "f":
			if len(h.Fix()) > 0 {
				h.action = HomeFix
				return h, tea.Quit
			}
		case "e":
			if it := h.Selected(); it != nil && it.State == "secret" {
				h.action = HomeEdit
				return h, tea.Quit
			}
		case "q", "esc", "ctrl+c":
			h.action = HomeQuit
			return h, tea.Quit
		}
	}
	return h, nil
}

// Fixable reports whether f re-applies an item in this state.
func Fixable(state string) bool {
	return state == "down" || state == "partial" || state == "missing" || state == "drifted"
}

// Fix lists the items f re-applies.
func (h *Home) Fix() []string {
	var out []string
	for _, r := range h.rows {
		if r.item != nil && Fixable(r.item.State) {
			out = append(out, r.item.ID)
		}
	}
	return out
}

// Selected is the item under the cursor.
func (h *Home) Selected() *HomeItem {
	if h.cursor < len(h.rows) {
		return h.rows[h.cursor].item
	}
	return nil
}

// detail is what the details panel shows for it.
func detail(it *HomeItem) []string {
	_, say := glyph(it.State)
	var out []string
	switch {
	case it.State == "secret":
		out = append(out, "To fill in: "+strings.Join(it.Inputs, ", "))
	case it.Detail != "" && it.State != "ok":
		out = append(out, say+": "+it.Detail)
	default:
		out = append(out, say)
	}
	if it.Source != "" {
		out = append(out, "How: "+it.Source)
	}
	return out
}

func (h *Home) View() string {
	if h.out {
		return ""
	}
	w := h.width
	var b strings.Builder
	gap := max(1, w-3-runewidth.StringWidth(h.title)-runewidth.StringWidth(h.sub))
	b.WriteString(" " + accentBd.Render(h.title) + strings.Repeat(" ", gap) + muted.Render(h.sub) + "\n")
	var sum []string
	for _, s := range []string{"ok", "down", "drifted", "partial", "missing", "secret", "unchecked"} {
		if n := h.counts[s]; n > 0 {
			g, say := glyph(s)
			sum = append(sum, fmt.Sprintf("%s %d %s", g, n, say))
		}
	}
	if h.avail > 0 {
		sum = append(sum, muted.Render(fmt.Sprintf("+ %d more to add", h.avail)))
	}
	b.WriteString(" " + strings.Join(sum, "   ") + "\n")
	b.WriteString(muted.Render(strings.Repeat("─", max(1, w-1))) + "\n")

	var det []string
	if h.cursor < len(h.rows) && h.rows[h.cursor].item != nil {
		it := h.rows[h.cursor].item
		if h.details {
			for _, l := range detail(it) {
				det = append(det, wrap(l, max(4, w-6))...)
			}
		} else {
			det = []string{runewidth.Truncate(detail(it)[0], max(4, w-6), "…")}
		}
	}
	listH := max(1, h.height-5-len(det))
	if h.cursor < h.top {
		h.top = h.cursor
	}
	// Keep the heading above the cursor's first item in view.
	for h.cursor-h.top >= listH {
		h.top++
	}
	if h.top > 0 && h.cursor == h.top && h.rows[h.top-1].item == nil {
		h.top--
	}
	// As wide as the longest label, so the notes get the rest.
	labelW := 14
	for _, r := range h.rows {
		if r.item != nil {
			labelW = max(labelW, runewidth.StringWidth(r.item.Label))
		}
	}
	labelW = min(labelW, 44, max(14, w/2))
	lines := 0
	for i := h.top; i < len(h.rows) && lines < listH; i++ {
		r := h.rows[i]
		if r.item == nil {
			b.WriteString(" " + accentBd.Render(r.head) + "\n")
			lines++
			continue
		}
		it := r.item
		g, _ := glyph(it.State)
		mark := "  "
		lab := runewidth.Truncate(it.Label, labelW, "…")
		if i == h.cursor {
			mark = accentBd.Render("›") + " "
			lab = bold.Render(lab)
		}
		pad := max(1, labelW+2-runewidth.StringWidth(runewidth.Truncate(it.Label, labelW, "…")))
		note := ""
		if problem(it.State) {
			n := it.Detail
			if it.State == "secret" {
				n = strings.Join(it.Inputs, ", ")
			}
			note = muted.Render(runewidth.Truncate(n, max(4, w-labelW-10), "…"))
		}
		b.WriteString(" " + mark + g + " " + lab + strings.Repeat(" ", pad) + note + "\n")
		lines++
	}
	for ; lines < listH; lines++ {
		b.WriteString("\n")
	}
	for _, l := range det {
		b.WriteString("   " + muted.Render(l) + "\n")
	}
	keys := "↑/↓ move · enter details"
	if it := h.Selected(); it != nil && it.State == "secret" {
		keys += " · e fill in"
	}
	if n := len(h.Fix()); n > 0 {
		keys += fmt.Sprintf(" · f fix %d", n)
	}
	keys += " · a add or change · x connect from a laptop · r check again · q quit"
	b.WriteString(" " + muted.Render(runewidth.Truncate(keys, max(4, w-2), "…")))
	return b.String()
}

// Run shows the home screen; it returns what it was left with.
func (h *Home) Run() (HomeAction, error) {
	h.out = false
	if _, err := tea.NewProgram(h, tea.WithAltScreen()).Run(); err != nil {
		return HomeQuit, err
	}
	h.out = true
	return h.action, nil
}
