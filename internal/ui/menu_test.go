package ui

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/eugenetaranov/yardo/internal/tui"
)

func testMenu(healthy bool) *Menu {
	items := []*BoardItem{
		{ID: "docker", Label: "Docker Engine + Compose", Tab: "k8s", Picked: true, State: "down", Detail: "docker isn't running", Source: "apt docker-ce"},
		{ID: "keys", Label: "Agent API keys", Tab: "logins", Picked: true, State: "secret", Inputs: []string{"~/.config/yardo/agent.env: OPENAI_API_KEY"}},
		{ID: "go", Label: "Go", Tab: "lang", Picked: true, State: "ok"},
		{ID: "wg", Label: "WireGuard server", Tab: "system"},
	}
	if healthy {
		items[0].State, items[1].State = "ok", "ok"
	}
	settings := []*BoardSetting{
		{Key: "mcp", Label: "MCP gateway", Value: "127.0.0.1", Choices: [][2]string{{"127.0.0.1", "this machine only"}, {"vpn", "this machine and your VPNs"}}},
		// The look: Settings' Accent colour, not network access.
		{Key: "accent", Label: "Accent colour", Section: LookSection, Value: "blue", Choices: [][2]string{{"blue", "Blue"}, {"green", "Green"}}, Swatches: map[string]string{"blue": "#0000ff"}},
	}
	info := HomeInfo{Host: "vm", User: "ada"}
	m := NewMenu(info, "Ubuntu 24.04  ·  10.0.0.5", items, settings, false)
	m.setups = []Setup{{ID: "ssh", What: "SSH", How: "ssh vm  ·  ada@10.0.0.5"}}
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	return m
}

func mk(m *Menu, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m.Update(msg)
	}
}

// The home: the tabs, Home's a block, then Needs you, each thing a row
// with its mark and the tab it's on, the cursor on its first; the tabs
// with something that needs you carry its mark, and the status line
// points at the panel. Nothing needing you, no panel.
func TestMenuHome(t *testing.T) {
	m := testMenu(false)
	v := ansi.Strip(m.View())
	for _, want := range []string{"2 things need you", "below: enter goes there", "─ Needs you ", "Machine", "[Home]",
		"Tools 3 ✗", "Settings", "type search", "Home"} {
		if !strings.Contains(v, want) {
			t.Errorf("home lacks %q:\n%s", want, v)
		}
	}
	for _, gone := range []string{"Updates", "Network access", "Check for updates", "›Tools"} {
		if strings.Contains(v, gone) {
			t.Errorf("home has %q:\n%s", gone, v)
		}
	}
	// In Needs you, on its first; the first line says what it is.
	if m.need != 0 || m.needList()[0].id != "fix:docker" {
		t.Errorf("starts on %d", m.need)
	}
	if l := lineWith(v, "│ ›✗ Start Docker Engine + Compose"); !strings.Contains(l, "Tools") || !strings.Contains(l, "Fill in OPENAI_API_KEY") {
		t.Errorf("Needs you, two to a line: %q", l)
	}
	onRow(m, "tools")
	v = ansi.Strip(m.View())
	if l := lineWith(v, "tools working"); !strings.Contains(l, "Start Docker Engine + Compose · Fill in OPENAI_API_KEY · 1 of 3 tools working") {
		t.Errorf("Tools' note: %q", l)
	}
	if !strings.Contains(v, "›Tools 3 ✗") || !strings.Contains(v, "below: ↓ chooses one") {
		t.Errorf("on the tabs:\n%s", v)
	}
	h := ansi.Strip(testMenu(true).View())
	if strings.Contains(h, "✗") || !strings.Contains(h, "All good") || strings.Contains(h, "Needs you") {
		t.Errorf("healthy home:\n%s", h)
	}
	if l := lineWith(h, "tools working"); !strings.Contains(l, "3 of 3 tools working") {
		t.Errorf("healthy Tools' note: %q", l)
	}
}

// Needs you's rows go where each is done: a fix and a value to Tools'
// Updates tab on the tool's row, nothing queued; the updates there,
// queued; the keys to Network's; a skill to its screen; the last run's
// failure to Last run. ↓ from the tabs goes in, ↑ from the first line
// back, tab along it and past the last back to the tabs.
func TestMenuNeedsYou(t *testing.T) {
	m := testMenu(false)
	m.items[2].Updates = []string{"go 1.24 → 1.25"}
	m.info.Keys = []AddKey{{Where: []string{"github.com: Settings › SSH keys"}}}
	m.info.Skills = []SkillRow{{Name: "pdf", Source: "drifted"}, {Name: "web", Off: "held"}, {Name: "ok", Source: "checked"}}
	m.info.LastRun = &LastRun{When: "today", Failed: []string{"docker"}}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	var ids []string
	for _, r := range m.needList() {
		ids = append(ids, r.id)
	}
	if got := strings.Join(ids, " "); got != "why fix:docker key:keys:OPENAI_API_KEY updates keys skill:pdf skill:web" {
		t.Errorf("needs: %s", got)
	}
	// Skills count: a tab mark, and the status line's number.
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Skills ▲") || !strings.Contains(v, "7 things need you") || !strings.Contains(v, "Network ○") {
		t.Errorf("marks:\n%s", v)
	}
	// One to a line, narrow: each with the tab it's on.
	for _, want := range [][2]string{{"Check pdf again", "Skills"}, {"Review web", "Skills"}, {"Add 1 key to github.com", "Network"}, {"See why the last run failed", "Last run"}, {"Install 1 update", "Tools"}} {
		if l := lineWith(v, want[0]+" "); !strings.HasPrefix(l, "│") || !strings.HasSuffix(strings.TrimRight(l, " │"), want[1]) {
			t.Errorf("%s: %q", want[0], l)
		}
	}
	go1 := func(id string) *Menu {
		m.screen, m.ended, m.result = "home", false, MenuResult{}
		onNeed(m, id)
		mk(m, "enter")
		return m
	}
	if r := go1("fix:docker").result; r.Kind != MenuFixes || r.ID != "docker" {
		t.Errorf("a fix: %+v", r)
	}
	if r := go1("key:keys:OPENAI_API_KEY").result; r.Kind != MenuFixes || r.ID != "keys" {
		t.Errorf("a value: %+v", r)
	}
	if r := go1("updates").result; r.Kind != MenuUpdatesTab {
		t.Errorf("updates: %+v", r)
	}
	if r := go1("keys").result; r.Kind != MenuConnection || r.ID != NetTab+":key" {
		t.Errorf("keys: %+v", r)
	}
	if go1("skill:web"); m.screen != "skills:skill" || m.skill().Name != "web" {
		t.Errorf("a skill: %s", m.screen)
	}
	if go1("why"); m.screen != "lastrun" {
		t.Errorf("why: %s", m.screen)
	}
	// Along it: ↑ out to the tabs, ↓ back in, tab past the last to the tabs.
	m.Open("home", "", false)
	onNeed(m, "why")
	mk(m, "up")
	if m.need != -1 || m.navCur() < 0 {
		t.Errorf("up: %d", m.need)
	}
	mk(m, "down")
	if m.need != 0 {
		t.Errorf("down: %d", m.need)
	}
	onNeed(m, "skill:web")
	mk(m, "tab")
	if m.need != -1 {
		t.Errorf("tab past the last: %d", m.need)
	}
	// A click on a row chooses it.
	m.View()
	for _, h := range m.Hits() {
		if h.Do != nil && h.Y0 == 5 {
			h.Do()
		}
	}
	if m.need != 1 {
		t.Errorf("click: %d", m.need)
	}
}

// Typing on the home searches the tools; / opens the search empty.
func TestMenuTyping(t *testing.T) {
	m := testMenu(true)
	mk(m, "k9s")
	if m.result.Kind != MenuTools || m.result.Search != "k9s" {
		t.Errorf("typed: %+v", m.result)
	}
	m = testMenu(true)
	mk(m, "/")
	if m.result.Kind != MenuTools || m.result.Search != "" {
		t.Errorf("/: %+v", m.result)
	}
}

