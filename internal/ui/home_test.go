package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testHome() *Home {
	items := []*HomeItem{
		{ID: "go", Label: "Go + gopls", Group: "Dev", State: "ok", Source: "mise use -g go"},
		{ID: "docker", Label: "Docker Engine", Group: "DevOps", State: "ok"},
		{ID: "mcpjungle-install", Label: "MCPJungle gateway", Group: "AI", State: "down", Detail: "mcpjungle@ada isn't running"},
		{ID: "agentkeys", Label: "Agent API keys", Group: "Configure", State: "secret", Inputs: []string{"~/.config/yardo/agent.env: OPENAI_API_KEY"}},
		{ID: "k8s", Label: "Kubernetes", Group: "DevOps", State: "partial", Detail: "mise k9s"},
	}
	h := NewHome("yardo · vm · ada", "checked 10:00", items, []string{"Dev", "DevOps", "AI", "Configure"}, 12)
	h.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	return h
}

// Problems first, under Needs attention; the rest in their groups.
func TestHomeView(t *testing.T) {
	h := testHome()
	v := h.View()
	att, dev := strings.Index(v, "Needs attention"), strings.Index(v, "Dev\n")
	if att < 0 || dev < att {
		t.Fatalf("view:\n%s", v)
	}
	for _, want := range []string{"mcpjungle@ada isn't running", "OPENAI_API_KEY", "+ 12 more to add", "1 down", "2 ok"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	// The cursor starts on the first item, not a heading.
	if h.rows[h.cursor].item == nil || h.rows[h.cursor].item.ID != "mcpjungle-install" {
		t.Errorf("cursor on %+v", h.rows[h.cursor])
	}
}

func TestHomeKeys(t *testing.T) {
	for key, want := range map[string]HomeAction{"a": HomeChange, "r": HomeRefresh, "q": HomeQuit} {
		h := testHome()
		h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if h.action != want {
			t.Errorf("%s: action %v, want %v", key, h.action, want)
		}
	}
	h := testHome()
	for i := 0; i < 10; i++ {
		h.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if it := h.rows[h.cursor].item; it == nil || it.ID != "docker" {
		t.Errorf("down past the end: cursor on %+v", h.rows[h.cursor])
	}
	h = testHome()
	h.Update(tea.KeyMsg{Type: tea.KeyDown})
	h.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(h.View(), "To fill in: ~/.config/yardo/agent.env: OPENAI_API_KEY") {
		t.Errorf("details:\n%s", h.View())
	}
}

// f re-applies what's down, partial, missing or drifted; e fills in an
// item's values, and only on such an item.
func TestHomeFixAndEdit(t *testing.T) {
	h := testHome()
	if got := strings.Join(h.Fix(), " "); got != "mcpjungle-install k8s" {
		t.Errorf("fix = %q", got)
	}
	if v := h.View(); !strings.Contains(v, "f fix 2") {
		t.Errorf("footer lacks f fix 2:\n%s", v)
	}
	h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}) // on the gateway: nothing to fill in
	if h.action != HomeQuit || h.out {
		t.Errorf("e on a down item: action %v", h.action)
	}
	h.Update(tea.KeyMsg{Type: tea.KeyDown}) // Agent API keys
	if !strings.Contains(h.View(), "e fill in") {
		t.Error("footer lacks e fill in on a secret item")
	}
	h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if h.action != HomeEdit || h.Selected().ID != "agentkeys" {
		t.Errorf("e: action %v on %s", h.action, h.Selected().ID)
	}
	h = testHome()
	h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if h.action != HomeFix {
		t.Errorf("f: action %v", h.action)
	}
}
