package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// HomeItem is one item on the home screen: picked (with its status) or
// available to add.
type HomeItem struct {
	ID, Label string
	// Group is its tab's group or tab (Dev, DevOps, AI, …, Configure); Tab
	// the picker tab that has it (to open the picker on).
	Group, Tab string
	// State is ok, down, drifted, partial, missing, secret or unchecked for
	// a picked item, "available" for one to add.
	State, Detail string
	// Inputs are the values left empty; Updates newer versions of its
	// tools (when checked); Source how it's installed.
	Inputs, Updates []string
	Source          string
}

// HomeInfo is what the home screen's header says.
type HomeInfo struct{ Host, User, Checked string }

// HomeAction is what the home screen was left with.
type HomeAction int

const (
	HomeQuit HomeAction = iota
	// HomeChange opens the picker, to add or change items (StartTab).
	HomeChange
	// HomeRefresh checks everything again.
	HomeRefresh
	// HomeConnect shows how to reach this machine from a laptop.
	HomeConnect
	// HomeFix re-applies what needs fixing (Fix lists it).
	HomeFix
	// HomeEdit opens the selected item's values to fill in (Selected).
	HomeEdit
	// HomeUpdates checks again, for newer versions too.
	HomeUpdates
	// HomeAccess changes who can reach the gateway, Docker and its ports.
	HomeAccess
)

var (
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	keyOn  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	rowOn  = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(lipgloss.Color("6")).Bold(true)
	border = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

// stateStyle colours a row by its state, as k9s does.
func stateStyle(state string) lipgloss.Style {
	switch state {
	case "ok":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	case "down", "missing":
		return red
	case "partial", "drifted":
		return yellow
	case "secret":
		return accent
	case "available":
		return lipgloss.NewStyle()
	}
	return muted
}

// stateSay is how a state reads in the STATE column.
func stateSay(state string) string {
	if state == "secret" {
		return "fill in"
	}
	return state
}

// severity sorts the installed view: problems first.
var severity = map[string]int{"down": 0, "drifted": 1, "partial": 2, "missing": 3, "secret": 4, "unchecked": 5, "ok": 6}

// problem reports whether an item belongs in the attention view.
func problem(state string) bool {
	return state != "ok" && state != "unchecked" && state != "available"
}

// Fixable reports whether f re-applies an item in this state.
func Fixable(state string) bool {
	return state == "down" || state == "partial" || state == "missing" || state == "drifted"
}

// The views: what's installed, what needs attention, what can be added.
const (
	viewInstalled = iota
	viewAttention
	viewAvailable
)

var viewNames = []string{"installed", "attention", "available"}

// Home is the screen `sudo yardo` opens on once something is set up, laid
// out like k9s: a header (the machine, the keys), a table of items (1
// installed, 2 needing attention, 3 available to add; / filters), and a
// line of views. It changes nothing itself: it hands its caller an action.
type Home struct {
	info      HomeInfo
	installed []*HomeItem
	available []*HomeItem
	view      int
	filter    string
	filtering bool
	cursor    map[int]int
	top       map[int]int
	width     int
	height    int
	details   bool
	action    HomeAction
	out       bool
	counts    map[string]int
}

// NewHome builds the home screen from the picked items (with their status)
// and those available to add. It opens on what needs attention, if
// anything does.
func NewHome(info HomeInfo, installed, available []*HomeItem) *Home {
	h := &Home{info: info, installed: installed, available: available, counts: map[string]int{},
		cursor: map[int]int{}, top: map[int]int{}, width: 100, height: 30}
	sort.SliceStable(h.installed, func(i, j int) bool {
		return severity[h.installed[i].State] < severity[h.installed[j].State]
	})
	for _, it := range installed {
		h.counts[it.State]++
	}
	if len(h.rows(viewAttention)) > 0 {
		h.view = viewAttention
	}
	return h
}

// rows are view's items, filtered.
func (h *Home) rows(view int) []*HomeItem {
	var src []*HomeItem
	switch view {
	case viewAvailable:
		src = h.available
	case viewAttention:
		for _, it := range h.installed {
			if problem(it.State) {
				src = append(src, it)
			}
		}
	default:
		src = h.installed
	}
	if h.filter == "" {
		return src
	}
	q := strings.ToLower(h.filter)
	var out []*HomeItem
	for _, it := range src {
		if strings.Contains(strings.ToLower(it.Label+" "+it.Group+" "+it.ID+" "+it.State), q) {
			out = append(out, it)
		}
	}
	return out
}

// Fix lists the items f re-applies.
func (h *Home) Fix() []string {
	var out []string
	for _, it := range h.installed {
		if Fixable(it.State) {
			out = append(out, it.ID)
		}
	}
	return out
}

// Selected is the item under the cursor.
func (h *Home) Selected() *HomeItem {
	r := h.rows(h.view)
	if c := h.cursor[h.view]; c < len(r) {
		return r[c]
	}
	return nil
}

// StartTab is the picker tab to open on: the selected item's.
func (h *Home) StartTab() string {
	if it := h.Selected(); it != nil {
		return it.Tab
	}
	return ""
}

func (h *Home) Init() tea.Cmd { return nil }

func (h *Home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = m.Width, m.Height
	case tea.KeyMsg:
		if h.filtering {
			return h, h.filterKey(m)
		}
		return h, h.key(m)
	}
	return h, nil
}