// Settings lists the look and the answers to change (network access is
// Network's); esc goes home onto it.
func TestMenuSettings(t *testing.T) {
	m := testMenu(false)
	m.items = append(m.items, &BoardItem{ID: "gitid", Label: "Git: name, email and defaults", Tab: "git", Picked: true, State: "ok", Changes: [][2]string{{"git name and email", "git"}}})
	onRow(m, "settings")
	mk(m, "enter")
	v := m.View()
	for _, want := range []string{"[Settings]", "Accent colour", "Blue",
		"Git name and email", "asks, then applies it again"} {
		if !strings.Contains(v, want) {
			t.Errorf("settings lacks %q:\n%s", want, v)
		}
	}
	for _, gone := range []string{"Check for updates", "Network access", "MCP gateway"} {
		if strings.Contains(v, gone) {
			t.Errorf("Settings has %q:\n%s", gone, v)
		}
	}
	onSetting(m, "change:git")
	mk(m, "enter")
	if r := m.result; r.Kind != MenuChange || r.ID != "gitid" || r.Change != "git" {
		t.Errorf("change: %+v", r)
	}
	m = testMenu(false)
	onRow(m, "settings")
	mk(m, "enter", "esc")
	if m.screen != "home" || m.rows()[m.sel].id != "settings" {
		t.Errorf("esc: on %s, row %d", m.screen, m.sel)
	}
}

// The look: the accent colour's choices, with a swatch; Apply hands it
// back.
func TestMenuLook(t *testing.T) {
	m := testMenu(false)
	onRow(m, "settings")
	mk(m, "enter")
	onSetting(m, "look")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Settings › Look") || !strings.Contains(v, "██") || strings.Contains(v, "MCP gateway") {
		t.Fatalf("look:\n%s", v)
	}
	mk(m, "right", "enter", "enter")
	if r := m.result; r.Kind != MenuApply || r.Settings["accent"] != "green" || len(r.Settings) != 1 {
		t.Errorf("result %+v", r)
	}
}

// Session on login: its own row and screen, the value it reads; Apply
// hands back the new one.
func TestMenuSession(t *testing.T) {
	m := testMenu(false)
	m.settings = append(m.settings, &BoardSetting{Key: "session", Label: "Session on login", Section: ShellSection, Value: "zellij",
		Choices: [][2]string{{"tmux", "tmux"}, {"zellij", "zellij"}, {"ask", "ask at the next login"}, {"off", "off"}}})
	onRow(m, "settings")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Session on login") || !strings.Contains(v, "zellij") || strings.Contains(v, "Session on login: zellij") {
		t.Fatalf("settings:\n%s", v)
	}
	onSetting(m, "shell")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Settings › Session on login") || strings.Contains(v, "MCP gateway") || strings.Contains(v, "Accent colour") {
		t.Fatalf("session:\n%s", v)
	}
	mk(m, "right", "right", "enter", "enter")
	if r := m.result; r.Kind != MenuApply || r.Settings["session"] != "off" || len(r.Settings) != 1 {
		t.Errorf("result %+v", r)
	}
}

// onSetting moves Settings' cursor to row id.
func onSetting(m *Menu, id string) {
	for n, r := range m.settingsRows() {
		if r.id == id {
			m.sel = n
		}
	}
}

// Network access (Network's, the screen the menu's): ←→ picks, each
// choice said in a sentence; Apply hands back only what changed; esc
// leaves nothing changed, back on Network.
func TestMenuAccess(t *testing.T) {
	open := func() *Menu {
		m := testMenu(false)
		m.open("access")
		return m
	}
	m := open()
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Network access") || !strings.Contains(v, "[Network]") || !strings.Contains(v, "(•) this machine only") || strings.Contains(v, "Accent colour") {
		t.Fatalf("access:\n%s", v)
	}
	mk(m, "down", "down", "down", "down")
	if m.sel != 1 {
		t.Errorf("past Apply: on row %d, want 1", m.sel)
	}
	mk(m, "up")
	mk(m, "right")
	if !strings.Contains(m.View(), "once it's on Tailscale") {
		t.Errorf("no sentence for the choice:\n%s", m.View())
	}
	mk(m, "enter", "enter") // on to Apply, apply
	if m.result.Kind != MenuApply || m.result.Settings["mcp"] != "vpn" || len(m.result.Settings) != 1 {
		t.Errorf("result %+v", m.result)
	}
	m = open()
	mk(m, "right", "esc")
	if m.screen != "home" || m.result.Kind != MenuConnection || m.result.ID != NetTab || !m.ended {
		t.Errorf("esc: screen %s, %+v", m.screen, m.result)
	}
}

// The home's Connection panel is Local settings' everyday setups (SSH,
// Docker, the gateway, pages), what to type and, lined up after it, the
// key fact, from the same rows: the rest (All of it, Claude Code, the clipboard) aren't on
// it. It isn't a stop: Local settings opens the screen (the app's).
func TestHomeConnect(t *testing.T) {
	m := testMenu(false)
	m.setups = []Setup{
		{ID: "script", What: "All of it", How: "sh yardo-vm.sh  ·  one script"},
		{ID: "ssh", What: "SSH", How: "ssh vm  ·  ada@vm.ts.net · Host vm in ~/.ssh/config"},
		{ID: "mcp-claude", What: "Claude Code", How: "claude mcp add vm http://vm.ts.net:8081/mcp"},
		{ID: "mcp-other", What: "Other clients", How: "http://vm.ts.net:8081/mcp  ·  token: sudo cat ~/.mcp/clients/client.token"},
		{ID: "clipboard", What: "iTerm2 clipboard", How: "iTerm2  ·  copies", Once: true},
	}
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	v := ansi.Strip(m.View())
	for _, want := range []string{"─ Connection ", "Local settings has the rest", "SSH      ssh vm                     ada@vm.ts.net", "MCP      http://vm.ts.net:8081/mcp  token: sudo cat ~/.mcp/clients/client.token"} {
		if !strings.Contains(v, want) {
			t.Errorf("home lacks %q:\n%s", want, v)
		}
	}
	for _, gone := range []string{"All of it", "claude mcp add", "iTerm2", "Laptop", "laptop"} {
		if strings.Contains(v, gone) {
			t.Errorf("home has %q:\n%s", gone, v)
		}
	}
	// The panel says what the screen says: change a setup, the panel follows.
	m.setups[1].How = "ssh vm  ·  ada@10.0.0.9"
	if l := lineWith(ansi.Strip(m.View()), "SSH "); !strings.Contains(l, "ada@10.0.0.9") {
		t.Errorf("not from the setups: %q", l)
	}
	onRow(m, LocalTab)
	mk(m, "enter")
	if !m.ended || m.result.Kind != MenuConnection || m.result.ID != LocalTab {
		t.Errorf("enter: %+v", m.result)
	}
	// A key to add marks Network, and is in Needs you; it isn't on the panel.
	m = testMenu(true)
	m.info.Keys = []AddKey{{Where: []string{"github.com: Settings › SSH keys"}}}
	v = ansi.Strip(m.View())
	if l := lineWith(v, "Network ○"); l == "" || lineWith(v, "Add 1 key to github.com") == "" || lineWith(v, "key to add") != "" {
		t.Errorf("a key to add:\n%s", v)
	}
	// Narrow: the panel still, and the footer's keys to q.
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "─ Connection ") || !strings.Contains(v, "ssh vm") || !strings.Contains(v, "q quit") {
		t.Errorf("narrow:\n%s", v)
	}
	// No setups, no panel, no Local settings tab.
	m.setups = nil
	if v := ansi.Strip(m.View()); strings.Contains(v, "─ Connection ") || strings.Contains(v, "Local settings") {
		t.Errorf("no setups:\n%s", v)
	}
}

