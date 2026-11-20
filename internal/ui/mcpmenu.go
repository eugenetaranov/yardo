package ui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/mcp"
)

// The MCP servers screens: the list (yardo's servers, then the ones found
// in ~/.mcp, then Add a server), one server's actions, deleting one, the
// catalog to add from, and a new server's name (or a renamed one's).

// Open shows screen first (the MCP servers), with toast; fromBoard: esc
// on it goes back to the board.
func (m *Menu) Open(screen, toast string, fromBoard bool) *Menu {
	m.screen, m.toast, m.fromBoard = screen, toast, fromBoard
	m.sortMCP()
	m.normalize()
	return m
}

func (m *Menu) hasMCP() bool {
	return slices.ContainsFunc(m.items, func(i *BoardItem) bool { return i.ID == "mcpjungle" && i.Picked })
}

// sortMCP puts the servers in the list's order, once (when the menu is
// made, or the list opened): yardo's, then the found ones, each by name.
func (m *Menu) sortMCP() {
	m.info.MCP = slices.Clone(m.info.MCP)
	slices.SortStableFunc(m.info.MCP, func(a, b MCPServer) int {
		if a.Yardo != b.Yardo {
			if a.Yardo {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// mcpServers are the list's servers, in sortMCP's order.
func (m *Menu) mcpServers() []*MCPServer {
	out := make([]*MCPServer, len(m.info.MCP))
	for n := range m.info.MCP {
		out[n] = &m.info.MCP[n]
	}
	return out
}

func (m *Menu) mcpServer() *MCPServer {
	ss := m.mcpServers()
	if m.mcpSel < 0 || m.mcpSel >= len(ss) {
		return nil
	}
	return ss[m.mcpSel]
}

// Switcher turns servers off or on as acts say (an act's Entry ""
// is all of them), then says how the servers are now (and whether all are
// off), and what happened.
type Switcher func(acts []MCPAct) (servers []MCPServer, allOff bool, toast string, err error)

// Switching has the menu save the servers turned off and on itself, in
// the background, with f.
func (m *Menu) Switching(f Switcher) *Menu {
	m.switcher = f
	return m
}

// mcpSwitched is a save done: the servers as they are now.
type mcpSwitched struct {
	servers []MCPServer
	allOff  bool
	toast   string
	err     error
}

// stage marks act to save (Save, or s): the screen shows it at once.
// Marking a server back as it's saved unmarks it.
func (m *Menu) stage(act MCPAct) {
	if m.pend == nil {
		m.pend, m.before = map[string]string{}, m.info
		m.before.MCP = slices.Clone(m.info.MCP)
	}
	m.showSwitch(act)
	saved := m.before.MCPAllOff
	now := m.info.MCPAllOff
	if act.Entry != "" {
		i := slices.IndexFunc(m.before.MCP, func(s MCPServer) bool { return s.Entry == act.Entry })
		saved = i >= 0 && m.before.MCP[i].State == "off" && m.before.MCP[i].Off == "own"
		now = act.Do == "off"
	}
	if now == saved {
		delete(m.pend, act.Entry)
	} else {
		m.pend[act.Entry] = act.Do
	}
	if len(m.pend) == 0 {
		m.unstage()
	}
}

// unstage drops what's marked: the servers as they're saved.
func (m *Menu) unstage() {
	if m.pend != nil {
		m.info.MCP, m.info.MCPAllOff = m.before.MCP, m.before.MCPAllOff
	}
	m.pend = nil
}

// staged are the marked acts, in the order to save them: all on first,
// then each server, then all off.
func (m *Menu) staged() []MCPAct {
	var out []MCPAct
	if m.pend[""] == "on" {
		out = append(out, MCPAct{Do: "on"})
	}
	for _, e := range slices.Sorted(maps.Keys(m.pend)) {
		if e != "" {
			out = append(out, MCPAct{Do: m.pend[e], Entry: e})
		}
	}
	if m.pend[""] == "off" {
		out = append(out, MCPAct{Do: "off"})
	}
	return out
}

// save applies what's marked, in the background: the menu stays.
func (m *Menu) save() tea.Cmd {
	switch {
	case len(m.pend) == 0:
		m.toast = "Nothing to save: space turns a server off or on."
		return nil
	case m.switching:
		m.toast = "Saving: a moment."
		return nil
	case m.switcher == nil:
		m.toast = "Can't save from here: sudo yardo mcp off|on <server>."
		return nil
	}
	m.switching = true
	acts, f := m.staged(), m.switcher
	m.toast = "Saving…"
	return func() tea.Msg {
		servers, allOff, toast, err := f(acts)
		return mcpSwitched{servers, allOff, toast, err}
	}
}

// showSwitch is act on the screen, before it's saved.
func (m *Menu) showSwitch(act MCPAct) {
	set := func(s *MCPServer, off bool, how string) {
		switch {
		case off && s.State != "off":
			s.was, s.State, s.Off = s.State, "off", how
		case !off && s.State == "off" && (act.Entry != "" || s.Off == "all"):
			s.State, s.Off = cmp.Or(s.was, "ok"), ""
		}
	}
	for _, s := range m.mcpServers() {
		switch {
		case !s.Yardo:
		case act.Entry == "":
			set(s, act.Do == "off", "all")
		case s.Entry == act.Entry:
			set(s, act.Do == "off", "own")
		}
	}
	if act.Entry == "" {
		m.info.MCPAllOff = act.Do == "off"
	}
}

// switched takes a save's outcome: the servers as they are now, or as
// they were before, when it failed.
func (m *Menu) switched(v mcpSwitched) {
	m.switching = false
	switch {
	case v.err != nil && v.servers == nil:
		m.unstage()
		m.toast = "✗ " + v.err.Error()
	case v.err != nil:
		// Part applied: the rows say how far.
		m.pend = nil
		m.info.MCP, m.info.MCPAllOff, m.toast = v.servers, v.allOff, "✗ "+v.err.Error()
	default:
		// Where they run from stays as it was read.
		from := map[string]MCPServer{}
		for _, s := range m.info.MCP {
			from[s.Name] = s
		}
		for n := range v.servers {
			f := from[v.servers[n].Name]
			v.servers[n].From, v.servers[n].Cached = f.From, f.Cached
		}
		m.pend = nil
		m.info.MCP, m.info.MCPAllOff, m.toast = v.servers, v.allOff, v.toast
	}
	m.sortMCP()
	m.normalize()
}

// lastRow is the list's last row: Save (with changes to save), else
// Add a server.
func (m *Menu) lastRow() int {
	if len(m.pend) > 0 {
		return len(m.mcpServers()) + 1
	}
	return len(m.mcpServers())
}

// hasAll says whether the list has its All servers row (above the
// servers, m.sel -1): there's a server of yardo's to turn off.
func (m *Menu) hasAll() bool {
	return slices.ContainsFunc(m.info.MCP, func(s MCPServer) bool { return s.Yardo })
}

// switchAct is space's act on the list's row sel (-1: All servers), or
// why there's none.
func (m *Menu) switchAct(sel int) (MCPAct, string) {
	if sel < 0 {
		if m.info.MCPAllOff {
			return MCPAct{Do: "on"}, ""
		}
		return MCPAct{Do: "off"}, ""
	}
	s := m.mcpServers()[sel]
	switch {
	case !s.Yardo:
		return MCPAct{}, ""
	case m.info.MCPAllOff:
		return MCPAct{}, "All servers are off: turn them on first (the All servers row)."
	case s.State == "off":
		return MCPAct{Do: "on", Entry: s.Entry}, ""
	}
	return MCPAct{Do: "off", Entry: s.Entry}, ""
}

func (m *Menu) isSpec(n string) bool {
	return slices.ContainsFunc(m.info.MCPSpecs, func(s MCPSpec) bool { return s.Name == n })
}

// taken are the names of the servers there are.
func (m *Menu) taken() []string {
	var out []string
	for _, s := range m.info.MCP {
		out = append(out, s.Name)
	}
	return out
}

// using are the servers of spec there are, by name.
func (m *Menu) using(spec string) []string {
	var out []string
	for _, s := range m.mcpServers() {
		if s.Spec == spec {
			out = append(out, s.Name)
		}
	}
	return out
}

// mcpAction is one of a server's actions: what it does, its label, a note.
type mcpAction struct{ id, label, note string }

func (m *Menu) mcpActions(s *MCPServer) []mcpAction {
	var out []mcpAction
	if s.Yardo && (s.State == "failed" || s.State == "pending") && !m.info.MCPDown {
		out = append(out, mcpAction{"retry", "Try again", "registers it as its settings are now"})
	}
	switch {
	case !s.Yardo && s.State == "unlisted":
		out = append(out, mcpAction{"adopt", "Add to yardo's list", "and registers it"})
	case s.Yardo && m.info.MCPAllOff:
		// Back on with all of them, from the list.
	case s.Yardo && s.State == "off":
		out = append(out, mcpAction{"on", "Turn it on", "agents see its tools again; its settings are as they were"})
	case s.Yardo:
		out = append(out, mcpAction{"off", "Turn it off", "keeps its settings; agents stop seeing its tools"})
	}
	out = append(out, mcpAction{"edit", "Edit its settings", "opens ~/.mcp/" + s.Name + "/env, then registers it"})
	if !m.info.MCPDown {
		if s.State != "nospec" && s.State != "retired" {
			out = append(out, mcpAction{"rename", "Rename it", "its settings move with it"})
		}
		out = append(out, mcpAction{"delete", "Delete it", "from the gateway, with its settings"})
	}
	return append(out, mcpAction{"back", "Back", ""})
}

// mcpWord is how a server's state reads, styled.
func mcpWord(s *MCPServer) string {
	switch s.State {
	case "ok":
		return green.Render("● ready")
	case "secret":
		if len(s.Keys) > 2 {
			return accent.Render(fmt.Sprintf("○ needs %d values", len(s.Keys)))
		}
		return accent.Render("○ needs " + strings.Join(s.Keys, ", "))
	case "failed":
		return yellow.Render("▲ didn't register: check its settings")
	case "pending":
		return accent.Render("○ not registered yet")
	case "nospec":
		return yellow.Render("▲ no longer in the catalog")
	case "off":
		if s.Off == "all" {
			return muted.Render("○ off with all")
		}
		return muted.Render("○ off")
	case "unlisted":
		return muted.Render("· not on yardo's list")
	case "byhand":
		return green.Render("● on, added by hand")
	case "new":
		return muted.Render("· not set up yet: applies next")
	case "retired":
		return yellow.Render("▲ retired: add " + s.Instead + " instead")
	}
	return muted.Render("· not checked")
}

func mcpType(s *MCPServer) string {
	if s.Spec == "" {
		return "own spec"
	}
	return s.Spec
}

// fromLine is where s runs from, and which of its image is here.
func fromLine(s *MCPServer) string {
	if s.Cached == "" {
		return "from " + s.From
	}
	return "from " + s.From + " · " + s.Cached
}

// examples names a few servers to add, and how many more there are.
func (m *Menu) examples() string {
	n := len(m.info.MCPSpecs)
	if n <= 3 {
		return "GitHub, Jira, Grafana"
	}
	return fmt.Sprintf("GitHub, Jira, Grafana and %d more", n-3)
}

func (m *Menu) mcpKey(k string, up, down, left, right bool) tea.Cmd {
	switch m.screen {
	case "mcp":
		n := len(m.mcpServers()) // the Add row is n, All servers -1
		top := 0
		if m.hasAll() {
			top = -1
		}
		leaving := m.leaving
		m.leaving = false
		switch {
		case (k == "esc" || k == "q") && len(m.pend) > 0 && !leaving:
			m.leaving = true
			m.toast = plural(len(m.pend), "change isn't saved", "changes aren't saved") + ": s saves, q again leaves without them."
		case k == "esc" || k == "q":
			m.unstage()
			if m.fromBoard {
				return m.quit(MenuResult{Kind: MenuTools})
			}
			m.back()
		case k == "s" || (k == "enter" && m.sel > n):
			return m.save()
		case up:
			m.sel = max(top, m.sel-1)
		case down:
			m.sel = min(m.lastRow(), m.sel+1)
		case k == " " || (k == "enter" && m.sel < 0):
			if m.sel >= n || m.switching {
				return nil
			}
			act, why := m.switchAct(m.sel)
			if why != "" {
				m.toast = why
			}
			if act.Do != "" {
				m.stage(act)
			}
		case m.sel < 0:
		case k == "e" && m.sel < n:
			s := m.mcpServers()[m.sel]
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "edit", Entry: s.Entry}})
		case k == "d" && m.sel < n:
			// Delete, asked first (Keep it chosen); esc comes back here.
			if !slices.ContainsFunc(m.mcpActions(m.mcpServers()[m.sel]), func(a mcpAction) bool { return a.id == "delete" }) {
				m.toast = "The gateway is stopped: start it first (Fix), then delete."
				return nil
			}
			m.mcpSel = m.sel
			m.screen, m.btn, m.delList = "mcp:delete", 1, true
		case k == "a" || (k == "enter" && m.sel == n):
			m.mcpSel = n
			m.screen, m.sel = "mcp:add", m.addSel
		case k == "enter":
			m.mcpSel = m.sel
			m.screen, m.sel = "mcp:server", 0
		}
	case "mcp:server":
		s := m.mcpServer()
		if s == nil {
			m.screen, m.sel = "mcp", 0
			return nil
		}
		as := m.mcpActions(s)
		switch {
		case k == "esc" || k == "q":
			m.screen, m.sel = "mcp", m.mcpSel
		case up:
			m.sel = max(0, m.sel-1)
		case down:
			m.sel = min(len(as)-1, m.sel+1)
		case k == "enter":
			switch as[m.sel].id {
			case "back":
				m.screen, m.sel = "mcp", m.mcpSel
			case "rename":
				m.screen, m.nameFor, m.renaming, m.name, m.nameErr = "mcp:name", s.Entry, true, []rune(s.Name), ""
			case "delete":
				m.screen, m.btn, m.delList = "mcp:delete", 1, false // Keep it, first
			case "off", "on":
				m.stage(MCPAct{Do: as[m.sel].id, Entry: s.Entry})
				m.screen, m.sel = "mcp", m.mcpSel
			default:
				return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: as[m.sel].id, Entry: s.Entry}})
			}
		}
	case "mcp:delete":
		s := m.mcpServer()
		keep := func() {
			m.screen = "mcp:server"
			if m.delList {
				m.screen, m.sel = "mcp", m.mcpSel
			}
		}
		switch {
		case k == "esc" || k == "q" || k == "n":
			keep()
		case left:
			m.btn = 0
		case right:
			m.btn = 1
		case (k == "y" || k == "enter" && m.btn == 0) && s != nil:
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "delete", Entry: s.Entry}})
		case k == "enter":
			keep()
		}
	case "mcp:add":
		specs := m.info.MCPSpecs
		switch {
		case k == "esc" || k == "q":
			m.addSel = m.sel
			m.screen, m.sel = "mcp", len(m.mcpServers())
		case up:
			m.sel = max(0, m.sel-1)
		case down:
			m.sel = max(0, min(len(specs)-1, m.sel+1))
		case k == "enter" && len(specs) > 0:
			m.addSel = m.sel
			spec := specs[m.sel].Name
			m.screen, m.nameFor, m.renaming, m.nameErr = "mcp:name", spec, false, ""
			m.name = nil
			if len(m.using(spec)) == 0 && !slices.Contains(m.taken(), spec) {
				m.name = []rune(spec)
			}
		}
	}
	return nil
}

