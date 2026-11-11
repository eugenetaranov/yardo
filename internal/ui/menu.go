package ui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// The menu is yardo's calm home: two lines of how the machine is, a row
// for each thing that needs doing, named by what it does ("Start Docker",
// "Fill in OPENAI_API_KEY", "Install 3 updates"), then three rows that
// never move: Tools (the board), MCP servers and Settings. Typing on the
// home searches the tools. Each row opens one focused screen, or does its
// thing at once; esc (or q) comes back. On the home, q quits.

// MenuKind is what the menu was left with.
type MenuKind int

const (
	MenuQuit MenuKind = iota
	// MenuTools opens the board, to add or remove tools (searching for
	// Result.Search, when it's set).
	MenuTools
	// MenuFix re-applies Result.IDs now (Fix all: every problem; ID is
	// the first); MenuEdit opens its values to fill in (Result.Inputs).
	MenuFix
	MenuEdit
	// MenuApply applies Result.Settings (network access, the look).
	MenuApply
	// MenuUpdates checks for newer versions again, in the background.
	MenuUpdates
	// MenuMCP does Result.MCP to an MCP server: add, edit, on, rename or
	// delete it.
	MenuMCP
	// MenuChange asks Result.ID's Result.Change question again (the git
	// name, the prompt, the dotfiles repo), then re-applies it.
	MenuChange
	// MenuUpgrade installs the newer versions the Updates screen lists
	// (mise up, apt's only-upgrade), once confirmed.
	MenuUpgrade
)

// MCPAct is what to do to an MCP server: Do (add, edit, on, rename,
// delete) to Entry (jira:work); To is a rename's new entry.
type MCPAct struct{ Do, Entry, To string }

// MenuResult is what the menu was left with.
type MenuResult struct {
	Kind     MenuKind
	ID       string
	Change   string
	Search   string
	IDs      []string
	Inputs   []string
	Settings map[string]string
	MCP      MCPAct
}

// menuRow is one of the home's (or Settings') rows: item and input are
// what a fix or fill-in row acts on.
type menuRow struct {
	id, label, note string
	item            *BoardItem
	input           string
}

// UpdatesMsg is a check for newer versions, done: each item's (by ID),
// and when.
type UpdatesMsg struct {
	ByID map[string][]string
	At   string
}

// Menu is the screen; see NewMenu.
type Menu struct {
	info     HomeInfo
	sub      string // the line under the title: the OS, the address
	items    []*BoardItem
	settings []*BoardSetting
	connect  []string
	checked  bool // updates were checked
	// wait waits for a check for updates running in the background (nil
	// for none).
	wait func() UpdatesMsg

	screen string // home, fix, secrets, updates, settings, access, look, connect, mcp, mcp:server, mcp:delete, mcp:add, mcp:name
	sel    int
	// The MCP screens: the server and catalog row chosen, the name typed
	// (for nameFor: a spec to add, or an entry to rename), its error;
	// fromBoard: esc on the list goes back to the board.
	mcpSel, addSel int
	name           []rune
	nameFor        string
	renaming       bool
	delList        bool // deleting from the list (d): esc goes back to it
	nameErr        string
	fromBoard      bool
	btn            int
	sure           bool            // updates: Update all was pressed once; enter again confirms
	shown          map[string]bool // fix: details shown
	pick           map[string]string
	toast          string
	width          int
	height         int
	result         MenuResult
	out            bool
}

// NewMenu builds the menu. sub is the header's second half (OS, address);
// checked: updates were looked for (Items carry them).
func NewMenu(info HomeInfo, sub string, items []*BoardItem, settings []*BoardSetting, connect []string, checked bool) *Menu {
	m := &Menu{info: info, sub: sub, items: items, settings: settings, connect: connect, checked: checked,
		screen: "home", shown: map[string]bool{}, pick: map[string]string{}, width: 100, height: 30}
	m.sortMCP()
	m.normalize()
	return m
}

// access are the settings Network access shows: who can reach what.
func (m *Menu) access() []*BoardSetting { return m.section("") }

// section are the settings in section (LookSection: the accent colour).
func (m *Menu) section(sec string) []*BoardSetting {
	var out []*BoardSetting
	for _, s := range m.settings {
		if s.Section == sec {
			out = append(out, s)
		}
	}
	return out
}