// lineWith is the line of v holding s.
func lineWith(v, s string) string {
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

func mcpMenu() *Menu {
	m := testMenu(true)
	m.items = append(m.items, &BoardItem{ID: "mcpjungle", Label: "MCP servers on MCPJungle", Tab: "agents", Picked: true, State: "ok"})
	m.info.MCP = []MCPServer{
		{Entry: "github", Name: "github", Spec: "github", About: "GitHub: repositories and pull requests", State: "ok", Yardo: true},
		{Entry: "jira", Name: "jira", Spec: "jira", About: "Jira and Confluence", State: "secret", Keys: []string{"JIRA_API_TOKEN"}, Yardo: true},
		{Entry: "jira:work", Name: "jira-work", Spec: "jira", About: "Jira and Confluence", State: "ok", Yardo: true},
		{Entry: "opensearch", Name: "opensearch", Spec: "opensearch", State: "failed", Detail: "Error: connection refused", Yardo: true},
		{Entry: "grafana:test", Name: "grafana-test", Spec: "grafana", State: "unlisted"},
	}
	m.info.MCPSpecs = []MCPSpec{{"aws-api", "AWS: run AWS CLI commands"}, {"github", "GitHub"}, {"grafana", "Grafana"}, {"jira", "Jira and Confluence"}}
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	return m
}

// key sends one key, as a terminal would (backspace too).
func key(m *Menu, k string) {
	switch k {
	case "backspace":
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	default:
		mk(m, k)
	}
}

// onRow moves the home's cursor to row id.
func onRow(m *Menu, id string) {
	m.need = -1
	for n, r := range m.rows() {
		if r.id == id {
			m.sel = n
		}
	}
}

// onNeed puts the home's cursor on the row of Needs you with id.
func onNeed(m *Menu, id string) {
	m.need = slices.IndexFunc(m.needList(), func(r menuRow) bool { return r.id == id })
}

// The home's MCP servers row (with the gateway only) opens the list:
// yardo's servers with their type and state, the ones found in ~/.mcp,
// then Add a server.
func TestMenuMCPList(t *testing.T) {
	if strings.Contains(testMenu(true).View(), "MCP servers") {
		t.Error("an MCP servers row without the gateway")
	}
	m := mcpMenu()
	// The ones that need you mark the tab, its note first; the panel lists them all.
	if v := ansi.Strip(m.View()); !strings.Contains(v, "MCP servers 4 ○") || !strings.Contains(v, "MCP servers") ||
		!strings.Contains(v, "● github") || !strings.Contains(v, "+ 1 more in ~/.mcp") {
		t.Errorf("home:\n%s", v)
	}
	onRow(m, "mcp")
	if v := m.View(); !strings.Contains(v, "Fill in jira · Check opensearch · github, jira, jira-work, opensearch") {
		t.Errorf("the tab's note:\n%s", v)
	}
	onRow(m, "mcp")
	mk(m, "enter")
	v := m.View()
	for _, want := range []string{"[MCP servers", "needs JIRA_API_TOKEN", "didn't register", "Also in ~/.mcp", "grafana-test", "· not on yardo's list",
		"+ Add a server", "<name>__<tool>"} {
		if !strings.Contains(v, want) {
			t.Errorf("list lacks %q:\n%s", want, v)
		}
	}
	// The selected one's description is under the list, in lines kept for
	// it: moving down, the rows stay where they are.
	at := func(v, name string) int {
		for n, l := range strings.Split(ansi.Strip(v), "\n") {
			if strings.Contains(l, fmt.Sprintf(" %s ", name)) {
				return n
			}
		}
		return -1
	}
	mk(m, "down")
	if v2 := m.View(); at(v, "opensearch") != at(v2, "opensearch") || at(v, "Agents reach") != at(v2, "Agents reach") {
		t.Errorf("rows moved:\n%s\n%s", v, v2)
	}
	mk(m, "up")
	if !strings.Contains(strings.Join(strings.Fields(ansi.Strip(v)), " "), "GitHub: repositories and pull requests") {
		t.Errorf("list lacks the description:\n%s", v)
	}
	if strings.Index(v, "opensearch") > strings.Index(v, "grafana-test") {
		t.Errorf("found servers before yardo's:\n%s", v)
	}
	m.info.MCPDown = true
	if !strings.Contains(m.View(), "The gateway is stopped") {
		t.Error("no stopped gateway line")
	}
	m = mcpMenu()
	m.info.MCP = []MCPServer{}
	onRow(m, "mcp")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "No MCP servers yet") || !strings.Contains(v, "› + Add a server") {
		t.Errorf("empty:\n%s", v)
	}
}

// Add: the catalog, then the name (the spec's when free); a name taken
// is said so on the screen; j, k and q are letters there.
func TestMenuMCPAdd(t *testing.T) {
	m := mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "n")
	if v := m.View(); !strings.Contains(v, "Add a server") || strings.Contains(v, "you have") {
		t.Fatalf("catalog:\n%s", v)
	}
	// A long description stays on its line, chosen or not: the rows
	// below don't move as the choice does.
	m.info.MCPSpecs[1].About = strings.Repeat("GitHub: repositories, issues, pull requests and Actions ", 6)
	before := strings.Count(m.View(), "\n")
	mk(m, "down")
	if after := strings.Count(m.View(), "\n"); after != before {
		t.Errorf("on the long one: %d lines, %d before:\n%s", after, before, m.View())
	}
	mk(m, "up")
	mk(m, "down", "down", "down", "enter") // jira: one there, no name given
	if v := m.View(); !strings.Contains(v, "Add jira") || !strings.Contains(v, "There's a jira already") {
		t.Fatalf("name:\n%s", v)
	}
	mk(m, "jira", "enter")
	if m.result.Kind != MenuQuit || !strings.Contains(m.View(), "There's a jira already.") {
		t.Errorf("taken: result %+v\n%s", m.result, m.View())
	}
	for range 4 {
		key(m, "backspace")
	}
	mk(m, "jkq")
	if !strings.Contains(m.View(), "› jkq") || m.screen != "mcp:name" {
		t.Errorf("j, k, q aren't text:\n%s", m.View())
	}
	for range 3 {
		key(m, "backspace")
	}
	mk(m, "work", "enter") // jira-work is there too
	if !strings.Contains(m.View(), "There's a jira-work already.") {
		t.Errorf("jira-work taken:\n%s", m.View())
	}
	for range 4 {
		key(m, "backspace")
	}
	mk(m, "team")
	if !strings.Contains(m.View(), "→ jira-team · its tools show as jira-team__<tool>") {
		t.Errorf("preview:\n%s", m.View())
	}
	mk(m, "enter")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "add", Entry: "jira:team"}) {
		t.Errorf("add: %+v", r)
	}
	// A spec not there yet is named after it: enter adds it.
	m = mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "n", "enter", "enter")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "add", Entry: "aws-api"}) {
		t.Errorf("add aws-api: %+v", r)
	}
}

// A server's screen: edit, rename (its name, to type over), delete (Keep
// it first); a found one off turns on.
func TestMenuMCPServer(t *testing.T) {
	open := func(n int) *Menu {
		m := mcpMenu()
		onRow(m, "mcp")
		mk(m, "enter")
		for range n {
			mk(m, "down")
		}
		mk(m, "enter")
		return m
	}
	m := open(2) // jira-work
	if v := m.View(); !strings.Contains(v, "MCP servers › jira-work") || !strings.Contains(v, "Edit its settings") || !strings.Contains(v, "~/.mcp/jira-work/env") {
		t.Fatalf("server:\n%s", v)
	}
	if !strings.Contains(m.View(), "› Turn it off") {
		t.Errorf("not Turn it off first:\n%s", m.View())
	}
	mk(m, "down", "enter")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "edit", Entry: "jira:work"}) {
		t.Errorf("edit: %+v", r)
	}
	m = open(2)
	mk(m, "down", "down", "down", "enter") // Rename
	if !strings.Contains(m.View(), "› jira-work") {
		t.Fatalf("rename:\n%s", m.View())
	}
	for range len("work") {
		key(m, "backspace")
	}
	mk(m, "prod", "enter")
	if r := m.result; r.MCP != (MCPAct{Do: "rename", Entry: "jira:work", To: "jira:prod"}) {
		t.Errorf("rename: %+v", r)
	}
	m = open(2)
	mk(m, "down", "down", "down", "down", "enter") // Delete
	if v := m.View(); !strings.Contains(v, "Delete jira-work?") || !strings.Contains(v, "can't be undone") {
		t.Fatalf("delete:\n%s", v)
	}
	if v := m.View(); !strings.Contains(v, "y yes") || !strings.Contains(v, "┌ Delete jira-work?") {
		t.Errorf("not the dialog:\n%s", v)
	}
	mk(m, "enter") // Keep it
	if m.result.Kind != MenuQuit || m.screen != "mcp:server" || m.dialog != nil {
		t.Errorf("keep: %+v on %s", m.result, m.screen)
	}
	mk(m, "enter") // Delete it (the row stays where it was)
	mk(m, "y")
	if r := m.result; r.MCP != (MCPAct{Do: "delete", Entry: "jira:work"}) {
		t.Errorf("delete: %+v", r)
	}
	m = open(4) // grafana-test, found, not on yardo's list
	if !strings.Contains(m.View(), "› Add to yardo's list") {
		t.Fatalf("found:\n%s", m.View())
	}
	mk(m, "enter")
	if r := m.result; r.MCP != (MCPAct{Do: "adopt", Entry: "grafana:test"}) {
		t.Errorf("adopt: %+v", r)
	}
}

