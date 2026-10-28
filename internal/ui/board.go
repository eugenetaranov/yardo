package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The board is yardo's one screen: the picker's tabs and checklist, where
// a ticked box means picked (installed) and a mark beside it its health.
// Space queues adding or removing an item, enter acts on the row (fix it,
// fill in its value, change a setting, show how to connect), a applies the
// queue. A "Needs you" tab comes first while something does; "This VM"
// holds the machine's settings and actions; "Review" lists the queue.

// HomeInfo is what the board's header says.
type HomeInfo struct{ Host, User, Checked string }

var (
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	keyOn  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	rowOn  = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(tui.Accent).Bold(true)
)

// SetAccent colours yardo's screens (and its prompts) with hex: the
// user's accent colour.
func SetAccent(hex string) {
	c := lipgloss.Color(hex)
	tui.Accent = c
	accent = lipgloss.NewStyle().Foreground(c)
	accentBd = accent.Bold(true)
	tabOn = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1A1A1A")).Background(c)
	rowOn = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(c).Bold(true)
}

// Fixable reports whether fixing (applying it again) is what a state
// needs.
func Fixable(state string) bool {
	return state == "down" || state == "partial" || state == "missing" || state == "drifted"
}

// BoardItem is one catalog item (or extra mise tool) on the board.
type BoardItem struct {
	ID, Label string
	// Tab is the section it's listed in (a catalog tab id, or ExtraTab).
	Tab string
	// Picked: it's among the saved picks (installed). Pending is "add" or
	// "remove" when a change to it is queued.
	Picked  bool
	Pending string
	// State is its status when picked (ok, down, drifted, partial,
	// missing, secret, unchecked; "" when not checked yet).
	State, Detail string
	// Inputs are the values left empty; Updates newer versions of its
	// tools; Source how it's installed.
	Inputs, Updates []string
	Source          string
	// Changes are the answers enter can change (label and key), such as
	// the git identity: a picked item's questions, asked again.
	Changes [][2]string
}

// BoardSetting is one of This VM's settings: who can reach a service, or
// how things look.
type BoardSetting struct {
	Key, Label string
	// Section is This VM's section it's listed in ("": who can reach it).
	Section string
	// Choices are value and label pairs, in order.
	Choices [][2]string
	// Value is the saved choice; Pending the one queued ("" for none).
	Value, Pending string
	// Swatches are a colour per choice, shown beside it (the accent).
	Swatches map[string]string
}

func (s *BoardSetting) label(v string) string {
	for _, c := range s.Choices {
		if c[0] == v {
			return c[1]
		}
	}
	return v
}

// BoardAction is a row that does something (This VM): connect, check
// for updates, check again.
type BoardAction struct{ ID, Label, Note string }

// BoardKind is what the board was left with.
type BoardKind int

const (
	BoardQuit BoardKind = iota
	// BoardApply applies the queue (Result's Adds, Removes, Settings).
	BoardApply
	// BoardFix re-applies Result.ID now.
	BoardFix
	// BoardEdit opens Result.ID's values to fill in.
	BoardEdit
	// BoardChange asks Result.ID's Result.Change question again, then
	// re-applies it.
	BoardChange
	// BoardRefresh checks everything again; BoardUpdates checks for newer
	// versions too.
	BoardRefresh
	BoardUpdates
)

// BoardResult is what the board was left with, and the queue.
type BoardResult struct {
	Kind BoardKind
	ID   string
	// Inputs are BoardEdit's item's values to fill in.
	Inputs []string
	// Change is BoardChange's answer to change (a BoardItem.Changes key).
	Change        string
	Adds, Removes []string
	// Settings are the queued settings, key to value.
	Settings map[string]string
}

// Board is the screen; see NewBoard.
type Board struct {
	info     HomeInfo
	first    bool
	tabs     []Tab
	items    []*BoardItem
	settings []*BoardSetting
	actions  []BoardAction
	connect  []string

	tab       int
	cursor    map[string]int
	search    *string
	panel     *panel
	flash     string
	width     int
	height    int
	result    BoardResult
	out       bool
	quitAsked bool
}

