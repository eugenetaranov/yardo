package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// The menu is yardo's calm home: a row for each thing that needs doing,
// named by what it does ("Start Docker", "Fill in OPENAI_API_KEY",
// "Install 3 updates"), the Connection panel, then Go to's entries that
// never move: Tools (the board), MCP servers, Settings and Last run.
// Typing on the home searches the tools. Each row opens one focused
// screen, or does its thing at once; q comes back. On the home, q quits.
// It's one of the app's views (see App).

// Paste starts a Connection line that's typed or put in a file,
// not said; the screen shows it apart. Unmark takes it out for a terminal.
const Paste = "\x1f"

func Unmark(s string) string { return strings.ReplaceAll(s, Paste, "") }

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
	// (mise up, apt's only-upgrade).
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
	// for none); check starts one (u), returning its wait.
	wait  func() UpdatesMsg
	check func() func() UpdatesMsg
	// asked: the check running was u's, which says what it found.
	asked bool
	// dropKey forgets a key under Keys to add (d on Connection).
	dropKey func(pub string) error
	// switcher saves the MCP servers turned off and on, in the
	// background; switching: a save is running.
	// pend are the servers marked off or on, not saved yet (entry: off
	// or on; "": all of them); before, the servers as they're saved;
	// leaving: q was pressed once with pend.
	switcher  Switcher
	switching bool
	pend      map[string]string
	before    HomeInfo
	leaving   bool

	screen string // home, fix, secrets, updates, settings, access, look, connect, lastrun, mcp, mcp:server, mcp:add, mcp:name
	sel    int
	// The MCP screens: the server and catalog row chosen, the name typed
	// (for nameFor: a spec to add, or an entry to rename), its error;
	// fromBoard: q on the list goes back to the board.
	mcpSel, addSel int
	name           []rune
	nameFor        string
	renaming       bool
	nameErr        string
	fromBoard      bool
	unticked       map[string]bool // fix: rows not to fix
	pick           map[string]string
	// dialog is the question open over the screen (deleting a server).
	dialog *Dialog
	toast  string
	width  int
	height int
	// hits are where the last Body drew what a click selects.
	hits   []Hit
	result MenuResult
	// ended: the menu was left, with result.
	ended bool
}

// NewMenu builds the menu. sub is the header's second half (OS, address);
// checked: updates were looked for (Items carry them).
func NewMenu(info HomeInfo, sub string, items []*BoardItem, settings []*BoardSetting, connect []string, checked bool) *Menu {
	m := &Menu{info: info, sub: sub, items: items, settings: settings, connect: connect, checked: checked,
		screen: "home", unticked: map[string]bool{}, pick: map[string]string{}, width: 100, height: 30}
	m.sortMCP()
	m.normalize()
	return m
}

