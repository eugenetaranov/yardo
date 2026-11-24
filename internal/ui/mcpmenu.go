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
// in ~/.mcp, then Add a server), one server's actions, the catalog to add
// from, and a new server's name (or a renamed one's). Deleting one asks
// in a dialog.

// Open shows screen first (the MCP servers), with toast; fromBoard: q
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
// made, or the list opened): yardo's, the held, then the found ones, each
// by name.
func (m *Menu) sortMCP() {
	m.info.MCP = slices.Clone(m.info.MCP)
	// yardo's, then the ones held for a decision, then the found ones.
	group := func(s MCPServer) int {
		switch {
		case s.Yardo:
			return 0
		case s.State == "held":
			return 1
		}
		return 2
	}
	slices.SortStableFunc(m.info.MCP, func(a, b MCPServer) int {
		if d := group(a) - group(b); d != 0 {
			return d
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

// stage marks act to apply (a): the screen shows it at once.
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
		m.toast = "Nothing to apply: space turns a server off or on."
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

// lastRow is the list's last row: Add a server.
func (m *Menu) lastRow() int { return len(m.mcpServers()) }

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
	if s.State == "held" {
		return []mcpAction{{"review", "Review it", "its report: install, fix and check again, or decline"}, {"back", "Back", ""}}
	}
	var out []mcpAction
	if s.State == "off" && s.Off == "tools-changed" {
		out = append(out, mcpAction{"recheck", "Check it again", "its tools changed since you approved it: their names and descriptions are checked"})
	}
	if s.Update != "" {
		out = append(out, mcpAction{"update", "Update it", fmt.Sprintf("to %s: checked first, this one stays until you approve it", shortRef(s.Update))})
	}
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
	out = append(out, mcpAction{"edit", "Edit its settings", fmt.Sprintf("opens ~/.mcp/%s/env, then registers it", s.Name)})
	if m.canRestart(s) {
		out = append(out, mcpAction{"restart", "Restart it", "afresh: its Docker containers removed, then registered again"})
	}
	if !m.info.MCPDown {
		if s.State != "nospec" && s.State != "retired" {
			out = append(out, mcpAction{"rename", "Rename it", "its settings move with it"})
		}
		out = append(out, mcpAction{"delete", "Delete it", "from the gateway, with its settings"})
	}
	return append(out, mcpAction{"back", "Back", ""})
}

// canRestart says whether s can be started afresh: yardo's, registered
// (or tried), and the gateway running.
func (m *Menu) canRestart(s *MCPServer) bool {
	return s.Yardo && !m.info.MCPDown && !m.info.MCPAllOff && !slices.Contains([]string{"off", "secret", "new", "nospec", "retired", "refused"}, s.State)
}

// orphansOf says whose the orphaned containers are: jira ×2, aws.
func (m *Menu) orphansOf() string {
	count := map[string]int{}
	var names []string
	for _, n := range m.info.MCPOrphans {
		if count[n] == 0 {
			names = append(names, n)
		}
		count[n]++
	}
	for i, n := range names {
		if count[n] > 1 {
			names[i] = fmt.Sprintf("%s ×%d", n, count[n])
		}
	}
	return strings.Join(names, ", ")
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
	case "held":
		return yellow.Render("▲ held: waiting for you")
	case "refused":
		if s.Detail == "unapproved" {
			return yellow.Render("▲ refused: its spec changed since you approved it")
		}
		return yellow.Render("▲ refused: its spec asks for " + s.Detail)
	case "pending":
		return accent.Render("○ not registered yet")
	case "nospec":
		return yellow.Render("▲ no longer in the catalog")
	case "off":
		if s.Off == "all" {
			return muted.Render("○ off with all")
		}
		if s.Off == "tools-changed" {
			return yellow.Render("▲ off: its tools changed since you approved it")
		}
		return muted.Render("○ off")
	case "unlisted":
		return muted.Render("· not on yardo's list")
	case "byhand":
		return green.Render("● on, added by hand")
	case "new":
		return muted.Render("· not set up yet: applies next")
	case "retired":
		return yellow.Render(fmt.Sprintf("▲ retired: add %s instead", s.Instead))
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
	return fmt.Sprintf("from %s · %s", s.From, s.Cached)
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
			m.toast = plural(len(m.pend), "change isn't applied", "changes aren't applied") + ": a applies, q again leaves without them."
		case k == "esc" || k == "q":
			m.unstage()
			if m.fromBoard {
				return m.quit(MenuResult{Kind: MenuTools})
			}
			m.back()
		case k == "a":
			return m.save()
		case k == "c" && len(m.info.MCPOrphans) > 0:
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "cleanup"}})
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
		case k == "r" && m.sel < 0:
			if m.info.MCPDown || m.info.MCPAllOff {
				m.toast = "Nothing to restart: the gateway is stopped, or all servers are off."
				return nil
			}
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "restart"}})
		case k == "r" && m.sel < n:
			s := m.mcpServers()[m.sel]
			if !m.canRestart(s) {
				m.toast = fmt.Sprintf("Nothing to restart: %s isn't running here.", s.Name)
				return nil
			}
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "restart", Entry: s.Entry}})
		case m.sel < 0:
		case k == "e" && m.sel < n:
			s := m.mcpServers()[m.sel]
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "edit", Entry: s.Entry}})
		case k == "d" && m.sel < n:
			// Delete, asked first (Keep it chosen); q comes back here.
			if !slices.ContainsFunc(m.mcpActions(m.mcpServers()[m.sel]), func(a mcpAction) bool { return a.id == "delete" }) {
				m.toast = "The gateway is stopped: start it first (Fix), then delete."
				return nil
			}
			m.mcpSel = m.sel
			m.askDelete(m.mcpServers()[m.sel])
		case k == "n" || (k == "enter" && m.sel == n):
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
				m.askDelete(s)
			case "off", "on":
				m.stage(MCPAct{Do: as[m.sel].id, Entry: s.Entry})
				m.screen, m.sel = "mcp", m.mcpSel
			default:
				return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: as[m.sel].id, Entry: s.Entry}})
			}
		}
	case "mcp:add":
		specs := m.info.MCPSpecs
		switch {
		case k == "esc" || k == "q":
			m.addSel = m.sel
			m.screen, m.sel = "mcp", m.lastRow()
		case up:
			m.sel = max(0, m.sel-1)
		case down:
			m.sel = min(len(specs), m.sel+1)
		case k == "enter" && m.sel == len(specs):
			m.addSel = m.sel
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "elsewhere"}})
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