// panel is the box under the list for a row's choices or text.
type panel struct {
	title string
	lines []string
	// opts are the choices (label, value); sel the selected one; swatches
	// a colour per value, shown beside its choice.
	opts     [][2]string
	sel      int
	swatches map[string]string
	// on is what a choice does.
	on func(value string) tea.Cmd
}

// Section ids the board adds to This VM.
const (
	accessSection  = "__access__"
	machineSection = "__machine__"
	needsTab       = "__needs__"
	boardReview    = "__review__"
)

// NewBoard builds the board: tabs (each with sections, or none), the
// items, This VM's settings and actions, and connect (how to reach the
// machine from a laptop, shown by its action). first: nothing is
// installed yet (no marks; the defaults come queued).
func NewBoard(info HomeInfo, first bool, tabs []Tab, items []*BoardItem, settings []*BoardSetting, actions []BoardAction, connect []string) *Board {
	b := &Board{info: info, first: first, items: items, settings: settings, actions: actions, connect: connect,
		cursor: map[string]int{}, width: 100, height: 30}
	if slices.ContainsFunc(items, func(i *BoardItem) bool { return i.Tab == ExtraTab }) {
		tabs = append(slices.Clone(tabs), Tab{ID: ExtraTab, Title: "Extra"})
	}
	b.tabs = tabs
	return b
}

// visibleTabs are the tabs as shown: Needs you first while something
// does, then the board's, then Review.
func (b *Board) visibleTabs() []Tab {
	var out []Tab
	if len(b.needs()) > 0 {
		out = append(out, Tab{ID: needsTab, Title: "Needs you"})
	}
	out = append(out, b.tabs...)
	return append(out, Tab{ID: boardReview, Title: "Review"})
}

func fixable(i *BoardItem) bool { return i.Picked && Fixable(i.State) }
func secret(i *BoardItem) bool  { return i.Picked && i.State == "secret" }
func newer(i *BoardItem) bool   { return i.Picked && len(i.Updates) > 0 }

// needs are the items Needs you lists.
func (b *Board) needs() []*BoardItem {
	var out []*BoardItem
	for _, i := range b.items {
		if fixable(i) || secret(i) || newer(i) {
			out = append(out, i)
		}
	}
	return out
}

func (b *Board) count(f func(*BoardItem) bool) int {
	n := 0
	for _, i := range b.items {
		if f(i) {
			n++
		}
	}
	return n
}

// queued counts the queue: items and settings.
func (b *Board) queued() int {
	n := b.count(func(i *BoardItem) bool { return i.Pending != "" })
	for _, s := range b.settings {
		if s.Pending != "" {
			n++
		}
	}
	return n
}

// row is one line of the list: a heading, or something to act on.
type row struct {
	head    string
	text    string
	item    *BoardItem
	setting *BoardSetting
	action  *BoardAction
	where   string // search results: the item's tab
}

func (r row) selectable() bool { return r.head == "" && r.text == "" }

// sectionsOf are a tab's sections: its own, or itself.
func sectionsOf(t Tab) []Tab {
	if len(t.Sections) > 0 {
		return t.Sections
	}
	return []Tab{t}
}

func (b *Board) tabTitle(section string) string {
	for _, t := range b.tabs {
		for _, s := range sectionsOf(t) {
			if s.ID == section {
				return t.Title
			}
		}
	}
	return ""
}

