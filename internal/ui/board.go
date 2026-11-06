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

// The board is the Tools screen: the picker's tabs and checklist, where a
// ticked box means picked (installed) and a mark beside it its health.
// Space queues adding or removing an item, enter acts on the row (fix it,
// fill in its value, change an answer, reinstall it), a applies the queue,
// which shows on a line above the keys. The machine's settings are the
// menu's (Settings).

// HomeInfo is what the board's header says.
type HomeInfo struct {
	Host, User, Checked string
	// MCP are the gateway's servers (yardo's, then the ones found in
	// ~/.mcp), MCPSpecs the catalog's, to add; MCPDown: the gateway isn't
	// running.
	MCP      []MCPServer
	MCPSpecs []MCPSpec
	MCPDown  bool
}

// MCPServer is one MCP server on the menu's MCP servers screen.
type MCPServer struct {
	Entry string // as the answers name it: jira, jira:work, mine
	Name  string // ~/.mcp/<name>, the gateway's name, its tools' prefix
	Spec  string // its catalog spec; "" for one with its own
	About string // the spec's description
	// State is ok, secret (Keys are empty), failed (not registered),
	// nospec (gone from the catalog), off or byhand (found in ~/.mcp: not
	// yardo's, registered or not), unchecked.
	State string
	Keys  []string
	Yardo bool // on yardo's list (else found in ~/.mcp)
	// Instead is what replaces it, when its maker retired it (State
	// retired): aws for aws-api.
	Instead string
	// Detail is why it didn't register (State failed).
	Detail string
	// From is where it runs from: its image or URL. Cached is which of
	// its image is pulled here ("cached: 0.23.1, built 2026-03-02", "not
	// pulled yet"); "" for a URL.
	From, Cached string
}

// MCPSpec is a catalog server, to add.
type MCPSpec struct{ Name, About string }

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
	return state == "down" || state == "partial" || state == "missing" || state == "drifted" || state == "outdated"
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

// BoardSetting is one of the menu's settings: who can reach a service
// (Network access), or how things look.
type BoardSetting struct {
	Key, Label string
	// Section is LookSection for the look ("": who can reach it).
	Section string
	// Choices are value and label pairs, in order.
	Choices [][2]string
	// Value is the saved choice.
	Value string
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
	// Settings are settings to change, key to value (the menu's).
	Settings map[string]string
}

// Board is the screen; see NewBoard.
type Board struct {
	info  HomeInfo
	first bool
	tabs  []Tab
	items []*BoardItem

	// tabID is the current tab; tab its index when last found.
	tabID     string
	tab       int
	fromMenu  bool
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
	// opts are the choices (label, value); sel the selected one.
	opts [][2]string
	sel  int
	// on is what a choice does.
	on func(value string) tea.Cmd
}

// LookSection is the look's settings' Section (the accent colour).
const LookSection = "__look__"

// NewBoard builds the board: tabs (each with sections, or none) and the
// items. first: nothing is installed yet (no marks; the defaults come
// queued).
func NewBoard(info HomeInfo, first bool, tabs []Tab, items []*BoardItem) *Board {
	b := &Board{info: info, first: first, items: items,
		cursor: map[string]int{}, width: 100, height: 30}
	if slices.ContainsFunc(items, func(i *BoardItem) bool { return i.Tab == ExtraTab }) {
		tabs = append(slices.Clone(tabs), Tab{ID: ExtraTab, Title: "Extra"})
	}
	b.tabs = tabs
	return b
}

// Search opens the board searching for q, as typing does.
func (b *Board) Search(q string) { b.search = &q }

// Open shows the tab with id; nothing when there's no such tab.
func (b *Board) Open(tabID string) {
	tabs := b.visibleTabs()
	if n := slices.IndexFunc(tabs, func(t Tab) bool { return t.ID == tabID }); n >= 0 {
		b.tabID, b.tab = tabID, n
	}
}

// Flash shows s on the footer's line until the next key.
func (b *Board) Flash(s string) { b.flash = s }

// SetFromMenu says the board was opened from the menu: esc goes back to
// it, rather than quitting.
func (b *Board) SetFromMenu(on bool) { b.fromMenu = on }

// current is the current tab's index in tabs (the visible ones): found by
// its id, or, gone, the one now at its place.
func (b *Board) current(tabs []Tab) int {
	if n := slices.IndexFunc(tabs, func(t Tab) bool { return t.ID == b.tabID }); n >= 0 {
		b.tab = n
		return n
	}
	b.tab = max(0, min(b.tab, len(tabs)-1))
	b.tabID = tabs[b.tab].ID
	return b.tab
}

