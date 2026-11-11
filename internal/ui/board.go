package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The board is the Tools screen: the picker's tabs and checklist, where a
// ticked box means picked (installed) and a mark beside it its health.
// Space ticks a tool to install or unticks one to uninstall, d asks to
// uninstall the row now, enter acts on the row (fix it, fill in its value,
// change an answer, reinstall, log in, forget it), a applies the queue,
// which shows on a line above the keys. Typing filters the list. The machine's settings are the
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
	// Dash is the home's dashboard.
	Dash Dashboard
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
	// Picked: it's among the saved picks (installed). Pending is "add",
	// "uninstall" or "forget" (yardo stops checking it; it stays
	// installed) when a change to it is queued.
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
	// Team is the team profile that picked it ("" for one picked by hand):
	// applying that profile again adds it back.
	Team string
	// Removable: yardo can uninstall it, which RemoveSay says, along with
	// NeededBy (the installed items that need it). One it can't says why:
	// KeepSay; a Protected one can't be forgotten either.
	Removable bool
	RemoveSay string
	NeededBy  []string
	KeepSay   string
	Protected bool
	// PartOf is the tool this one is part of (a VPN login): it isn't a
	// row of its own; its tool's details add or remove it.
	PartOf string
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
	// BoardApply applies the queue (Result's Uninstalls, Forgets, Adds,
	// Settings).
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
	Adds, Forgets []string
	// Uninstalls are the items to uninstall, with what needs them.
	Uninstalls []string
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
	tabID    string
	tab      int
	fromMenu bool
	cursor   map[string]int
	// search filters the rows; typing: keys go to it.
	search    *string
	typing    bool
	panel     *panel
	flash     string
	width     int
	height    int
	result    BoardResult
	out       bool
	quitAsked bool
	// plan says what the queued uninstalls do (see SetPlanner).
	plan Planner
}

// Planner says, for each item to uninstall, what uninstalling it does:
// what goes, stays and why, what goes with it. adds are the items queued
// to install, which may need one of them back.
type Planner func(uninstalls, adds []string) map[string]UninstallPlan

// SetPlanner sets what the question before applying says about each
// uninstall; without one it says the item's RemoveSay.
func (b *Board) SetPlanner(p Planner) { b.plan = p }

// panel is the box under the list for a row's choices or text.
type panel struct {
	title string
	lines []string
	// opts are the choices (label, value); sel the selected one; yn: y
	// and n choose "yes" and "no" too.
	opts [][2]string
	sel  int
	yn   bool
	// on is what a choice does.
	on func(value string) tea.Cmd
}

// LookSection is the look's settings' Section (the accent colour).
const LookSection = "__look__"

// InstalledTab is the board's first tab, once anything is installed: every
// tool installed, from all the tabs, to find one to fix or uninstall.
const InstalledTab = "__installed__"

// NewBoard builds the board: tabs (each with sections, or none) and the
// items. first: nothing is installed yet (no marks; the defaults come
// queued).
func NewBoard(info HomeInfo, first bool, tabs []Tab, items []*BoardItem) *Board {
	b := &Board{info: info, first: first, items: items,
		cursor: map[string]int{}, width: 100, height: 30}
	if slices.ContainsFunc(items, func(i *BoardItem) bool { return i.Tab == ExtraTab }) {
		tabs = append(slices.Clone(tabs), Tab{ID: ExtraTab, Title: "Extra"})
	}
	if !first && slices.ContainsFunc(items, func(i *BoardItem) bool { return i.Picked }) {
		tabs = append([]Tab{{ID: InstalledTab, Title: "Installed"}}, tabs...)
	}
	b.tabs = tabs
	return b
}

// Search opens the board filtered by q, typing it, as typing does.
func (b *Board) Search(q string) { b.search, b.typing = &q, true }

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

