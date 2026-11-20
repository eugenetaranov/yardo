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
// which shows on a line above the keys. Typing filters the list. The
// machine's settings are the menu's (Settings). It's one of the app's
// views (see App).

// HomeInfo is what the board's header says.
type HomeInfo struct {
	Host, User, Checked string
	// MCP are the gateway's servers (yardo's, then the ones found in
	// ~/.mcp), MCPSpecs the catalog's, to add; MCPDown: the gateway isn't
	// running; MCPAllOff: all servers are turned off.
	MCP       []MCPServer
	MCPSpecs  []MCPSpec
	MCPDown   bool
	MCPAllOff bool
	// Dash is the home's dashboard.
	Dash Dashboard
	// Connect is the home's Connection panel; Keys the public keys runs
	// made, to add on their hosts (Connection's Keys to add).
	Connect []ConnectRow
	Keys    []AddKey
	// LastRun is how the last run went (nil: none yet).
	LastRun *LastRun
}

// ConnectRow is one way a laptop reaches this machine: What (SSH, Docker,
// the MCP gateway), the Use to type, and a Note: where it goes, or where
// a secret it needs is kept (never the secret).
type ConnectRow struct{ What, Use, Note string }

// MCPServer is one MCP server on the menu's MCP servers screen.
type MCPServer struct {
	Entry string // as the answers name it: jira, jira:work, mine
	Name  string // ~/.mcp/<name>, the gateway's name, its tools' prefix
	Spec  string // its catalog spec; "" for one with its own
	About string // the spec's description
	// State is ok, secret (Keys are empty), failed (not registered),
	// nospec (gone from the catalog), off (turned off: Off says how),
	// unlisted or byhand (found in ~/.mcp: not yardo's, registered or
	// not), unchecked.
	State string
	// Off is "own" for a server turned off by itself, "all" for one off
	// only because all are.
	Off string
	// was is its state before the menu turned it off (back to it on).
	was   string
	Keys  []string
	Yardo bool // on yardo's list (else found in ~/.mcp)
	// Instead is what replaces it, when its maker retired it (State
	// retired): aws for aws-api.
	Instead string
	// Detail is why it didn't register (State failed); Note, what it may
	// do, as registered (an AWS server's read-only or not).
	Detail, Note string
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

// SetLook colours yardo's screens (and its prompts) with the look key
// names (tui.Looks), hex being the user's accent colour, Classic's.
func SetLook(key, hex string) {
	tui.Use(key, lipgloss.Color(hex))
	l := tui.Cur
	accent = lipgloss.NewStyle().Foreground(tui.Accent)
	accentBd = accent.Bold(true)
	muted = lipgloss.NewStyle().Foreground(l.Muted)
	green = lipgloss.NewStyle().Foreground(l.OK).Bold(true)
	red = lipgloss.NewStyle().Foreground(l.Bad)
	yellow = lipgloss.NewStyle().Foreground(l.Warn)
	keyOn = lipgloss.NewStyle().Foreground(l.Info).Bold(true)
	tabOn = lipgloss.NewStyle().Bold(true).Foreground(l.Ink).Background(tui.Accent)
	rowOn = lipgloss.NewStyle().Foreground(l.Ink).Background(tui.Accent).Bold(true)
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
	dialog    *Dialog
	flash     string
	width     int
	height    int
	result    BoardResult
	quitAsked bool
	// ended: the board was left, with result; hits are where the last
	// Body drew what a click selects.
	ended bool
	hits  []Hit
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

// Items are the board's items (the queue is their Pending).
func (b *Board) Items() []*BoardItem { return b.items }

// Search opens the board filtered by q, typing it, as typing does.
func (b *Board) Search(q string) { b.search, b.typing = &q, true }

// Open shows the tab with id; nothing when there's no such tab.
func (b *Board) Open(tabID string) {
	tabs := b.visibleTabs()
	if n := slices.IndexFunc(tabs, func(t Tab) bool { return t.ID == tabID }); n >= 0 {
		b.tabID, b.tab = tabID, n
	}
}

// Flash shows s on the toast line until the next key.
func (b *Board) Flash(s string) { b.flash = s }

// SetFromMenu says the board has the menu to go back to: q goes home
// (the queue kept), rather than quitting.
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
func (b *Board) inRow(i *BoardItem) bool { return folded(b.items, i) }

// folded reports whether i is part of another of items (a login, in its
// tool's): that item's row shows its state and values, so i isn't
// counted on its own.
func folded(items []*BoardItem, i *BoardItem) bool {
	return i.PartOf != "" && slices.ContainsFunc(items, func(t *BoardItem) bool { return t.ID == i.PartOf })
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
	case MoveMsg:
		b.flash = ""
		if b.dialog == nil {
			b.typing = false
			b.move(b.rows(), m.N)
		}
	case tea.KeyMsg:
		if m.Type != tea.KeyEsc && m.String() != "q" {
			b.quitAsked = false
		}
		b.flash = ""
		switch {
		case b.dialog != nil:
			if m.String() == "ctrl+c" {
				return b, b.done(BoardQuit, "")
			}
			return b, dialogKey(&b.dialog, m.String())
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
	b.ended = true
	return nil
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
		b.dialog = &Dialog{Title: "yardo can't uninstall " + short(i.Label), YN: true, Sel: 1,
			Lines: []string{capital(why) + ".", "Forget it instead? It stays installed, and yardo stops checking it."},
			Opts:  [][2]string{{"Forget it (y)", "yes"}, {"Keep it (n)", "no"}},
			On: func(v string) tea.Cmd {
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
		if b.dialog == nil && b.count(func(i *BoardItem) bool { return i.Pending == "uninstall" }) > 0 {
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
		if m.String() != "ctrl+c" && b.search != nil {
			b.search = nil // the filter goes first
			return nil
		}
		// From the menu, the queue goes home with it; with none, quitting
		// drops it: asked once.
		if m.String() != "ctrl+c" && b.queued() > 0 && !b.quitAsked && !b.fromMenu {
			b.quitAsked = true
			b.flash = fmt.Sprintf("%s not applied yet. a applies; %s again quits without them.", plural(b.queued(), "change", "changes"), m.String())
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
			b.dialog = b.itemPanel(i, []string{i.Detail}, [2]string{fixLabel(i.State), "fix"}, func() tea.Cmd {
				return b.done(BoardFix, i.ID)
			})
		case secret(i):
			// Filling in comes first; its answers (the MCP servers) can
			// still be changed.
			b.dialog = b.itemPanel(i, []string{"Empty: " + strings.Join(i.Inputs, ", ")}, [2]string{"Fill in " + plural(len(i.Inputs), "value", "values"), "edit"}, func() tea.Cmd {
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
			b.dialog = b.itemPanel(i, lines, [2]string{"Reinstall (rewrites its files, installs what's missing)", "fix"}, func() tea.Cmd {
				return b.done(BoardFix, i.ID)
			})
		}
	}
	return nil
}

// itemPanel is a picked item's panel: lines, then first (choosing it calls
// do), changing each of its answers, logging in or out (its parts),
// uninstalling it and forgetting it. Any other value does nothing.
func (b *Board) itemPanel(i *BoardItem, lines []string, first [2]string, do func() tea.Cmd) *Dialog {
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
	return &Dialog{Title: short(i.Label) + ": " + stateWord(i.State), Lines: lines, Opts: opts,
		On: func(v string) tea.Cmd {
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
	b.dialog = &Dialog{Title: title, Lines: lines, YN: true,
		Opts: append(opts, [2]string{keep, "no"}),
		On: func(v string) tea.Cmd {
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
	b.dialog = &Dialog{Title: title, Lines: lines, Opts: opts,
		On: func(v string) tea.Cmd {
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
	m := tui.Cur.Mark
	switch {
	case i.State == "down" || i.State == "missing":
		return m.Down, red
	case i.State == "partial" || i.State == "drifted" || i.State == "outdated":
		return m.Partial, yellow
	case i.State == "secret":
		return m.Need, accent
	case i.State == "unchecked":
		return m.Unchecked, muted
	case len(i.Updates) > 0:
		return m.Update, yellow
	}
	return m.OK, green
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

func padTo(s string, w int) string {
	s = runewidth.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(0, w-runewidth.StringWidth(s)))
}

// health is how the installed tools are, in a few words.
func (b *Board) health() string {
	if b.first {
		return muted.Render("nothing installed yet: a installs what's queued")
	}
	var right []string
	// Health counts its rows: a part's is in its tool's.
	own := func(f func(*BoardItem) bool) func(*BoardItem) bool {
		return func(i *BoardItem) bool { return !b.inRow(i) && f(i) }
	}
	if n := b.count(own(fixable)); n > 0 {
		right = append(right, red.Render(fmt.Sprintf("%s %d to fix", tui.Cur.Mark.Down, n)))
	}
	if n := b.count(own(secret)); n > 0 {
		right = append(right, accent.Render(fmt.Sprintf("%s %d to fill in", tui.Cur.Mark.Need, n)))
	}
	if n := b.count(own(newer)); n > 0 {
		right = append(right, yellow.Render(fmt.Sprintf("%s %d with updates", tui.Cur.Mark.Update, n)))
	}
	if len(right) == 0 {
		right = append(right, green.Render(tui.Cur.Mark.OK+" all well"))
	}
	if b.info.Checked != "" {
		right = append(right, muted.Render("updates checked "+b.info.Checked))
	}
	return strings.Join(right, muted.Render(" · "))
}

// tabBar is the tabs, each with its worst mark, and the health at the
// right; a click on a tab opens it.
func (b *Board) tabBar(w, y int) string {
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
	line, x := " ", 1
	for n, t := range tabs {
		m := tui.Cur.Mark
		label := " " + tui.Title(t.Title) + [...]string{"", " " + m.Update, " " + m.Need, " " + m.Down}[worst[t.ID]] + " "
		lw := runewidth.StringWidth(label)
		if x+lw > w-1 {
			break
		}
		if n > 0 {
			line += " "
			x++
		}
		if n == cur && b.search == nil {
			line += tabOn.Render(label)
		} else {
			line += muted.Render(label)
		}
		id := t.ID
		b.hits = append(b.hits, Hit{Y0: y, Y1: y, X0: x, X1: x + lw, Do: func() {
			b.search, b.typing, b.tabID = nil, false, id
		}})
		x += lw
	}
	if h := b.health(); lipgloss.Width(line)+2+lipgloss.Width(h) < w {
		line += strings.Repeat(" ", w-1-lipgloss.Width(line)-lipgloss.Width(h)) + h
	}
	return line
}

// queueSay is the queue in words: install Java · uninstall NetBird.
func (b *Board) queueSay() string {
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
	return strings.Join(q, " · ")
}

// Path is where the board is: Home › Tools › its tab (or the search).
func (b *Board) Path() string {
	p := "Tools › "
	if b.fromMenu {
		p = "Home › " + p
	}
	if b.search != nil {
		return p + "Search"
	}
	tabs := b.visibleTabs()
	return p + tabs[b.current(tabs)].Title
}

// Back is what q does.
func (b *Board) Back() string {
	switch {
	case b.dialog != nil:
		return "cancel"
	case b.search != nil && b.typing:
		return ""
	case b.search != nil:
		return "clear the filter"
	case b.fromMenu:
		return "home"
	}
	return "quit"
}

// Keys are the footer's: what each key does on the row.
func (b *Board) Keys() []Key {
	if b.dialog != nil {
		return dialogKeys(b.dialog)
	}
	if b.search != nil && b.typing {
		return []Key{{Key: "enter", What: "into the list"}, {Key: "space", What: "add/uninstall"}, {Key: "esc", What: "clear"}}
	}
	_, r := b.selected(b.rows())
	var keys []Key
	first := Key{Key: "enter", What: "details"}
	switch {
	case r == nil:
	case r.item.Pending != "":
		first = Key{Key: "space", What: "cancel"}
	case !r.item.Picked:
		first = Key{Key: "space", What: "add"}
	case fixable(r.item):
		first = Key{Key: "enter", What: "fix"}
	case secret(r.item):
		first = Key{Key: "enter", What: "fill in"}
	case len(r.item.Changes) > 0:
		first = Key{Key: "enter", What: "change"}
	}
	keys = append(keys, first)
	if r != nil && r.item != nil && r.item.Picked && r.item.Pending == "" {
		keys = append(keys, Key{Key: "space", What: "uninstall"}, Key{Key: "d", What: "uninstall"})
	}
	if n := b.queued(); n > 0 {
		keys = append(keys, Key{Key: "a", What: fmt.Sprintf("APPLY %d", n), On: true})
	}
	return append(keys, Key{Key: "←→", What: "tabs"}, Key{Key: "/", What: "search"})
}

// Help are every key the board takes, for the keys screen.
func (b *Board) Help() []Key {
	return []Key{
		{Key: "space", What: "tick a tool to install, untick one to uninstall; again takes it off the queue"},
		{Key: "enter", What: "the row's details: fix, fill in, change, reinstall, log in, forget"},
		{Key: "a", What: "apply the queue, asked first"},
		{Key: "d", What: "uninstall the row, asked first"},
		{Key: "/", What: "search (or just type, from the home)"},
		{Key: "←→ h l", What: "the previous or next tab"},
		{Key: "tab", What: "the next tab"},
		{Key: "ctrl+a", What: "in a search with no match: add it as an extra mise tool"},
	}
}

// Toast is the line the board says above its keys.
func (b *Board) Toast() string { return b.flash }

// Hits are where the last Body drew what a click selects.
func (b *Board) Hits() []Hit { return b.hits }

// Typing: the search field has the keys.
func (b *Board) Typing() bool { return b.search != nil && b.typing && b.dialog == nil }

// View is the board drawn alone, as the app draws it.
func (b *Board) View() string { return standalone(b, b.info, b.width, b.height) }

// Body is the board under the header, w by h: the tabs, the search, the
// list, a dialog, the queue.
func (b *Board) Body(w, h int) string {
	b.hits = nil
	rule := muted.Render(strings.Repeat("─", w-1))
	var out []string
	out = append(out, b.tabBar(w, 0))
	rs := b.rows()
	// The search, at the top, by what it found.
	if b.search != nil {
		cursor, hint := " ", "enter: into the list · esc clears"
		if b.typing {
			cursor = "▏"
		} else {
			hint = "/ edits · esc clears"
		}
		found := 0
		for _, x := range rs {
			if x.selectable() {
				found++
			}
		}
		q := runewidth.Truncate(*b.search, 40, "…") + cursor
		out = append(out, " "+accentBd.Render("Search ")+field.Render(" "+q+strings.Repeat(" ", max(1, 26-runewidth.StringWidth(q))))+"  "+
			muted.Render(plural(found, "match", "matches")+" · "+hint))
	}
	out = append(out, rule)
	head := len(out)

	cur, _ := b.selected(rs)
	var pl []string
	// The queue, on a line of its own above the keys.
	var ql []string
	if b.queued() > 0 && b.dialog == nil {
		ql = append(ql, " "+accentBd.Render("Queued: ")+runewidth.Truncate(b.queueSay(), max(10, w-24), "…")+muted.Render("   a applies"))
	}
	if b.dialog != nil {
		pl = dialogLines(b.dialog, w, 0, nil)
	}
	listH := max(1, h-head-len(pl)-len(ql))
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
	top := 0
	if cur >= listH {
		top = cur - listH + 1
	}
	for n := top; n < len(rs) && len(out) < head+listH; n++ {
		x := rs[n]
		switch {
		case x.head != "":
			out = append(out, " "+accentBd.Render(tui.Title(x.head)))
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
		// One cursor, as the menu's: the row in the accent colour, › at
		// its start.
		lead := "   "
		if n == cur && b.dialog == nil {
			lead = " › "
		}
		plain := fmt.Sprintf("%s%s %s %s  ", lead, box, mk, padTo(label, labelW))
		noteW := max(4, w-runewidth.StringWidth(plain)-1)
		k := n
		b.hits = append(b.hits, Hit{Y0: len(out), Y1: len(out), Do: func() { b.cursor[b.key()], b.typing = k, false }})
		// Each row a line, the selected one's note cut too: wrapped, it
		// would move the rows below as the selection moves.
		if n == cur && b.dialog == nil {
			out = append(out, rowOn.Render(padTo(plain+runewidth.Truncate(note, noteW, "…"), w-1)))
		} else {
			out = append(out, fmt.Sprintf("%s%s %s %s  ", lead, box, ms.Render(mk), padTo(label, labelW))+ns.Render(runewidth.Truncate(note, noteW, "…")))
		}
	}
	for len(out) < head+listH {
		out = append(out, "")
	}
	if b.dialog != nil {
		b.hits = nil // the dialog's options only
		pl = dialogLines(b.dialog, w, len(out), &b.hits)
	}
	out = append(out, pl...)
	out = append(out, ql...)
	return strings.Join(out, "\n")
}
