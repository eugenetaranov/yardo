package ui

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The menu is yardo's calm home: panels that show how the machine is,
// Needs you first ("Start Docker", "Fill in OPENAI_API_KEY", "Install 3
// updates"), each a row that goes where it's done; and along the top, on
// every screen, the tabs that open everything else: Tools (the board),
// MCP servers, Skills, Network, Local settings, Settings and Last run (1
// to 9 jump to them), each with something in it that needs you carrying
// its mark. Typing on the home searches the tools. Each tab opens one
// focused screen; q comes back. On the home, q quits. It's one of the
// app's views (see App).

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
	// MenuApply applies Result.Settings (network access, the look).
	MenuApply
	// MenuUpdates checks for newer versions again, in the background.
	MenuUpdates
	// MenuMCP does Result.MCP to an MCP server: add, edit, on, restart,
	// rename or delete it (restart with no Entry: all of them); cleanup
	// removes the orphaned containers.
	MenuMCP
	// MenuChange asks Result.ID's Result.Change question again (the git
	// name, the prompt, the dotfiles repo), then re-applies it.
	MenuChange
	// MenuUpdatesTab opens Tools on the Updates tab, every fix and update
	// queued.
	MenuUpdatesTab
	// MenuConnection opens Connection on Result.ID's tab (NetTab,
	// LocalTab), and after a colon on the first row of that kind (key).
	MenuConnection
	// MenuSkills does Result.MCP to a skill (Entry: its name): add (from
	// elsewhere), check, review (one held) or delete it.
	MenuSkills
	// MenuFixes opens Tools on the Updates tab, on Result.ID's row (the
	// first, with none); nothing queued.
	MenuFixes
	// MenuReviewed: a review installed, declined or checked again
	// Result.MCP.Entry (Do: what it did); the machine is read again and
	// Result.ID's screen (skills, mcp, home) shown, saying Result.Toast.
	MenuReviewed
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
	Toast    string
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
	checked  bool // updates were checked
	// wait waits for a check for updates running in the background (nil
	// for none); check starts one (u), returning its wait.
	wait  func() UpdatesMsg
	check func() func() UpdatesMsg
	// asked: the check running was u's, which says what it found.
	asked bool
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
	// The same for the skills (skBefore, skBeforeAll: as they're saved),
	// and the skill chosen.
	skSwitcher  SkillSwitcher
	skSwitching bool
	skPend      map[string]string
	skBefore    []SkillRow
	skBeforeAll bool
	skSel       int
	// reviewer runs the Review screen's checks and decisions; rv is the
	// review open (screen review), rvFrom the screen it came from.
	reviewer Reviewer
	rvCtx    context.Context
	rv       *review
	rvFrom   string

	screen string // home, settings, access, look, shell, lastrun, mcp, mcp:server, mcp:add, mcp:name, skills, skills:skill, review
	sel    int
	// need is the home's cursor in Needs you (-1: on the tabs, at sel).
	need int
	// setups are Local settings' (the app's Connection's): the home's
	// Connection panel says the everyday ones.
	setups []Setup
	// The MCP screens: the server and catalog row chosen, the name typed
	// (for nameFor: a spec to add, or an entry to rename), its error;
	// fromBoard: q on the list goes back to the board.
	mcpSel, addSel int
	name           []rune
	nameFor        string
	renaming       bool
	nameErr        string
	fromBoard      bool
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
func NewMenu(info HomeInfo, sub string, items []*BoardItem, settings []*BoardSetting, checked bool) *Menu {
	m := &Menu{info: info, sub: sub, items: items, settings: settings, checked: checked,
		screen: "home", need: -1, pick: map[string]string{}, width: 100, height: 30}
	m.sortMCP()
	m.normalize()
	// On Tools; in Needs you, on its first, when something needs you.
	m.sel = max(0, slices.IndexFunc(m.rows(), func(r menuRow) bool { return r.id == "tools" }))
	if len(m.needList()) > 0 {
		m.need = 0
	}
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
	switch m.screen {
	case "look":
		return m.section(LookSection)
	case "shell":
		return m.section(ShellSection)
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
		return fmt.Sprintf("Update %s's setup", n)
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
	return fmt.Sprintf("%s: %s", stateWord(i.State), i.Detail)
}

// todo is what needs doing: the last run, when it failed; each problem
// (or, with three or more, one for them all), each value to fill in (or
// one for three or more), the MCP servers that aren't ready, and the
// updates. Each is on a tab (see needs).
func (m *Menu) todo() []menuRow {
	var out []menuRow
	if l := m.info.LastRun; l != nil && !l.OK {
		out = append(out, menuRow{id: "why", label: "See why the last run failed", note: fmt.Sprintf("%s · %s", l.When, l.say())})
	}
	if ps := m.filter(problem); len(ps) <= 2 {
		for _, i := range ps {
			out = append(out, menuRow{id: "fix:" + i.ID, label: fixVerb(i), note: fixNote(i), item: i})
		}
	} else {
		var why []string
		for _, i := range ps {
			why = append(why, fmt.Sprintf("%s %s", short(i.Label), stateWord(i.State)))
		}
		out = append(out, menuRow{id: "fix", label: "Fix " + plural(len(ps), "thing", "things"), note: strings.Join(why, " · ")})
	}
	var ins []menuRow
	for _, i := range m.filter(secret) {
		for _, in := range i.Inputs {
			_, k, _ := strings.Cut(in, ": ")
			ins = append(ins, menuRow{id: fmt.Sprintf("key:%s:%s", i.ID, k), label: "Fill in " + k, note: short(i.Label) + " needs it · opens its file", item: i, input: in})
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
			case x.State == "failed" || x.State == "refused" || x.State == "retired" || x.State == "nospec":
				out = append(out, menuRow{id: "mcp:" + x.Name, label: "Check " + x.Name, note: strings.TrimPrefix(ansi.Strip(mcpWord(x)), tui.Cur.Mark.Warn+" ")})
			}
		}
		if n := len(m.info.MCPOrphans); n > 0 {
			out = append(out, menuRow{id: "mcporphans", label: "Clean up " + plural(n, "orphaned container", "orphaned containers"), note: m.orphansOf()})
		}
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

// needList is what needs you, as Needs you lists it: todo's, then the
// keys to add, then the skills changed since checked or held for you.
func (m *Menu) needList() []menuRow {
	out := m.todo()
	if n := len(m.info.Keys); n > 0 {
		var fors []string
		for _, k := range m.info.Keys {
			fors = append(fors, k.For())
		}
		out = append(out, menuRow{id: "keys", label: fmt.Sprintf("Add %s to %s", plural(n, "key", "keys"), strings.Join(fors, ", ")), note: "this machine's SSH keys, for git there: copy, then add"})
	}
	for _, s := range m.info.Skills {
		switch {
		case s.Source == "drifted":
			out = append(out, menuRow{id: "skill:" + s.Name, label: fmt.Sprintf("Check %s again", s.Name), note: "a skill changed since checked: restore it, or check it again"})
		case s.Off == "held":
			out = append(out, menuRow{id: "held:skill:" + s.Name, label: "Review " + s.Name, note: cmpOr(s.Held, "a skill held for your decision")})
		}
	}
	for _, s := range m.info.MCP {
		if s.State == "held" {
			out = append(out, menuRow{id: "held:mcp:" + s.Name, label: "Review " + s.Name, note: cmpOr(s.Held, "an MCP server held for your decision")})
		}
	}
	return out
}

// needTab is the tab along the top what needs you (by its row's id) is
// on: the last run's failure on Last run, the MCP servers' on MCP
// servers, the skills' on Skills, the keys to add on Network, the rest
// (fixes, values, updates) on Tools.
func needTab(id string) string {
	switch {
	case id == "why":
		return "lastrun"
	case strings.HasPrefix(id, "held:skill:"):
		return "skills"
	case strings.HasPrefix(id, "mcp"), strings.HasPrefix(id, "held:mcp:"):
		return "mcp"
	case strings.HasPrefix(id, "skill:"):
		return "skills"
	case id == "keys":
		return NetTab
	}
	return "tools"
}

// needs are what needs you, by the tab it's on (see needTab).
func (m *Menu) needs() map[string][]menuRow {
	out := map[string][]menuRow{}
	for _, r := range m.needList() {
		out[needTab(r.id)] = append(out[needTab(r.id)], r)
	}
	return out
}

// goNeed goes where r, a row of Needs you, is done: the board's Updates
// tab for updates (queued) and fixes and values (on the tool's row), the
// server or the skill, Network's keys, Last run.
func (m *Menu) goNeed(r menuRow) tea.Cmd {
	switch {
	case r.id == "updates":
		return m.quit(MenuResult{Kind: MenuUpdatesTab})
	case r.id == "why":
		m.open("lastrun")
	case r.id == "keys":
		return m.quit(MenuResult{Kind: MenuConnection, ID: NetTab + ":key"})
	case strings.HasPrefix(r.id, "held:"):
		kind, name, _ := strings.Cut(strings.TrimPrefix(r.id, "held:"), ":")
		return m.openReview(kind, name)
	case strings.HasPrefix(r.id, "skill:"):
		n := slices.IndexFunc(m.info.Skills, func(s SkillRow) bool { return "skill:"+s.Name == r.id })
		m.open("skills")
		if n >= 0 {
			m.skSel, m.screen = n, "skills:skill"
		}
	case strings.HasPrefix(r.id, "mcp"):
		_, name, _ := strings.Cut(r.id, ":")
		m.open("mcp")
		if n := slices.IndexFunc(m.mcpServers(), func(s *MCPServer) bool { return s.Name == name }); n >= 0 {
			m.sel = n
		}
	default:
		id := ""
		if r.item != nil {
			id = r.item.ID
		}
		return m.quit(MenuResult{Kind: MenuFixes, ID: id})
	}
	m.need = -1
	return nil
}

// openTab opens the tab along the top with id, from the home.
func (m *Menu) openTab(id string) tea.Cmd {
	switch id {
	case "tools":
		return m.quit(MenuResult{Kind: MenuTools})
	case NetTab, LocalTab:
		return m.quit(MenuResult{Kind: MenuConnection, ID: id})
	}
	m.open(id)
	return nil
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

// rows are the home's tabs: Tools, MCP servers (with the gateway), Skills
// (when there are any), Network, Local settings (with something to set
// up), Settings and Last run. A tab's note starts with what on it needs
// you.
func (m *Menu) rows() []menuRow {
	need := m.needs()
	tab := func(id, label, note string) menuRow {
		var ls []string
		for _, r := range need[id] {
			ls = append(ls, r.label)
		}
		return menuRow{id: id, label: label, note: strings.Join(append(ls, note), " · ")}
	}
	var out []menuRow
	picked := len(m.filter(func(i *BoardItem) bool { return i.Picked }))
	ok := len(m.filter(func(i *BoardItem) bool { return i.Picked && i.State == "ok" }))
	tools := fmt.Sprintf("%d of %d tools working", ok, picked)
	if n := len(m.filter(func(i *BoardItem) bool { return i.Picked && i.State == "unchecked" })); n > 0 {
		tools += fmt.Sprintf(", %d not checked", n)
	}
	// A machine set up from a team profile says how much is the team's.
	team, teams := 0, map[string]bool{}
	for _, i := range m.items {
		if i.Picked && i.Team != "" {
			team++
			teams[i.Team] = true
		}
	}
	if team > 0 {
		tools += fmt.Sprintf(" · %d from the team profile %s", team, strings.Join(slices.Sorted(maps.Keys(teams)), ", "))
	}
	// The updates: Needs you has what's newer; else whether they were
	// looked for, and when.
	switch {
	case m.wait != nil:
		tools += " · checking for updates…"
	case len(m.filter(newer)) > 0:
	case m.checked:
		tools += " · nothing newer"
	default:
		tools += " · updates not checked yet"
	}
	if m.info.Checked != "" && m.wait == nil {
		tools += " · checked " + m.info.Checked
	}
	tools += " · fix, add or uninstall · or just start typing"
	if len(m.filter(newer)) == 0 && m.wait == nil {
		tools += " · u checks apt and mise, in the background"
	}
	if n := m.queued(); n > 0 {
		tools = plural(n, "change", "changes") + " queued · a there applies"
	}
	out = append(out, tab("tools", "Tools", tools))
	if m.hasMCP() {
		var names []string
		off := 0
		for _, s := range m.info.MCP {
			if s.Yardo {
				names = append(names, s.Name)
				if s.State == "off" {
					off++
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
		}
		out = append(out, tab("mcp", "MCP servers", note))
	}
	if n := len(m.info.Skills); n > 0 {
		on := 0
		for _, s := range m.info.Skills {
			if s.Off == "" {
				on++
			}
		}
		note := fmt.Sprintf("%d of %d on · turn one off, or all", on, n)
		if m.info.SkillsAllOff {
			note = "all off · space on All skills turns them back on"
		}
		// An update (what's changed since checked, or held, is Needs
		// you's, said first).
		var upd []string
		for _, s := range m.info.Skills {
			if s.Update != "" && s.Source != "drifted" && s.Off != "held" {
				upd = append(upd, s.Name)
			}
		}
		if len(upd) > 0 {
			note = "update available: " + strings.Join(upd, ", ")
		}
		out = append(out, tab("skills", "Skills", note))
	}
	out = append(out, tab(NetTab, "Network", "what this machine is on: start, stop, add, remove · who can reach what · keys to add"))
	if len(m.setups) > 0 {
		out = append(out, tab(LocalTab, "Local settings", "to use this machine from your computer: SSH, Docker, Claude Code, Codex, pages · c copies"))
	}
	out = append(out, tab("settings", "Settings", "git, prompt, look"))
	if l := m.info.LastRun; l != nil {
		out = append(out, tab("lastrun", "Last run", fmt.Sprintf("%s · %s", l.When, l.say())))
	}
	return out
}

// changeLabels name the answers Settings can change (BoardItem.Changes'
// keys); the MCP servers have their own row.
var changeLabels = map[string]string{"git": "Git name and email", "prompt": "Shell prompt", "dotfiles": "Dotfiles repo"}

// settingsRows are Settings' rows.
func (m *Menu) settingsRows() []menuRow {
	var out []menuRow
	// now is what ss are set to; named, each says its name (a row named
	// after its one setting says only its value).
	now := func(ss []*BoardSetting, named bool) string {
		var out []string
		for _, s := range ss {
			if named {
				out = append(out, fmt.Sprintf("%s: %s", s.Label, s.label(s.Value)))
			} else {
				out = append(out, s.label(s.Value))
			}
		}
		return strings.Join(out, " · ")
	}
	// The look: one row (the theme, the accent colour), one screen.
	if ls := m.section(LookSection); len(ls) > 0 {
		label := ls[0].Label
		if len(ls) > 1 {
			label = "Look"
		}
		out = append(out, menuRow{id: "look", label: label, note: now(ls, len(ls) > 1)})
	}
	// The shell: one row per setting, one screen.
	if ss := m.section(ShellSection); len(ss) > 0 {
		label := ss[0].Label
		if len(ss) > 1 {
			label = "Shell"
		}
		out = append(out, menuRow{id: "shell", label: label, note: now(ss, len(ss) > 1)})
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

// Checker has u (and enter on Updates, with nothing newer) start a check for
// updates with start, in the background; without one, the menu is left
// with MenuUpdates.
func (m *Menu) Checker(start func() func() UpdatesMsg) *Menu {
	m.check = start
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
	case skillsSwitched:
		m.skillsSaved(v)
	case reviewStage:
		if v.r == m.rv {
			return m, m.reviewed(m.rv.took(v))
		}
	case reviewRan:
		if v.r == m.rv {
			return m, m.reviewed(m.rv.took(v))
		}
	case editedMsg:
		if m.rv != nil {
			m.rv.took(v)
		}
	case UpdatesMsg:
		for _, i := range m.items {
			if i.Picked {
				i.Updates = v.ByID[i.ID]
			}
		}
		m.wait, m.checked, m.info.Checked = nil, true, v.At
		switch n := len(m.filter(newer)); {
		case n == 0:
			m.toast = "No updates: everything is current."
		case m.asked:
			m.toast = plural(n, "update", "updates") + " found: they're on Tools › Updates."
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
		case m.screen == "review" && m.rv != nil:
			cmd = m.reviewed(m.rv.key(v.String()))
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
	case "home":
		// Needs you's; out of it, no cursor to move (the tabs are the
		// app's).
		if m.need >= 0 {
			m.need = clamp(m.need+n, 0, len(m.needList())-1)
		}
	case "settings", "access", "look", "shell", "mcp:server", "mcp:add", "lastrun", "skills:skill":
		m.sel = clamp(m.sel+n, -toEnd, toEnd)
	case "skills":
		m.sel = clamp(m.sel+n, -1, len(m.info.Skills)-1)
	case "mcp":
		top := 0
		if m.hasAll() {
			top = -1
		}
		m.sel = clamp(m.sel+n, top, m.lastRow())
	case "review":
		if m.rv != nil {
			m.rv.moveBy(n)
		}
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
	case "lastrun":
		if m.info.LastRun == nil {
			m.back()
		}
	case "mcp:server":
		if m.mcpServer() == nil {
			m.screen, m.sel = "mcp", 0
		}
	case "skills:skill":
		if m.skill() == nil {
			m.screen, m.sel = "skills", 0
		}
	case "review":
		if m.rv == nil {
			m.screen = cmpOr(m.rvFrom, "home")
		}
	}
	switch m.screen {
	case "home":
		m.sel = clamp(m.sel, 0, len(m.rows())-1)
		m.need = clamp(m.need, -1, len(m.needList())-1)
	case "settings":
		m.sel = clamp(m.sel, 0, len(m.settingsRows())-1)
	case "access", "look", "shell":
		m.sel = clamp(m.sel, 0, len(m.choices())) // Apply is the last
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
		m.sel = clamp(m.sel, 0, len(m.info.MCPSpecs)) // then From elsewhere
	case "skills":
		top := -1 // All skills, above the skills
		if len(m.info.Skills) == 0 {
			top = 0
		}
		m.sel = clamp(m.sel, top, len(m.info.Skills)-1)
	case "skills:skill":
		m.sel = clamp(m.sel, 0, len(m.skillActions(m.skill()))-1)
	}
}

// listRoom is how many lines a screen has for its list.
func (m *Menu) listRoom() int { return max(4, m.height-8) }

func (m *Menu) quit(r MenuResult) tea.Cmd {
	if r.Kind != MenuQuit && r.Kind != MenuTools && len(m.pend)+len(m.skPend) > 0 {
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
	case "access", "look", "shell":
		m.pick = map[string]string{}
	case "mcp":
		m.sortMCP()
	}
}

// back goes up a screen (Settings' own go back to it, Network access to
// Network, the rest home), onto the row of the screen it came from.
func (m *Menu) back() {
	from := m.screen
	m.screen, m.sel, m.dialog, m.need = "home", 0, nil, -1
	if from == "access" {
		m.result, m.ended = MenuResult{Kind: MenuConnection, ID: NetTab}, true
		return
	}
	rows := m.rows
	switch from {
	case "look", "shell":
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
		if m.need >= 0 {
			return m.needKey(k, up, down)
		}
		// The tabs are the app's (←→, 1–9, a click); here, Needs you.
		switch {
		case (down || k == "tab") && len(m.needList()) > 0:
			m.need = 0
		case k == "shift+tab" && len(m.needList()) > 0:
			m.need = len(m.needList()) - 1
		case k == "q":
			return m.quit(MenuResult{Kind: MenuQuit})
		case k == "esc":
			m.toast = "q quits"
		case k == "u":
			return m.checkUpdates()
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
	case "access", "look", "shell":
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
	case "skills", "skills:skill":
		return m.skillsKey(k, up, down)
	case "lastrun":
		switch {
		case back:
			m.back()
		case up:
			m.sel--
		case down:
			m.sel++
		}
	}
	return nil
}

// needKey is k with the home's cursor in Needs you: ↑↓ one row at a time
// (across a line's two, then down; ←→ are the tabs'), up past the first
// out of it, tab on to the next (past the last, out), enter goes there.
func (m *Menu) needKey(k string, up, down bool) tea.Cmd {
	ns := m.needList()
	switch {
	case up:
		m.need--
	case down:
		m.need = min(len(ns)-1, m.need+1)
	case k == "tab":
		if m.need++; m.need >= len(ns) {
			m.need = -1
		}
	case k == "shift+tab":
		m.need--
	case k == "q":
		return m.quit(MenuResult{Kind: MenuQuit})
	case k == "esc":
		m.need = -1
	case k == "u":
		return m.checkUpdates()
	case k == "enter":
		return m.goNeed(ns[m.need])
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
func (m *Menu) View() string {
	nav, tab := m.nav(m.width)
	return standalone(m, m.info, nav, tab, m.width, m.height)
}

// Toast is the line the menu says above its keys.
func (m *Menu) Toast() string {
	if m.screen == "review" && m.rv != nil && m.rv.Toast() != "" {
		return m.rv.Toast()
	}
	return m.toast
}

// Hits are where the last Body drew what a click selects.
func (m *Menu) Hits() []Hit { return m.hits }

// Typing: a name is being typed (or an add-on's, to install it).
func (m *Menu) Typing() bool {
	return m.screen == "mcp:name" && m.dialog == nil || m.screen == "review" && m.rv != nil && m.rv.typing()
}

// asking: the screen has a question open, or a review is running a
// check (its keys are its own till it's done).
func (m *Menu) asking() bool {
	return m.dialog != nil || m.screen == "review" && m.rv != nil && (m.rv.dialog != nil || m.rv.busy != "")
}

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
		return "Home › Network › Network access"
	case "look":
		return "Home › Settings › Look"
	case "shell":
		if ss := m.section(ShellSection); len(ss) == 1 {
			return "Home › Settings › " + ss[0].Label
		}
		return "Home › Settings › Shell"
	case "lastrun":
		return "Home › Last run"
	case "skills", "skills:skill":
		return "Home › " + m.skillsPath()
	case "review":
		if m.rv != nil {
			return "Home › " + m.rv.Path()
		}
	}
	return "Home › " + m.mcpPath()
}

// Back is what q does.
func (m *Menu) Back() string {
	switch {
	case m.dialog != nil:
		return "" // the dialog's keys say what esc and q do
	case m.screen == "home":
		return "quit"
	case m.screen == "review" && m.rv != nil:
		return m.rv.Back()
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
		if m.need >= 0 {
			return []Key{{Key: "↑↓", What: "choose", Keep: true}, {Key: "enter", What: "go there", Keep: true}, {Key: "u", What: "updates"}, {Key: "type", What: "search", Keep: true}}
		}
		if len(m.needList()) > 0 {
			return []Key{{Key: "↓", What: "needs you", Keep: true}, {Key: "u", What: "updates"}, {Key: "type", What: "search", Keep: true}}
		}
		return []Key{{Key: "u", What: "updates"}, {Key: "type", What: "search", Keep: true}}
	case "settings":
		return []Key{{Key: "↑↓", What: "choose"}, {Key: "enter", What: "open"}}
	case "access", "look", "shell":
		return []Key{{Key: "↑↓", What: "setting"}, {Key: "←→", What: "choose"}, {Key: "a", What: "apply", On: len(m.changed()) > 0}}
	case "lastrun":
		return []Key{{Key: "↑↓", What: "scroll"}}
	case "skills", "skills:skill":
		return m.skillsKeys()
	case "review":
		if m.rv != nil {
			return m.rv.Keys()
		}
	}
	return m.mcpKeys()
}

// Help are the screen's keys, for the keys screen.
func (m *Menu) Help() []Key {
	switch m.screen {
	case "home":
		return []Key{{Key: "←→ h l", What: "along the tabs at the top"},
			{Key: "↓ j tab", What: "into Needs you (↑ back to the tabs, esc too)"},
			{Key: "enter", What: "open the tab, or go where the thing that needs you is done"},
			{Key: "u", What: "Tools' Updates tab; with nothing there, check for updates"}, {Key: "a letter, /", What: "search the tools"}, {Key: "q", What: "quit"}}
	case "review":
		if m.rv != nil {
			return m.rv.Help()
		}
	}
	return m.Keys()
}

// Body is the screen under the header, w by h.
func (m *Menu) Body(w, h int) string {
	m.hits = nil
	if m.screen == "home" {
		return m.homeView(w, h)
	}
	if m.screen == "review" && m.rv != nil {
		s := m.rv.Body(w, h)
		m.hits = m.rv.Hits()
		return s
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
	case "access", "look", "shell":
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
			apply = fmt.Sprintf("%s   %s", accentBd.Render(fmt.Sprintf("[ %s ]", label)), muted.Render("a applies from anywhere · q leaves without changing anything"))
			if m.sel == len(as) {
				apply = fmt.Sprintf("%s   %s", rowOn.Render(fmt.Sprintf("  %s  ", label)), muted.Render("q leaves without changing anything"))
			}
		} else if m.sel == len(as) {
			apply = fmt.Sprintf("%s   %s", rowOn.Render(" Apply "), muted.Render("nothing changed yet: ←→ picks a choice"))
		}
		m.hit(at(), 0, 0, m.selects(len(as)))
		add("  " + apply)
	case "lastrun":
		ls := m.lastRunLines(w)
		room := h - len(b)
		start := clamp(m.sel, 0, max(0, len(ls)-room))
		add(ls[start:min(len(ls), start+room)]...)
	case "mcp", "mcp:server", "mcp:add", "mcp:name":
		add(m.mcpView(w, h-len(b))...)
	case "skills", "skills:skill":
		add(m.skillsView(w, h-len(b))...)
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
		return []string{fmt.Sprintf("  %s  %s", rowOn.Render(padTo(" › "+r.label, col+3)), note)}
	}
	return []string{fmt.Sprintf("     %s  %s", padTo(r.label, col), note)}
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
