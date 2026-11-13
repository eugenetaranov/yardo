package ui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The choices are one checklist: a toggle is a row of its own, a multi a
// heading with a box per option, a one or a pick a heading with a radio
// per option (a one's first is None). Space ticks, enter is done.

// ckRow is a checklist line: a heading (q's, not selectable) or a row
// that ticks key in q.
type ckRow struct {
	q    *Question
	key  string
	head bool
}

type checklist struct {
	qs            []*Question
	rows          []ckRow
	sel, top      int
	width, height int
	quit, out     bool
}

func newChecklist(qs []*Question) *checklist {
	c := &checklist{qs: qs, width: 80, height: 24}
	for _, q := range qs {
		switch q.Kind {
		case "toggle":
			c.rows = append(c.rows, ckRow{q: q, key: "on"})
			continue
		}
		c.rows = append(c.rows, ckRow{q: q, head: true})
		if q.Kind == "one" {
			c.rows = append(c.rows, ckRow{q: q, key: ""})
		}
		for _, o := range q.Options {
			c.rows = append(c.rows, ckRow{q: q, key: o[0]})
		}
	}
	c.sel = c.step(-1, 1)
	return c
}

// step is the next selectable row from i, going by d; i itself if none.
func (c *checklist) step(i, d int) int {
	for j := i + d; j >= 0 && j < len(c.rows); j += d {
		if !c.rows[j].head {
			return j
		}
	}
	return max(i, 0)
}

// on reports whether r is ticked.
func (c *checklist) on(r ckRow) bool {
	if r.key == "" {
		return len(r.q.Picked) == 0
	}
	return slices.Contains(r.q.Picked, r.key)
}

// tick ticks r: a toggle or multi's box flips, a radio is picked.
func (c *checklist) tick(r ckRow) {
	q := r.q
	switch q.Kind {
	case "toggle":
		if len(q.Picked) > 0 {
			q.Picked = nil
		} else {
			q.Picked = []string{"on"}
		}
	case "multi":
		if slices.Contains(q.Picked, r.key) {
			q.Picked = slices.DeleteFunc(q.Picked, func(k string) bool { return k == r.key })
			return
		}
		// In the options' order.
		var picked []string
		for _, o := range q.Options {
			if o[0] == r.key || slices.Contains(q.Picked, o[0]) {
				picked = append(picked, o[0])
			}
		}
		q.Picked = picked
	default:
		q.Picked = nil
		if r.key != "" {
			q.Picked = []string{r.key}
		}
	}
}

func (c *checklist) Init() tea.Cmd { return nil }

func (c *checklist) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width, c.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			c.quit = true
			return c, tea.Quit
		case "enter":
			return c, tea.Quit
		case "up", "k", "shift+tab":
			c.sel = c.step(c.sel, -1)
		case "down", "j", "tab":
			c.sel = c.step(c.sel, 1)
		case "home", "g":
			c.sel = c.step(-1, 1)
		case "end", "G":
			c.sel = c.step(len(c.rows), -1)
		case " ", "x":
			if c.sel < len(c.rows) {
				c.tick(c.rows[c.sel])
			}
		}
	}
	return c, nil
}

// lines are the checklist's lines, and which is the selected row's.
func (c *checklist) lines(w int) ([]string, int) {
	col := 8
	for _, r := range c.rows {
		if r.q.Kind == "toggle" {
			col = max(col, runewidth.StringWidth(r.q.Title))
		}
	}
	col = min(col, 26)
	var out []string
	cur := 0
	for n, r := range c.rows {
		prev := n > 0 && c.rows[n-1].q.Kind != "toggle"
		if r.head || (r.q.Kind == "toggle" && prev) {
			out = append(out, "")
		}
		if r.head {
			l := "  " + bold.Render(tui.Title(r.q.Title))
			if r.q.Note != "" {
				l += "  " + muted.Render(runewidth.Truncate(r.q.Note, max(10, w-runewidth.StringWidth(r.q.Title)-6), "…"))
			}
			out = append(out, l)
			continue
		}
		box := "[ ]"
		if c.on(r) {
			box = "[x]"
		}
		if r.q.Kind != "toggle" && r.q.Kind != "multi" {
			box = "( )"
			if c.on(r) {
				box = "(•)"
			}
		}
		label, note, indent := r.q.Title, r.q.Note, ""
		if r.q.Kind != "toggle" {
			label, note, indent = "None", "", "   "
			for _, o := range r.q.Options {
				if o[0] == r.key {
					label = o[1]
				}
			}
		}
		name := box + " " + label
		if r.q.Kind == "toggle" {
			name = box + " " + padTo(label, col)
		}
		name = runewidth.Truncate(name, max(10, w-10), "…")
		l := "  " + indent + "   " + bold.Render(name) + " "
		if !c.on(r) {
			l = "  " + indent + "   " + muted.Render(name) + " "
		}
		if n == c.sel {
			cur = len(out)
			l = "  " + indent + rowOn.Render(" › "+name+" ")
		}
		if note != "" {
			l += "  " + muted.Render(runewidth.Truncate(note, max(10, w-col-16), "…"))
		}
		out = append(out, l)
	}
	return out, cur
}

func (c *checklist) View() string {
	if c.out {
		return ""
	}
	w, h := max(50, c.width), max(12, c.height)
	head := []string{"", "  " + accentBd.Render(tui.Cur.Brand) + muted.Render("  ·  ") + bold.Render(tui.Title("Choices")),
		"  " + muted.Render("Tick what this machine should have. Nothing changes until you confirm.")}
	body, cur := c.lines(w)
	room := max(3, h-len(head)-3)
	if cur < c.top+1 {
		c.top = max(0, cur-2) // its heading, above it, shows too
	}
	if cur >= c.top+room {
		c.top = cur - room + 1
	}
	c.top = min(c.top, max(0, len(body)-room))
	body = body[c.top:min(len(body), c.top+room)]
	for len(body) < room {
		body = append(body, "")
	}
	k := func(key, what string) string { return keyOn.Render(key) + " " + what }
	keys := strings.Join([]string{k("↑↓", "choose"), k("space", "tick"), k("enter", "done"), k("esc", "cancel")}, "   ")
	return strings.Join(append(append(head, body...), "", "  "+keys), "\n")
}
