package ui

import (
	"fmt"
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
	return m
}

func (m *Menu) hasMCP() bool {
	return slices.ContainsFunc(m.items, func(i *BoardItem) bool { return i.ID == "mcpjungle" && i.Picked })
}

// mcpServers are the list's servers in order: yardo's, then the found ones.
func (m *Menu) mcpServers() []*MCPServer {
	var yardo, found []*MCPServer
	for n := range m.info.MCP {
		s := &m.info.MCP[n]
		if s.Yardo {
			yardo = append(yardo, s)
		} else {
			found = append(found, s)
		}
	}
	by := func(a, b *MCPServer) int { return strings.Compare(a.Name, b.Name) }
	slices.SortStableFunc(yardo, by)
	slices.SortStableFunc(found, by)
	return append(yardo, found...)
}

func (m *Menu) mcpServer() *MCPServer {
	ss := m.mcpServers()
	if m.mcpSel < 0 || m.mcpSel >= len(ss) {
		return nil
	}
	return ss[m.mcpSel]
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
	if !s.Yardo && s.State == "off" {
		out = append(out, mcpAction{"on", "Turn it on", "adds it to yardo's servers and registers it"})
	}
	if s.Yardo && (s.State == "failed" || s.State == "pending") && !m.info.MCPDown {
		out = append(out, mcpAction{"retry", "Try again", "registers it as its settings are now"})
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
		return muted.Render("· off")
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
		n := len(m.mcpServers()) // the Add row is n
		switch {
		case k == "esc" || k == "q":
			if m.fromBoard {
				return m.quit(MenuResult{Kind: MenuTools})
			}
			m.home()
		case up:
			m.sel = max(0, m.sel-1)
		case down:
			m.sel = min(n, m.sel+1)
		case k == "e" && m.sel < n:
			s := m.mcpServers()[m.sel]
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "edit", Entry: s.Entry}})
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
				m.screen, m.btn = "mcp:delete", 1 // Keep it, first
			default:
				return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: as[m.sel].id, Entry: s.Entry}})
			}
		}
	case "mcp:delete":
		s := m.mcpServer()
		switch {
		case k == "esc" || k == "q":
			m.screen = "mcp:server"
		case left:
			m.btn = 0
		case right:
			m.btn = 1
		case k == "enter" && m.btn == 0 && s != nil:
			return m.quit(MenuResult{Kind: MenuMCP, MCP: MCPAct{Do: "delete", Entry: s.Entry}})
		case k == "enter":
			m.screen = "mcp:server"
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

// mcpView is the MCP screens' title, lines and keys; room is how many
// lines the screen has for them.
func (m *Menu) mcpView(w, room int) (string, []string, string) {
	k := func(key, what string) string { return keyOn.Render(key) + " " + what }
	var b []string
	add := func(s ...string) { b = append(b, s...) }
	switch m.screen {
	case "mcp":
		ss := m.mcpServers()
		m.sel = min(m.sel, len(ss))
		if m.info.MCPDown {
			add("  "+yellow.Render("▲  The gateway is stopped: fix it from Home first."), "")
		}
		if len(ss) == 0 {
			add("  No MCP servers yet. Each one gives your coding agents tools:", "  "+m.examples()+".", "")
		}
		var lines []string
		selLine := 0
		heading := false
		for n, s := range ss {
			if !s.Yardo && !heading {
				heading = true
				lines = append(lines, "", "     "+muted.Render("Also in ~/.mcp (not on yardo's list)"))
			}
			cols := padTo(mcpType(s), 13) + "  " + mcpWord(s)
			if n == m.sel {
				selLine = len(lines)
				lines = append(lines, "  "+rowOn.Render(padTo(" › "+s.Name, 21))+"  "+cols)
				if s.About != "" {
					lines = append(lines, "       "+muted.Render(runewidth.Truncate(s.About, max(10, w-10), "…")))
				}
			} else {
				lines = append(lines, "     "+padTo(s.Name, 18)+"  "+cols)
			}
		}
		lines = append(lines, "")
		if m.sel == len(ss) {
			selLine = len(lines)
			lines = append(lines, "  "+rowOn.Render(padTo(" › + Add a server", 21))+"  "+muted.Render(m.examples()))
		} else {
			lines = append(lines, "     "+padTo("+ Add a server", 18)+"  "+muted.Render(m.examples()))
		}
		add(window(lines, selLine, room-len(b)-2)...)
		add("", "  "+muted.Render("Agents reach their tools through the gateway as <name>__<tool>."))
		return "MCP servers", b, strings.Join([]string{k("↑↓", "choose"), k("enter", "open"), k("e", "edit its settings"), k("a", "add"), k("esc", "back")}, "   ")
	case "mcp:server", "mcp:delete":
		s := m.mcpServer()
		if s == nil {
			m.screen = "mcp"
			return m.mcpView(w, room)
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
				"  "+btn(0, "Delete")+"   "+btn(1, "Keep it"))
			return title, b, strings.Join([]string{k("←→", "choose"), k("enter", "do it"), k("esc", "back")}, "   ")
		}
		as := m.mcpActions(s)
		m.sel = min(m.sel, len(as)-1)
		for n, a := range as {
			if n == m.sel {
				add("  " + rowOn.Render(padTo(" › "+a.label, 24)) + "  " + muted.Render(a.note))
			} else {
				add("     " + padTo(a.label, 21) + "  " + muted.Render(a.note))
			}
		}
		return title, b, strings.Join([]string{k("↑↓", "choose"), k("enter", "do it"), k("esc", "back")}, "   ")
	case "mcp:add":
		specs := m.info.MCPSpecs
		m.sel = min(m.sel, max(0, len(specs)-1))
		var lines []string
		for n, s := range specs {
			note := ""
			if have := m.using(s.Name); len(have) > 0 {
				note = "   " + accent.Render("you have "+strings.Join(have, ", "))
			}
			about := muted.Render(runewidth.Truncate(s.About, max(10, w-50), "…"))
			if n == m.sel {
				lines = append(lines, "  "+rowOn.Render(padTo(" › "+s.Name, 21))+"  "+about+note)
			} else {
				lines = append(lines, "     "+padTo(s.Name, 18)+"  "+about+note)
			}
		}
		add(window(lines, m.sel, room-3)...)
		add("", "  "+muted.Render("Adding one you have makes another: a second Jira, another AWS account."))
		return "MCP servers  ›  Add a server", b, strings.Join([]string{k("↑↓", "choose"), k("enter", "next"), k("esc", "back")}, "   ")
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