// rows are what the current tab (or the search) lists.
func (b *Board) rows() []row {
	var out []row
	if b.search != nil {
		q := strings.ToLower(*b.search)
		if q == "" {
			return []row{{text: "Type a name: wire, vpn, docker, python…"}}
		}
		for _, i := range b.items {
			if strings.Contains(strings.ToLower(i.Label+" "+i.ID), q) {
				out = append(out, row{item: i, where: b.tabTitle(i.Tab)})
			}
		}
		if len(out) == 0 {
			msg := "No match."
			if ValidExtra(*b.search) {
				msg = fmt.Sprintf("No match: ctrl-a adds %q as an extra mise tool (mise use -g %s).", *b.search, *b.search)
			}
			out = append(out, row{text: msg})
		}
		return out
	}
	t := b.visibleTabs()[b.tab]
	switch t.ID {
	case needsTab:
		add := func(head string, f func(*BoardItem) bool) {
			first := true
			for _, i := range b.items {
				if f(i) {
					if first {
						out = append(out, row{head: head})
						first = false
					}
					out = append(out, row{item: i})
				}
			}
		}
		add("Broken", fixable)
		add("Needs a value", secret)
		add("Updates", newer)
		return out
	case boardReview:
		for _, i := range b.items {
			if i.Pending != "" {
				out = append(out, row{item: i})
			}
		}
		for _, s := range b.settings {
			if s.Pending != "" {
				out = append(out, row{setting: s})
			}
		}
		if len(out) == 0 {
			out = append(out, row{text: "Nothing queued: space on a row adds or removes it, enter on a setting changes it."})
		}
		return out
	}
	heads := len(t.Sections) > 0
	for _, s := range sectionsOf(t) {
		var rs []row
		for _, st := range b.settings {
			if st.Section == s.ID || (st.Section == "" && s.ID == accessSection) {
				rs = append(rs, row{setting: st})
			}
		}
		switch s.ID {
		case accessSection:
		case machineSection:
			for i := range b.actions {
				rs = append(rs, row{action: &b.actions[i]})
			}
		default:
			for _, i := range b.items {
				if i.Tab == s.ID {
					rs = append(rs, row{item: i})
				}
			}
		}
		if len(rs) == 0 {
			continue
		}
		if heads {
			out = append(out, row{head: s.Title})
		}
		out = append(out, rs...)
	}
	return out
}

func (b *Board) key() string {
	if b.search != nil {
		return "?search"
	}
	return b.visibleTabs()[b.tab].ID
}

// selected is the row under the cursor (moved onto one that can be).
func (b *Board) selected(rs []row) (int, *row) {
	c, ok := b.cursor[b.key()]
	if !ok || c >= len(rs) || !rs[c].selectable() {
		c = slices.IndexFunc(rs, row.selectable)
		if c < 0 {
			return -1, nil
		}
		b.cursor[b.key()] = c
	}
	return c, &rs[c]
}

func (b *Board) move(d int) {
	rs := b.rows()
	c, _ := b.selected(rs)
	if c < 0 {
		return
	}
	for j := c + d; j >= 0 && j < len(rs); j += d {
		if rs[j].selectable() {
			b.cursor[b.key()] = j
			return
		}
	}
}

func (b *Board) Init() tea.Cmd { return nil }

func (b *Board) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		b.width, b.height = m.Width, m.Height
	case tea.KeyMsg:
		if m.Type != tea.KeyEsc && m.String() != "q" {
			b.quitAsked = false
		}
		b.flash = ""
		switch {
		case b.panel != nil:
			return b, b.panelKey(m)
		case b.search != nil:
			return b, b.searchKey(m)
		}
		return b, b.listKey(m)
	}
	return b, nil
}

func (b *Board) done(k BoardKind, id string) tea.Cmd {
	b.result = BoardResult{Kind: k, ID: id, Settings: map[string]string{}}
	for _, i := range b.items {
		switch i.Pending {
		case "add":
			b.result.Adds = append(b.result.Adds, i.ID)
		case "remove":
			b.result.Removes = append(b.result.Removes, i.ID)
		}
	}
	for _, s := range b.settings {
		if s.Pending != "" {
			b.result.Settings[s.Key] = s.Pending
		}
	}
	return tea.Quit
}

// toggle queues adding or removing i, or takes it off the queue.
func (b *Board) toggle(i *BoardItem) {
	switch {
	case i.Pending != "":
		i.Pending = ""
	case i.Picked:
		i.Pending = "remove"
	default:
		i.Pending = "add"
	}
}

