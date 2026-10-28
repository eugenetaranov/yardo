package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

// The menu is yardo's calm home: three lines of how the machine is, then a
// short menu of plain actions that changes with it ("Fix 2 problems",
// "Fill in 1 secret", "Add or remove tools", "Who can connect", "Connect
// from a laptop", updates). Each opens one focused screen; esc comes back.
// Adding and removing tools opens the board.

// MenuKind is what the menu was left with.
type MenuKind int

const (
	MenuQuit MenuKind = iota
	// MenuTools opens the board, to add or remove tools.
	MenuTools
	// MenuFix re-applies Result.ID now; MenuEdit opens its values to
	// fill in (Result.Inputs).
	MenuFix
	MenuEdit
	// MenuApply applies Result.Settings (who can connect).
	MenuApply
	// MenuUpdates checks for newer versions (slow).
	MenuUpdates
)

// MenuResult is what the menu was left with.
type MenuResult struct {
	Kind     MenuKind
	ID       string
	Inputs   []string
	Settings map[string]string
}

// menuRow is one of the home's actions.
type menuRow struct{ id, label, note string }

// Menu is the screen; see NewMenu.
type Menu struct {
	info     HomeInfo
	sub      string // the line under the title: the OS, the address
	items    []*BoardItem
	settings []*BoardSetting
	connect  []string
	checked  bool // updates were checked

	screen string // home, fix, secrets, access, connect, updates
	sel    int
	btn    int
	shown  map[string]bool // fix: details shown
	pick   map[string]string
	toast  string
	width  int
	height int
	result MenuResult
	out    bool
}

// NewMenu builds the menu. sub is the header's second half (OS, address);
// checked: updates were looked for (Items carry them).
func NewMenu(info HomeInfo, sub string, items []*BoardItem, settings []*BoardSetting, connect []string, checked bool) *Menu {
	return &Menu{info: info, sub: sub, items: items, settings: settings, connect: connect, checked: checked,
		screen: "home", shown: map[string]bool{}, pick: map[string]string{}, width: 100, height: 30}
}