// choices are the settings the access or look screen shows.
func (m *Menu) choices() []*BoardSetting {
	if m.screen == "look" {
		return m.section(LookSection)
	}
	return m.access()
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

// fixVerb is a fix row's label: what fixing i does.
func fixVerb(i *BoardItem) string {
	n := short(i.Label)
	switch i.State {
	case "down":
		return "Start " + n
	case "missing":
		return "Install " + n
	case "partial":
		return "Finish setting up " + n
	case "drifted":
		return "Restore " + n
	case "outdated":
		return "Update " + n + "'s setup"
	}
	return "Fix " + n
}

// fixNote is what a fix row says beside it.
func fixNote(i *BoardItem) string {
	switch i.State {
	case "drifted":
		return "edited by hand: restoring puts back yardo's version · " + i.Detail
	case "outdated":
		return "set up by an older yardo: this one ships newer files"
	}
	if i.Detail == "" {
		return stateWord(i.State)
	}
	return stateWord(i.State) + ": " + i.Detail
}

// todo are the home's rows for what needs doing: each problem (or, with
// three or more, one row for them all), each value to fill in (or one row
// for three or more), and the updates.
func (m *Menu) todo() []menuRow {
	var out []menuRow
	if ps := m.filter(fixable); len(ps) <= 2 {
		for _, i := range ps {
			out = append(out, menuRow{id: "fix:" + i.ID, label: fixVerb(i), note: fixNote(i), item: i})
		}
	} else {
		var why []string
		for _, i := range ps {
			why = append(why, short(i.Label)+" "+stateWord(i.State))
		}
		out = append(out, menuRow{id: "fix", label: "Fix " + plural(len(ps), "thing", "things"), note: strings.Join(why, " · ")})
	}
	var ins []menuRow
	for _, i := range m.filter(secret) {
		for _, in := range i.Inputs {
			_, k, _ := strings.Cut(in, ": ")
			ins = append(ins, menuRow{id: "key:" + i.ID + ":" + k, label: "Fill in " + k, note: short(i.Label) + " needs it · opens its file", item: i, input: in})
		}
	}
	if len(ins) <= 2 {
		out = append(out, ins...)
	} else {
		var keys []string
		for _, r := range ins {
			keys = append(keys, strings.TrimPrefix(r.label, "Fill in "))
		}
		out = append(out, menuRow{id: "secrets", label: "Fill in " + plural(len(ins), "value", "values"), note: strings.Join(keys, ", ")})
	}
	if us := m.filter(newer); len(us) > 0 {
		var v []string
		for _, i := range us {
			v = append(v, strings.Join(i.Updates, ", "))
		}
		out = append(out, menuRow{id: "updates", label: "Install " + plural(len(us), "update", "updates"), note: strings.Join(v, " · ")})
	}
	return out
}

// rows are the home's rows, as the machine is now: what needs doing, then
// Tools, MCP servers (with the gateway) and Settings.
func (m *Menu) rows() []menuRow {
	out := m.todo()
	out = append(out, menuRow{id: "tools", label: "Tools", note: "yours: fix, add or uninstall · or just start typing"})
	if m.hasMCP() {
		var names []string
		problems := 0
		for _, s := range m.info.MCP {
			if s.Yardo {
				names = append(names, s.Name)
				if s.State != "ok" && s.State != "unchecked" {
					problems++
				}
			}
		}
		note := "none yet: add GitHub, Jira, Grafana and more"
		if len(names) > 0 {
			note = strings.Join(names, ", ")
			if problems > 0 {
				note += " · " + plural(problems, "needs you", "need you")
			}
		}
		out = append(out, menuRow{id: "mcp", label: "MCP servers", note: note})
	}
	return append(out, menuRow{id: "settings", label: "Settings", note: "network access, git, prompt, look, laptop, updates"})
}

// changeLabels name the answers Settings can change (BoardItem.Changes'
// keys); the MCP servers have their own row.
var changeLabels = map[string]string{"git": "Git name and email", "prompt": "Shell prompt", "dotfiles": "Dotfiles repo"}

// settingsRows are Settings' rows.
func (m *Menu) settingsRows() []menuRow {
	var out []menuRow
	if as := m.access(); len(as) > 0 {
		var now []string
		for _, s := range as {
			now = append(now, s.Label+": "+s.label(s.Value))
		}
		out = append(out, menuRow{id: "access", label: "Network access", note: strings.Join(now, " · ")})
	}
	for _, s := range m.section(LookSection) {
		out = append(out, menuRow{id: "look", label: s.Label, note: s.label(s.Value) + " · yardo, your prompt and tmux"})
	}
	for _, i := range m.items {
		if !i.Picked {
			continue
		}
		for _, c := range i.Changes {
			if l, ok := changeLabels[c[1]]; ok {
				out = append(out, menuRow{id: "change:" + c[1], label: l, note: "asks, then applies it again", item: i})
			}
		}
	}
	out = append(out, menuRow{id: "connect", label: "Connect from a laptop", note: "Docker over SSH, the MCP gateway"})
	check := menuRow{id: "check", label: "Check for updates", note: "apt and mise, in the background"}
	switch {
	case m.wait != nil:
		check.note = "checking now…"
	case m.checked:
		check.label, check.note = "Check again", "none found"
		if n := len(m.filter(newer)); n > 0 {
			check.note = plural(n, "update", "updates") + " found"
		}
		if m.info.Checked != "" {
			check.note += " at " + m.info.Checked
		}
	}
	return append(out, check)
}

// Short is a label without its details: what's in parentheses, or after
// the first comma.
func Short(label string) string { return short(label) }

func short(label string) string {
	s, _, _ := strings.Cut(label, " (")
	s, _, _ = strings.Cut(s, ", ")
	return s
}

// Checking says a check for updates is running: wait waits for it, and
// the menu shows what it found.
func (m *Menu) Checking(wait func() UpdatesMsg) *Menu {
	m.wait = wait
	return m
}

func (m *Menu) Init() tea.Cmd {
	if m.wait == nil {
		return nil
	}
	return func() tea.Msg { return m.wait() }
}

func (m *Menu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case UpdatesMsg:
		for _, i := range m.items {
			if i.Picked {
				i.Updates = v.ByID[i.ID]
			}
		}
		m.wait, m.checked, m.info.Checked = nil, true, v.At
		m.normalize()
	case tea.KeyMsg:
		m.toast = ""
		m.normalize()
		var cmd tea.Cmd
		switch {
		case m.screen == "mcp:name":
			cmd = m.nameKey(v)
		case m.screen == "home" && (v.Type == tea.KeyRunes && string(v.Runes) != "q" || v.String() == "/"):
			// Typing searches the tools.
			cmd = m.quit(MenuResult{Kind: MenuTools, Search: strings.TrimPrefix(string(v.Runes), "/")})
		default:
			cmd = m.key(v.String())
		}
		m.normalize()
		return m, cmd
	}
	return m, nil
}