// inRow reports whether i is part of another item's row (a login, in its
// tool's), so not a row of its own.
func (b *Board) inRow(i *BoardItem) bool {
	return i.PartOf != "" && slices.ContainsFunc(b.items, func(t *BoardItem) bool { return t.ID == i.PartOf })
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
			if !b.inRow(i) && strings.Contains(strings.ToLower(i.Label+" "+i.ID), q) {
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
	if t.ID == InstalledTab {
		for _, i := range b.items {
			if i.Picked && !b.inRow(i) {
				out = append(out, row{item: i, where: b.tabTitle(i.Tab)})
			}
		}
		return out
	}
	heads := len(t.Sections) > 0
	for _, s := range sectionsOf(t) {
		var rs []row
		for _, i := range b.items {
			if i.Tab == s.ID && !b.inRow(i) {
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
		case b.search != nil && b.typing:
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
		case "forget":
			b.result.Forgets = append(b.result.Forgets, i.ID)
		case "uninstall":
			b.result.Uninstalls = append(b.result.Uninstalls, i.ID)
		}
	}
	return tea.Quit
}

// toggle ticks i (queues installing it) or unticks it (queues
// uninstalling it), or takes it off the queue.
func (b *Board) toggle(i *BoardItem) {
	switch {
	case i.Pending != "":
		i.Pending = ""
	case i.Picked:
		b.untick(i)
	default:
		i.Pending = "add"
	}
}

// untick queues uninstalling i. One yardo can't uninstall says why, and
// offers to forget it (it stays installed); a protected one only says
// why.
func (b *Board) untick(i *BoardItem) {
	switch {
	case i.Protected:
		b.flash = "yardo keeps " + short(i.Label) + ": " + i.KeepSay + "."
	case !i.Removable:
		why := i.KeepSay
		if why == "" {
			why = "its own installer or settings set it up"
		}
		b.panel = &panel{title: "yardo can't uninstall " + short(i.Label), yn: true, sel: 1,
			lines: []string{capital(why) + ".", "Forget it instead? It stays installed, and yardo stops checking it."},
			opts:  [][2]string{{"Forget it (y)", "yes"}, {"Keep it (n)", "no"}},
			on: func(v string) tea.Cmd {
				if v == "yes" {
					i.Pending = "forget"
				}
				return nil
			}}
	default:
		i.Pending = "uninstall"
		if i.Team != "" {
			b.flash = fmt.Sprintf("%s is on the team profile %s: once uninstalled, applying the profile again skips it.", short(i.Label), i.Team)
		}
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
	case "d":
		if r == nil || r.item == nil {
			return nil
		}
		if i := r.item; !i.Picked {
			b.flash = short(i.Label) + " isn't installed."
		} else if i.Pending == "" {
			b.untick(i)
		}
		if b.panel == nil && b.count(func(i *BoardItem) bool { return i.Pending == "uninstall" }) > 0 {
			b.confirm(false)
		}
	case "a":
		if b.queued() == 0 {
			b.flash = "Nothing queued. Space adds or uninstalls a tool."
			return nil
		}
		b.confirm(false)
	case "/":
		s := ""
		b.search, b.typing = &s, true
		delete(b.cursor, "?search")
	case "esc", "q", "ctrl+c":
		if m.String() == "esc" && b.search != nil {
			b.search = nil // the filter goes first
			return nil
		}
		if m.String() != "ctrl+c" && b.queued() > 0 && !b.quitAsked {
			b.quitAsked = true
			leave := "quits"
			if b.fromMenu {
				leave = "goes back"
			}
			b.flash = fmt.Sprintf("%s not applied yet. a applies; %s again %s without them.", plural(b.queued(), "change", "changes"), m.String(), leave)
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
		b.search, b.typing = nil, false
	case tea.KeyBackspace:
		if s := []rune(*b.search); len(s) > 0 {
			*b.search = string(s[:len(s)-1])
		} else {
			b.search, b.typing = nil, false
		}
		delete(b.cursor, "?search")
	case tea.KeyUp:
		b.move(rs, -1)
	case tea.KeyDown:
		// Into the rows, keeping the filter: every key works there.
		if r != nil {
			b.typing = false
		}
	case tea.KeySpace:
		if r != nil && r.item != nil {
			b.toggle(r.item)
		}
	case tea.KeyEnter:
		if r == nil {
			b.search, b.typing = nil, false
			return nil
		}
		b.typing = false // into the rows, keeping the filter
	case tea.KeyCtrlA:
		if q := *b.search; ValidExtra(q) && !slices.ContainsFunc(b.items, func(i *BoardItem) bool { return i.ID == "mise:"+q }) {
			b.items = append(b.items, &BoardItem{ID: "mise:" + q, Label: q + " (extra mise tool)", Tab: ExtraTab, Pending: "add", Source: "mise use -g " + q})
			if !slices.ContainsFunc(b.tabs, func(t Tab) bool { return t.ID == ExtraTab }) {
				b.tabs = append(b.tabs, Tab{ID: ExtraTab, Title: "Extra"})
			}
			b.search, b.typing = nil, false
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
	switch k := m.String(); {
	case p.yn && (k == "y" || k == "n"):
		b.panel = nil
		if k == "y" && p.on != nil {
			return p.on("yes")
		}
		return nil
	}
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
// do), changing each of its answers, logging in or out (its parts),
// uninstalling it and forgetting it. Any other value does nothing.
func (b *Board) itemPanel(i *BoardItem, lines []string, first [2]string, do func() tea.Cmd) *panel {
	opts := append([][2]string{first}, changeOpts(i)...)
	parts := b.parts(i)
	for n, p := range parts {
		name := short(p.Label)
		login := strings.HasSuffix(name, "login")
		switch {
		case !p.Picked && p.Pending == "" && login:
			opts = append(opts, [2]string{"Log in", fmt.Sprint("part:", n)})
		case !p.Picked && p.Pending == "":
			opts = append(opts, [2]string{"Add " + name, fmt.Sprint("part:", n)})
		case p.Picked && p.Pending == "" && p.Removable && login:
			opts = append(opts, [2]string{"Log out (" + short(i.Label) + " stays)", fmt.Sprint("part:", n)})
		case p.Picked && p.Pending == "" && p.Removable:
			opts = append(opts, [2]string{"Remove " + name + " (" + short(i.Label) + " stays)", fmt.Sprint("part:", n)})
		}
	}
	if i.Removable {
		opts = append(opts, [2]string{"Uninstall", "uninstall"})
	} else if i.KeepSay != "" {
		lines = append(lines, "yardo can't uninstall it: "+i.KeepSay+".")
	}
	if !i.Protected {
		opts = append(opts, [2]string{"Forget it: stays installed, yardo stops checking it", "forget"})
	}
	return &panel{title: short(i.Label) + ": " + stateWord(i.State), lines: lines, opts: opts,
		on: func(v string) tea.Cmd {
			switch v {
			case first[1]:
				return do()
			case "forget":
				i.Pending = "forget"
				return nil
			case "uninstall":
				i.Pending = "uninstall"
				return nil
			}
			if n, ok := strings.CutPrefix(v, "part:"); ok {
				k, _ := strconv.Atoi(n)
				if p := parts[k]; p.Picked {
					p.Pending = "uninstall"
				} else {
					p.Pending = "add"
				}
				return nil
			}
			return b.change(i, v)
		}}
}

// parts are the items that are part of i (its login).
func (b *Board) parts(i *BoardItem) []*BoardItem {
	var out []*BoardItem
	for _, p := range b.items {
		if p.PartOf == i.ID {
			out = append(out, p)
		}
	}
	return out
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

// confirm asks before applying the queue: each uninstall with what its
// removal does, what stays and what goes with it (from the planner;
// full: every package and file), then the rest.
func (b *Board) confirm(full bool) {
	var lines, uninstalls, adds []string
	for _, i := range b.items {
		switch i.Pending {
		case "uninstall":
			uninstalls = append(uninstalls, i.ID)
		case "add":
			adds = append(adds, i.ID)
		}
	}
	var plans map[string]UninstallPlan
	if b.plan != nil && len(uninstalls) > 0 {
		plans = b.plan(uninstalls, adds)
	}
	everything := 0
	for _, i := range b.items {
		if i.Pending != "uninstall" {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "− Uninstall "+short(i.Label))
		pl, ok := plans[i.ID]
		if !ok {
			pl = UninstallPlan{Say: i.RemoveSay, Also: i.NeededBy}
		}
		everything += len(pl.Off)
		for _, l := range pl.Lines(full) {
			lines = append(lines, "    "+l)
		}
	}
	for _, i := range b.items {
		switch i.Pending {
		case "forget":
			lines = append(lines, "  Forget "+short(i.Label)+": it stays installed")
		case "add":
			lines = append(lines, "+ Install "+short(i.Label))
		}
	}
	lines = append(lines, "")
	n := b.queued()
	title, do, keep := "Apply "+plural(n, "change", "changes")+"?", "Apply (y)", "Not yet (n)"
	if len(uninstalls) == n {
		title, do, keep = "Uninstall "+plural(n, "tool", "tools")+"?", "Uninstall (y)", "Cancel (n)"
	}
	opts := [][2]string{{do, "yes"}}
	if !full && everything > 0 {
		opts = append(opts, [2]string{fmt.Sprintf("List everything that goes (%d)", everything), "list"})
	}
	b.panel = &panel{title: title, lines: lines, yn: true,
		opts: append(opts, [2]string{keep, "no"}),
		on: func(v string) tea.Cmd {
			switch v {
			case "yes":
				return b.done(BoardApply, "")
			case "list":
				b.confirm(true)
			}
			return nil
		}}
}

// ShowResult opens the board on how an uninstall went: title, a line per
// tool, until dismissed; retry are the ones that failed, to try again.
func (b *Board) ShowResult(title string, lines, retry []string) {
	opts := [][2]string{{"OK", "ok"}}
	var again []*BoardItem
	for _, id := range retry {
		if n := slices.IndexFunc(b.items, func(i *BoardItem) bool { return i.ID == id }); n >= 0 {
			again = append(again, b.items[n])
		}
	}
	if len(again) > 0 {
		var names []string
		for _, i := range again {
			names = append(names, short(i.Label))
		}
		opts = append(opts, [2]string{"Retry " + strings.Join(names, ", "), "retry"})
	}
	b.panel = &panel{title: title, lines: lines, opts: opts,
		on: func(v string) tea.Cmd {
			if v == "retry" {
				for _, i := range again {
					i.Pending = "uninstall"
				}
				b.confirm(false)
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
	if !i.Picked || i.State == "" || i.Pending != "" {
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
	case i.Pending == "forget":
		return pre + "will forget it: stays installed", yellow
	case i.Pending == "uninstall":
		return pre + "will uninstall", red
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
	case i.Team != "":
		return pre + "from the team profile " + i.Team, muted
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
		right = append(right, accentBd.Render(plural(n, "change", "changes")+" queued"))
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
		if b.info.Checked != "" {
			right = append(right, muted.Render("updates checked "+b.info.Checked))
		}
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
		if lvl > worst[InstalledTab] {
			worst[InstalledTab] = lvl
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
			if b.panel.yn {
				keys = append(keys, k("y/n", "yes or no"))
			}
		} else {
			keys = []string{k("esc", "close")}
		}
	case b.search != nil && b.typing:
		keys = []string{accentBd.Render("/" + *b.search + "_"), k("enter", "into the list"), k("space", "add/uninstall"), k("esc", "clear")}
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
			keys = append(keys, k("space", "uninstall"))
		}
		if n := b.queued(); n > 0 {
			keys = append(keys, accentBd.Render("a")+" "+accentBd.Render(fmt.Sprintf("APPLY %d", n)))
		}
		if b.search != nil {
			keys = append([]string{accentBd.Render("/" + *b.search)}, keys...)
		}
		leave := k("q", "quit")
		if b.search != nil {
			leave = k("esc", "clear the filter")
		} else if b.fromMenu {
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
			// An indented line stays indented, wrapped.
			body := strings.TrimLeft(l, " ")
			pad := strings.Repeat(" ", len(l)-len(body))
			if body == "" {
				pl = append(pl, " "+accentBd.Render("│"))
				continue
			}
			for _, wl := range wrap(body, w-8-len(pad)) {
				pl = append(pl, " "+accentBd.Render("│")+"  "+pad+wl)
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
				q = append(q, "install "+short(i.Label))
			case "forget":
				q = append(q, "forget "+short(i.Label))
			case "uninstall":
				q = append(q, "uninstall "+short(i.Label))
			}
		}
		ql = append(ql, " "+accentBd.Render("Queued: ")+runewidth.Truncate(strings.Join(q, " · "), max(10, w-24), "…")+muted.Render("   a applies"))
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
		case i.Pending == "forget":
			box = "[ ]"
		case i.Pending == "uninstall":
			box = "[−]"
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