// askDelete asks before deleting s (Keep it chosen first).
func (m *Menu) askDelete(s *MCPServer) {
	entry := s.Entry
	m.dialog = &Dialog{Title: fmt.Sprintf("Delete %s?", s.Name), YN: true, Sel: 1,
		Lines: []string{fmt.Sprintf("It leaves the gateway, and ~/.mcp/%s (its settings and keys) is deleted.", s.Name), "This can't be undone."},
		Opts:  [][2]string{{"Delete (y)", "yes"}, {"Keep it (n)", "no"}},
		On: func(v string) tea.Cmd {
			if v != "yes" {
				return nil
			}
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "delete", Entry: entry}})
		}}
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
	return fmt.Sprintf("%s%s.", strings.ToUpper(msg[:1]), msg[1:])
}

// window keeps line sel of lines in view, at most n of them.
func window(lines []string, sel, n int) []string {
	start := windowStart(len(lines), sel, n)
	if start == 0 && (len(lines) <= n || n <= 0) {
		return lines
	}
	return lines[start : start+n]
}

// windowStart is the first of l lines window shows, keeping sel in view.
func windowStart(l, sel, n int) int {
	if l <= n || n <= 0 {
		return 0
	}
	return min(max(0, sel-n/2), l-n)
}

// mcpAbout is how many lines the MCP servers screen keeps for the chosen
// server's details.
const mcpAbout = 3

// mcpPath is where an MCP screen is, from the home.
func (m *Menu) mcpPath() string {
	switch m.screen {
	case "mcp:server":
		if s := m.mcpServer(); s != nil {
			return "MCP servers › " + s.Name
		}
	case "mcp:add":
		return "MCP servers › Add a server"
	case "mcp:name":
		if m.renaming {
			_, n, _ := mcp.Instance(m.nameFor)
			return "MCP servers › Rename " + n
		}
		return "MCP servers › Add " + m.nameFor
	}
	return "MCP servers"
}