func (b *Board) listKey(m tea.KeyMsg) tea.Cmd {
	tabs := b.visibleTabs()
	rs := b.rows()
	_, r := b.selected(rs)
	switch m.String() {
	case "up", "k":
		b.move(-1)
	case "down", "j":
		b.move(1)
	case "pgup":
		for n := 0; n < 10; n++ {
			b.move(-1)
		}
	case "pgdown":
		for n := 0; n < 10; n++ {
			b.move(1)
		}
	case "left", "h", "shift+tab":
		b.tab = (b.tab + len(tabs) - 1) % len(tabs)
	case "right", "l", "tab":
		b.tab = (b.tab + 1) % len(tabs)
	case " ":
		switch {
		case r != nil && r.item != nil:
			b.toggle(r.item)
		case r != nil && r.setting != nil:
			r.setting.Pending = ""
		}
	case "enter":
		return b.act(r)
	case "a":
		if b.queued() == 0 {
			b.flash = "Nothing queued: space on a row adds or removes it."
			return nil
		}
		b.confirm()
	case "/":
		s := ""
		b.search = &s
	case "esc", "q", "ctrl+c":
		if m.String() != "ctrl+c" && b.queued() > 0 && !b.quitAsked {
			b.quitAsked = true
			b.flash = fmt.Sprintf("%d change(s) queued and not applied: a applies them; %s again quits without them.", b.queued(), m.String())
			return nil
		}
		return b.done(BoardQuit, "")
	}
	// The tab may have gone (Needs you, once nothing does).
	b.tab = min(b.tab, len(b.visibleTabs())-1)
	return nil
}

func (b *Board) searchKey(m tea.KeyMsg) tea.Cmd {
	rs := b.rows()
	_, r := b.selected(rs)
	switch m.Type {
	case tea.KeyEsc:
		b.search = nil
	case tea.KeyBackspace:
		if s := []rune(*b.search); len(s) > 0 {
			*b.search = string(s[:len(s)-1])
		} else {
			b.search = nil
		}
		delete(b.cursor, "?search")
	case tea.KeyUp:
		b.move(-1)
	case tea.KeyDown:
		b.move(1)
	case tea.KeySpace:
		if r != nil && r.item != nil {
			b.toggle(r.item)
		}
	case tea.KeyEnter:
		if r == nil {
			b.search = nil
			return nil
		}
		return b.act(r)
	case tea.KeyCtrlA:
		if q := *b.search; ValidExtra(q) && !slices.ContainsFunc(b.items, func(i *BoardItem) bool { return i.ID == "mise:"+q }) {
			b.items = append(b.items, &BoardItem{ID: "mise:" + q, Label: q + " (extra mise tool)", Tab: ExtraTab, Pending: "add", Source: "mise use -g " + q})
			if !slices.ContainsFunc(b.tabs, func(t Tab) bool { return t.ID == ExtraTab }) {
				b.tabs = append(b.tabs, Tab{ID: ExtraTab, Title: "Extra"})
			}
			b.search = nil
		}
	case tea.KeyCtrlC:
		return b.done(BoardQuit, "")
	case tea.KeyRunes:
		*b.search += string(m.Runes)
		delete(b.cursor, "?search")
	}
	return nil
}

func (b *Board) panelKey(m tea.KeyMsg) tea.Cmd {
	p := b.panel
	switch m.String() {
	case "esc", "q":
		b.panel = nil
	case "up", "k":
		p.sel = max(0, p.sel-1)
	case "down", "j":
		p.sel = min(max(0, len(p.opts)-1), p.sel+1)
	case "enter", " ":
		b.panel = nil
		if len(p.opts) > 0 && p.on != nil {
			return p.on(p.opts[p.sel][1])
		}
	case "ctrl+c":
		return b.done(BoardQuit, "")
	}
	return nil
}