// esc goes back a screen at a time, home onto the MCP servers row; from
// the board, the list's esc goes back to it. Open shows a toast.
func TestMenuMCPBack(t *testing.T) {
	m := mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "n", "enter")
	for _, want := range []string{"mcp:add", "mcp", "home"} {
		mk(m, "esc")
		if m.screen != want {
			t.Fatalf("esc: on %s, want %s", m.screen, want)
		}
	}
	if m.rows()[m.sel].id != "mcp" {
		t.Errorf("home on %q", m.rows()[m.sel].id)
	}
	m = mcpMenu().Open("mcp", "jira-work deleted.", true)
	if !strings.Contains(m.View(), "jira-work deleted.") {
		t.Errorf("no toast:\n%s", m.View())
	}
	mk(m, "esc")
	if m.result.Kind != MenuTools {
		t.Errorf("from the board: %+v", m.result)
	}
}

// A long catalog scrolls: the chosen row stays in view.
func TestMenuMCPScroll(t *testing.T) {
	m := mcpMenu()
	for n := range 30 {
		m.info.MCPSpecs = append(m.info.MCPSpecs, MCPSpec{Name: fmt.Sprintf("spec%02d", n), About: "a server"})
	}
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 20})
	onRow(m, "mcp")
	mk(m, "enter", "n")
	for range len(m.info.MCPSpecs) - 1 {
		mk(m, "down")
	}
	if v := m.View(); !strings.Contains(v, "› spec29") || strings.Contains(v, "aws-api") {
		t.Errorf("scrolled:\n%s", v)
	}
	// Last, From elsewhere.
	mk(m, "down", "down")
	if v := m.View(); !strings.Contains(v, "› From elsewhere") {
		t.Errorf("From elsewhere:\n%s", v)
	}
	mk(m, "enter")
	if r := m.result; r.Kind != MenuMCP || r.MCP.Do != "elsewhere" {
		t.Errorf("enter: %+v", r)
	}
}

// A server held for a decision reads held, and offers only Review it.
func TestMenuMCPHeld(t *testing.T) {
	m := mcpMenu()
	m.info.MCP = append(m.info.MCP, MCPServer{Entry: "xmcp", Name: "xmcp", State: "held"})
	m = NewMenu(m.info, "", m.items, nil, true)
	onRow(m, "mcp")
	mk(m, "enter")
	// Below yardo's, above the found ones (and their heading).
	at := slices.IndexFunc(m.info.MCP, func(s MCPServer) bool { return s.State == "held" })
	if at != len(m.info.MCP)-2 {
		t.Errorf("held is row %d of %d", at, len(m.info.MCP))
	}
	for range at {
		mk(m, "down")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "▲ held: waiting for you") || strings.Index(v, "xmcp") > strings.Index(v, "Also in ~/.mcp") {
		t.Fatalf("list:\n%s", v)
	}
	mk(m, "enter")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "› Review it") || strings.Contains(v, "Edit its settings") {
		t.Fatalf("server:\n%s", v)
	}
	mk(m, "enter")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "review", Entry: "xmcp"}) {
		t.Errorf("review: %+v", r)
	}
}

// A server that didn't register says why, and offers Try again first;
// its note (what it may do) shows too.
func TestMenuMCPFailed(t *testing.T) {
	m := mcpMenu()
	m.info.MCP[3].Note = "Read-only: docs only."
	onRow(m, "mcp")
	mk(m, "enter", "down", "down", "down", "enter") // opensearch
	if v := m.View(); !strings.Contains(v, "Error: connection refused") || !strings.Contains(v, "› Try again") || !strings.Contains(v, "Read-only: docs only.") {
		t.Fatalf("failed server:\n%s", v)
	}
	mk(m, "enter")
	if r := m.result; r.MCP != (MCPAct{Do: "retry", Entry: "opensearch"}) {
		t.Errorf("retry: %+v", r)
	}
}

// A server refused for its Docker options asks for a look on the home,
// says which option, and has nothing to retry or restart.
func TestMenuMCPRefused(t *testing.T) {
	m := mcpMenu()
	m.info.MCP[3].State, m.info.MCP[3].Detail = "refused", "--privileged"
	if v := m.View(); !strings.Contains(v, "2 things need you") || !strings.Contains(v, "▲ opensearch       refused") || strings.Contains(v, "Fill in opensearch") {
		t.Errorf("home:\n%s", v)
	}
	onRow(m, "mcp")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "refused: its spec asks for --privileged") {
		t.Errorf("list:\n%s", v)
	}
	if m.canRestart(&m.info.MCP[3]) {
		t.Error("a refused server can restart")
	}
}

// r restarts the server under the cursor, or all of them on All
// servers; a server off, or not set up, has nothing to restart; its
// screen offers Restart it after Edit.
func TestMenuMCPRestart(t *testing.T) {
	m := mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "r")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "restart", Entry: "github"}) {
		t.Errorf("r: %+v", r)
	}
	m = mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "up", "r")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "restart"}) {
		t.Errorf("r on All servers: %+v", r)
	}
	for _, off := range []bool{false, true} {
		m = mcpMenu()
		if off {
			m.info.MCP[0].State, m.info.MCP[0].Off = "off", "own" // github
			onRow(m, "mcp")
			mk(m, "enter", "r")
		} else {
			onRow(m, "mcp")
			mk(m, "enter", "down", "r") // jira, waiting for its token
		}
		if m.result.Kind == MenuMCP || !strings.Contains(m.toast, "Nothing to restart") {
			t.Errorf("r (off %v): %+v, %q", off, m.result, m.toast)
		}
	}
	m = mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "enter")
	v := m.View()
	if e, r := strings.Index(v, "Edit its settings"), strings.Index(v, "Restart it"); e < 0 || r < e {
		t.Fatalf("server screen:\n%s", v)
	}
	mk(m, "down", "down", "enter")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "restart", Entry: "github"}) {
		t.Errorf("Restart it: %+v", r)
	}
}

// e on the list edits the server under the cursor (not the Add row).
func TestMenuMCPEditKey(t *testing.T) {
	m := mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "down", "e")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "edit", Entry: "jira"}) {
		t.Errorf("e: %+v", r)
	}
	m = mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter")
	for range len(m.info.MCP) {
		mk(m, "down")
	}
	mk(m, "e")
	if m.result.Kind != MenuQuit || m.screen != "mcp" {
		t.Errorf("e on Add: %+v", m.result)
	}
	if !strings.Contains(m.View(), "e edit its settings") {
		t.Errorf("keys:\n%s", m.View())
	}
}

// The fixes are Tools': its mark is a problem's when there's one, a
// value's when there's only that. Not installed is no problem: yardo
// never set it up.
func TestMenuFixAll(t *testing.T) {
	m := testMenu(false)
	m.items[2].State = "drifted" // Go too
	if ns := m.needs()["tools"]; len(ns) != 3 || ns[0].id != "fix:docker" || ns[1].id != "fix:go" {
		t.Fatalf("two problems and a value: %+v", ns)
	}
	m = testMenu(false)
	m.items[2].State = "missing"
	if v := m.View(); strings.Contains(v, "Install Go") || !strings.Contains(v, "2 things need you") {
		t.Errorf("never set up, offered as a fix:\n%s", v)
	}
	m = testMenu(true)
	m.items[1].State = "secret"
	if l := lineWith(ansi.Strip(m.View()), "Tools 3"); !strings.Contains(l, "Tools 3 ○") {
		t.Errorf("a value's mark: %q", l)
	}
}