// mcpKeys are an MCP screen's keys.
func (m *Menu) mcpKeys() []Key {
	switch m.screen {
	case "mcp:server":
		return []Key{{Key: "↑↓", What: "choose"}, {Key: "enter", What: "do it"}}
	case "mcp:add":
		return []Key{{Key: "↑↓", What: "choose"}, {Key: "enter", What: "next"}}
	case "mcp:name":
		return []Key{{Key: "enter", What: map[bool]string{true: "rename", false: "add"}[m.renaming]}, {Key: "esc", What: "back"}}
	}
	apply := "apply"
	if n := len(m.pend); n > 0 {
		apply = fmt.Sprintf("apply %d", n)
	}
	keys := []Key{{Key: "↑↓", What: "choose"}, {Key: "space", What: "on/off"}, {Key: "a", What: apply, On: len(m.pend) > 0},
		{Key: "enter", What: "open"}, {Key: "e", What: "edit its settings"}, {Key: "r", What: "restart"}, {Key: "d", What: "delete"}, {Key: "n", What: "add"}}
	if len(m.info.MCPOrphans) > 0 {
		// First, so a narrow footer keeps it.
		keys = slices.Insert(keys, 1, Key{Key: "c", What: "clean up", On: true})
	}
	return keys
}

// mcpView is the MCP screens' lines; room is how many lines the screen
// has for them. Its rows' hits are recorded from body line top.
func (m *Menu) mcpView(w, room int) []string {
	top := 1 // under the body's blank first line
	var b []string
	add := func(s ...string) { b = append(b, s...) }
	switch m.screen {
	case "mcp":
		ss := m.mcpServers()
		if m.info.MCPDown {
			add("  "+yellow.Render("▲  The gateway is stopped: fix it from Home first."), "")
		}
		if n := len(m.info.MCPOrphans); n > 0 {
			// A click on it cleans up, as c does.
			m.hits = append(m.hits, Hit{Y0: top + len(b), Y1: top + len(b), Key: "c"})
			add(fmt.Sprintf("  %s%s  %s", yellow.Render("▲  "+plural(n, "orphaned container", "orphaned containers")), muted.Render(fmt.Sprintf(" (%s): the gateway lost them, they still run.", m.orphansOf())), accentBd.Render("c cleans up")), "")
		}
		if len(ss) == 0 {
			add("  No MCP servers yet. Each one gives your coding agents tools:", fmt.Sprintf("  %s.", m.examples()), "")
		}
		var lines []string
		var rowOf []int
		row := func(n int, l string) {
			for len(rowOf) < len(lines) {
				rowOf = append(rowOf, -2)
			}
			rowOf = append(rowOf, n)
			lines = append(lines, l)
		}
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
				row(-1, fmt.Sprintf("  %s  %s", rowOn.Render(padTo(" › All servers", 21)), word))
			} else {
				row(-1, fmt.Sprintf("     %s  %s", padTo("All servers", 18), word))
			}
			lines = append(lines, "")
		}
		heading := false
		for n, s := range ss {
			if !s.Yardo && s.State != "held" && !heading {
				heading = true
				lines = append(lines, "", "     "+muted.Render("Also in ~/.mcp (not on yardo's list)"))
			}
			cols := fmt.Sprintf("%s  %s", padTo(mcpType(s), 13), mcpWord(s))
			name := padTo(s.Name, 18)
			if s.Yardo && m.info.MCPAllOff {
				// Dimmed: all are off; these come back with them.
				cols, name = fmt.Sprintf("%s  %s", muted.Render(padTo(mcpType(s), 13)), mcpWord(s)), muted.Render(name)
			}
			if _, marked := m.pend[s.Entry]; marked {
				cols += accent.Render("  · not saved")
			}
			if n == m.sel {
				selLine = len(lines)
				row(n, fmt.Sprintf("  %s  %s", rowOn.Render(padTo(" › "+s.Name, 21)), cols))
			} else {
				row(n, fmt.Sprintf("     %s  %s", name, cols))
			}
		}
		lines = append(lines, "")
		if m.sel == len(ss) {
			selLine = len(lines)
			row(len(ss), fmt.Sprintf("  %s  %s", rowOn.Render(padTo(" › + Add a server", 21)), muted.Render(m.examples())))
		} else {
			row(len(ss), fmt.Sprintf("     %s  %s", padTo("+ Add a server", 18), muted.Render(m.examples())))
		}
		if len(m.pend) > 0 {
			// What's marked takes effect with a.
			lines = append(lines, "", fmt.Sprintf("  %s  %s", accentBd.Render("a applies "+plural(len(m.pend), "change", "changes")), muted.Render("q leaves without them")))
		}
		for len(rowOf) < len(lines) {
			rowOf = append(rowOf, -2)
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
		n := room - len(b) - 3 - mcpAbout
		start := windowStart(len(lines), selLine, n)
		for c, l := range window(lines, selLine, n) {
			if r := rowOf[start+c]; r > -2 {
				m.hit(top+len(b), 0, 0, m.selects(r))
			}
			add(l)
		}
		add("")
		add(about...)
		add("", "  "+muted.Render("Agents reach their tools through the gateway as <name>__<tool>."))
		return b
	case "mcp:server":
		s := m.mcpServer()
		if s == nil { // normalize goes back to the list first
			return nil
		}
		what := fmt.Sprintf("a %s server", s.Spec)
		if s.Spec == "" {
			what = "with its own spec"
		}
		if !s.Yardo {
			what += ", found in ~/.mcp"
		}
		add(fmt.Sprintf("  %s%s", bold.Render(s.Name), muted.Render(" · "+what)))
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
		add("  "+muted.Render(fmt.Sprintf("Settings in ~/.mcp/%s/env · tools show as %s__<tool>", s.Name, s.Name)), "")
		as := m.mcpActions(s)
		for n, a := range as {
			m.hit(top+len(b), 0, 0, m.selects(n))
			if n == m.sel {
				add(fmt.Sprintf("  %s  %s", rowOn.Render(padTo(" › "+a.label, 24)), muted.Render(a.note)))
			} else {
				add(fmt.Sprintf("     %s  %s", padTo(a.label, 21), muted.Render(a.note)))
			}
		}
		return b
	case "mcp:add":
		specs := m.info.MCPSpecs
		var lines []string
		for n, s := range specs {
			// One line each, the chosen one too: a long description
			// wrapped would move the rows below as the choice moves.
			about := muted.Render(runewidth.Truncate(s.About, max(10, w-27), "…"))
			if n == m.sel {
				lines = append(lines, fmt.Sprintf("  %s  %s", rowOn.Render(padTo(" › "+s.Name, 21)), about))
			} else {
				lines = append(lines, fmt.Sprintf("     %s  %s", padTo(s.Name, 18), about))
			}
		}
		// Last, one from elsewhere: checked before it's installed.
		elsewhere := muted.Render("a Docker image, an https URL or a spec file: held and checked first")
		if m.sel == len(specs) {
			lines = append(lines, fmt.Sprintf("  %s  %s", rowOn.Render(padTo(" › From elsewhere", 21)), elsewhere))
		} else {
			lines = append(lines, fmt.Sprintf("     %s  %s", padTo("From elsewhere", 18), elsewhere))
		}
		start := windowStart(len(lines), m.sel, room-3)
		for c, l := range window(lines, m.sel, room-3) {
			m.hit(top+len(b), 0, 0, m.selects(start+c))
			add(l)
		}
		add("", "  "+muted.Render("Adding one again makes another: a second Jira, another AWS account."))
		return b
	case "mcp:name":
		add("  "+bold.Render("Name it"), fmt.Sprintf("  %s%s%s", accentBd.Render("› "), string(m.name), accent.Render("_")))
		if entry, err := m.nameEntry(); err == nil {
			_, n, _ := mcp.Instance(entry)
			add("    " + muted.Render(fmt.Sprintf("→ %s · its tools show as %s__<tool>, its settings go in ~/.mcp/%s", n, n, n)))
		} else {
			add("")
		}
		add("")
		if m.nameErr != "" {
			add("  " + yellow.Render(m.nameErr))
		} else if !m.renaming && len(m.using(m.nameFor)) > 0 {
			add("  " + muted.Render(fmt.Sprintf("There's a %s already: give this one a name such as work or prod.", m.nameFor)))
		} else if !m.renaming {
			add("  " + muted.Render(fmt.Sprintf("Enter keeps %s; another name, such as work, makes it %s-work.", m.nameFor, m.nameFor)))
		}
		return b
	}
	return nil
}
