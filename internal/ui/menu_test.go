package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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
		{Key: "api", Label: "Docker API", Value: "off", Choices: [][2]string{{"off", "off"}, {"lan", "local network"}}},
		// The look: Settings' Accent colour, not network access.
		{Key: "accent", Label: "Accent colour", Section: LookSection, Value: "blue", Choices: [][2]string{{"blue", "Blue"}, {"green", "Green"}}, Swatches: map[string]string{"blue": "#0000ff"}},
	}
	m := NewMenu(HomeInfo{Host: "vm", User: "ada"}, "Ubuntu 24.04  ·  10.0.0.5", items, settings, []string{"Docker, over SSH:", "  docker context create vm …"}, false)
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
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m.Update(msg)
	}
}

// The menu says what's wrong in plain words, a row for each thing named by
// what it does, then Tools and Settings.
func TestMenuHome(t *testing.T) {
	m := testMenu(false)
	v := m.View()
	for _, want := range []string{"Needs you: 1 thing to fix, 1 value to fill in", "1 tool working", "updates not checked",
		"Start Docker Engine + Compose", "stopped: docker isn't running", "Fill in OPENAI_API_KEY", "Agent API keys needs it",
		"Tools", "or just start typing", "Settings", "type search tools"} {
		if !strings.Contains(v, want) {
			t.Errorf("home lacks %q:\n%s", want, v)
		}
	}
	for _, gone := range []string{"Network access", "Connect from a laptop", "Check for updates", "What would you like to do?"} {
		if strings.Contains(v, gone) {
			t.Errorf("home has %q:\n%s", gone, v)
		}
	}
	// The rows that never move come after a rule.
	if strings.Index(v, "───") > strings.Index(v, "Tools") || strings.Index(v, "───") < strings.Index(v, "OPENAI_API_KEY") {
		t.Errorf("no rule before Tools:\n%s", v)
	}
	h := testMenu(true).View()
	if strings.Contains(h, "Start ") || strings.Contains(h, "Fill in") || strings.Contains(h, "───") || !strings.Contains(h, "All good.") || !strings.Contains(h, "3 tools working") {
		t.Errorf("healthy home:\n%s", h)
	}
}

// A problem's row fixes it at once; drifted ones say restoring undoes the
// edits.
func TestMenuFix(t *testing.T) {
	m := testMenu(false)
	mk(m, "enter")
	if m.result.Kind != MenuFix || m.result.ID != "docker" || len(m.result.IDs) != 1 {
		t.Errorf("result %+v", m.result)
	}
	m = testMenu(true)
	m.items[2].State, m.items[2].Detail = "drifted", "~/.zshrc"
	if v := m.View(); !strings.Contains(v, "Restore Go") || !strings.Contains(v, "puts back yardo's version") {
		t.Errorf("drifted:\n%s", v)
	}
}