// clamp is v within lo and hi; lo when hi < lo (an empty list).
func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

// normalize keeps the screen and its cursor valid as the menu is now: a
// screen with nothing left on it goes back, and the cursor stays on a
// row. It runs after every key (and when the menu is made), so View only
// reads.
func (m *Menu) normalize() {
	switch m.screen {
	case "fix":
		if len(m.filter(fixable)) == 0 {
			m.back()
		}
	case "secrets":
		if len(m.filter(secret)) == 0 {
			m.back()
		}
	case "updates":
		if len(m.filter(newer)) == 0 {
			m.back()
		}
	case "mcp:server", "mcp:delete":
		if m.mcpServer() == nil {
			m.screen, m.sel = "mcp", 0
		}
	}
	switch m.screen {
	case "home":
		m.sel = clamp(m.sel, 0, len(m.rows())-1)
	case "fix":
		m.sel = clamp(m.sel, 0, m.fixRows()-1)
	case "secrets":
		m.sel = clamp(m.sel, 0, len(m.filter(secret))-1)
	case "settings":
		m.sel = clamp(m.sel, 0, len(m.settingsRows())-1)
	case "access", "look":
		m.sel = clamp(m.sel, 0, len(m.choices())) // Apply is the last
	case "mcp":
		m.sel = clamp(m.sel, 0, len(m.mcpServers())) // Add a server is the last
	case "mcp:server":
		m.sel = clamp(m.sel, 0, len(m.mcpActions(m.mcpServer()))-1)
	case "mcp:add":
		m.sel = clamp(m.sel, 0, len(m.info.MCPSpecs)-1)
	}
}

// fixRows are the fix screen's rows: a card per problem, then Fix all
// (with two or more).
func (m *Menu) fixRows() int {
	n := len(m.filter(fixable))
	if n >= 2 {
		n++
	}
	return n
}