// act is enter on a row.
func (b *Board) act(r *row) tea.Cmd {
	if r == nil {
		return nil
	}
	switch {
	case r.setting != nil:
		s := r.setting
		cur := s.Value
		if s.Pending != "" {
			cur = s.Pending
		}
		p := &panel{title: s.Label, swatches: s.Swatches}
		if s.Section == "" || s.Section == accessSection {
			p.title = "Who can reach the " + s.Label
		}
		for n, c := range s.Choices {
			mark := "( ) "
			if c[0] == s.Value {
				mark = "(•) "
			}
			p.opts = append(p.opts, [2]string{mark + c[1], c[0]})
			if c[0] == cur {
				p.sel = n
			}
		}
		p.on = func(v string) tea.Cmd {
			s.Pending = ""
			if v != s.Value {
				s.Pending = v
			}
			return nil
		}
		b.panel = p
	case r.action != nil:
		switch r.action.ID {
		case "connect":
			b.panel = &panel{title: "Connect from a laptop", lines: b.connect}
		case "updates":
			return b.done(BoardUpdates, "")
		case "refresh":
			return b.done(BoardRefresh, "")
		}
	case r.item != nil:
		i := r.item
		switch {
		case i.Pending != "" || !i.Picked:
			b.toggle(i)
		case fixable(i):
			b.panel = &panel{title: i.Label + ": " + stateWord(i.State),
				lines: []string{i.Detail},
				opts:  append([][2]string{{"Fix it now (apply it again)", "fix"}}, append(changeOpts(i), [2]string{"Remove it (stays installed; yardo stops looking after it)", "remove"})...),
				on: func(v string) tea.Cmd {
					if v == "fix" {
						return b.done(BoardFix, i.ID)
					}
					if cmd := b.change(i, v); cmd != nil {
						return cmd
					}
					i.Pending = "remove"
					return nil
				}}
		case secret(i):
			cmd := b.done(BoardEdit, i.ID)
			b.result.Inputs = i.Inputs
			return cmd
		default:
			var lines []string
			if newer(i) {
				lines = append(lines, "Newer: "+strings.Join(i.Updates, ", "), "Update with mise up (your tools) or sudo apt upgrade (the system's).")
			}
			if i.Source != "" {
				lines = append(lines, "How: "+i.Source)
			}
			if i.State == "unchecked" {
				lines = append(lines, "yardo can't check this one yet (the bash installer runs it).")
			}
			lines = append(lines, "Space removes it.")
			b.panel = &panel{title: i.Label + ": " + stateWord(i.State), lines: lines, opts: changeOpts(i),
				on: func(v string) tea.Cmd { return b.change(i, v) }}
		}
	}
	return nil
}

// changeOpts are the panel's options for i's answers: "Change …", each
// asked again and then applied.
func changeOpts(i *BoardItem) [][2]string {
	var opts [][2]string
	for _, c := range i.Changes {
		opts = append(opts, [2]string{"Change " + c[0] + " (asks, then applies " + i.Label + " again)", "change:" + c[1]})
	}
	return opts
}

// change leaves the board to change i's answer v ("change:<key>"), or is
// nil when v is another option.
func (b *Board) change(i *BoardItem, v string) tea.Cmd {
	key, ok := strings.CutPrefix(v, "change:")
	if !ok {
		return nil
	}
	cmd := b.done(BoardChange, i.ID)
	b.result.Change = key
	return cmd
}

// confirm asks before applying the queue.
func (b *Board) confirm() {
	var lines []string
	for _, i := range b.items {
		switch i.Pending {
		case "add":
			lines = append(lines, "+ "+i.Label)
		case "remove":
			lines = append(lines, "- "+i.Label+" (stays installed; yardo stops looking after it)")
		}
	}
	for _, s := range b.settings {
		if s.Pending != "" {
			lines = append(lines, "~ "+s.Label+": "+s.label(s.Pending))
		}
	}
	b.panel = &panel{title: fmt.Sprintf("Apply %d change(s)?", b.queued()), lines: lines,
		opts: [][2]string{{"Apply (asks what new items need, then installs)", "apply"}, {"Not yet", "no"}},
		on: func(v string) tea.Cmd {
			if v == "apply" {
				return b.done(BoardApply, "")
			}
			return nil
		}}
}

// stateWord is how a state reads.
func stateWord(s string) string {
	switch s {
	case "ok":
		return "ready"
	case "down":
		return "stopped"
	case "partial":
		return "half set up"
	case "drifted":
		return "changed by hand"
	case "missing":
		return "not installed"
	case "secret":
		return "needs a value"
	case "unchecked":
		return "not checked"
	}
	return s
}

// mark is a row's health mark and its style.
func mark(i *BoardItem) (string, lipgloss.Style) {
	if !i.Picked || i.State == "" {
		return " ", muted
	}
	switch {
	case i.State == "down" || i.State == "missing":
		return "✗", red
	case i.State == "partial" || i.State == "drifted":
		return "~", yellow
	case i.State == "secret":
		return "?", accent
	case i.State == "unchecked":
		return "·", muted
	case len(i.Updates) > 0:
		return "↑", yellow
	}
	return "✓", green
}