// A value's row opens it at once, one key a row; Tools opens the board;
// with three values or more, one row lists them.
func TestMenuSecretsAndTools(t *testing.T) {
	m := testMenu(false)
	mk(m, "down", "enter")
	if m.result.Kind != MenuEdit || m.result.ID != "keys" || strings.Join(m.result.Inputs, " ") != "~/.config/yardo/agent.env: OPENAI_API_KEY" {
		t.Errorf("fill in: %+v", m.result)
	}
	m = testMenu(false)
	onRow(m, "tools")
	mk(m, "enter")
	if m.result.Kind != MenuTools || m.result.Search != "" {
		t.Errorf("tools: %+v", m.result)
	}
	m = testMenu(false)
	m.items[1].Inputs = append(m.items[1].Inputs, "~/.config/yardo/agent.env: GEMINI_API_KEY", "~/.config/yardo/agent.env: XAI_API_KEY")
	if v := m.View(); !strings.Contains(v, "Fill in 3 values") || !strings.Contains(v, "OPENAI_API_KEY, GEMINI_API_KEY, XAI_API_KEY") {
		t.Errorf("three values:\n%s", v)
	}
	onRow(m, "secrets")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Home  ›  Fill in 1 value") {
		t.Errorf("values screen:\n%s", v)
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

// Settings lists network access, the look, the answers to change, how to
// connect from a laptop and the update check; esc goes home onto it.
func TestMenuSettings(t *testing.T) {
	m := testMenu(false)
	m.items = append(m.items, &BoardItem{ID: "gitid", Label: "Git: name, email and defaults", Tab: "git", Picked: true, State: "ok", Changes: [][2]string{{"git name and email", "git"}}})
	onRow(m, "settings")
	mk(m, "enter")
	v := m.View()
	for _, want := range []string{"Home  ›  Settings", "Network access", "MCP gateway: this machine only", "Accent colour", "Blue",
		"Git name and email", "asks, then applies it again", "Connect from a laptop", "Check for updates"} {
		if !strings.Contains(v, want) {
			t.Errorf("settings lacks %q:\n%s", want, v)
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
	if v := m.View(); !strings.Contains(v, "Home  ›  Settings  ›  Look") || !strings.Contains(v, "██") || strings.Contains(v, "MCP gateway") {
		t.Fatalf("look:\n%s", v)
	}
	mk(m, "right", "enter", "enter")
	if r := m.result; r.Kind != MenuApply || r.Settings["accent"] != "green" || len(r.Settings) != 1 {
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

// Network access: ←→ picks, each choice said in a sentence; Apply hands
// back only what changed; esc leaves nothing changed, back on Settings.
func TestMenuAccess(t *testing.T) {
	open := func() *Menu {
		m := testMenu(false)
		onRow(m, "settings")
		mk(m, "enter")
		onSetting(m, "access")
		mk(m, "enter")
		return m
	}
	m := open()
	if v := m.View(); !strings.Contains(v, "Home  ›  Settings  ›  Network access") || !strings.Contains(v, "(•) this machine only") || strings.Contains(v, "Accent colour") {
		t.Fatalf("access:\n%s", v)
	}
	mk(m, "down", "down", "down", "down")
	if m.sel != 2 {
		t.Errorf("past Apply: on row %d, want 2", m.sel)
	}
	mk(m, "up", "up")
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
	if m.screen != "settings" || m.settingsRows()[m.sel].id != "access" || m.result.Kind != MenuQuit || m.out {
		t.Errorf("esc: screen %s", m.screen)
	}
}

func TestMenuConnect(t *testing.T) {
	m := testMenu(false)
	onRow(m, "settings")
	mk(m, "enter")
	onSetting(m, "connect")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Home  ›  Settings  ›  Connect from a laptop") || !strings.Contains(v, "docker context create vm") {
		t.Errorf("connect:\n%s", v)
	}
	mk(m, "esc")
	if m.screen != "settings" || m.settingsRows()[m.sel].id != "connect" {
		t.Errorf("back on %s, row %d", m.screen, m.sel)
	}
}

func mcpMenu() *Menu {
	m := testMenu(true)
	m.items = append(m.items, &BoardItem{ID: "mcpjungle", Label: "MCP servers on MCPJungle", Tab: "agents", Picked: true, State: "ok"})
	m.info.MCP = []MCPServer{
		{Entry: "github", Name: "github", Spec: "github", About: "GitHub: repositories and pull requests", State: "ok", Yardo: true},
		{Entry: "jira", Name: "jira", Spec: "jira", About: "Jira and Confluence", State: "secret", Keys: []string{"JIRA_API_TOKEN"}, Yardo: true},
		{Entry: "jira:work", Name: "jira-work", Spec: "jira", About: "Jira and Confluence", State: "ok", Yardo: true},
		{Entry: "opensearch", Name: "opensearch", Spec: "opensearch", State: "failed", Detail: "Error: connection refused", Yardo: true},
		{Entry: "grafana:test", Name: "grafana-test", Spec: "grafana", State: "off"},
	}
	m.info.MCPSpecs = []MCPSpec{{"aws-api", "AWS: run AWS CLI commands"}, {"github", "GitHub"}, {"grafana", "Grafana"}, {"jira", "Jira and Confluence"}}
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
	for n, r := range m.rows() {
		if r.id == id {
			m.sel = n
		}
	}
}

// The home's MCP servers row (with the gateway only) opens the list:
// yardo's servers with their type and state, the ones found in ~/.mcp,
// then Add a server.
func TestMenuMCPList(t *testing.T) {
	if strings.Contains(testMenu(true).View(), "MCP servers") {
		t.Error("an MCP servers row without the gateway")
	}
	m := mcpMenu()
	if v := m.View(); !strings.Contains(v, "github, jira, jira-work, opensearch · 2 need you") {
		t.Errorf("home note:\n%s", v)
	}
	onRow(m, "mcp")
	mk(m, "enter")
	v := m.View()
	for _, want := range []string{"Home  ›  MCP servers", "needs JIRA_API_TOKEN", "didn't register", "Also in ~/.mcp", "grafana-test", "· off",
		"+ Add a server", "<name>__<tool>"} {
		if !strings.Contains(v, want) {
			t.Errorf("list lacks %q:\n%s", want, v)
		}
	}
	// The selected one's description is right of it, wrapped as it needs.
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "› github") && !strings.Contains(l, "GitHub:") {
			t.Errorf("description not beside the row:\n%s", v)
		}
	}
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
	mk(m, "enter", "a")
	if v := m.View(); !strings.Contains(v, "Add a server") || !strings.Contains(v, "you have jira, jira-work") || !strings.Contains(v, "you have github") {
		t.Fatalf("catalog:\n%s", v)
	}
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
	mk(m, "enter", "a", "enter", "enter")
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
	if v := m.View(); !strings.Contains(v, "MCP servers  ›  jira-work") || !strings.Contains(v, "Edit its settings") || !strings.Contains(v, "~/.mcp/jira-work/env") {
		t.Fatalf("server:\n%s", v)
	}
	mk(m, "enter")
	if r := m.result; r.Kind != MenuMCP || r.MCP != (MCPAct{Do: "edit", Entry: "jira:work"}) {
		t.Errorf("edit: %+v", r)
	}
	m = open(2)
	mk(m, "down", "enter") // Rename
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
	mk(m, "down", "down", "enter") // Delete
	if v := m.View(); !strings.Contains(v, "Delete jira-work?") || !strings.Contains(v, "can't be undone") {
		t.Fatalf("delete:\n%s", v)
	}
	mk(m, "enter") // Keep it
	if m.result.Kind != MenuQuit || m.screen != "mcp:server" {
		t.Errorf("keep: %+v on %s", m.result, m.screen)
	}
	mk(m, "enter") // Delete it (the row stays where it was)
	mk(m, "left", "enter")
	if r := m.result; r.MCP != (MCPAct{Do: "delete", Entry: "jira:work"}) {
		t.Errorf("delete: %+v", r)
	}
	m = open(4) // grafana-test, found off
	if !strings.Contains(m.View(), "› Turn it on") {
		t.Fatalf("found:\n%s", m.View())
	}
	mk(m, "enter")
	if r := m.result; r.MCP != (MCPAct{Do: "on", Entry: "grafana:test"}) {
		t.Errorf("on: %+v", r)
	}
}

// esc goes back a screen at a time, home onto the MCP servers row; from
// the board, the list's esc goes back to it. Open shows a toast.
func TestMenuMCPBack(t *testing.T) {
	m := mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "a", "enter")
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
	mk(m, "enter", "a")
	for range len(m.info.MCPSpecs) {
		mk(m, "down")
	}
	if v := m.View(); !strings.Contains(v, "› spec29") || strings.Contains(v, "aws-api") {
		t.Errorf("scrolled:\n%s", v)
	}
}

// A server that didn't register says why, and offers Try again first.
func TestMenuMCPFailed(t *testing.T) {
	m := mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "down", "down", "down", "enter") // opensearch
	if v := m.View(); !strings.Contains(v, "Error: connection refused") || !strings.Contains(v, "› Try again") {
		t.Fatalf("failed server:\n%s", v)
	}
	mk(m, "enter")
	if r := m.result; r.MCP != (MCPAct{Do: "retry", Entry: "opensearch"}) {
		t.Errorf("retry: %+v", r)
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

// Two problems are a row each; three or more are one row, whose screen
// has Fix all.
func TestMenuFixAll(t *testing.T) {
	m := testMenu(false)
	m.items[2].State = "missing" // Go too
	if v := m.View(); !strings.Contains(v, "Start Docker Engine + Compose") || !strings.Contains(v, "Install Go") {
		t.Fatalf("two problems:\n%s", v)
	}
	onRow(m, "fix:go")
	mk(m, "enter")
	if r := m.result; r.Kind != MenuFix || r.ID != "go" || len(r.IDs) != 1 {
		t.Errorf("fix one: %+v", r)
	}
	m = testMenu(false)
	m.items[2].State = "missing"
	m.items[3].Picked, m.items[3].State = true, "partial"
	if v := m.View(); !strings.Contains(v, "Fix 3 things") {
		t.Fatalf("no Fix 3 things:\n%s", v)
	}
	mk(m, "enter")
	if !strings.Contains(m.View(), "Fix all 3") {
		t.Fatalf("no Fix all:\n%s", m.View())
	}
	mk(m, "down", "down", "down", "down")
	if m.sel != 3 {
		t.Fatalf("on row %d, want Fix all (3)", m.sel)
	}
	mk(m, "enter")
	if r := m.result; r.Kind != MenuFix || strings.Join(r.IDs, " ") != "docker go wg" {
		t.Errorf("fix all: %+v", r)
	}
}

// esc and q go back from every screen; on the home q quits, esc only
// says so.
func TestMenuEscQ(t *testing.T) {
	m := testMenu(false)
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
		for _, id := range []string{"access", "look", "connect"} {
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
	if m.result.Kind != MenuQuit || m.out {
		t.Errorf("q: %+v", m.result)
	}
}

// Checked, with nothing newer: Check again, with the time; Open shows
// that nothing was found.
func TestMenuCheckAgain(t *testing.T) {
	m := NewMenu(HomeInfo{Host: "vm", Checked: "14:05"}, "", testMenu(true).items, nil, nil, true)
	if v := m.View(); !strings.Contains(v, "nothing newer · checked 14:05") {
		t.Errorf("home:\n%s", v)
	}
	onRow(m, "settings")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Check again") || !strings.Contains(v, "none found at 14:05") {
		t.Errorf("no Check again:\n%s", v)
	}
	onSetting(m, "check")
	mk(m, "enter")
	if m.result.Kind != MenuUpdates {
		t.Errorf("check again: %+v", m.result)
	}
	m = NewMenu(HomeInfo{Host: "vm"}, "", testMenu(true).items, nil, nil, true).Open("home", "✓ No updates: everything is current.", false)
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
	// The values filled in elsewhere: their screen is empty, so home.
	m = testMenu(false)
	m.items[1].Inputs = append(m.items[1].Inputs, "f: B", "f: C")
	onRow(m, "secrets")
	mk(m, "enter")
	m.items[1].State = "ok"
	mk(m, "down")
	if m.screen != "home" || m.sel < 0 {
		t.Errorf("empty secrets: on %s, row %d", m.screen, m.sel)
	}
	// An empty catalog: down stays at 0.
	m = mcpMenu()
	m.info.MCPSpecs = nil
	onRow(m, "mcp")
	mk(m, "enter", "a", "down", "down")
	if m.screen != "mcp:add" || m.sel != 0 {
		t.Errorf("empty catalog: on %s, row %d", m.screen, m.sel)
	}
}

// The servers are sorted once, when the menu is made: yardo's by name,
// then the found ones; the caller's slice is left as it was.
func TestMenuMCPSorted(t *testing.T) {
	info := HomeInfo{MCP: []MCPServer{{Name: "zed"}, {Name: "jira", Yardo: true}, {Name: "abc"}, {Name: "github", Yardo: true}}}
	m := NewMenu(info, "", nil, nil, nil, false)
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

// Updates: enter asks, esc takes it back, enter twice updates.
func TestMenuUpgrade(t *testing.T) {
	m := testMenu(true)
	m.items[2].Updates = []string{"go 1.25.1 → 1.25.2"}
	m.checked = true
	if !strings.Contains(m.View(), "Install 1 update") || !strings.Contains(m.View(), "working · 1 update") {
		t.Fatalf("home:\n%s", m.View())
	}
	onRow(m, "updates")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Update all") || m.sure {
		t.Fatalf("updates screen:\n%s", v)
	}
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Update 1 item?") {
		t.Fatalf("no question:\n%s", v)
	}
	mk(m, "esc")
	if m.sure || m.screen != "updates" || m.out {
		t.Fatalf("esc should take the question back: sure %v screen %s", m.sure, m.screen)
	}
	mk(m, "enter", "enter")
	if m.result.Kind != MenuUpgrade {
		t.Errorf("result %+v, want MenuUpgrade", m.result)
	}
}

// A check running in the background: the home says so, Settings won't
// start another; when it's done, its updates show.
func TestMenuChecking(t *testing.T) {
	m := testMenu(true)
	done := make(chan struct{})
	m.Checking(func() UpdatesMsg {
		<-done
		return UpdatesMsg{ByID: map[string][]string{"go": {"go 1.25.1 → 1.25.2"}}, At: "09:05"}
	})
	if m.Init() == nil || !strings.Contains(m.View(), "checking for updates…") {
		t.Fatalf("checking:\n%s", m.View())
	}
	onRow(m, "settings")
	mk(m, "enter")
	onSetting(m, "check")
	if !strings.Contains(m.View(), "checking now…") {
		t.Errorf("settings:\n%s", m.View())
	}
	mk(m, "enter")
	if m.result.Kind != MenuQuit || !strings.Contains(m.View(), "Checking already") {
		t.Errorf("check again while checking: %+v", m.result)
	}
	close(done)
	m.Update(m.wait())
	mk(m, "esc")
	if v := m.View(); !strings.Contains(v, "Install 1 update") || !strings.Contains(v, "1 update · checked 09:05") {
		t.Errorf("done:\n%s", v)
	}
}

// The home's dashboard: what's installed, the containers, the services
// and what listens beyond this machine; what doesn't fit is counted.
func TestMenuDashboard(t *testing.T) {
	m := testMenu(true)
	m.info.Dash = Dashboard{Docker: true,
		Containers: []Container{{Image: "postgres:16", Name: "myapp-db-1", Ports: []string{"5432"}}},
		Services:   []Service{{Name: "ssh", Up: true}, {Name: "fail2ban"}},
		Listening:  []Listener{{"22", "tcp", "ssh", "all"}, {"41641", "udp", "tailscale", "all"}, {"8080", "tcp", "docker", "vpn"}, {"631", "tcp", "cupsd", "local"}}}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Installed   3: Docker Engine + Compose · Agent API keys · Go", "Containers  1: postgres:16 (myapp-db-1) :5432",
		"Services    ssh (22/tcp) · fail2ban (failed) · tailscale (41641/udp) · docker (8080/tcp VPN only)"} {
		if !strings.Contains(v, want) {
			t.Errorf("dashboard lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Listening") {
		t.Errorf("listening apart from the services:\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	// Narrow: every tool, on as many lines as it takes; the services cut.
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Installed   3: Docker Engine + Compose · Agent API keys\n              Go\n") || !strings.Contains(v, "+ 3 more") {
		t.Errorf("narrow:\n%s", v)
	}
	m.info.Dash = Dashboard{Docker: true}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Containers  0\n") || strings.Contains(v, "Services") {
		t.Errorf("nothing running:\n%s", v)
	}
}

// A machine set up from a team profile says how much is the team's.
func TestMenuTeam(t *testing.T) {
	m := testMenu(true)
	m.items[0].Team, m.items[2].Team = "platform", "platform"
	if v := m.View(); !strings.Contains(v, "3 tools working · 2 from the team profile platform · updates not checked") {
		t.Errorf("home:\n%s", v)
	}
}

// The tools installed take the lines there are, the last saying how many
// more when they don't all fit.
func TestWrapList(t *testing.T) {
	parts := []string{"zsh", "Neovim", "tmux", "Go", "Python", "Node", "Docker", "Kubernetes"}
	if got := ansi.Strip(strings.Join(wrapList(parts, 20, 10), "|")); got != "zsh · Neovim · tmux|Go · Python · Node|Docker · Kubernetes" {
		t.Errorf("all: %q", got)
	}
	if got := ansi.Strip(strings.Join(wrapList(parts, 20, 2), "|")); got != "zsh · Neovim · tmux|Go · Python + 3 more" {
		t.Errorf("two lines: %q", got)
	}
}

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
	if v := m.View(); !strings.Contains(v, "Fill in 3 values") || strings.Count(v, "JIRA_URL") != 1 {
		t.Errorf("counted twice:\n%s", v)
	}
}