// esc and q go back from every screen; on the home q quits, esc only
// says so.
func TestMenuEscQ(t *testing.T) {
	m := testMenu(false)
	mk(m, "esc") // out of Needs you
	if m.need != -1 || m.ended {
		t.Errorf("esc in Needs you: %d", m.need)
	}
	mk(m, "esc")
	if m.result.Kind != MenuQuit || m.screen != "home" || !strings.Contains(m.View(), "q quits") {
		t.Errorf("esc on home: %+v\n%s", m.result, m.View())
	}
	for _, k := range []string{"esc", "q"} {
		m := testMenu(false)
		onRow(m, "settings")
		mk(m, "enter", k)
		if m.screen != "home" || m.rows()[m.sel].id != "settings" {
			t.Errorf("%s on settings: on %s, row %d", k, m.screen, m.sel)
		}
		for _, id := range []string{"look"} {
			m := testMenu(false)
			onRow(m, "settings")
			mk(m, "enter")
			onSetting(m, id)
			mk(m, "enter", k)
			if m.screen != "settings" || m.settingsRows()[m.sel].id != id {
				t.Errorf("%s on %s: on %s, row %d", k, id, m.screen, m.sel)
			}
		}
	}
	m = testMenu(false)
	mk(m, "q")
	if m.result.Kind != MenuQuit || !m.ended {
		t.Errorf("q: %+v", m.result)
	}
}

// With nothing newer, u checks again, in the background, the menu
// staying; Tools' note says when it last did.
func TestMenuCheckAgain(t *testing.T) {
	m := NewMenu(HomeInfo{Host: "vm", Checked: "14:05"}, "", testMenu(true).items, nil, true)
	onRow(m, "tools")
	if v := m.View(); !strings.Contains(v, "nothing newer · checked 14:05") {
		t.Errorf("home:\n%s", v)
	}
	// u checks, without a checker by leaving the menu.
	mk(m, "u")
	if m.result.Kind != MenuUpdates {
		t.Errorf("u: %+v", m.result)
	}
	m = NewMenu(HomeInfo{Host: "vm"}, "", testMenu(true).items, nil, false)
	onRow(m, "tools")
	if v := m.View(); !strings.Contains(v, "not checked yet") {
		t.Fatalf("never checked:\n%s", v)
	}
	started := 0
	m.Checker(func() func() UpdatesMsg {
		started++
		return func() UpdatesMsg { return UpdatesMsg{ByID: map[string][]string{}, At: "15:00"} }
	})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if started != 1 || cmd == nil || m.ended || !strings.Contains(m.View(), "Checking for updates") {
		t.Fatalf("u: started %d, ended %v\n%s", started, m.ended, m.View())
	}
	mk(m, "u")
	if started != 1 || !strings.Contains(m.View(), "Checking already") {
		t.Errorf("u again: started %d\n%s", started, m.View())
	}
	m.Update(cmd())
	if v := m.View(); !strings.Contains(v, "No updates: everything is current.") || !strings.Contains(v, "nothing newer · checked 15:00") {
		t.Errorf("found:\n%s", v)
	}
	m = NewMenu(HomeInfo{Host: "vm"}, "", testMenu(true).items, nil, true).Open("home", "✓ No updates: everything is current.", false)
	if !strings.Contains(m.View(), "No updates: everything is current.") {
		t.Errorf("no toast:\n%s", m.View())
	}
}

// View only reads; Update keeps the screen and cursor valid, never -1.
func TestMenuNormalize(t *testing.T) {
	m := testMenu(false)
	m.sel = 99
	m.View()
	if m.sel != 99 {
		t.Errorf("View moved the cursor to %d", m.sel)
	}
	mk(m, "down")
	if m.sel != len(m.rows())-1 {
		t.Errorf("cursor %d, want the last row", m.sel)
	}
	// An empty catalog: down stays at 0.
	m = mcpMenu()
	m.info.MCPSpecs = nil
	onRow(m, "mcp")
	mk(m, "enter", "n", "down", "down")
	if m.screen != "mcp:add" || m.sel != 0 {
		t.Errorf("empty catalog: on %s, row %d", m.screen, m.sel)
	}
}

// The servers are sorted once, when the menu is made: yardo's by name,
// then the found ones; the caller's slice is left as it was.
func TestMenuMCPSorted(t *testing.T) {
	info := HomeInfo{MCP: []MCPServer{{Name: "zed"}, {Name: "jira", Yardo: true}, {Name: "abc"}, {Name: "github", Yardo: true}}}
	m := NewMenu(info, "", nil, nil, false)
	var got []string
	for _, s := range m.mcpServers() {
		got = append(got, s.Name)
	}
	if strings.Join(got, " ") != "github jira abc zed" {
		t.Errorf("order: %v", got)
	}
	if info.MCP[0].Name != "zed" {
		t.Error("sorted the caller's servers")
	}
}

// Updates: a row of Needs you, Tools marked; enter leaves the menu for
// Tools' Updates tab (see TestAppUpdates).
func TestMenuInstallUpdates(t *testing.T) {
	m := testMenu(true)
	m.items[2].Updates = []string{"go 1.25.1 → 1.25.2"}
	m.checked = true
	if v := ansi.Strip(m.View()); lineWith(v, "Tools 3 ↑") == "" || lineWith(v, "↑ Install 1 update") == "" {
		t.Fatalf("home:\n%s", v)
	}
	if n := NewMenu(m.info, "", m.items, nil, true); n.need != 0 || n.needList()[0].id != "updates" {
		t.Errorf("starts on %d", n.need)
	}
	onNeed(m, "updates")
	if v := m.View(); !strings.Contains(v, "Install 1 update: go 1.25.1 → 1.25.2") {
		t.Errorf("note:\n%s", v)
	}
	mk(m, "enter")
	if m.result.Kind != MenuUpdatesTab {
		t.Errorf("result %+v, want MenuUpdatesTab", m.result)
	}
}

// A check that finds nothing newer says everything is current.
func TestMenuNothingNewer(t *testing.T) {
	m := testMenu(true)
	m.Update(UpdatesMsg{ByID: map[string][]string{}, At: "09:05"})
	if !strings.Contains(m.View(), "No updates: everything is current.") {
		t.Errorf("toast:\n%s", m.View())
	}
}

// A check running in the background: Tools says so, u won't start
// another; when it's done, its updates show.
func TestMenuChecking(t *testing.T) {
	m := testMenu(true)
	done := make(chan struct{})
	m.Checking(func() UpdatesMsg {
		<-done
		return UpdatesMsg{ByID: map[string][]string{"go": {"go 1.25.1 → 1.25.2"}}, At: "09:05"}
	})
	onRow(m, "tools")
	if m.Init() == nil || !strings.Contains(m.View(), "checking for updates…") {
		t.Fatalf("checking:\n%s", m.View())
	}
	mk(m, "u")
	if m.result.Kind != MenuQuit || !strings.Contains(m.View(), "Checking already") {
		t.Errorf("check again while checking: %+v", m.result)
	}
	close(done)
	m.Update(m.wait())
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Tools 3 ↑") || !strings.Contains(v, "Install 1 update") || !strings.Contains(v, "checked 09:05") {
		t.Errorf("done:\n%s", v)
	}
}

