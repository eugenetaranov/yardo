package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
		"Add or remove tools", "Who can connect", "MCP gateway: this machine only", "Connect from a laptop", "Check for updates", "1 needs attention"} {
		if !strings.Contains(v, want) {
			t.Errorf("home lacks %q:\n%s", want, v)
		}
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
	if v := m.View(); !strings.Contains(v, "Home  ›  Who can connect") || !strings.Contains(v, "(•) this machine only") {
		t.Fatalf("access:\n%s", v)
	}
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
	mk(m, "down", "down", "down", "down", "enter")
	if v := m.View(); !strings.Contains(v, "Home  ›  Connect from a laptop") || !strings.Contains(v, "docker context create vm") {
		t.Errorf("connect:\n%s", v)
	}
	mk(m, "esc")
	if m.screen != "home" || m.rows()[m.sel].id != "connect" {
		t.Errorf("back on %s, row %d", m.screen, m.sel)
	}
}