// nameKey types the name: every key is text but enter, esc, backspace.
func (m *Menu) nameKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m.quit(MenuResult{Kind: MenuQuit})
	case tea.KeyEsc:
		if m.renaming {
			m.screen = "mcp:server"
		} else {
			m.screen, m.sel = "mcp:add", m.addSel
		}
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.name) > 0 {
			m.name = m.name[:len(m.name)-1]
		}
		m.nameErr = ""
	case tea.KeyRunes:
		m.name = append(m.name, msg.Runes...)
		m.nameErr = ""
	case tea.KeyEnter:
		entry, err := m.nameEntry()
		if err != nil {
			m.nameErr = sentence(err.Error())
			return nil
		}
		if m.renaming {
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "rename", Entry: m.nameFor, To: entry}})
		}
		return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "add", Entry: entry}})
	}
	return nil
}

// nameEntry is the entry the name typed makes, or why it can't.
func (m *Menu) nameEntry() (string, error) {
	if m.renaming {
		isSpec := m.isSpec
		if s := m.mcpServer(); s != nil && s.Spec == "" {
			// Its own spec, even named after a catalog one: any name.
			isSpec = func(n string) bool { return n != s.Entry && m.isSpec(n) }
		}
		return mcp.RenameEntry(m.nameFor, string(m.name), isSpec, m.taken())
	}
	return mcp.EntryFor(m.nameFor, string(m.name), m.isSpec, m.taken())
}

