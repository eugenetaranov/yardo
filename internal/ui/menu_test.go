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
		// The look: This VM's, not who can connect.
		{Key: "accent", Label: "Accent colour", Section: "__look__", Value: "blue", Choices: [][2]string{{"blue", "Blue"}, {"green", "Green"}}},
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

// The menu says what's wrong in plain words, and offers it first.
func TestMenuHome(t *testing.T) {
	m := testMenu(false)
	v := m.View()
	for _, want := range []string{"What would you like to do?", "Fix 1 problem", "Docker Engine + Compose stopped", "Fill in 1 secret", "OPENAI_API_KEY",
		"Add or remove tools", "Who can connect", "MCP gateway: this machine only", "Change settings", "git, prompt, dotfiles, accent colour…",
		"Connect from a laptop", "Check for updates", "1 needs attention"} {
		if !strings.Contains(v, want) {
			t.Errorf("home lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Accent colour:") {
		t.Errorf("the accent colour in who can connect's note:\n%s", v)
	}
	h := testMenu(true).View()
	if strings.Contains(h, "Fix ") || strings.Contains(h, "Fill in") || !strings.Contains(h, "Everything is healthy. 3 tools ready.") {
		t.Errorf("healthy home:\n%s", h)
	}
}

// Fix: one card per problem; Fix it hands the item back.
func TestMenuFix(t *testing.T) {
	m := testMenu(false)
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Home  ›  Fix 1 problem") || !strings.Contains(v, "docker isn't running") {
		t.Fatalf("fix screen:\n%s", v)
	}
	mk(m, "right", "enter")
	if !strings.Contains(m.View(), "apt docker-ce") {
		t.Errorf("details not shown:\n%s", m.View())
	}
	mk(m, "left", "enter")
	if m.result.Kind != MenuFix || m.result.ID != "docker" {
		t.Errorf("result %+v", m.result)
	}
}

// Fill in hands back the item's values; Add or remove opens the board.
func TestMenuSecretsAndTools(t *testing.T) {
	m := testMenu(false)
	mk(m, "down", "enter", "enter")
	if m.result.Kind != MenuEdit || m.result.ID != "keys" || len(m.result.Inputs) != 1 {
		t.Errorf("fill in: %+v", m.result)
	}
	m = testMenu(false)
	mk(m, "down", "down", "enter")
	if m.result.Kind != MenuTools {
		t.Errorf("tools: %+v", m.result)
	}
}

// Who can connect: ←→ picks, each choice said in a sentence; Apply hands
// back only what changed; esc leaves nothing changed.
func TestMenuAccess(t *testing.T) {
	m := testMenu(false)
	mk(m, "down", "down", "down", "enter")
	if v := m.View(); !strings.Contains(v, "Home  ›  Who can connect") || !strings.Contains(v, "(•) this machine only") || strings.Contains(v, "Accent colour") {
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
	m = testMenu(false)
	mk(m, "down", "down", "down", "enter", "right", "esc")
	if m.screen != "home" || m.result.Kind != MenuQuit || m.out {
		t.Errorf("esc: screen %s", m.screen)
	}
}

func TestMenuConnect(t *testing.T) {
	m := testMenu(false)
	onRow(m, "connect")
	mk(m, "enter")
	if v := m.View(); !strings.Contains(v, "Home  ›  Connect from a laptop") || !strings.Contains(v, "docker context create vm") {
		t.Errorf("connect:\n%s", v)
	}
	mk(m, "esc")
	if m.screen != "home" || m.rows()[m.sel].id != "connect" {
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

// Change settings, after Who can connect, opens the board on This VM.
func TestMenuSettings(t *testing.T) {
	m := testMenu(false)
	rs := m.rows()
	for n, r := range rs {
		if r.id == "settings" && (n == 0 || rs[n-1].id != "access") {
			t.Errorf("Change settings isn't after Who can connect: %v", rs)
		}
	}
	onRow(m, "settings")
	mk(m, "enter")
	if m.result.Kind != MenuSettings {
		t.Errorf("settings: %+v", m.result)
	}
}

// With two problems or more, Fix all hands them all back; one is fixed
// alone, with no Fix all.
func TestMenuFixAll(t *testing.T) {
	m := testMenu(false)
	mk(m, "enter")
	if strings.Contains(m.View(), "Fix all") {
		t.Errorf("Fix all for one problem:\n%s", m.View())
	}
	m = testMenu(false)
	m.items[2].State = "missing" // Go too
	mk(m, "enter")
	if !strings.Contains(m.View(), "Fix all 2") {
		t.Fatalf("no Fix all:\n%s", m.View())
	}
	mk(m, "down", "down", "down")
	if m.sel != 2 {
		t.Fatalf("on row %d, want Fix all (2)", m.sel)
	}
	mk(m, "enter")
	if r := m.result; r.Kind != MenuFix || len(r.IDs) != 2 || r.IDs[0] != "docker" || r.IDs[1] != "go" {
		t.Errorf("fix all: %+v", r)
	}
	m = testMenu(false)
	m.items[2].State = "missing"
	mk(m, "enter", "down", "enter")
	if r := m.result; r.Kind != MenuFix || r.ID != "go" || len(r.IDs) != 1 {
		t.Errorf("fix one: %+v", r)
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
	for _, id := range []string{"fix", "secrets", "access", "connect"} {
		for _, k := range []string{"esc", "q"} {
			m := testMenu(false)
			onRow(m, id)
			mk(m, "enter", k)
			if m.screen != "home" || m.rows()[m.sel].id != id {
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
	if v := m.View(); !strings.Contains(v, "Check again") || !strings.Contains(v, "none found at 14:05") {
		t.Errorf("no Check again:\n%s", v)
	}
	onRow(m, "check")
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
	// The secret filled in elsewhere: its screen is empty, so home.
	m = testMenu(false)
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