// The home's panels: the services (the failed first), what they listen
// on and who can reach it, then the containers, the MCP servers' counted.
func TestMenuDashboard(t *testing.T) {
	m := testMenu(true)
	m.info.Dash = Dashboard{Docker: true,
		Containers: []Container{{Image: "postgres:16", Name: "myapp-db-1", Ports: []string{"5432"}, Reach: "all", Count: 1},
			{Image: "mcp-proxy-for-aws-and-more:1.2.3", Ports: []string{"9000"}, Reach: "vpn", Count: 2},
			{Image: "mcp-proxy-for-aws", MCP: "aws", Count: 4}, {Image: "github-mcp-server", MCP: "github", Count: 1}},
		Services: []Service{{Name: "ssh", Up: true}, {Name: "docker", Up: true}, {Name: "fail2ban"}},
		Listening: []Listener{{"22", "tcp", "ssh", "all"}, {"2222", "tcp", "ssh", "local"}, {"41641", "udp", "tailscale", "all"},
			{"11434", "tcp", "ollama", "all"}, {"631", "tcp", "cupsd", "local"}, {"9100", "tcp", "", "100.64.1.7"}}}
	got := ansi.Strip(strings.Join(m.serviceLines(52), "\n"))
	want := `  service    port              reachable from
` + tui.Cur.Mark.Down + ` fail2ban   failed to start
● ssh        22/tcp, 2222/tcp  any network
● docker     running
● tailscale  41641/udp         any network
● ollama     11434/tcp         any network
● cupsd      631/tcp           local
● other      9100/tcp          100.64.1.7 only
─ containers ───────────────────────────────────────
● postgres:16         myapp-db-1  :5432  any network
● mcp-pro…e:1.2.3 ×2              :9000  VPN only
MCP servers: aws ×4, github`
	if got != want {
		t.Errorf("panel:\n%s\nwant:\n%s", got, want)
	}
	// The reach is said in yellow when it's open and needn't be.
	if l := m.serviceLines(52)[5]; !strings.Contains(l, yellow.Render("any network")) {
		t.Errorf("ollama, open: %q", l)
	}
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Services & containers") || !strings.Contains(v, "● ssh        22/tcp, 2222/tcp") {
		t.Errorf("wide:\n%s", v)
	}
	// Narrow and short: the services fold to a line, the failed first,
	// the containers counted last; narrow and tall, the panels stack.
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Services  fail2ban failed") || !strings.Contains(v, "· 8 containers") || strings.Contains(v, "Services & containers") {
		t.Errorf("narrow:\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 40})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Services & containers") || !strings.Contains(v, "● ssh") {
		t.Errorf("narrow, tall:\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m.info.Dash = Dashboard{Docker: true}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "none running") || strings.Contains(v, "22/tcp") {
		t.Errorf("nothing running:\n%s", v)
	}
}

// The MCP servers panel: the ready two to a line, the second where the
// others' words are; one not checked yet said so, as on the list.
func TestMenuMCPLines(t *testing.T) {
	m := mcpMenu()
	m.info.MCP = append(m.info.MCP, MCPServer{Entry: "seq", Name: "seq", Spec: "seq", State: "unchecked", Yardo: true})
	got := ansi.Strip(strings.Join(m.mcpLines()[2:], "\n"))
	want := `● github           ● jira-work
○ jira             needs JIRA_API_TOKEN
▲ opensearch       didn't register: check its settings
· seq              not checked
+ 1 more in ~/.mcp`
	if got != want {
		t.Errorf("panel:\n%s\nwant:\n%s", got, want)
	}
	if s := ansi.Strip(m.mcpSummary()); s != "2 of 5 ready · 1 not checked" {
		t.Errorf("summary: %s", s)
	}
}

// The Machine panel: CPU (its average, a sparkline), load against the
// cores, memory, disk, uptime.
func TestMenuMachine(t *testing.T) {
	saved := machine.m
	t.Cleanup(func() { machine.m = saved })
	machine.m = Machine{Read: true, CPU: []float64{0.1, 0.2, 0.06}, Load: [3]float64{5.5, 0.38, 0.31}, Cores: 4,
		MemUsed: 3 << 30, MemTotal: 8 << 30, DiskUsed: 33 << 30, DiskTotal: 80 << 30, Up: 76 * time.Hour}
	v := ansi.Strip(testMenu(true).View())
	for _, want := range []string{"CPU 12%", "now", "Load 5.50 0.38 0.31", "on 4 cores", "38%", "3.0 GB of 8.0 GB", "41%", "of 80 GB", "Uptime 3d 4h"} {
		if !strings.Contains(v, want) {
			t.Errorf("machine lacks %q:\n%s", want, v)
		}
	}
	if got := spark([]float64{0, 0.5, 1}, 2); got != "▅█" {
		t.Errorf("spark = %q", got)
	}
}

// A machine set up from a team profile says how much is the team's.
func TestMenuTeam(t *testing.T) {
	m := testMenu(true)
	m.items[0].Team, m.items[2].Team = "platform", "platform"
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	if v := m.View(); !strings.Contains(v, "3 of 3 tools working") || !strings.Contains(v, "2 from the team profile platform") {
		t.Errorf("home:\n%s", v)
	}
}

// The tools installed take the lines there are, the last saying how many
// more when they don't all fit.

// d on the MCP servers list asks to delete the one under the cursor; y
// deletes, n goes back to the list.
func TestMenuMCPDelete(t *testing.T) {
	m := mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "down", "down", "d") // jira-work
	if v := m.View(); !strings.Contains(v, "Delete jira-work?") || !strings.Contains(v, "Keep it (n)") {
		t.Fatalf("question:\n%s", v)
	}
	mk(m, "n")
	if m.screen != "mcp" || m.sel != 2 {
		t.Errorf("n: on %s, row %d", m.screen, m.sel)
	}
	mk(m, "d", "y")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "delete", Entry: "jira:work"}) {
		t.Errorf("y: %+v", r)
	}
}

// A part (a login) folded into its tool's row is counted once: by the row.
func TestMenuPartsCountedOnce(t *testing.T) {
	m := testMenu(false)
	in := []string{"~/.mcp/github/env: GITHUB_PERSONAL_ACCESS_TOKEN", "~/.mcp/jira/env: JIRA_URL"}
	m.items = append(m.items,
		&BoardItem{ID: "gw", Label: "MCP gateway", Tab: "ai", Picked: true, State: "secret", Inputs: in},
		&BoardItem{ID: "gw-servers", Label: "MCP servers", Tab: "ai", Picked: true, State: "secret", Inputs: in, PartOf: "gw"})
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	if v := m.View(); !strings.Contains(v, "Fill in 3 values") || !strings.Contains(v, "2 things need you") {
		t.Errorf("counted twice:\n%s", v)
	}
}

// Network access: Apply says nothing changed until a choice does, then
// stands out with how many; a applies from any row, enter on an
// unchanged screen says so.
func TestMenuAccessApply(t *testing.T) {
	m := testMenu(false)
	m.open("access")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "nothing changed yet") || strings.Contains(v, "Apply 1 change") {
		t.Errorf("unchanged:\n%s", v)
	}
	mk(m, "a")
	if m.result.Kind == MenuApply || m.screen != "access" || !strings.Contains(m.View(), "Nothing changed") {
		t.Errorf("a, unchanged: %+v on %s", m.result, m.screen)
	}
	mk(m, "right")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[ Apply 1 change ]") || !strings.Contains(v, "a applies from anywhere") {
		t.Errorf("changed:\n%s", v)
	}
	mk(m, "a")
	if m.result.Kind != MenuApply || m.result.Settings["mcp"] != "vpn" {
		t.Errorf("a: %+v", m.result)
	}
}