func (m *Menu) filter(f func(*BoardItem) bool) []*BoardItem {
	var out []*BoardItem
	for _, i := range m.items {
		if f(i) {
			out = append(out, i)
		}
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// rows are the home's actions, as the machine is now.
func (m *Menu) rows() []menuRow {
	var out []menuRow
	if ps := m.filter(fixable); len(ps) > 0 {
		var why []string
		for _, i := range ps {
			why = append(why, short(i.Label)+" "+stateWord(i.State))
		}
		out = append(out, menuRow{"fix", "Fix " + plural(len(ps), "problem", "problems"), strings.Join(why, " · ")})
	}
	if ss := m.filter(secret); len(ss) > 0 {
		var keys []string
		for _, i := range ss {
			for _, in := range i.Inputs {
				_, k, _ := strings.Cut(in, ": ")
				keys = append(keys, k)
			}
		}
		out = append(out, menuRow{"secrets", "Fill in " + plural(len(ss), "secret", "secrets"), strings.Join(keys, ", ")})
	}
	out = append(out, menuRow{"tools", "Add or remove tools", "VPNs, languages, AI tools, databases and more"})
	if len(m.settings) > 0 {
		var now []string
		for _, s := range m.settings {
			now = append(now, s.Label+": "+s.label(s.Value))
		}
		out = append(out, menuRow{"access", "Who can connect", strings.Join(now, " · ")})
	}
	out = append(out, menuRow{"connect", "Connect from a laptop", "Docker over SSH, the MCP gateway"})
	if us := m.filter(newer); len(us) > 0 {
		var v []string
		for _, i := range us {
			v = append(v, strings.Join(i.Updates, ", "))
		}
		out = append(out, menuRow{"updates", "See " + plural(len(us), "update", "updates"), strings.Join(v, " · ")})
	} else if !m.checked {
		out = append(out, menuRow{"check", "Check for updates", "apt and mise; takes a while"})
	}
	return out
}

// short is a label without its details: what's in parentheses, or after
// the first comma.
func short(label string) string {
	s, _, _ := strings.Cut(label, " (")
	s, _, _ = strings.Cut(s, ", ")
	return s
}

func (m *Menu) Init() tea.Cmd { return nil }

func (m *Menu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case tea.KeyMsg:
		m.toast = ""
		return m, m.key(v.String())
	}
	return m, nil
}

func (m *Menu) quit(r MenuResult) tea.Cmd {
	m.result = r
	return tea.Quit
}

// open goes to screen, at its first row.
func (m *Menu) open(screen string) {
	m.screen, m.sel, m.btn = screen, 0, 0
	if screen == "access" {
		m.pick = map[string]string{}
	}
}

// home goes back, onto the row of the screen it came from.
func (m *Menu) home() {
	from := m.screen
	m.screen, m.sel, m.btn = "home", 0, 0
	for n, r := range m.rows() {
		if r.id == from {
			m.sel = n
		}
	}
}

func (m *Menu) key(k string) tea.Cmd {
	if k == "ctrl+c" {
		return m.quit(MenuResult{Kind: MenuQuit})
	}
	up, down := k == "up" || k == "k", k == "down" || k == "j"
	left, right := k == "left" || k == "h", k == "right" || k == "l"
	switch m.screen {
	case "home":
		rs := m.rows()
		switch {
		case up:
			m.sel = max(0, m.sel-1)
		case down:
			m.sel = min(len(rs)-1, m.sel+1)
		case k == "q" || k == "esc":
			return m.quit(MenuResult{Kind: MenuQuit})
		case k == "enter":
			switch id := rs[m.sel].id; id {
			case "tools":
				return m.quit(MenuResult{Kind: MenuTools})
			case "check":
				return m.quit(MenuResult{Kind: MenuUpdates})
			default:
				m.open(id)
			}
		}
	case "fix":
		ps := m.filter(fixable)
		switch {
		case k == "esc":
			m.home()
		case up:
			m.sel, m.btn = max(0, m.sel-1), 0
		case down:
			m.sel, m.btn = min(len(ps)-1, m.sel+1), 0
		case left:
			m.btn = 0
		case right:
			m.btn = 1
		case k == "enter" && len(ps) > 0:
			i := ps[m.sel]
			if m.btn == 1 {
				m.shown[i.ID] = !m.shown[i.ID]
				return nil
			}
			return m.quit(MenuResult{Kind: MenuFix, ID: i.ID})
		}
	case "secrets":
		ss := m.filter(secret)
		switch {
		case k == "esc":
			m.home()
		case up:
			m.sel = max(0, m.sel-1)
		case down:
			m.sel = min(len(ss)-1, m.sel+1)
		case k == "enter" && len(ss) > 0:
			return m.quit(MenuResult{Kind: MenuEdit, ID: ss[m.sel].ID, Inputs: ss[m.sel].Inputs})
		}
	case "access":
		n := len(m.settings)
		switch {
		case k == "esc":
			m.home()
		case up:
			m.sel = max(0, m.sel-1)
		case down:
			m.sel = min(n, m.sel+1)
		case (left || right) && m.sel < n:
			s := m.settings[m.sel]
			cur := m.value(s)
			idx := 0
			for c, ch := range s.Choices {
				if ch[0] == cur {
					idx = c
				}
			}
			if left {
				idx = max(0, idx-1)
			} else {
				idx = min(len(s.Choices)-1, idx+1)
			}
			m.pick[s.Key] = s.Choices[idx][0]
		case k == "enter" && m.sel < n:
			m.sel = n // on to Apply
		case k == "enter":
			set := map[string]string{}
			for _, s := range m.settings {
				if v, ok := m.pick[s.Key]; ok && v != s.Value {
					set[s.Key] = v
				}
			}
			if len(set) == 0 {
				m.home()
				return nil
			}
			return m.quit(MenuResult{Kind: MenuApply, Settings: set})
		}
	case "connect":
		if k == "esc" || k == "enter" || k == "q" {
			m.home()
		}
	case "updates":
		if k == "esc" || k == "enter" || k == "q" {
			m.home()
		}
	}
	return nil
}

// value is a setting's choice on the access screen.
func (m *Menu) value(s *BoardSetting) string {
	if v, ok := m.pick[s.Key]; ok {
		return v
	}
	return s.Value
}

// what each access choice means, in a sentence.
var accessSay = map[string]map[string]string{
	"mcp": {
		"127.0.0.1": "Only programs on this machine can use it.",
		"vpn":       "Your laptop too, once it's on Tailscale, NetBird or WireGuard, with a token.",
		"public":    "Anyone with a token, over HTTPS at your domain name.",
		"0.0.0.0":   "Anyone who can reach this machine, with a token, over plain HTTP.",
	},
	"ports": {
		"all":   "Ports you publish with docker run -p are open to every network.",
		"vpn":   "Published ports only from this machine and your VPNs.",
		"local": "Published ports only from this machine.",
	},
	"api": {
		"off": "Use Docker from your laptop over SSH. Nothing else opens.",
		"lan": "Port 2376 with client certificates, from your network and VPNs.",
		"vpn": "Port 2376 with client certificates, from your VPNs only.",
	},
}

func (m *Menu) View() string {
	if m.out {
		return ""
	}
	w, h := max(50, m.width), max(12, m.height)
	var b []string
	add := func(s ...string) { b = append(b, s...) }
	add("", "  "+accentBd.Render("yardo")+muted.Render("  ·  "+m.info.Host+"  ·  "+m.sub), "")
	title, keys := "", ""
	k := func(key, what string) string { return keyOn.Render(key) + " " + what }

	switch m.screen {
	case "home":
		ps, ss, us := len(m.filter(fixable)), len(m.filter(secret)), len(m.filter(newer))
		ready := len(m.filter(func(i *BoardItem) bool { return i.Picked && i.State == "ok" }))
		var sum []string
		if ps+ss == 0 {
			sum = append(sum, green.Render("●  Everything is healthy. "+plural(ready, "tool", "tools")+" ready."))
		} else {
			sum = append(sum, green.Render("●  "+plural(ready, "tool", "tools")+" ready"))
			if ps > 0 {
				sum = append(sum, yellow.Render("▲  "+plural(ps, "needs attention", "need attention")))
			}
			if ss > 0 {
				sum = append(sum, accent.Render("○  "+plural(ss, "needs a value", "need a value")))
			}
		}
		if us > 0 {
			sum = append(sum, keyOn.Render("↑  "+plural(us, "update", "updates")))
		}
		add("  "+strings.Join(sum, "      "), "", "", "  "+bold.Render("What would you like to do?"), "")
		rs := m.rows()
		m.sel = min(m.sel, len(rs)-1)
		for n, r := range rs {
			note := muted.Render(runewidth.Truncate(r.note, max(4, w-36), "…"))
			if n == m.sel {
				add("  " + rowOn.Render(padTo(" › "+r.label, 28)) + "  " + note)
			} else {
				add("     " + padTo(r.label, 25) + "  " + note)
			}
		}
		keys = strings.Join([]string{k("↑↓", "choose"), k("enter", "open"), k("q", "quit")}, "   ")
	case "fix":
		ps := m.filter(fixable)
		if len(ps) == 0 {
			m.home()
			return m.View()
		}
		m.sel = min(m.sel, len(ps)-1)
		title = "Fix " + plural(len(ps), "problem", "problems")
		for n, i := range ps {
			add("  " + yellow.Render("▲  ") + bold.Render(padTo(short(i.Label), 56)) + "  " + yellow.Render(stateWord(i.State)))
			add("     " + muted.Render(runewidth.Truncate(i.Detail, w-8, "…")))
			btn := func(n2 int, s string) string {
				if n == m.sel && m.btn == n2 {
					return rowOn.Render(" " + s + " ")
				}
				return muted.Render("[ ") + s + muted.Render(" ]")
			}
			add("     " + btn(0, "Fix it") + "   " + btn(1, "Show how it's set up"))
			if m.shown[i.ID] && i.Source != "" {
				for _, l := range wrap(i.Source, w-10) {
					add("       " + muted.Render(l))
				}
			}
			add("")
		}
		keys = strings.Join([]string{k("↑↓ ←→", "choose"), k("enter", "do it"), k("esc", "back")}, "   ")
	case "secrets":
		ss := m.filter(secret)
		if len(ss) == 0 {
			m.home()
			return m.View()
		}
		m.sel = min(m.sel, len(ss)-1)
		title = "Fill in " + plural(len(ss), "secret", "secrets")
		for n, i := range ss {
			label := padTo(short(i.Label), 40)
			if n == m.sel {
				label = rowOn.Render(" › " + label)
			} else {
				label = "   " + label
			}
			add("  "+label+"  "+accent.Render(strings.Join(i.Inputs, ", ")), "")
		}
		add("  " + muted.Render("Enter opens the file in your editor; yardo never stores the value."))
		keys = strings.Join([]string{k("↑↓", "choose"), k("enter", "fill in"), k("esc", "back")}, "   ")
	case "access":
		title = "Who can connect"
		for n, s := range m.settings {
			on := n == m.sel
			name := bold.Render("  " + s.Label)
			if on {
				name = accentBd.Render("› " + s.Label)
			}
			add("  " + name)
			var cs []string
			cur := m.value(s)
			for _, c := range s.Choices {
				if c[0] == cur {
					st := bold
					if on {
						st = accentBd
					}
					cs = append(cs, st.Render("(•) "+c[1]))
				} else {
					cs = append(cs, muted.Render("( ) "+c[1]))
				}
			}
			for _, l := range wrapStyled(cs, w-8) {
				add("     " + l)
			}
			add("     "+muted.Render(accessSay[s.Key][cur]), "")
		}
		apply := muted.Render("[ ") + "Apply" + muted.Render(" ]")
		if m.sel == len(m.settings) {
			apply = rowOn.Render(" Apply ")
		}
		add("  " + apply + "   " + muted.Render("esc leaves without changing anything"))
		keys = strings.Join([]string{k("↑↓", "setting"), k("←→", "choose"), k("enter", "apply"), k("esc", "back")}, "   ")
	case "connect":
		title = "Connect from a laptop"
		for _, l := range m.connect {
			if strings.HasPrefix(l, "  ") {
				add("  " + keyOn.Render(l))
			} else {
				add("  " + l)
			}
		}
		keys = k("esc", "back")
	case "updates":
		title = "Updates"
		for _, i := range m.filter(newer) {
			add("  " + keyOn.Render("↑  ") + padTo(short(i.Label), 40) + muted.Render(strings.Join(i.Updates, ", ")))
		}
		add("", "  "+muted.Render("yardo doesn't upgrade on its own: mise up updates your tools, sudo apt upgrade the system's."))
		keys = k("esc", "back")
	}
	if title != "" {
		b = append(b[:3], append([]string{"  " + muted.Render("Home  ›  ") + bold.Render(title), ""}, b[3:]...)...)
	}
	for len(b) < h-2 {
		b = append(b, "")
	}
	if len(b) > h-2 {
		b = b[:h-2]
	}
	if m.toast != "" {
		b = append(b, "  "+green.Render(m.toast))
	} else {
		b = append(b, "")
	}
	b = append(b, "  "+keys)
	return strings.Join(b, "\n")
}

// wrapStyled lays rendered pieces out in lines at most w wide.
func wrapStyled(parts []string, w int) []string {
	var out []string
	line := ""
	for _, p := range parts {
		if line != "" && runewidth.StringWidth(stripANSI(line))+4+runewidth.StringWidth(stripANSI(p)) > w {
			out = append(out, line)
			line = ""
		}
		if line != "" {
			line += "    "
		}
		line += p
	}
	return append(out, line)
}

// stripANSI drops terminal styling, to measure text.
func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Run shows the menu; it returns what it was left with.
func (m *Menu) Run() (MenuResult, error) {
	m.out = false
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return MenuResult{Kind: MenuQuit}, err
	}
	m.out = true
	return m.result, nil
}