// visibleTabs are the tabs as shown.
func (b *Board) visibleTabs() []Tab { return b.tabs }

func fixable(i *BoardItem) bool { return i.Picked && Fixable(i.State) }
func secret(i *BoardItem) bool  { return i.Picked && i.State == "secret" }
func newer(i *BoardItem) bool   { return i.Picked && len(i.Updates) > 0 }

func (b *Board) count(f func(*BoardItem) bool) int {
	n := 0
	for _, i := range b.items {
		if f(i) {
			n++
		}
	}
	return n
}

// queued counts the queue.
func (b *Board) queued() int {
	return b.count(func(i *BoardItem) bool { return i.Pending != "" })
}

// row is one line of the list: a heading, or something to act on.
type row struct {
	head  string
	text  string
	item  *BoardItem
	where string // search results: the item's tab
}

func (r row) selectable() bool { return r.head == "" && r.text == "" }

// label is what a selectable row says.
func (r row) label() string {
	if r.item != nil {
		return r.item.Label
	}
	return ""
}

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
	tabs := b.visibleTabs()
	t := tabs[b.current(tabs)]
	heads := len(t.Sections) > 0
	for _, s := range sectionsOf(t) {
		var rs []row
		for _, i := range b.items {
			if i.Tab == s.ID {
				rs = append(rs, row{item: i})
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
	tabs := b.visibleTabs()
	return tabs[b.current(tabs)].ID
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

// move moves the cursor n selectable rows of rs (the rows listed) down,
// or up when n < 0, as far as there are.
func (b *Board) move(rs []row, n int) {
	c, _ := b.selected(rs)
	if c < 0 {
		return
	}
	d := 1
	if n < 0 {
		d, n = -1, -n
	}
	for j := c + d; j >= 0 && j < len(rs) && n > 0; j += d {
		if rs[j].selectable() {
			c, n = j, n-1
		}
	}
	b.cursor[b.key()] = c
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
	cur := b.current(tabs)
	rs := b.rows()
	_, r := b.selected(rs)
	switch m.String() {
	case "up", "k":
		b.move(rs, -1)
	case "down", "j":
		b.move(rs, 1)
	case "pgup":
		b.move(rs, -10)
	case "pgdown":
		b.move(rs, 10)
	case "left", "h", "shift+tab":
		b.tabID = tabs[(cur+len(tabs)-1)%len(tabs)].ID
	case "right", "l", "tab":
		b.tabID = tabs[(cur+1)%len(tabs)].ID
	case " ":
		if r != nil && r.item != nil {
			b.toggle(r.item)
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
			leave := "quits"
			if b.fromMenu {
				leave = "goes back"
			}
			b.flash = fmt.Sprintf("%d change(s) queued and not applied: a applies them; %s again %s without them.", b.queued(), m.String(), leave)
			return nil
		}
		return b.done(BoardQuit, "")
	}
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
		b.move(rs, -1)
	case tea.KeyDown:
		b.move(rs, 1)
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
	case r.item != nil:
		i := r.item
		switch {
		case i.Pending != "" || !i.Picked:
			b.toggle(i)
		case fixable(i):
			b.panel = b.itemPanel(i, []string{i.Detail}, [2]string{fixLabel(i.State), "fix"}, func() tea.Cmd {
				return b.done(BoardFix, i.ID)
			})
		case secret(i):
			// Filling in comes first; its answers (the MCP servers) can
			// still be changed.
			b.panel = b.itemPanel(i, []string{"Empty: " + strings.Join(i.Inputs, ", ")}, [2]string{"Fill in " + plural(len(i.Inputs), "value", "values"), "edit"}, func() tea.Cmd {
				cmd := b.done(BoardEdit, i.ID)
				b.result.Inputs = i.Inputs
				return cmd
			})
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
			b.panel = b.itemPanel(i, lines, [2]string{"Reinstall (rewrites its files, installs what's missing)", "fix"}, func() tea.Cmd {
				return b.done(BoardFix, i.ID)
			})
		}
	}
	return nil
}

// itemPanel is a picked item's panel: lines, then first (choosing it calls
// do), changing each of its answers, and removing it. Any other value does
// nothing.
func (b *Board) itemPanel(i *BoardItem, lines []string, first [2]string, do func() tea.Cmd) *panel {
	opts := append([][2]string{first}, changeOpts(i)...)
	opts = append(opts, [2]string{"Stop managing it (it stays installed)", "remove"})
	return &panel{title: i.Label + ": " + stateWord(i.State), lines: lines, opts: opts,
		on: func(v string) tea.Cmd {
			switch v {
			case first[1]:
				return do()
			case "remove":
				i.Pending = "remove"
				return nil
			}
			return b.change(i, v)
		}}
}

// changeOpts are the panel's options for i's answers: "Change …", each
// asked again and then applied.
func changeOpts(i *BoardItem) [][2]string {
	var opts [][2]string
	for _, c := range i.Changes {
		label := "Change " + c[0] + " (asks, then applies " + i.Label + " again)"
		if c[1] == "mcp" {
			label = "Manage " + c[0] + " (add, edit, rename or delete one)"
		}
		opts = append(opts, [2]string{label, "change:" + c[1]})
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
			lines = append(lines, "- "+i.Label+" (yardo stops managing it; it stays installed)")
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
		return "working"
	case "down":
		return "stopped"
	case "partial":
		return "half set up"
	case "drifted":
		return "edited by hand"
	case "outdated":
		return "set up by an older yardo"
	case "missing":
		return "not installed"
	case "secret":
		return "needs a value"
	case "unchecked":
		return "not checked yet"
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
	case i.State == "partial" || i.State == "drifted" || i.State == "outdated":
		return "~", yellow
	case i.State == "secret":
		return "○", accent
	case i.State == "unchecked":
		return "·", muted
	case len(i.Updates) > 0:
		return "↑", yellow
	}
	return "✓", green
}

// note is what a row says after its label, and its style.
func (b *Board) note(r row) (string, lipgloss.Style) {
	i := r.item
	pre := ""
	if r.where != "" {
		pre = r.where + " · "
	}
	switch {
	case i.Pending == "add":
		return pre + "will install", green
	case i.Pending == "remove":
		return pre + "will stop managing it (stays installed)", red
	case !i.Picked || i.State == "":
		return strings.TrimSuffix(pre, " · "), muted
	case i.State == "secret":
		return pre + "fill in " + strings.Join(i.Inputs, ", "), accent
	case Fixable(i.State):
		return pre + stateWord(i.State) + ": " + i.Detail, red
	case len(i.Updates) > 0:
		return pre + strings.Join(i.Updates, ", ") + " available", yellow
	case i.State == "unchecked":
		return pre + stateWord(i.State), muted
	}
	return strings.TrimSuffix(pre, " · "), muted
}

// fixLabel is what fixing an item in state does, said as a choice.
func fixLabel(state string) string {
	switch state {
	case "down":
		return "Fix it (start it again)"
	case "drifted":
		return "Fix it (puts back yardo's version: your edits go)"
	case "outdated":
		return "Fix it (sets it up with this yardo's files)"
	}
	return "Fix it (sets it up again)"
}

// beside lays a selected row out: head (rendered, at col cells wide), then
// each of notes in style to its right, wrapped to the w left: as many lines
// as they take, the ones after under the first. Too narrow a column for
// them, they go below head instead.
func beside(head string, col, w int, style lipgloss.Style, notes ...string) []string {
	indent := col
	if w-col < 24 {
		indent = min(col, 7)
	}
	var wrapped []string
	for _, n := range notes {
		if n != "" {
			wrapped = append(wrapped, wrap(n, max(10, w-indent))...)
		}
	}
	out := []string{head}
	for i, l := range wrapped {
		if i == 0 && indent == col {
			out[0] = head + style.Render(l)
			continue
		}
		out = append(out, strings.Repeat(" ", indent)+style.Render(l))
	}
	return out
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
			right = append(right, accent.Render(fmt.Sprintf("○ %d to fill in", n)))
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
	// Each tab's worst: 3 something to fix, 2 to fill in, 1 an update.
	tabOf := map[string]string{}
	for _, t := range b.tabs {
		for _, s := range sectionsOf(t) {
			tabOf[s.ID] = t.ID
		}
	}
	worst := map[string]int{}
	for _, i := range b.items {
		lvl := 0
		switch {
		case fixable(i):
			lvl = 3
		case secret(i):
			lvl = 2
		case newer(i):
			lvl = 1
		}
		if t, ok := tabOf[i.Tab]; ok && lvl > worst[t] {
			worst[t] = lvl
		}
	}
	tabs := b.visibleTabs()
	cur := b.current(tabs)
	var parts []string
	for n, t := range tabs {
		label := " " + t.Title + [...]string{"", " ↑", " ○", " ✗"}[worst[t.ID]] + " "
		if n == cur && b.search == nil {
			parts = append(parts, tabOn.Render(label))
		} else {
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
		leave := k("q", "quit")
		if b.fromMenu {
			leave = k("esc", "home")
		}
		keys = append(keys, k("←→", "tabs"), k("/", "search"), leave)
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
			line := "  " + o[0]
			if n == p.sel {
				line = "› " + rowOn.Render(padTo(o[0], w-14))
			}
			pl = append(pl, " "+accentBd.Render("│")+" "+line)
		}
		pl = append(pl, " "+accentBd.Render("└"+strings.Repeat("─", max(0, w-4))))
	}
	// The queue, on a line of its own above the keys.
	var ql []string
	if b.queued() > 0 && b.panel == nil {
		var q []string
		for _, i := range b.items {
			switch i.Pending {
			case "add":
				q = append(q, "+ "+short(i.Label))
			case "remove":
				q = append(q, "- "+short(i.Label))
			}
		}
		ql = append(ql, " "+accentBd.Render("Queued: ")+runewidth.Truncate(strings.Join(q, ", "), max(10, w-24), "…")+muted.Render("   a applies"))
	}
	listH := max(1, h-5-len(pl)-len(ql))
	// Labels as wide as the longest one, leaving the notes beside them
	// room (up to 2/5 of the screen) when any row has one.
	// Each row's note, worked out once.
	type noted struct {
		text  string
		style lipgloss.Style
	}
	notes := make([]noted, len(rs))
	longest, noteMax := 0, 0
	for n, x := range rs {
		if !x.selectable() {
			continue
		}
		longest = max(longest, runewidth.StringWidth(x.label()))
		notes[n].text, notes[n].style = b.note(x)
		noteMax = max(noteMax, runewidth.StringWidth(notes[n].text))
	}
	reserve := 0
	if noteMax > 0 {
		reserve = min(noteMax, max(30, w*2/5))
	}
	labelW := max(20, min(longest, w-14-reserve))
	// The selected row's note wraps under itself: the lines it takes more.
	more := 0
	if cur >= 0 && cur < len(rs) && b.panel == nil && rs[cur].head == "" && rs[cur].text == "" {
		note := notes[cur].text
		more = max(0, len(wrap(note, max(4, w-labelW-12)))-1)
		more = min(more, max(0, listH-1))
	}
	top := 0
	if cur >= listH-more {
		top = cur - (listH - more) + 1
	}
	for n := top; n < len(rs) && len(out) < 3+listH; n++ {
		x := rs[n]
		switch {
		case x.head != "":
			out = append(out, " "+accentBd.Render(x.head))
			continue
		case x.text != "":
			out = append(out, "  "+muted.Render(x.text))
			continue
		}
		i, label := x.item, x.label()
		box := "[ ]"
		switch {
		case i.Pending == "add":
			box = "[+]"
		case i.Pending == "remove":
			box = "[-]"
		case i.Picked:
			box = "[x]"
		}
		mk, ms := mark(i)
		note, ns := notes[n].text, notes[n].style
		pointer := " "
		if n == cur && b.panel == nil {
			pointer = ">"
		}
		plain := fmt.Sprintf(" %s%s %s %s  ", pointer, box, mk, padTo(label, labelW))
		noteW := max(4, w-runewidth.StringWidth(plain)-1)
		if n == cur && b.panel == nil {
			ls := wrap(note, noteW)
			if note == "" {
				ls = []string{""}
			}
			pad := strings.Repeat(" ", runewidth.StringWidth(plain))
			for j, l := range ls {
				if j > 0 && len(out) >= 3+listH {
					break
				}
				lead := plain
				if j > 0 {
					lead = pad
				}
				out = append(out, rowOn.Render(padTo(lead+l, w-1)))
			}
		} else {
			out = append(out, fmt.Sprintf(" %s%s %s %s  ", pointer, box, ms.Render(mk), padTo(label, labelW))+ns.Render(runewidth.Truncate(note, noteW, "…")))
		}
	}
	for len(out) < 3+listH {
		out = append(out, "")
	}
	out = append(out, pl...)
	out = append(out, ql...)
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