// note is what a row says after its label, and its style.
func (b *Board) note(r row) (string, lipgloss.Style) {
	switch {
	case r.setting != nil:
		s := r.setting
		if s.Pending != "" {
			return s.label(s.Value) + " → " + s.label(s.Pending), accentBd
		}
		return s.label(s.Value), muted
	case r.action != nil:
		return r.action.Note, muted
	}
	i := r.item
	pre := ""
	if r.where != "" {
		pre = r.where + " · "
	}
	switch {
	case i.Pending == "add":
		return pre + "will install", green
	case i.Pending == "remove":
		return pre + "will stop looking after it", red
	case !i.Picked || i.State == "":
		return strings.TrimSuffix(pre, " · "), muted
	case i.State == "secret":
		return pre + "fill in " + strings.Join(i.Inputs, ", "), accent
	case Fixable(i.State):
		return pre + stateWord(i.State) + ": " + i.Detail, red
	case len(i.Updates) > 0:
		return pre + strings.Join(i.Updates, ", ") + " available", yellow
	}
	return strings.TrimSuffix(pre, " · "), muted
}

func padTo(s string, w int) string {
	s = runewidth.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(0, w-runewidth.StringWidth(s)))
}

func (b *Board) header(w int) string {
	left := " " + accentBd.Render("yardo") + muted.Render(" · "+b.info.Host+" · "+b.info.User)
	var right []string
	if n := b.queued(); n > 0 {
		right = append(right, accentBd.Render(fmt.Sprintf("%d change(s) queued", n)))
	}
	if b.first {
		right = append(right, muted.Render("nothing installed yet: a installs what's queued"))
	} else {
		if n := b.count(fixable); n > 0 {
			right = append(right, red.Render(fmt.Sprintf("✗ %d to fix", n)))
		}
		if n := b.count(secret); n > 0 {
			right = append(right, accent.Render(fmt.Sprintf("? %d to fill in", n)))
		}
		if n := b.count(newer); n > 0 {
			right = append(right, yellow.Render(fmt.Sprintf("↑ %d with updates", n)))
		}
		if len(right) == 0 {
			right = append(right, green.Render("✓ all well"))
		}
		right = append(right, muted.Render("checked "+b.info.Checked))
	}
	r := strings.Join(right, muted.Render(" · ")) + " "
	return left + strings.Repeat(" ", max(1, w-lipgloss.Width(left)-lipgloss.Width(r))) + r
}

func (b *Board) tabBar(w int) string {
	var parts []string
	for n, t := range b.visibleTabs() {
		badge := ""
		switch t.ID {
		case needsTab:
			badge = fmt.Sprintf(" %d", len(b.needs()))
		case boardReview:
			if q := b.queued(); q > 0 {
				badge = fmt.Sprintf(" %d", q)
			}
		default:
			in := func(i *BoardItem) bool {
				return slices.ContainsFunc(sectionsOf(t), func(s Tab) bool { return s.ID == i.Tab })
			}
			switch {
			case b.count(func(i *BoardItem) bool { return in(i) && fixable(i) }) > 0:
				badge = " ✗"
			case b.count(func(i *BoardItem) bool { return in(i) && secret(i) }) > 0:
				badge = " ?"
			case b.count(func(i *BoardItem) bool { return in(i) && newer(i) }) > 0:
				badge = " ↑"
			}
		}
		label := " " + t.Title + badge + " "
		switch {
		case n == b.tab && b.search == nil:
			parts = append(parts, tabOn.Render(label))
		case t.ID == needsTab:
			parts = append(parts, red.Render(label))
		default:
			parts = append(parts, muted.Render(label))
		}
	}
	return " " + runewidth.Truncate(strings.Join(parts, " "), w, "")
}