// filterKey edits the filter (/ … enter keeps it, esc drops it).
func (h *Home) filterKey(m tea.KeyMsg) tea.Cmd {
	switch m.Type {
	case tea.KeyEnter:
		h.filtering = false
	case tea.KeyEsc:
		h.filtering, h.filter = false, ""
	case tea.KeyBackspace:
		if r := []rune(h.filter); len(r) > 0 {
			h.filter = string(r[:len(r)-1])
		}
	case tea.KeyCtrlC:
		h.action = HomeQuit
		return tea.Quit
	case tea.KeySpace:
		h.filter += " "
	case tea.KeyRunes:
		h.filter += string(m.Runes)
	}
	h.cursor[h.view], h.top[h.view] = 0, 0
	return nil
}

func (h *Home) key(m tea.KeyMsg) tea.Cmd {
	n := len(h.rows(h.view))
	move := func(d int) { h.cursor[h.view] = max(0, min(n-1, h.cursor[h.view]+d)) }
	quit := func(a HomeAction) tea.Cmd { h.action = a; return tea.Quit }
	switch m.String() {
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "pgup", "ctrl+b":
		move(-10)
	case "pgdown", "ctrl+f":
		move(10)
	case "home", "g":
		h.cursor[h.view] = 0
	case "end", "G":
		move(n)
	case "1", "2", "3":
		h.view, h.details = int(m.Runes[0]-'1'), false
	case "tab", "right", "l":
		h.view, h.details = (h.view+1)%3, false
	case "shift+tab", "left", "h":
		h.view, h.details = (h.view+2)%3, false
	case "/":
		h.filtering = true
	case "esc":
		switch {
		case h.filter != "":
			h.filter = ""
		case h.details:
			h.details = false
		default:
			return quit(HomeQuit)
		}
	case "enter":
		if it := h.Selected(); it != nil && it.State == "available" {
			return quit(HomeChange)
		}
		h.details = !h.details
	case "a", "i":
		return quit(HomeChange)
	case "c":
		return quit(HomeAccess)
	case "f":
		if len(h.Fix()) > 0 {
			return quit(HomeFix)
		}
	case "e":
		if it := h.Selected(); it != nil && it.State == "secret" {
			return quit(HomeEdit)
		}
	case "x":
		return quit(HomeConnect)
	case "u":
		return quit(HomeUpdates)
	case "r":
		return quit(HomeRefresh)
	case "q", "ctrl+c":
		return quit(HomeQuit)
	}
	return nil
}

// note is what the NOTE column says for it.
func note(it *HomeItem) string {
	switch {
	case it.State == "available":
		return it.Source
	case it.State == "secret":
		return strings.Join(it.Inputs, ", ")
	case problem(it.State) || it.State == "unchecked":
		return it.Detail
	case len(it.Updates) > 0:
		return "▲ " + strings.Join(it.Updates, ", ")
	}
	return ""
}

// detail is the details pane for it.
func detail(it *HomeItem) []string {
	out := []string{"Item: " + it.Label + "  (" + it.ID + ")", "State: " + stateSay(it.State)}
	switch {
	case it.State == "available":
		out[1] = "State: not installed; enter opens the picker on " + it.Group
	case it.State == "secret":
		out = append(out, "Fill in: "+strings.Join(it.Inputs, ", ")+"  (e)")
	case it.Detail != "" && it.State != "ok":
		out = append(out, "Why: "+it.Detail)
	}
	if len(it.Updates) > 0 {
		out = append(out, "Newer: "+strings.Join(it.Updates, ", ")+"  (mise up, or sudo apt upgrade)")
	}
	if it.Source != "" && it.State != "available" {
		out = append(out, "How: "+it.Source)
	}
	return out
}

// header is the top: the machine on the left, the keys on the right.
func (h *Home) header(w int) []string {
	var st []string
	for _, s := range []string{"ok", "down", "drifted", "partial", "missing", "secret", "unchecked"} {
		if n := h.counts[s]; n > 0 {
			st = append(st, stateStyle(s).Render(fmt.Sprintf("%d %s", n, stateSay(s))))
		}
	}
	left := []string{
		accentBd.Render("Host:    ") + h.info.Host,
		accentBd.Render("User:    ") + h.info.User,
		accentBd.Render("Checked: ") + h.info.Checked,
		accentBd.Render("Status:  ") + strings.Join(st, "  "),
	}
	k := func(key, what string) string { return keyOn.Render("<"+key+">") + " " + what }
	fix := k("f", "fix")
	if n := len(h.Fix()); n > 0 {
		fix = k("f", fmt.Sprintf("fix %d", n))
	}
	cols := [][]string{
		{fix, k("e", "fill in"), k("enter", "details"), k("1-3", "views")},
		{k("a", "add/change"), k("c", "access"), k("/", "filter"), k("q", "quit")},
		{k("x", "connect"), k("u", "updates"), k("r", "refresh"), ""},
	}
	leftW := 0
	for _, l := range left {
		leftW = max(leftW, lipgloss.Width(l))
	}
	const colW = 18
	out := make([]string, len(left))
	for i := range out {
		line := " " + left[i] + strings.Repeat(" ", leftW-lipgloss.Width(left[i])+4)
		for _, c := range cols {
			if lipgloss.Width(line)+colW > w {
				break
			}
			line += c[i] + strings.Repeat(" ", max(1, colW-lipgloss.Width(c[i])))
		}
		out[i] = line
	}
	return out
}