// changed are the access or look screen's choices that differ from the
// saved ones, by key.
func (m *Menu) changed() map[string]string {
	set := map[string]string{}
	for _, s := range m.choices() {
		if v, ok := m.pick[s.Key]; ok && v != s.Value {
			set[s.Key] = v
		}
	}
	return set
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

// filter are the items f picks; not a part (a login), which its tool's
// row stands for.
func (m *Menu) filter(f func(*BoardItem) bool) []*BoardItem {
	var out []*BoardItem
	for _, i := range m.items {
		if !folded(m.items, i) && f(i) {
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

// problem: i was set up and isn't as it was (stopped, edited by hand, or
// set up by an older yardo). An item picked but never set up ("not
// installed", "half set up": yardo never applied it) is no problem to
// fix: Tools installs it.
func problem(i *BoardItem) bool {
	return i.Picked && (i.State == "down" || i.State == "drifted" || i.State == "outdated")
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

// todo are the home's rows for what needs doing: the last run, when it
// failed; each problem (or, with three or more, one row for them all),
// each value to fill in (or one row for three or more), and the updates,
// or checking for them, when that was never done.
func (m *Menu) todo() []menuRow {
	var out []menuRow
	if l := m.info.LastRun; l != nil && !l.OK {
		out = append(out, menuRow{id: "why", label: "See why the last run failed", note: l.When + " · " + l.say()})
	}
	if ps := m.filter(problem); len(ps) <= 2 {
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
	// yardo's MCP servers that aren't ready.
	if m.hasMCP() {
		for _, x := range m.mcpServers() {
			switch {
			case !x.Yardo:
			case x.State == "secret":
				out = append(out, menuRow{id: "mcpkey:" + x.Name, label: "Fill in " + x.Name, note: strings.Join(x.Keys, ", ")})
			case x.State == "failed" || x.State == "retired" || x.State == "nospec":
				out = append(out, menuRow{id: "mcp:" + x.Name, label: "Check " + x.Name, note: strings.TrimPrefix(ansi.Strip(mcpWord(x)), "▲ ")})
			}
		}
	}
	if us := m.filter(newer); len(us) > 0 {
		var v []string
		for _, i := range us {
			v = append(v, strings.Join(i.Updates, ", "))
		}
		out = append(out, menuRow{id: "updates", label: "Install " + plural(len(us), "update", "updates"), note: strings.Join(v, " · ")})
	} else if !m.checked && m.wait == nil && m.info.Checked == "" {
		out = append(out, menuRow{id: "check", label: "Check for updates", note: "apt and mise, in the background · or u, from the home"})
	}
	return out
}

// queued counts what's queued on Tools (the board shares the items).
func (m *Menu) queued() int {
	n := 0
	for _, i := range m.items {
		if i.Pending != "" {
			n++
		}
	}
	return n
}

// hasConnect: the home has its Connection panel.
func (m *Menu) hasConnect() bool { return len(m.connect) > 0 || len(m.info.Connect) > 0 || len(m.info.Keys) > 0 }

// gotoAt is where Go to's entries start among the home's rows.
func (m *Menu) gotoAt() int {
	for n, r := range m.rows() {
		if r.id == "tools" {
			return n
		}
	}
	return 0
}

// rows are the home's rows, as the machine is now: what needs doing, the
// Connection panel, then Go to's: Tools, MCP servers (with the gateway),
// Settings and Last run.
func (m *Menu) rows() []menuRow {
	out := m.todo()
	if m.hasConnect() {
		out = append(out, menuRow{id: "connect", label: "Connection", note: "from a laptop: SSH, Docker, the MCP gateway (mcpjungle), copy and paste"})
	}
	tools := menuRow{id: "tools", label: "Tools", note: "yours: fix, add or uninstall · or just start typing"}
	if n := m.queued(); n > 0 {
		tools.note = plural(n, "change", "changes") + " queued · a there applies"
	}
	out = append(out, tools)
	if m.hasMCP() {
		var names []string
		problems, off := 0, 0
		for _, s := range m.info.MCP {
			if s.Yardo {
				names = append(names, s.Name)
				switch s.State {
				case "ok", "unchecked":
				case "off":
					off++
				default:
					problems++
				}
			}
		}
		note := "none yet: add GitHub, Jira, Grafana and more"
		if len(names) > 0 {
			note = strings.Join(names, ", ")
			if m.info.MCPAllOff {
				note = "all off · " + note
			} else if off > 0 {
				note += fmt.Sprintf(" · %d off", off)
			}
			if problems > 0 {
				note += " · " + plural(problems, "needs you", "need you")
			}
		}
		out = append(out, menuRow{id: "mcp", label: "MCP servers", note: note})
	}
	out = append(out, menuRow{id: "settings", label: "Settings", note: "network access, git, prompt, look"})
	if l := m.info.LastRun; l != nil {
		out = append(out, menuRow{id: "lastrun", label: "Last run", note: l.When + " · " + l.say()})
	}
	return out
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
	// The look: one row (the theme, the accent colour), one screen.
	if ls := m.section(LookSection); len(ls) > 0 {
		var now []string
		for _, s := range ls {
			now = append(now, s.Label+": "+s.label(s.Value))
		}
		label := ls[0].Label
		if len(ls) > 1 {
			label = "Look"
		}
		out = append(out, menuRow{id: "look", label: label, note: strings.Join(now, " · ")})
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
	return out
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

// Checker has u (and Needs you's Check for updates) start a check for
// updates with start, in the background; without one, the menu is left
// with MenuUpdates.
func (m *Menu) Checker(start func() func() UpdatesMsg) *Menu {
	m.check = start
	return m
}

// KeyDropper has d on a key under Keys to add forget it with f.
func (m *Menu) KeyDropper(f func(pub string) error) *Menu {
	m.dropKey = f
	return m
}

func (m *Menu) Init() tea.Cmd {
	cmds := []tea.Cmd{readMachine}
	if m.wait != nil {
		cmds = append(cmds, m.waitCmd())
	}
	return tea.Batch(cmds...)
}

func (m *Menu) waitCmd() tea.Cmd {
	wait := m.wait
	return func() tea.Msg { return wait() }
}

func (m *Menu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case machineMsg:
		v.took()
		// The first two readings close together: the CPU shows sooner.
		if len(machine.m.CPU) == 0 {
			return m, tickMachine(time.Second)
		}
		return m, tickMachine(machineEvery)
	case mcpSwitched:
		m.switched(v)
	case UpdatesMsg:
		for _, i := range m.items {
			if i.Picked {
				i.Updates = v.ByID[i.ID]
			}
		}
		m.wait, m.checked, m.info.Checked = nil, true, v.At
		switch n := len(m.filter(newer)); {
		case !m.asked:
		case n > 0:
			m.toast = plural(n, "update", "updates") + " found: Install " + plural(n, "update", "updates") + " is on the home."
		default:
			m.toast = "No updates: everything is current."
		}
		m.asked = false
		m.normalize()
	case MoveMsg:
		m.toast = ""
		m.move(v.N)
		m.normalize()
	case tea.KeyMsg:
		m.toast = ""
		m.normalize()
		var cmd tea.Cmd
		switch {
		case m.dialog != nil:
			if v.String() == "ctrl+c" {
				cmd = m.quit(MenuResult{Kind: MenuQuit})
				break
			}
			cmd = dialogKey(&m.dialog, v.String())
		case m.screen == "mcp:name":
			cmd = m.nameKey(v)
		case m.screen == "home" && v.Type == tea.KeyRunes && !homeKey(string(v.Runes)) || m.screen == "home" && v.String() == "/":
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

// homeKey: k is one of the home's keys, not the start of a search.
func homeKey(k string) bool {
	switch k {
	case "q", "h", "j", "k", "l", "g", "G", "u", "?":
		return true
	}
	return false
}

// move moves the cursor n rows (normalize keeps it on one).
func (m *Menu) move(n int) {
	switch m.screen {
	case "home", "fix", "secrets", "settings", "access", "look", "mcp:server", "mcp:add", "updates", "lastrun":
		m.sel = clamp(m.sel+n, -toEnd, toEnd)
	case "connect":
		m.sel = clamp(m.sel+n, -toEnd, toEnd)
	case "mcp":
		top := 0
		if m.hasAll() {
			top = -1
		}
		m.sel = clamp(m.sel+n, top, m.lastRow())
	}
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
		if len(m.filter(problem)) == 0 {
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
	case "lastrun":
		if m.info.LastRun == nil {
			m.back()
		}
	case "mcp:server":
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
	case "updates":
		m.sel = clamp(m.sel, 0, len(m.filter(newer))-1)
	case "connect":
		m.sel = clamp(m.sel, 0, len(m.info.Keys)-1)
	case "lastrun":
		m.sel = clamp(m.sel, 0, max(0, len(m.lastRunLines(m.width))-m.listRoom()))
	case "mcp":
		top := 0
		if m.hasAll() {
			top = -1 // All servers, above the servers
		}
		m.sel = clamp(m.sel, top, m.lastRow()) // then Add a server
	case "mcp:server":
		m.sel = clamp(m.sel, 0, len(m.mcpActions(m.mcpServer()))-1)
	case "mcp:add":
		m.sel = clamp(m.sel, 0, len(m.info.MCPSpecs)-1)
	}
}

// listRoom is how many lines a screen has for its list.
func (m *Menu) listRoom() int { return max(4, m.height-6) }

// fixRows are the fix screen's rows: a checklist, a problem a row.
func (m *Menu) fixRows() int { return len(m.filter(problem)) }

// ticked are the fix screen's ticked problems' ids.
func (m *Menu) ticked() []string {
	var ids []string
	for _, i := range m.filter(problem) {
		if !m.unticked[i.ID] {
			ids = append(ids, i.ID)
		}
	}
	return ids
}

func (m *Menu) quit(r MenuResult) tea.Cmd {
	if r.Kind != MenuQuit && r.Kind != MenuTools && len(m.pend) > 0 {
		// It leaves the menu: what's marked would be lost.
		m.toast = "Apply first (a), or turn back what you changed."
		return nil
	}
	m.result, m.ended = r, true
	return nil
}

// open goes to screen, at its first row.
func (m *Menu) open(screen string) {
	m.screen, m.sel, m.dialog = screen, 0, nil
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
	m.screen, m.sel, m.dialog = "home", 0, nil
	rows := m.rows
	switch from {
	case "access", "look":
		m.screen, rows = "settings", m.settingsRows
	}
	for n, r := range rows() {
		if r.id == from {
			m.sel = n
		}
	}
}

// checkUpdates starts a check for updates (u), in the background.
func (m *Menu) checkUpdates() tea.Cmd {
	switch {
	case m.wait != nil:
		m.toast = "Checking already: what it finds shows here."
		return nil
	case m.check == nil:
		return m.quit(MenuResult{Kind: MenuUpdates})
	}
	m.wait, m.asked = m.check(), true
	m.toast = "Checking for updates in the background: what it finds shows here."
	return m.waitCmd()
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
		rs := m.rows()
		r := rs[m.sel]
		at, todo := m.gotoAt(), len(m.todo())
		con := slices.IndexFunc(rs, func(r menuRow) bool { return r.id == "connect" })
		switch {
		case up:
			m.sel--
		case down:
			m.sel++
		case right && m.sel < at:
			// To Go to, then along it.
			m.sel = at
		case right:
			m.sel++
		case left && m.sel > at:
			m.sel--
		case left:
		case k == "tab":
			// Needs you, Connection, Go to, then back round.
			switch {
			case m.sel < todo && con >= 0:
				m.sel = con
			case m.sel < at:
				m.sel = at
			default:
				m.sel = 0
			}
		case k == "shift+tab":
			switch {
			case m.sel >= at && con >= 0:
				m.sel = con
			case m.sel >= at || m.sel == con:
				m.sel = 0
			default:
				m.sel = at
			}
		case k == "q":
			return m.quit(MenuResult{Kind: MenuQuit})
		case k == "esc":
			m.toast = "q quits"
		case k == "u" || k == "enter" && r.id == "check":
			return m.checkUpdates()
		case k == "enter" && strings.HasPrefix(r.id, "fix:"):
			return m.quit(MenuResult{Kind: MenuFix, ID: r.item.ID, IDs: []string{r.item.ID}})
		case k == "enter" && strings.HasPrefix(r.id, "key:"):
			return m.quit(MenuResult{Kind: MenuEdit, ID: r.item.ID, Inputs: []string{r.input}})
		case k == "enter" && r.id == "tools":
			return m.quit(MenuResult{Kind: MenuTools})
		case k == "enter" && r.id == "why":
			m.open("lastrun")
		case k == "enter" && (strings.HasPrefix(r.id, "mcp:") || strings.HasPrefix(r.id, "mcpkey:")):
			// The MCP servers, on this one.
			_, name, _ := strings.Cut(r.id, ":")
			m.open("mcp")
			for n, x := range m.mcpServers() {
				if x.Name == name {
					m.sel = n
				}
			}
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
		case k == "enter":
			m.open(r.id)
		}
	case "fix":
		ps := m.filter(problem)
		switch {
		case back:
			m.back()
		case up:
			m.sel--
		case down:
			m.sel++
		case (k == " " || k == "x") && m.sel < len(ps):
			id := ps[m.sel].ID
			m.unticked[id] = !m.unticked[id]
		case k == "enter":
			ids := m.ticked()
			if len(ids) == 0 {
				m.toast = "Nothing ticked: space ticks a row."
				return nil
			}
			return m.quit(MenuResult{Kind: MenuFix, ID: ids[0], IDs: ids})
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
		case k == "enter" || k == "a":
			set := m.changed()
			if len(set) == 0 {
				m.toast = "Nothing changed: ←→ picks a setting's choice."
				return nil
			}
			return m.quit(MenuResult{Kind: MenuApply, Settings: set})
		}
	case "mcp", "mcp:server", "mcp:add":
		return m.mcpKey(k, up, down, left, right)
	case "connect":
		switch {
		case back:
			m.back()
		case up:
			m.sel--
		case down:
			m.sel++
		case k == "d" && m.sel < len(m.info.Keys):
			key := m.info.Keys[m.sel]
			if m.dropKey != nil {
				if err := m.dropKey(key.Pub); err != nil {
					m.toast = "✗ " + err.Error()
					return nil
				}
			}
			m.info.Keys = slices.Delete(slices.Clone(m.info.Keys), m.sel, m.sel+1)
			m.toast = "Dismissed the " + key.For() + " key."
		}
	case "lastrun":
		switch {
		case back:
			m.back()
		case up:
			m.sel--
		case down:
			m.sel++
		}
	case "updates":
		switch {
		case back:
			m.back()
		case up:
			m.sel--
		case down:
			m.sel++
		case k == "enter" && len(m.filter(newer)) > 0:
			return m.quit(MenuResult{Kind: MenuUpgrade})
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

// View is the menu drawn alone, as the app draws it.
func (m *Menu) View() string { return standalone(m, m.info, m.width, m.height) }

// Toast is the line the menu says above its keys.
func (m *Menu) Toast() string { return m.toast }

// Hits are where the last Body drew what a click selects.
func (m *Menu) Hits() []Hit { return m.hits }

// Typing: a name is being typed.
func (m *Menu) Typing() bool { return m.screen == "mcp:name" && m.dialog == nil }

// hit records that a click on body line y (columns x0 to x1; x1 0: all
// of it) selects with do.
func (m *Menu) hit(y, x0, x1 int, do func()) {
	m.hits = append(m.hits, Hit{Y0: y, Y1: y, X0: x0, X1: x1, Do: do})
}

// selects is a click's do: the cursor onto row n.
func (m *Menu) selects(n int) func() { return func() { m.sel = n } }

// Path is where the screen is, from the home.
func (m *Menu) Path() string {
	switch m.screen {
	case "home":
		return "Home"
	case "settings":
		return "Home › Settings"
	case "access":
		return "Home › Settings › Network access"
	case "look":
		return "Home › Settings › Look"
	case "fix":
		return "Home › Fix " + plural(len(m.filter(problem)), "thing", "things")
	case "secrets":
		return "Home › Fill in " + plural(len(m.filter(secret)), "value", "values")
	case "updates":
		return "Home › Updates"
	case "connect":
		return "Home › Connection"
	case "lastrun":
		return "Home › Last run"
	}
	return "Home › " + m.mcpPath()
}

// Back is what q does.
func (m *Menu) Back() string {
	switch {
	case m.dialog != nil:
		return "cancel"
	case m.screen == "home":
		return "quit"
	case m.screen == "mcp:name":
		return ""
	case m.screen == "mcp" && m.fromBoard:
		return "back to Tools"
	}
	return "back"
}

// Keys are the footer's: what each key does on this screen.
func (m *Menu) Keys() []Key {
	if m.dialog != nil {
		return dialogKeys(m.dialog)
	}
	switch m.screen {
	case "home":
		return []Key{{Key: "↑↓←→", What: "choose"}, {Key: "tab", What: "next"}, {Key: "enter", What: "open"}, {Key: "u", What: "updates"}, {Key: "type", What: "search"}}
	case "settings", "secrets":
		return []Key{{Key: "↑↓", What: "choose"}, {Key: "enter", What: map[bool]string{true: "fill in", false: "open"}[m.screen == "secrets"]}}
	case "fix":
		return []Key{{Key: "↑↓", What: "choose"}, {Key: "space", What: "tick"}, {Key: "enter", What: "fix " + fmt.Sprint(len(m.ticked()))}}
	case "access", "look":
		return []Key{{Key: "↑↓", What: "setting"}, {Key: "←→", What: "choose"}, {Key: "a", What: "apply", On: len(m.changed()) > 0}}
	case "connect":
		if len(m.info.Keys) > 0 {
			return []Key{{Key: "↑↓", What: "choose a key"}, {Key: "d", What: "dismiss it"}}
		}
		return nil
	case "lastrun":
		return []Key{{Key: "↑↓", What: "scroll"}}
	case "updates":
		return []Key{{Key: "enter", What: "update " + fmt.Sprint(len(m.filter(newer)))}}
	}
	return m.mcpKeys()
}

// Help are the screen's keys, for the keys screen.
func (m *Menu) Help() []Key {
	switch m.screen {
	case "home":
		return []Key{{Key: "↑↓ j k", What: "move through Needs you, Connection and Go to"}, {Key: "←→ h l", What: "to Go to, and along it"},
			{Key: "tab", What: "the next panel: Needs you, Connection, Go to"}, {Key: "enter", What: "open the row, or do what it says"},
			{Key: "u", What: "check for updates, in the background"}, {Key: "a letter, /", What: "search the tools"}, {Key: "q", What: "quit"}}
	}
	return m.Keys()
}

// Body is the screen under the header, w by h.
func (m *Menu) Body(w, h int) string {
	m.hits = nil
	if m.screen == "home" {
		return m.homeView(w, h)
	}
	var b []string
	add := func(s ...string) { b = append(b, s...) }
	// at: the body line the next add lands on.
	at := func() int { return len(b) }
	add("")
	switch m.screen {
	case "settings":
		rs := m.settingsRows()
		col := labelCol(rs, w)
		for n, r := range rs {
			m.hit(at(), 0, 0, m.selects(n))
			add(m.row(n == m.sel, r, col, w)...)
		}
	case "fix":
		ps := m.filter(problem)
		add("  "+muted.Render("What yardo set up that isn't as it was. Enter fixes the ticked ones, in one run."), "")
		col := 8
		for _, i := range ps {
			col = max(col, runewidth.StringWidth(short(i.Label)))
		}
		col = min(col, 34)
		var lines []string
		var rowOf []int
		selLine := 0
		for n, i := range ps {
			box := "[x]"
			if m.unticked[i.ID] {
				box = "[ ]"
			}
			name := padTo(runewidth.Truncate(short(i.Label), col, "…"), col)
			line := box + " " + name + "  " + padTo(stateWord(i.State), 24)
			if n == m.sel {
				selLine = len(lines)
				line = rowOn.Render(" › " + line + " ")
			} else {
				line = "   " + bold.Render(box+" "+name) + "  " + yellow.Render(padTo(stateWord(i.State), 24))
			}
			lines = append(lines, "  "+line+"  "+muted.Render(runewidth.Truncate(fixVerb(i), max(w-col-40, 10), "…")))
			rowOf = append(rowOf, n)
			// The details, under every row.
			for _, l := range wrap(strings.TrimSpace(i.Detail+"  "+i.Source), w-12) {
				lines = append(lines, "         "+muted.Render(l))
				rowOf = append(rowOf, n)
			}
		}
		m.addList(&b, lines, rowOf, selLine, h-len(b))
	case "secrets":
		ss := m.filter(secret)
		for n, i := range ss {
			label := padTo(short(i.Label), 40)
			if n == m.sel {
				label = rowOn.Render(" › " + label)
			} else {
				label = "   " + label
			}
			m.hit(at(), 0, 0, m.selects(n))
			add("  "+label+"  "+accent.Render(strings.Join(i.Inputs, ", ")), "")
		}
		add("  " + muted.Render("Enter opens the file in your editor; yardo never stores the value."))
	case "access", "look":
		as := m.choices()
		for n, s := range as {
			on := n == m.sel
			name := bold.Render("  " + s.Label)
			if on {
				name = accentBd.Render("› " + s.Label)
			}
			m.hit(at(), 0, 0, m.selects(n))
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
		// Apply stands out once something's changed.
		n := len(m.changed())
		apply := muted.Render("[ Apply ]   nothing changed yet: ←→ picks a choice")
		if n > 0 {
			label := "Apply " + plural(n, "change", "changes")
			apply = accentBd.Render("[ "+label+" ]") + "   " + muted.Render("a applies from anywhere · q leaves without changing anything")
			if m.sel == len(as) {
				apply = rowOn.Render("  "+label+"  ") + "   " + muted.Render("q leaves without changing anything")
			}
		} else if m.sel == len(as) {
			apply = rowOn.Render(" Apply ") + "   " + muted.Render("nothing changed yet: ←→ picks a choice")
		}
		m.hit(at(), 0, 0, m.selects(len(as)))
		add("  " + apply)
	case "connect":
		for _, l := range m.connect {
			if p, ok := strings.CutPrefix(l, Paste); ok {
				add("  " + keyOn.Render(p))
			} else {
				add("  " + l)
			}
		}
		if len(m.info.Keys) > 0 {
			add("", "  "+bold.Render("Keys to add")+muted.Render("  ·  d dismisses one once it's added"))
			for n, k := range m.info.Keys {
				add("")
				for c, wh := range k.Where {
					l := wh
					if c == 0 && n == m.sel {
						l = rowOn.Render(" › " + l + " ")
					} else {
						l = "   " + l
					}
					m.hit(at(), 0, 0, m.selects(n))
					add("  " + l)
				}
				add("", keyOn.Render(k.Pub), "")
				if k.Path != "" {
					add("     " + muted.Render("Show it again: cat "+k.Path))
				}
			}
		}
	case "lastrun":
		ls := m.lastRunLines(w)
		room := h - len(b)
		start := clamp(m.sel, 0, max(0, len(ls)-room))
		add(ls[start:min(len(ls), start+room)]...)
	case "mcp", "mcp:server", "mcp:add", "mcp:name":
		add(m.mcpView(w, h-len(b))...)
	case "updates":
		us := m.filter(newer)
		for n, i := range us {
			line := padTo(short(i.Label), 40) + muted.Render(strings.Join(i.Updates, ", "))
			if n == m.sel {
				line = rowOn.Render(" › "+padTo(short(i.Label), 40)) + " " + muted.Render(strings.Join(i.Updates, ", "))
			} else {
				line = "   " + keyOn.Render("↑ ") + line
			}
			m.hit(at(), 0, 0, m.selects(n))
			add("  " + line)
		}
		add("", "  "+bold.Render("Enter updates "+plural(len(us), "item", "items"))+"  "+muted.Render("mise up for the tools, apt for the packages; nothing else changes."),
			"  "+muted.Render("yardo doesn't upgrade on its own."))
	}
	if m.dialog != nil {
		dl := dialogLines(m.dialog, w, 0, nil)
		cut := max(0, min(len(b), h-len(dl)))
		var dh []Hit
		dl = dialogLines(m.dialog, w, cut, &dh)
		b = append(b[:cut], dl...)
		m.hits = dh
	}
	return strings.Join(b, "\n")
}

// addList adds lines to b, room lines at most, scrolled to keep line sel
// in view (a long Fix); a click on a line selects row rowOf[line].
func (m *Menu) addList(b *[]string, lines []string, rowOf []int, sel, room int) {
	start := windowStart(len(lines), sel, room)
	for n, l := range window(lines, sel, room) {
		m.hit(len(*b), 0, 0, m.selects(rowOf[start+n]))
		*b = append(*b, l)
	}
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

// row is one of the home's (or Settings') rows, its label col cells wide,
// its note cut to the line: the selected one's too (wrapped, it would move
// the rows below as the selection moves).
func (m *Menu) row(on bool, r menuRow, col, w int) []string {
	note := muted.Render(runewidth.Truncate(r.note, max(4, w-col-9), "…"))
	if on {
		return []string{"  " + rowOn.Render(padTo(" › "+r.label, col+3)) + "  " + note}
	}
	return []string{"     " + padTo(r.label, col) + "  " + note}
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