// sentence is msg with a capital and a full stop.
func sentence(msg string) string {
	if msg == "" {
		return ""
	}
	return strings.ToUpper(msg[:1]) + msg[1:] + "."
}

// window keeps line sel of lines in view, at most n of them.
func window(lines []string, sel, n int) []string {
	if len(lines) <= n || n <= 0 {
		return lines
	}
	start := min(max(0, sel-n/2), len(lines)-n)
	return lines[start : start+n]
}

// mcpAbout is how many lines the MCP servers screen keeps for the chosen
// server's details.
const mcpAbout = 3

// mcpView is the MCP screens' title, lines and keys; room is how many
// lines the screen has for them.
func (m *Menu) mcpView(w, room int) (string, []string, string) {
	k := func(key, what string) string { return keyOn.Render(key) + " " + what }
	var b []string
	add := func(s ...string) { b = append(b, s...) }
	switch m.screen {
	case "mcp":
		ss := m.mcpServers()
		if m.info.MCPDown {
			add("  "+yellow.Render("▲  The gateway is stopped: fix it from Home first."), "")
		}
		if len(ss) == 0 {
			add("  No MCP servers yet. Each one gives your coding agents tools:", "  "+m.examples()+".", "")
		}
		var lines []string
		selLine := 0
		if m.hasAll() {
			on, all := 0, 0
			for _, s := range ss {
				if s.Yardo {
					all++
					if s.State != "off" {
						on++
					}
				}
			}
			word := green.Render("● on") + muted.Render(fmt.Sprintf(" · %d of %d", on, all))
			if m.info.MCPAllOff {
				word = accent.Render("○ off") + muted.Render(" · space turns them back on")
			}
			if _, marked := m.pend[""]; marked {
				word += accent.Render("  · not saved")
			}
			if m.sel < 0 {
				lines = append(lines, "  "+rowOn.Render(padTo(" › All servers", 21))+"  "+word)
			} else {
				lines = append(lines, "     "+padTo("All servers", 18)+"  "+word)
			}
			lines = append(lines, "")
		}
		heading := false
		for n, s := range ss {
			if !s.Yardo && !heading {
				heading = true
				lines = append(lines, "", "     "+muted.Render("Also in ~/.mcp (not on yardo's list)"))
			}
			cols := padTo(mcpType(s), 13) + "  " + mcpWord(s)
			name := padTo(s.Name, 18)
			if s.Yardo && m.info.MCPAllOff {
				// Dimmed: all are off; these come back with them.
				cols, name = muted.Render(padTo(mcpType(s), 13))+"  "+mcpWord(s), muted.Render(name)
			}
			if _, marked := m.pend[s.Entry]; marked {
				cols += accent.Render("  · not saved")
			}
			if n == m.sel {
				selLine = len(lines)
				lines = append(lines, "  "+rowOn.Render(padTo(" › "+s.Name, 21))+"  "+cols)
			} else {
				lines = append(lines, "     "+name+"  "+cols)
			}
		}
		lines = append(lines, "")
		if m.sel == len(ss) {
			selLine = len(lines)
			lines = append(lines, "  "+rowOn.Render(padTo(" › + Add a server", 21))+"  "+muted.Render(m.examples()))
		} else {
			lines = append(lines, "     "+padTo("+ Add a server", 18)+"  "+muted.Render(m.examples()))
		}
		if len(m.pend) > 0 {
			// The button: what's marked takes effect when it's pressed.
			label := "Save " + plural(len(m.pend), "change", "changes")
			lines = append(lines, "")
			if m.sel > len(ss) {
				selLine = len(lines)
				lines = append(lines, "  "+rowOn.Render(" "+label+" ")+"  "+muted.Render("or s, from anywhere here"))
			} else {
				lines = append(lines, "  "+muted.Render("[ ")+label+muted.Render(" ]")+"  "+muted.Render("s"))
			}
		}
		// The chosen server's details, under the list, in lines that don't
		// change in number: the rows never move.
		var about []string
		if m.sel < 0 {
			about = []string{"Turns every server off at once (agents see none of their tools), and back on: each as you left it."}
		} else if m.sel < len(ss) {
			s := ss[m.sel]
			about = wrap(s.About, max(20, w-8))
			if len(about) > mcpAbout-1 {
				about = append(about[:mcpAbout-2], runewidth.Truncate(strings.Join(about[mcpAbout-2:], " "), max(20, w-8), "…"))
			}
			if s.From != "" {
				about = append(about, runewidth.Truncate(fromLine(s), max(20, w-8), "…"))
			}
		}
		for len(about) < mcpAbout {
			about = append(about, "")
		}
		for n, l := range about {
			if l != "" {
				about[n] = "     " + muted.Render(l)
			}
		}
		add(window(lines, selLine, room-len(b)-3-mcpAbout)...)
		add("")
		add(about...)
		add("", "  "+muted.Render("Agents reach their tools through the gateway as <name>__<tool>."))
		keys := []string{k("↑↓", "choose"), k("space", "on/off")}
		if len(m.pend) > 0 {
			keys = append(keys, k("s", "save"))
		}
		keys = append(keys, k("enter", "open"), k("e", "edit its settings"), k("d", "delete"), k("a", "add"), k("q", "back"))
		return "MCP servers", b, strings.Join(keys, "   ")
	case "mcp:server", "mcp:delete":
		s := m.mcpServer()
		if s == nil { // normalize goes back to the list first
			return "MCP servers", nil, k("q", "back")
		}
		what := "a " + s.Spec + " server"
		if s.Spec == "" {
			what = "with its own spec"
		}
		if !s.Yardo {
			what += ", found in ~/.mcp"
		}
		add("  " + bold.Render(s.Name) + muted.Render(" · "+what))
		if s.About != "" {
			add("  " + s.About)
		}
		if s.Note != "" {
			for _, l := range wrap(s.Note, w-4) {
				add("  " + l)
			}
		}
		if s.From != "" {
			add("  " + muted.Render(fromLine(s)))
		}
		add("  " + mcpWord(s))
		if s.Detail != "" {
			for _, l := range wrap(s.Detail, w-6) {
				add("    " + yellow.Render(l))
			}
		}
		add("  "+muted.Render("Settings in ~/.mcp/"+s.Name+"/env · tools show as "+s.Name+"__<tool>"), "")
		title := "MCP servers  ›  " + s.Name
		if m.screen == "mcp:delete" {
			btn := func(n int, label string) string {
				if m.btn == n {
					return rowOn.Render(" " + label + " ")
				}
				return muted.Render("[ ") + label + muted.Render(" ]")
			}
			add("  "+bold.Render("Delete "+s.Name+"?"),
				"  It leaves the gateway, and ~/.mcp/"+s.Name+" (its settings and keys) is deleted.",
				"  This can't be undone.", "",
				"  "+btn(0, "Delete (y)")+"   "+btn(1, "Keep it (n)"))
			return title, b, strings.Join([]string{k("y", "delete"), k("n", "keep it"), k("←→", "choose"), k("enter", "do it")}, "   ")
		}
		as := m.mcpActions(s)
		for n, a := range as {
			if n == m.sel {
				add("  " + rowOn.Render(padTo(" › "+a.label, 24)) + "  " + muted.Render(a.note))
			} else {
				add("     " + padTo(a.label, 21) + "  " + muted.Render(a.note))
			}
		}
		return title, b, strings.Join([]string{k("↑↓", "choose"), k("enter", "do it"), k("q", "back")}, "   ")
	case "mcp:add":
		specs := m.info.MCPSpecs
		var lines []string
		for n, s := range specs {
			// One line each, the chosen one too: a long description
			// wrapped would move the rows below as the choice moves.
			about := muted.Render(runewidth.Truncate(s.About, max(10, w-27), "…"))
			if n == m.sel {
				lines = append(lines, "  "+rowOn.Render(padTo(" › "+s.Name, 21))+"  "+about)
			} else {
				lines = append(lines, "     "+padTo(s.Name, 18)+"  "+about)
			}
		}
		add(window(lines, m.sel, room-3)...)
		add("", "  "+muted.Render("Adding one again makes another: a second Jira, another AWS account."))
		return "MCP servers  ›  Add a server", b, strings.Join([]string{k("↑↓", "choose"), k("enter", "next"), k("q", "back")}, "   ")
	case "mcp:name":
		title, verb := "MCP servers  ›  Add "+m.nameFor, "add"
		if m.renaming {
			_, n, _ := mcp.Instance(m.nameFor)
			title, verb = "MCP servers  ›  Rename "+n, "rename"
		}
		add("  "+bold.Render("Name it"), "  "+accentBd.Render("› ")+string(m.name)+accent.Render("_"))
		if entry, err := m.nameEntry(); err == nil {
			_, n, _ := mcp.Instance(entry)
			add("    " + muted.Render("→ "+n+" · its tools show as "+n+"__<tool>, its settings go in ~/.mcp/"+n))
		} else {
			add("")
		}
		add("")
		if m.nameErr != "" {
			add("  " + yellow.Render(m.nameErr))
		} else if !m.renaming && len(m.using(m.nameFor)) > 0 {
			add("  " + muted.Render("There's a "+m.nameFor+" already: give this one a name such as work or prod."))
		} else if !m.renaming {
			add("  " + muted.Render("Enter keeps "+m.nameFor+"; another name, such as work, makes it "+m.nameFor+"-work."))
		}
		return title, b, strings.Join([]string{k("enter", verb), k("esc", "back")}, "   ")
	}
	return "", nil, ""
}