// With nothing needing you, tab and → go along the tabs, tab round from
// the last to the first; shift+tab goes back. With something, tab goes
// into Needs you, and on past its last back to the tabs.
func TestMenuTab(t *testing.T) {
	m := testMenu(true)
	var got []string
	for range 5 {
		mk(m, "tab")
		got = append(got, m.rows()[m.sel].id)
	}
	if strings.Join(got, " ") != "net local settings tools net" {
		t.Errorf("tab: %v", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if id := m.rows()[m.sel].id; id != "settings" {
		t.Errorf("shift+tab round: on %s", id)
	}
	m = testMenu(false)
	onRow(m, "tools")
	var at []int
	for range 3 {
		mk(m, "tab")
		at = append(at, m.need)
	}
	if fmt.Sprint(at) != "[0 1 -1]" || m.rows()[m.sel].id != "tools" {
		t.Errorf("tab with Needs you: %v", at)
	}
}

// A row is a line, its note cut, the selected one's too: the rows below
// stay put as the selection moves.
func TestMenuRowOneLine(t *testing.T) {
	m := testMenu(true)
	r := menuRow{label: "Network access", note: strings.Repeat("MCP gateway: everywhere, plain HTTP (with a token) · ", 4)}
	for _, on := range []bool{true, false} {
		if ls := m.row(on, r, 25, 100); len(ls) != 1 || ansi.StringWidth(ls[0]) > 98 {
			t.Errorf("selected %v: %d lines, %d wide", on, len(ls), ansi.StringWidth(ls[0]))
		}
	}
}

// All servers is the row above the servers: how many are on, or off; up
// from the first server reaches it, and the rows don't move as it does.
func TestMenuMCPAllRow(t *testing.T) {
	m := mcpMenu()
	m.info.MCP[2].State, m.info.MCP[2].Off = "off", "own" // jira-work
	onRow(m, "mcp")
	mk(m, "enter")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "All servers") || !strings.Contains(v, "● on · 3 of 4") || !strings.Contains(v, "○ off") || !strings.Contains(v, "space on/off") {
		t.Fatalf("list:\n%s", v)
	}
	if strings.Index(v, "All servers") > strings.Index(v, "github") {
		t.Errorf("All servers isn't above the servers:\n%s", v)
	}
	at := func(v, s string) int {
		for n, l := range strings.Split(ansi.Strip(v), "\n") {
			if strings.Contains(l, s) {
				return n
			}
		}
		return -1
	}
	mk(m, "up")
	v2 := m.View()
	if !strings.Contains(v2, "› All servers") || at(v, "opensearch") != at(v2, "opensearch") || at(v, "Agents reach") != at(v2, "Agents reach") {
		t.Errorf("on All servers, rows moved:\n%s\n%s", v, v2)
	}
	mk(m, "e", "d")
	if m.result.Kind == MenuMCP || m.screen != "mcp" {
		t.Errorf("e, d on All servers: %+v on %s", m.result, m.screen)
	}
	// All off: it says so, and each server reads off with them.
	m = mcpMenu()
	m.info.MCPAllOff = true
	for n := range 4 {
		m.info.MCP[n].State, m.info.MCP[n].Off = "off", "all"
	}
	onRow(m, "mcp")
	mk(m, "enter")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "○ off · space turns them back on") || strings.Count(v, "○ off with all") != 4 {
		t.Errorf("all off:\n%s", v)
	}
	// No server of yardo's: no All servers row.
	m = mcpMenu()
	m.info.MCP = m.info.MCP[4:]
	onRow(m, "mcp")
	mk(m, "enter", "up")
	if v := m.View(); strings.Contains(v, "All servers") || !strings.Contains(v, "› grafana-test") {
		t.Errorf("found only:\n%s", v)
	}
}

// The home's panel lists a server off apart; all off says so on the
// gateway's line and in the tab's note.
func TestMenuMCPHomeOff(t *testing.T) {
	m := mcpMenu()
	m.info.MCP[2].State, m.info.MCP[2].Off = "off", "own"
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "○ jira-work") || strings.Contains(v, "● jira-work") {
		t.Errorf("home, jira-work off:\n%s", v)
	}
	if s := ansi.Strip(m.mcpSummary()); s != "1 of 3 ready · 1 off" {
		t.Errorf("summary %q", s)
	}
	onRow(m, "mcp")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "· 1 off") {
		t.Errorf("the tab's note:\n%s", v)
	}
	m = mcpMenu()
	m.info.MCPAllOff = true
	for n := range 4 {
		m.info.MCP[n].State, m.info.MCP[n].Off = "off", "all"
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "all servers off") || strings.Contains(v, "MCP servers 4 ○") {
		t.Errorf("home, all off:\n%s", v)
	}
	if s := ansi.Strip(m.mcpSummary()); s != "all off" {
		t.Errorf("summary %q", s)
	}
}

// Orphaned containers need you: the home says so on its MCP servers
// panel and tab; the list says whose, and c (or a click on that line)
// cleans up. With none, c does nothing.
func TestMenuMCPOrphans(t *testing.T) {
	m := mcpMenu()
	m.info.MCPOrphans = []string{"jira", "aws", "jira"}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "▲ 3 orphaned containers · MCP servers › c") || !strings.Contains(v, "MCP servers") {
		t.Errorf("home:\n%s", v)
	}
	if _, ok := m.needs()["mcp"]; !ok {
		t.Error("the MCP servers tab isn't marked")
	}
	onRow(m, "mcp")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Clean up 3 orphaned containers") {
		t.Errorf("the tab's note:\n%s", v)
	}
	mk(m, "enter")
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "▲  3 orphaned containers (jira ×2, aws)") || !strings.Contains(v, "c cleans up") || !strings.Contains(v, "c clean up") {
		t.Fatalf("list:\n%s", v)
	}
	if !slices.ContainsFunc(m.Hits(), func(h Hit) bool { return h.Key == "c" }) {
		t.Error("the line isn't clickable")
	}
	mk(m, "c")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "cleanup"}) {
		t.Errorf("c: %+v", r)
	}
	m = mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "c")
	if m.result.Kind == MenuMCP || strings.Contains(ansi.Strip(m.View()), "orphaned") {
		t.Errorf("no orphans, c: %+v\n%s", m.result, m.View())
	}
}

// switchMenu is the MCP servers list with a switcher that records what
// it's given and says the servers are as marked (or fails with fail).
func switchMenu(fail *error) (*Menu, *[][]MCPAct) {
	m := mcpMenu()
	ran := &[][]MCPAct{}
	m.Switching(func(acts []MCPAct) ([]MCPServer, bool, string, error) {
		*ran = append(*ran, acts)
		if fail != nil && *fail != nil {
			return nil, false, "", *fail
		}
		return slices.Clone(m.info.MCP), m.info.MCPAllOff, "✓ saved", nil
	})
	onRow(m, "mcp")
	mk(m, "enter")
	return m, ran
}

// runCmd runs cmd's message through the menu, as the program would.
func runCmd(m *Menu, cmd tea.Cmd) {
	if cmd != nil {
		m.Update(cmd())
	}
}

// Space marks a server, or all of them, off or on: the screen shows it
// at once, "not saved", and nothing runs until a; marking one back
// unmarks it; s is no key.
func TestMenuMCPSpaceMarks(t *testing.T) {
	m, ran := switchMenu(nil)
	mk(m, "down", " ") // jira
	v := ansi.Strip(m.View())
	if m.result.Kind == MenuMCP || len(*ran) != 0 || m.info.MCP[1].State != "off" || !strings.Contains(v, "not saved") || !strings.Contains(v, "a applies 1 change") || !strings.Contains(v, "a apply 1") {
		t.Fatalf("marked jira off:\n%s", v)
	}
	mk(m, "s")
	if len(*ran) != 0 || m.ended {
		t.Errorf("s did something: ran %v", *ran)
	}
	mk(m, " ")
	if v := ansi.Strip(m.View()); strings.Contains(v, "not saved") || strings.Contains(v, "a applies") || m.info.MCP[1].State != "secret" {
		t.Errorf("marked back:\n%s", v)
	}
	// All servers: every one of yardo's off, the found one left be.
	mk(m, "up", "up", " ")
	if !m.info.MCPAllOff || m.info.MCP[0].Off != "all" || m.info.MCP[4].State == "off" {
		t.Errorf("all off: %+v", m.info.MCP)
	}
	mk(m, "down", " ")
	if !strings.Contains(m.View(), "turn them on first") {
		t.Errorf("a server while all are off:\n%s", m.View())
	}
	// A found server: nothing to mark.
	m, _ = switchMenu(nil)
	mk(m, "down", "down", "down", "down", " ")
	if strings.Contains(m.View(), "not saved") {
		t.Errorf("a found server was marked:\n%s", m.View())
	}
}

// a runs what's marked in one go, in the background; its servers replace
// the rows. Failing, the rows are back as they were.
func TestMenuMCPSave(t *testing.T) {
	m, ran := switchMenu(nil)
	mk(m, "down", " ", "down", "down", " ") // jira, opensearch off
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if cmd == nil || !strings.Contains(m.View(), "Saving") {
		t.Fatalf("a:\n%s", m.View())
	}
	runCmd(m, cmd)
	want := [][]MCPAct{{{Do: "off", Entry: "jira"}, {Do: "off", Entry: "opensearch"}}}
	if !reflect.DeepEqual(*ran, want) || strings.Contains(m.View(), "not saved") || !strings.Contains(m.View(), "✓ saved") {
		t.Errorf("saved: ran %v\n%s", *ran, m.View())
	}
	// All on first, then each server.
	m, ran = switchMenu(nil)
	m.info.MCPAllOff = true
	mk(m, "up", " ")
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	runCmd(m, cmd)
	if !reflect.DeepEqual(*ran, [][]MCPAct{{{Do: "on"}}}) {
		t.Errorf("all on: ran %v", *ran)
	}
	// It fails: back as saved, saying why.
	fail := errors.New("couldn't turn jira off: the gateway said no")
	m, _ = switchMenu(&fail)
	mk(m, "down", " ")
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	runCmd(m, cmd)
	if m.info.MCP[1].State != "secret" || !strings.Contains(m.View(), "the gateway said no") || strings.Contains(m.View(), "not saved") {
		t.Errorf("failed:\n%s", m.View())
	}
}

