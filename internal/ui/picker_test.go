package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestPicker(t *testing.T) *Picker {
	t.Helper()
	tabs := []Tab{{"system", "System"}, {"shell", "Shell & CLI"}, {"empty", "Empty"}}
	entries := []*Entry{
		{Tab: "system", ID: "docker", Label: "Docker Engine + Compose", Source: "apt docker-ce"},
		{Tab: "shell", ID: "zsh", Label: "zsh (login shell)", Source: "apt zsh"},
		{Tab: "shell", ID: "tmux", Label: "tmux", Source: "apt install tmux"},
		{Tab: MiseTab, ID: "mise:starship", Label: "starship — prompt", Source: "mise use -g starship", Names: []string{"starship"}},
	}
	p := NewPicker("yardo", tabs, entries, Options{Review: true})
	p.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return p
}

func press(p *Picker, keys ...any) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		var m tea.KeyMsg
		switch v := k.(type) {
		case tea.KeyType:
			m = tea.KeyMsg{Type: v}
		case string:
			m = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(v)}
		}
		_, cmd = p.Update(m)
	}
	return cmd
}

func TestPickerTabsAndToggle(t *testing.T) {
	p := newTestPicker(t)
	if len(p.tabs) != 3 || p.tabs[2].ID != reviewTab {
		t.Fatalf("tabs = %v (empty tabs dropped, Review last)", p.tabs)
	}
	press(p, tea.KeySpace) // docker
	press(p, "2", tea.KeyDown, tea.KeySpace)
	if got := strings.Join(p.Selected(), " "); got != "docker tmux" {
		t.Errorf("selected = %q", got)
	}
	press(p, tea.KeyEnter)
	if p.view() != reviewTab || len(p.items(reviewTab)) != 2 {
		t.Errorf("enter on the last category should go to Review: %s", p.view())
	}
	if cmd := press(p, tea.KeyEnter); cmd == nil || p.Outcome() != Done {
		t.Error("enter on Review confirms")
	}
}

func TestPickerSearch(t *testing.T) {
	p := newTestPicker(t)
	press(p, "s", "t", "a", "r")
	if p.view() != searchTab || len(p.results) == 0 || p.results[0].ID != "mise:starship" {
		t.Fatalf("search results = %v", ids(p.results, 5))
	}
	press(p, "2") // a digit is part of the query while searching
	if p.query != "star2" {
		t.Errorf("query = %q", p.query)
	}
	press(p, tea.KeyBackspace, tea.KeySpace)
	if !p.entries[3].On {
		t.Error("space should tick the search result")
	}
	if !strings.Contains(p.View(), "Search: ") {
		t.Error("search line not drawn")
	}
	press(p, tea.KeyEsc)
	if p.query != "" {
		t.Error("esc clears the search")
	}
	if cmd := press(p, tea.KeyEsc); cmd == nil || p.Outcome() != Quit {
		t.Error("esc on an empty search quits")
	}
}

func TestPickerEnterOnSearchGoesToReview(t *testing.T) {
	p := newTestPicker(t)
	press(p, "z", "s", "h", tea.KeySpace, tea.KeyEnter)
	if p.view() != reviewTab || p.query != "" {
		t.Errorf("view %s query %q", p.view(), p.query)
	}
}

// The install screen: enter on a search only clears it (the configure
// screen is a step away, not a search result away); enter on the last tab
// moves on.
func TestPickerInstallScreen(t *testing.T) {
	tabs := []Tab{{"system", "System"}, {"shell", "Shell & CLI"}}
	entries := []*Entry{{Tab: "system", ID: "docker", Label: "Docker"}, {Tab: "shell", ID: "zsh", Label: "zsh"}}
	p := NewPicker("yardo [1/2] Install", tabs, entries, Options{Next: "configure"})
	p.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd := press(p, "z", "s", "h", tea.KeySpace, tea.KeyEnter); cmd != nil || p.query != "" || p.Outcome() != Quit {
		t.Errorf("enter on a search: cmd %v query %q", cmd, p.query)
	}
	press(p, "2")
	if v := p.View(); !strings.Contains(v, "enter configure →") {
		t.Errorf("last tab should say where enter goes:\n%s", v)
	}
	if cmd := press(p, tea.KeyEnter); cmd == nil || p.Outcome() != Done {
		t.Error("enter on the last tab is done")
	}
}

// The configure screen: esc goes back to the install screen, ctrl-c quits.
func TestPickerBack(t *testing.T) {
	entries := []*Entry{{Tab: "git", ID: "gitid", Label: "Git name and email"}}
	p := NewPicker("yardo [2/2] Configure", []Tab{{"git", "Git & SSH"}}, entries, Options{Review: true, Back: true, Note: "Installing: Docker"})
	p.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(p.View(), "esc back") {
		t.Errorf("hint should say esc goes back:\n%s", p.View())
	}
	press(p, tea.KeyRight)
	if !strings.Contains(p.View(), "Installing: Docker") {
		t.Errorf("Review should show the note:\n%s", p.View())
	}
	if cmd := press(p, tea.KeyEsc); cmd == nil || p.Outcome() != Back {
		t.Errorf("esc: outcome %v, want Back", p.Outcome())
	}
	p.back = false
	if cmd := press(p, tea.KeyCtrlC); cmd == nil || p.Outcome() != Quit {
		t.Errorf("ctrl-c: outcome %v, want Quit", p.Outcome())
	}
}

func TestPickerView(t *testing.T) {
	p := newTestPicker(t)
	press(p, tea.KeySpace)
	v := p.View()
	for _, want := range []string{"yardo", "1 selected", "System (1)", "Docker Engine + Compose", "apt docker-ce", "[x]", "esc quit"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if n := strings.Count(v, "\n"); n > 30 {
		t.Errorf("view is %d lines, taller than the terminal", n)
	}
}

func TestWrap(t *testing.T) {
	got := wrap("Kubernetes CLIs (kubectl, kubectx, helm 3, k9s)", 16)
	for _, l := range got {
		if len(l) > 16 {
			t.Errorf("line %q wider than 16", l)
		}
	}
	if strings.Join(got, " ") != "Kubernetes CLIs (kubectl, kubectx, helm 3, k9s)" {
		t.Errorf("wrap lost words: %q", got)
	}
}
