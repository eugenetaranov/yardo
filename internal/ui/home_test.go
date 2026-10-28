package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testHome() *Home {
	items := []*HomeItem{
		{ID: "go", Label: "Go + gopls", Group: "Dev", Tab: "group:Dev", State: "ok", Source: "mise use -g go"},
		{ID: "docker", Label: "Docker Engine", Group: "DevOps", Tab: "group:DevOps", State: "ok", Updates: []string{"docker-ce 27.3 → 27.4"}},
		{ID: "mcpjungle-install", Label: "MCPJungle gateway", Group: "AI", Tab: "ai", State: "down", Detail: "mcpjungle@ada isn't running"},
		{ID: "agentkeys", Label: "Agent API keys", Group: "Configure", Tab: "agents", State: "secret", Inputs: []string{"~/.config/yardo/agent.env: OPENAI_API_KEY"}},
		{ID: "k8s", Label: "Kubernetes", Group: "DevOps", Tab: "group:DevOps", State: "partial", Detail: "mise k9s"},
	}
	avail := []*HomeItem{
		{ID: "tailscale-install", Label: "Tailscale", Group: "System", Tab: "system", State: "available", Source: "curl tailscale.com/install.sh"},
		{ID: "openvpn-server", Label: "OpenVPN server", Group: "System", Tab: "system", State: "available"},
	}
	h := NewHome(HomeInfo{Host: "vm", User: "ada", Checked: "10:00"}, items, avail)
	h.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	return h
}

func key(h *Home, k string) {
	switch k {
	case "down":
		h.Update(tea.KeyMsg{Type: tea.KeyDown})
	case "enter":
		h.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		h.Update(tea.KeyMsg{Type: tea.KeyEsc})
	default:
		h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
}

// It opens on what needs attention, problems by severity; the header has
// the machine and the keys; the views are numbered at the bottom.
func TestHomeView(t *testing.T) {
	h := testHome()
	v := h.View()
	for _, want := range []string{"Host:    vm", "User:    ada", "<f> fix 2", "<a> add/change", "<c> access", " attention[3] ",
		"NAME", "GROUP", "STATE", "NOTE", "mcpjungle@ada isn't running", "OPENAI_API_KEY", "<2> attention (3)", "<3> available (2)"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if s := h.Selected(); s == nil || s.ID != "mcpjungle-install" {
		t.Errorf("selected %+v, want the down gateway first", s)
	}
	key(h, "1")
	if v := h.View(); !strings.Contains(v, " installed[5] ") || !strings.Contains(v, "▲ docker-ce 27.3 → 27.4") {
		t.Errorf("installed view:\n%s", v)
	}
}

func TestHomeKeys(t *testing.T) {
	for k, want := range map[string]HomeAction{"a": HomeChange, "c": HomeAccess, "x": HomeConnect, "u": HomeUpdates, "r": HomeRefresh, "q": HomeQuit, "f": HomeFix} {
		h := testHome()
		key(h, k)
		if h.action != want {
			t.Errorf("%s: action %v, want %v", k, h.action, want)
		}
	}
	h := testHome()
	if got := strings.Join(h.Fix(), " "); got != "mcpjungle-install k8s" {
		t.Errorf("fix = %q", got)
	}
	key(h, "e") // on the down gateway: nothing to fill in
	if h.action != HomeQuit || h.out {
		t.Errorf("e on a down item: action %v", h.action)
	}
	key(h, "down") // Kubernetes (partial), then Agent API keys (fill in)
	key(h, "down")
	key(h, "e")
	if h.action != HomeEdit || h.Selected().ID != "agentkeys" {
		t.Errorf("e: action %v on %s", h.action, h.Selected().ID)
	}
}

// 3 lists what can be added; enter opens the picker on its tab. Enter on a
// picked item shows its details.
func TestHomeAvailableAndDetails(t *testing.T) {
	h := testHome()
	key(h, "enter")
	if v := h.View(); !strings.Contains(v, "Why: mcpjungle@ada isn't running") || h.action != HomeQuit {
		t.Errorf("details:\n%s", v)
	}
	key(h, "3")
	if v := h.View(); !strings.Contains(v, " available[2] ") || !strings.Contains(v, "Tailscale") {
		t.Errorf("available view:\n%s", v)
	}
	key(h, "down")
	key(h, "enter")
	if h.action != HomeChange || h.StartTab() != "system" || h.Selected().ID != "openvpn-server" {
		t.Errorf("enter on available: action %v, tab %q", h.action, h.StartTab())
	}
}

// / filters the view; esc clears it.
func TestHomeFilter(t *testing.T) {
	h := testHome()
	key(h, "1")
	key(h, "/")
	for _, r := range "kube" {
		key(h, string(r))
	}
	key(h, "enter")
	if r := h.rows(h.view); len(r) != 1 || r[0].ID != "k8s" {
		t.Errorf("filtered rows = %v", r)
	}
	if v := h.View(); !strings.Contains(v, " installed(/kube)[1] ") {
		t.Errorf("title:\n%s", v)
	}
	key(h, "esc")
	if len(h.rows(h.view)) != 5 || h.action != HomeQuit || h.out {
		t.Error("esc didn't clear the filter")
	}
}

func TestPickerFocus(t *testing.T) {
	p := NewPicker("yardo", []Tab{{ID: "group:Dev", Title: "Dev", Sections: []Tab{{ID: "lang", Title: "Languages"}}}, {ID: "system", Title: "System"}},
		[]*Entry{{Tab: "lang", ID: "go", Label: "Go"}, {Tab: "system", ID: "tailscale-install", Label: "Tailscale"}}, Options{})
	p.Focus("system")
	if p.view() != "system" {
		t.Errorf("focus system: on %s", p.view())
	}
	p.Focus("lang")
	if p.view() != "group:Dev" {
		t.Errorf("focus a section: on %s", p.view())
	}
}