func (m *Menu) quit(r MenuResult) tea.Cmd {
	m.result = r
	return tea.Quit
}

// open goes to screen, at its first row.
func (m *Menu) open(screen string) {
	m.screen, m.sel, m.btn, m.sure = screen, 0, 0, false
	switch screen {
	case "access", "look":
		m.pick = map[string]string{}
	case "mcp":
		m.sortMCP()
	}
}

// back goes up a screen (Settings' own go back to it, the rest home),
// onto the row of the screen it came from.
func (m *Menu) back() {
	from := m.screen
	m.screen, m.sel, m.btn, m.sure = "home", 0, 0, false
	rows := m.rows
	switch from {
	case "access", "look", "connect":
		m.screen, rows = "settings", m.settingsRows
	}
	for n, r := range rows() {
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
	// esc and q go back on every screen but the home; there, q quits and
	// esc only says so. up and down move; normalize keeps the cursor on a
	// row.
	back := k == "esc" || k == "q"
	switch m.screen {
	case "home":
		r := m.rows()[m.sel]
		switch {
		case up:
			m.sel--
		case down:
			m.sel++
		case k == "q":
			return m.quit(MenuResult{Kind: MenuQuit})
		case k == "esc":
			m.toast = "q quits"
		case k == "enter" && strings.HasPrefix(r.id, "fix:"):
			return m.quit(MenuResult{Kind: MenuFix, ID: r.item.ID, IDs: []string{r.item.ID}})
		case k == "enter" && strings.HasPrefix(r.id, "key:"):
			return m.quit(MenuResult{Kind: MenuEdit, ID: r.item.ID, Inputs: []string{r.input}})
		case k == "enter" && r.id == "tools":
			return m.quit(MenuResult{Kind: MenuTools})
		case k == "enter":
			m.open(r.id)
		}
	case "settings":
		r := m.settingsRows()[m.sel]
		switch {
		case back:
			m.back()
		case up:
			m.sel--
		case down:
			m.sel++
		case k == "enter" && strings.HasPrefix(r.id, "change:"):
			return m.quit(MenuResult{Kind: MenuChange, ID: r.item.ID, Change: strings.TrimPrefix(r.id, "change:")})
		case k == "enter" && r.id == "check" && m.wait != nil:
			m.toast = "Checking already: what it finds shows on the home."
		case k == "enter" && r.id == "check":
			return m.quit(MenuResult{Kind: MenuUpdates})
		case k == "enter":
			m.open(r.id)
		}
	case "fix":
		ps := m.filter(fixable)
		switch {
		case back:
			m.back()
		case up:
			m.sel, m.btn = m.sel-1, 0
		case down:
			m.sel, m.btn = m.sel+1, 0
		case left:
			m.btn = 0
		case right:
			m.btn = 1
		case k == "enter" && len(ps) >= 2 && m.sel == len(ps): // Fix all
			ids := make([]string, len(ps))
			for n, i := range ps {
				ids[n] = i.ID
			}
			return m.quit(MenuResult{Kind: MenuFix, ID: ids[0], IDs: ids})
		case k == "enter" && m.sel < len(ps):
			i := ps[m.sel]
			if m.btn == 1 {
				m.shown[i.ID] = !m.shown[i.ID]
				return nil
			}
			return m.quit(MenuResult{Kind: MenuFix, ID: i.ID, IDs: []string{i.ID}})
		}
	case "secrets":
		ss := m.filter(secret)
		switch {
		case back:
			m.back()
		case up:
			m.sel--
		case down:
			m.sel++
		case k == "enter" && m.sel < len(ss):
			return m.quit(MenuResult{Kind: MenuEdit, ID: ss[m.sel].ID, Inputs: ss[m.sel].Inputs})
		}
	case "access", "look":
		as := m.choices()
		n := len(as)
		switch {
		case back:
			m.back()
		case up:
			m.sel--
		case down:
			m.sel++
		case (left || right) && m.sel < n:
			s := as[m.sel]
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
			for _, s := range as {
				if v, ok := m.pick[s.Key]; ok && v != s.Value {
					set[s.Key] = v
				}
			}
			if len(set) == 0 {
				m.back()
				return nil
			}
			return m.quit(MenuResult{Kind: MenuApply, Settings: set})
		}
	case "mcp", "mcp:server", "mcp:delete", "mcp:add":
		return m.mcpKey(k, up, down, left, right)
	case "connect":
		if back || k == "enter" {
			m.back()
		}
	case "updates":
		// enter asks, enter again updates; esc takes the question back.
		switch {
		case back && m.sure:
			m.sure = false
		case back:
			m.back()
		case k == "enter" && m.sure:
			return m.quit(MenuResult{Kind: MenuUpgrade})
		case k == "enter" && len(m.filter(newer)) > 0:
			m.sure = true
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
		add(m.status()...)
		add("")
		// The tools installed get the lines the rest leaves: the header and
		// status, the other dashboard lines, the rows, the keys.
		rs := m.rows()
		room := h - len(b) - 2 - 4 - len(rs) - 1 - 3
		if dl := m.dashLines(w, room); len(dl) > 0 {
			add(dl...)
			add("")
		}
		todo := len(m.todo())
		col := labelCol(rs, w)
		for n, r := range rs {
			if n == todo && todo > 0 {
				add("  " + muted.Render(strings.Repeat("─", min(60, w-6))))
			}
			add(m.row(n == m.sel, r, col, w)...)
		}
		keys = strings.Join([]string{k("↑↓", "choose"), k("enter", "open"), k("type", "search tools"), k("q", "quit")}, "   ")
	case "settings":
		title = "Settings"
		rs := m.settingsRows()
		col := labelCol(rs, w)
		for n, r := range rs {
			add(m.row(n == m.sel, r, col, w)...)
		}
		keys = strings.Join([]string{k("↑↓", "choose"), k("enter", "open"), k("esc", "back")}, "   ")
	case "fix":
		ps := m.filter(fixable)
		title = "Fix " + plural(len(ps), "thing", "things")
		for n, i := range ps {
			add("  " + yellow.Render("▲  ") + bold.Render(padTo(short(i.Label), 56)) + "  " + yellow.Render(stateWord(i.State)))
			add("     " + muted.Render(runewidth.Truncate(i.Detail, w-8, "…")))
			btn := func(n2 int, s string) string {
				if n == m.sel && m.btn == n2 {
					return rowOn.Render(" " + s + " ")
				}
				return muted.Render("[ ") + s + muted.Render(" ]")
			}
			add("     " + btn(0, fixVerb(i)) + "   " + btn(1, "Details"))
			if m.shown[i.ID] && i.Source != "" {
				for _, l := range wrap(i.Source, w-10) {
					add("       " + muted.Render(l))
				}
			}
			add("")
		}
		if len(ps) >= 2 {
			all := fmt.Sprintf("Fix all %d", len(ps))
			if m.sel == len(ps) {
				all = rowOn.Render(" " + all + " ")
			} else {
				all = muted.Render("[ ") + all + muted.Render(" ]")
			}
			add("  " + all + "   " + muted.Render("in one run"))
		}
		keys = strings.Join([]string{k("↑↓ ←→", "choose"), k("enter", "do it"), k("esc", "back")}, "   ")
	case "secrets":
		ss := m.filter(secret)
		title = "Fill in " + plural(len(ss), "value", "values")
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
	case "access", "look":
		title = "Settings  ›  Network access"
		if m.screen == "look" {
			title = "Settings  ›  Look"
		}
		as := m.choices()
		for n, s := range as {
			on := n == m.sel
			name := bold.Render("  " + s.Label)
			if on {
				name = accentBd.Render("› " + s.Label)
			}
			add("  " + name)
			var cs []string
			cur := m.value(s)
			for _, c := range s.Choices {
				sw := ""
				if hex, ok := s.Swatches[c[0]]; ok {
					sw = lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("██") + " "
				}
				if c[0] == cur {
					st := bold
					if on {
						st = accentBd
					}
					cs = append(cs, sw+st.Render("(•) "+c[1]))
				} else {
					cs = append(cs, sw+muted.Render("( ) "+c[1]))
				}
			}
			for _, l := range wrapStyled(cs, w-8) {
				add("     " + l)
			}
			if say := accessSay[s.Key][cur]; say != "" {
				add("     " + muted.Render(say))
			}
			add("")
		}
		apply := muted.Render("[ ") + "Apply" + muted.Render(" ]")
		if m.sel == len(as) {
			apply = rowOn.Render(" Apply ")
		}
		add("  " + apply + "   " + muted.Render("esc leaves without changing anything"))
		keys = strings.Join([]string{k("↑↓", "setting"), k("←→", "choose"), k("enter", "apply"), k("esc", "back")}, "   ")
	case "connect":
		title = "Settings  ›  Connect from a laptop"
		for _, l := range m.connect {
			if strings.HasPrefix(l, "  ") {
				add("  " + keyOn.Render(l))
			} else {
				add("  " + l)
			}
		}
		keys = k("esc", "back")
	case "mcp", "mcp:server", "mcp:delete", "mcp:add", "mcp:name":
		var lines []string
		title, lines, keys = m.mcpView(w, h-len(b)-6)
		add(lines...)
	case "updates":
		title = "Updates"
		for _, i := range m.filter(newer) {
			add("  " + keyOn.Render("↑  ") + padTo(short(i.Label), 40) + muted.Render(strings.Join(i.Updates, ", ")))
		}
		n := len(m.filter(newer))
		add("")
		if m.sure {
			add("  " + bold.Render("Update "+plural(n, "item", "items")+"?") + "  " + muted.Render("mise up for the tools, apt for the packages; nothing else changes"))
			keys = strings.Join([]string{k("enter", "yes, update"), k("esc", "no")}, "   ")
		} else {
			add("  "+rowOn.Render(" Update all "),
				"", "  "+muted.Render("yardo doesn't upgrade on its own."))
			keys = strings.Join([]string{k("enter", "update"), k("esc", "back")}, "   ")
		}
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

// status are the home's first lines: how the machine is, in words, then
// what's working and when updates were looked for.
func (m *Menu) status() []string {
	ps, ss := len(m.filter(fixable)), 0
	for _, i := range m.filter(secret) {
		ss += len(i.Inputs)
	}
	ready := len(m.filter(func(i *BoardItem) bool { return i.Picked && i.State == "ok" }))
	head := green.Render("All good.")
	if ps+ss > 0 {
		var need []string
		if ps > 0 {
			need = append(need, plural(ps, "thing", "things")+" to fix")
		}
		if ss > 0 {
			need = append(need, plural(ss, "value", "values")+" to fill in")
		}
		head = yellow.Render("Needs you: " + strings.Join(need, ", "))
	}
	sub := []string{plural(ready, "tool", "tools") + " working"}
	// A machine set up from a team profile says so, and how much is the
	// team's.
	team, teams := 0, map[string]bool{}
	for _, i := range m.items {
		if i.Picked && i.Team != "" {
			team++
			teams[i.Team] = true
		}
	}
	if team > 0 {
		sub = append(sub, fmt.Sprintf("%d from the team profile %s", team, strings.Join(slices.Sorted(maps.Keys(teams)), ", ")))
	}
	switch us := len(m.filter(newer)); {
	case m.wait != nil:
		sub = append(sub, "checking for updates…")
	case us > 0:
		sub = append(sub, plural(us, "update", "updates"))
	case m.checked:
		sub = append(sub, "nothing newer")
	default:
		sub = append(sub, "updates not checked")
	}
	if m.info.Checked != "" && m.wait == nil {
		sub = append(sub, "checked "+m.info.Checked)
	}
	return []string{"  " + head, "  " + muted.Render(strings.Join(sub, " · "))}
}

// labelCol is how wide rs' labels are laid out: the longest, at least 25
// cells and at most half the screen.
func labelCol(rs []menuRow, w int) int {
	col := 25
	for _, r := range rs {
		col = max(col, runewidth.StringWidth(r.label))
	}
	return min(col, w/2)
}

// row is one of the home's (or Settings') rows, its label col cells wide:
// the selected one with its note wrapped beside it, the others with theirs
// cut to the line.
func (m *Menu) row(on bool, r menuRow, col, w int) []string {
	if on {
		return beside("  "+rowOn.Render(padTo(" › "+r.label, col+3))+"  ", col+7, w-2, muted, r.note)
	}
	return []string{"     " + padTo(r.label, col) + "  " + muted.Render(runewidth.Truncate(r.note, max(4, w-col-9), "…"))}
}

// wrapStyled lays rendered pieces out in lines at most w wide.
func wrapStyled(parts []string, w int) []string {
	var out []string
	line := ""
	for _, p := range parts {
		if line != "" && lipgloss.Width(line)+4+lipgloss.Width(p) > w {
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

// Run shows the menu; it returns what it was left with.
func (m *Menu) Run() (MenuResult, error) {
	m.out = false
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return MenuResult{Kind: MenuQuit}, err
	}
	m.out = true
	return m.result, nil
}