// Leaving with changes not saved asks once; q again leaves without them.
// Edit, delete and the rest wait for a save.
func TestMenuMCPUnsaved(t *testing.T) {
	m, _ := switchMenu(nil)
	mk(m, " ", "q")
	if m.screen != "mcp" || !strings.Contains(m.View(), "1 change isn't applied") {
		t.Fatalf("q once:\n%s", m.View())
	}
	mk(m, "q")
	if m.screen != "home" || m.info.MCP[0].State != "ok" {
		t.Errorf("q again: on %s, github %s", m.screen, m.info.MCP[0].State)
	}
	m, _ = switchMenu(nil)
	mk(m, " ", "e")
	if m.result.Kind == MenuMCP || !strings.Contains(m.View(), "Apply first") {
		t.Errorf("e with a change marked: %+v\n%s", m.result, m.View())
	}
}

// A server's screen marks it off or on, and goes back to the list; while
// all are off, it offers neither.
func TestMenuMCPServerMarks(t *testing.T) {
	m, ran := switchMenu(nil)
	mk(m, "enter") // github
	if !strings.Contains(m.View(), "› Turn it off") {
		t.Fatalf("server:\n%s", m.View())
	}
	mk(m, "enter")
	if m.screen != "mcp" || m.info.MCP[0].State != "off" || !strings.Contains(m.View(), "not saved") || len(*ran) != 0 {
		t.Errorf("Turn it off: on %s\n%s", m.screen, m.View())
	}
	m, _ = switchMenu(nil)
	m.info.MCPAllOff, m.info.MCP[0].State, m.info.MCP[0].Off = true, "off", "all"
	mk(m, "enter")
	if v := m.View(); strings.Contains(v, "Turn it") || !strings.Contains(v, "› Edit its settings") {
		t.Errorf("all off:\n%s", v)
	}
}

// Last run: a tab saying when and how it went; on a failure, marked, and
// the first tab, its screen with the failing step's output and the log.
func TestMenuLastRun(t *testing.T) {
	m := testMenu(true)
	m.info.LastRun = &LastRun{When: "today at 14:05", OK: true, Lines: []RunLine{{Status: "ok", Text: "Go"}}}
	onRow(m, "lastrun")
	if v := m.View(); strings.Contains(v, "See why the last run failed") || !strings.Contains(v, "Last run") || !strings.Contains(v, "today at 14:05 · ✓ it worked") {
		t.Errorf("worked:\n%s", v)
	}
	m = testMenu(true)
	m.info.LastRun = &LastRun{When: "yesterday at 09:12", Failed: []string{"apt"}, Log: "/var/log/yardo.log",
		Lines: []RunLine{{Status: "ok", Text: "Go"}, {Status: "fail", Text: "apt: one transaction failed", Tail: []string{"E: Unable to locate package nope"}}},
		Notes: []string{"Docker: log out and in"}}
	m = NewMenu(m.info, "", m.items, nil, true)
	if m.needList()[m.need].id != "why" || !strings.Contains(ansi.Strip(m.View()), "Last run ✗") {
		t.Fatalf("not on its row in Needs you:\n%s", m.View())
	}
	mk(m, "enter")
	v := m.View()
	for _, want := range []string{"[Last run", "yesterday at 09:12", "✗ apt: one transaction failed", "E: Unable to locate package nope", "Docker: log out and in", "/var/log/yardo.log"} {
		if !strings.Contains(v, want) {
			t.Errorf("Last run lacks %q:\n%s", want, v)
		}
	}
	mk(m, "q")
	if m.screen != "home" {
		t.Errorf("q: on %s", m.screen)
	}
}

// On the home, ←→ (h l) go along the tabs, stopping at the ends; k
// stays, j goes down into Needs you.
func TestMenuHomeKeys(t *testing.T) {
	m := testMenu(false)
	onRow(m, "tools")
	mk(m, "right")
	if id := m.rows()[m.sel].id; id != NetTab {
		t.Errorf("→: on %s", id)
	}
	mk(m, "l", "l", "l")
	if id := m.rows()[m.sel].id; id != "settings" {
		t.Errorf("l past the end: on %s", id)
	}
	mk(m, "h", "k")
	if id := m.rows()[m.sel].id; id != LocalTab || m.need != -1 {
		t.Errorf("h k: on %s", id)
	}
	mk(m, "j")
	if m.need != 0 || m.ended {
		t.Errorf("j: on %d (ended %v)", m.need, m.ended)
	}
}

// The home at 120 and 80 columns: the tabs at the top, the one under the
// cursor a block of the accent colour, Machine two lines under the status.
func TestMenuHomeLayout(t *testing.T) {
	for _, w := range []int{120, 80} {
		m := testMenu(false)
		m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		v := ansi.Strip(m.View())
		ls := strings.Split(v, "\n")
		mach := -1
		for n, l := range ls {
			if strings.Contains(l, "─ Machine ") {
				mach = n
			}
		}
		if mach < 0 || !strings.HasPrefix(strings.TrimSpace(ls[mach+3]), "└") {
			t.Errorf("%d: Machine isn't two lines:\n%s", w, v)
		}
		if !strings.Contains(ls[1], "Tools 3 ✗") || !strings.Contains(ls[1], "Network") || strings.Contains(v, "─ Go to ") {
			t.Errorf("%d: the tabs aren't at the top:\n%s", w, v)
		}
		if !strings.Contains(m.View(), tabOn.Render("[Home]")) || !strings.Contains(ls[7], "›✗ Start Docker") {
			t.Errorf("%d: Home isn't a block, or the cursor isn't on Needs you's first:\n%s", w, m.View())
		}
		// The panels are as tall as they need: with room, nothing is cut.
		if strings.Contains(v, "more") {
			t.Errorf("%d: a panel is cut with room to spare:\n%s", w, v)
		}
		for _, l := range ls {
			if ansi.StringWidth(l) > w {
				t.Errorf("%d: a line %d wide: %q", w, ansi.StringWidth(l), l)
			}
		}
	}
}

// A server from elsewhere with a new image offers Update it; one off
// because its tools changed says so and offers Check it again.
func TestMenuMCPUpdateRecheck(t *testing.T) {
	m := mcpMenu()
	m.info.MCP = append(m.info.MCP,
		MCPServer{Entry: "clock", Name: "clock", Yardo: true, State: "ok", Update: "mcp/time@sha256:" + strings.Repeat("9", 64)},
		MCPServer{Entry: "remote", Name: "remote", Yardo: true, State: "off", Off: "tools-changed"})
	m = NewMenu(m.info, "", m.items, nil, true)
	for _, c := range []struct{ name, row, do string }{{"clock", "Update it", "update"}, {"remote", "Check it again", "recheck"}} {
		m.Open("mcp", "", true)
		for range 15 {
			mk(m, "up")
		}
		for n := 0; n < 30 && !strings.Contains(ansi.Strip(m.View()), fmt.Sprintf("› %s ", c.name)); n++ {
			mk(m, "down")
		}
		if c.name == "remote" && !strings.Contains(ansi.Strip(m.View()), "▲ off: its tools changed since you approved it") {
			t.Errorf("list:\n%s", ansi.Strip(m.View()))
		}
		mk(m, "enter")
		if v := ansi.Strip(m.View()); !strings.Contains(v, "› "+c.row) {
			t.Fatalf("%s:\n%s", c.name, v)
		}
		mk(m, "enter")
		if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: c.do, Entry: c.name}) {
			t.Errorf("%s: %+v", c.name, r)
		}
		m.result = MenuResult{}
	}
}