func (b *Board) footer(r *row) string {
	k := func(key, what string) string { return keyOn.Render(key) + " " + what }
	var keys []string
	switch {
	case b.panel != nil:
		if len(b.panel.opts) > 0 {
			keys = []string{k("↑↓", "choose"), k("enter", "do it"), k("esc", "back")}
		} else {
			keys = []string{k("esc", "close")}
		}
	case b.search != nil:
		keys = []string{accentBd.Render("/" + *b.search + "_"), k("space", "add/remove"), k("enter", "act"), k("esc", "done")}
	default:
		first := k("enter", "details")
		switch {
		case r == nil:
		case r.setting != nil:
			first = k("enter", "change")
		case r.action != nil:
			first = k("enter", "open")
		case r.item.Pending != "":
			first = k("space", "cancel")
		case !r.item.Picked:
			first = k("space", "add")
		case fixable(r.item):
			first = k("enter", "fix")
		case secret(r.item):
			first = k("enter", "fill in")
		case len(r.item.Changes) > 0:
			first = k("enter", "change")
		}
		keys = append(keys, first)
		if r != nil && r.item != nil && r.item.Picked && r.item.Pending == "" {
			keys = append(keys, k("space", "remove"))
		}
		if n := b.queued(); n > 0 {
			keys = append(keys, accentBd.Render("a")+" "+accentBd.Render(fmt.Sprintf("APPLY %d", n)))
		}
		keys = append(keys, k("←→", "tabs"), k("/", "search"), k("q", "quit"))
	}
	if b.flash != "" {
		return " " + yellow.Render(b.flash)
	}
	return " " + strings.Join(keys, "   ")
}

func (b *Board) View() string {
	if b.out {
		return ""
	}
	w, h := max(50, b.width), max(12, b.height)
	rule := muted.Render(strings.Repeat("─", w-1))
	var out []string
	out = append(out, b.header(w), b.tabBar(w), rule)

	rs := b.rows()
	cur, r := b.selected(rs)
	var pl []string
	if p := b.panel; p != nil {
		pl = append(pl, " "+accentBd.Render("┌ "+p.title+" "+strings.Repeat("─", max(0, w-6-runewidth.StringWidth(p.title)))))
		for _, l := range p.lines {
			for _, wl := range wrap(l, w-8) {
				pl = append(pl, " "+accentBd.Render("│")+"  "+wl)
			}
		}
		for n, o := range p.opts {
			sw := ""
			if hex, ok := p.swatches[o[1]]; ok {
				sw = lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("███") + " "
			}
			line := "  " + sw + o[0]
			if n == p.sel {
				line = "› " + sw + rowOn.Render(padTo(o[0], w-14))
			}
			pl = append(pl, " "+accentBd.Render("│")+" "+line)
		}
		pl = append(pl, " "+accentBd.Render("└"+strings.Repeat("─", max(0, w-4))))
	}
	listH := max(1, h-5-len(pl))
	top := 0
	if cur >= listH {
		top = cur - listH + 1
	}
	labelW := min(44, max(20, w*2/5))
	for n := top; n < len(rs) && n < top+listH; n++ {
		x := rs[n]
		switch {
		case x.head != "":
			out = append(out, " "+accentBd.Render(x.head))
			continue
		case x.text != "":
			out = append(out, "  "+muted.Render(x.text))
			continue
		}
		box, mk, ms, label := "   ", " ", muted, ""
		switch {
		case x.item != nil:
			i := x.item
			box = "[ ]"
			switch {
			case i.Pending == "add":
				box = "[+]"
			case i.Pending == "remove":
				box = "[-]"
			case i.Picked:
				box = "[x]"
			}
			mk, ms = mark(i)
			label = i.Label
		case x.setting != nil:
			mk, ms, label = "›", muted, x.setting.Label
		case x.action != nil:
			mk, ms, label = "›", accent, x.action.Label
		}
		note, ns := b.note(x)
		pointer := " "
		if n == cur && b.panel == nil {
			pointer = ">"
		}
		plain := fmt.Sprintf(" %s%s %s %s  ", pointer, box, mk, padTo(label, labelW))
		noteW := max(4, w-runewidth.StringWidth(plain)-1)
		if n == cur && b.panel == nil {
			out = append(out, rowOn.Render(padTo(plain+note, w-1)))
		} else {
			out = append(out, fmt.Sprintf(" %s%s %s %s  ", pointer, box, ms.Render(mk), padTo(label, labelW))+ns.Render(runewidth.Truncate(note, noteW, "…")))
		}
	}
	for len(out) < 3+listH {
		out = append(out, "")
	}
	out = append(out, pl...)
	out = append(out, rule, b.footer(r))
	return strings.Join(out, "\n")
}

// Run shows the board; it returns what it was left with.
func (b *Board) Run() (BoardResult, error) {
	b.out = false
	if _, err := tea.NewProgram(b, tea.WithAltScreen()).Run(); err != nil {
		return BoardResult{Kind: BoardQuit}, err
	}
	b.out = true
	return b.result, nil
}