// pad cuts or pads s to w cells.
func pad(s string, w int) string {
	s = runewidth.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(0, w-runewidth.StringWidth(s)))
}

func (h *Home) View() string {
	if h.out {
		return ""
	}
	w, ht := max(40, h.width), max(12, h.height)
	var b strings.Builder
	head := h.header(w)
	for _, l := range head {
		b.WriteString(l + "\n")
	}

	rows := h.rows(h.view)
	title := fmt.Sprintf(" %s[%d] ", viewNames[h.view], len(rows))
	if h.filter != "" || h.filtering {
		title = fmt.Sprintf(" %s(/%s)[%d] ", viewNames[h.view], h.filter, len(rows))
	}
	inner := w - 2
	tw := runewidth.StringWidth(title)
	tl := max(0, (inner-tw)/2)
	b.WriteString(border.Render("┌"+strings.Repeat("─", tl)) + accentBd.Render(title) +
		border.Render(strings.Repeat("─", max(0, inner-tl-tw))+"┐") + "\n")

	// Columns: NAME, GROUP, STATE, NOTE (the rest).
	nameW := min(44, max(16, inner*2/5))
	groupW, stateW := 12, 10
	noteW := max(4, inner-nameW-groupW-stateW-5)
	line := func(s string) { b.WriteString(border.Render("│") + s + border.Render("│") + "\n") }
	line(bold.Render(" " + pad("NAME", nameW) + " " + pad("GROUP", groupW) + " " + pad("STATE", stateW) + " " + pad("NOTE", noteW) + " "))

	var det []string
	if it := h.Selected(); h.details && it != nil {
		for _, l := range detail(it) {
			det = append(det, wrap(l, inner-4)...)
		}
	}
	// Header, the box's top, the column names, its bottom, the views; and
	// the details with their rule.
	listH := max(1, ht-len(head)-4-len(det))
	if len(det) > 0 {
		listH--
	}
	cur := h.cursor[h.view]
	if cur >= len(rows) {
		cur = max(0, len(rows)-1)
		h.cursor[h.view] = cur
	}
	top := h.top[h.view]
	if cur < top {
		top = cur
	}
	if cur >= top+listH {
		top = cur - listH + 1
	}
	h.top[h.view] = top
	shown := 0
	for i := top; i < len(rows) && shown < listH; i++ {
		it := rows[i]
		cells := " " + pad(it.Label, nameW) + " " + pad(it.Group, groupW) + " " + pad(stateSay(it.State), stateW) + " " + pad(note(it), noteW) + " "
		if i == cur {
			line(rowOn.Render(cells))
		} else {
			line(stateStyle(it.State).Render(cells))
		}
		shown++
	}
	if len(rows) == 0 {
		msg := map[int]string{viewInstalled: "Nothing picked yet: a adds.", viewAttention: "Nothing needs attention.", viewAvailable: "Everything is picked."}[h.view]
		if h.filter != "" {
			msg = "Nothing matches the filter: esc clears it."
		}
		line(muted.Render(" " + pad(msg, inner-2) + " "))
		shown++
	}
	for ; shown < listH; shown++ {
		line(strings.Repeat(" ", inner))
	}
	if len(det) > 0 {
		b.WriteString(border.Render("├"+strings.Repeat("─", inner)+"┤") + "\n")
		for _, l := range det {
			line(" " + pad(l, inner-2) + " ")
		}
	}
	b.WriteString(border.Render("└"+strings.Repeat("─", inner)+"┘") + "\n")

	// The views, as k9s's crumbs; the filter being typed.
	attention := 0
	for _, it := range h.installed {
		if problem(it.State) {
			attention++
		}
	}
	var crumbs []string
	for i, n := range viewNames {
		c := fmt.Sprintf(" <%d> %s ", i+1, n)
		switch i {
		case viewAttention:
			c = fmt.Sprintf(" <%d> %s (%d) ", i+1, n, attention)
		case viewAvailable:
			c = fmt.Sprintf(" <%d> %s (%d) ", i+1, n, len(h.available))
		}
		if i == h.view {
			crumbs = append(crumbs, tabOn.Render(c))
		} else {
			crumbs = append(crumbs, muted.Render(c))
		}
	}
	foot := " " + strings.Join(crumbs, " ")
	if h.filtering {
		foot += "   " + accentBd.Render("/") + h.filter + "_"
	}
	b.WriteString(foot)
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
