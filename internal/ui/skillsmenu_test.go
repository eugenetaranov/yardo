package ui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// skillsMenu is the Skills list, with a switcher that records what it's
// given and says the skills are as marked.
func skillsMenu() (*Menu, *[][]MCPAct) {
	m := testMenu(true)
	m.info.Skills = []SkillRow{
		{Name: "archify", Source: "theirs", Where: "For Claude Code (~/.claude/skills)"},
		{Name: "grill-me", Source: "yardo"},
		{Name: "humanizer", Source: "changed", Off: "own"},
	}
	m = NewMenu(m.info, "", m.items, nil, true)
	ran := &[][]MCPAct{}
	m.SwitchingSkills(func(acts []MCPAct) ([]SkillRow, bool, string, error) {
		*ran = append(*ran, acts)
		return slices.Clone(m.info.Skills), m.info.SkillsAllOff, "✓ saved", nil
	})
	onRow(m, "skills")
	mk(m, "enter")
	return m, ran
}

// The home has a Skills tab; its list shows each skill, where it's from
// and whether it's on.
func TestMenuSkills(t *testing.T) {
	m, _ := skillsMenu()
	v := ansi.Strip(m.View())
	for _, want := range []string{"Home › Skills", "All skills", "2 of 3", "archify", "not yardo's", "grill-me", "yardo's", "humanizer", "yardo's, edited", "○ off", "For Claude Code"} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q:\n%s", want, v)
		}
	}
	mk(m, "q")
	if m.screen != "home" || m.rows()[m.sel].id != "skills" {
		t.Errorf("q: on %s", m.screen)
	}
	m = testMenu(true)
	if slices.ContainsFunc(m.rows(), func(r menuRow) bool { return r.id == "skills" }) {
		t.Error("a Skills tab with no skills")
	}
}

// Space marks a skill, a applies in the background; all off dims the rows
// and refuses one on; leaving unsaved asks once.
func TestMenuSkillsSwitch(t *testing.T) {
	m, ran := skillsMenu()
	mk(m, "down", " ") // grill-me
	v := ansi.Strip(m.View())
	if m.info.Skills[1].Off != "own" || !strings.Contains(v, "not saved") || !strings.Contains(v, "a applies 1 change") || len(*ran) != 0 {
		t.Fatalf("marked grill-me off:\n%s", v)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	runCmd(m, cmd)
	if !reflect.DeepEqual(*ran, [][]MCPAct{{{Do: "off", Entry: "grill-me"}}}) || strings.Contains(m.View(), "not saved") || !strings.Contains(m.View(), "✓ saved") {
		t.Errorf("a: ran %v\n%s", *ran, m.View())
	}
	// All off: each that was on goes with all; one can't come on alone.
	m, _ = skillsMenu()
	mk(m, "up", " ")
	if !m.info.SkillsAllOff || m.info.Skills[0].Off != "all" || m.info.Skills[2].Off != "own" {
		t.Errorf("all off: %+v", m.info.Skills)
	}
	mk(m, "down", " ")
	if !strings.Contains(m.View(), "turn them on first") {
		t.Errorf("one while all are off:\n%s", m.View())
	}
	mk(m, "up", " ") // all back on: humanizer stays off
	if m.info.SkillsAllOff || m.info.Skills[0].Off != "" || m.info.Skills[2].Off != "own" || strings.Contains(m.View(), "not saved") {
		t.Errorf("all on: %+v\n%s", m.info.Skills, m.View())
	}
	// q with a change asks once.
	m, _ = skillsMenu()
	mk(m, " ", "q")
	if m.screen != "skills" || !strings.Contains(m.View(), "1 change isn't applied") {
		t.Fatalf("q once:\n%s", m.View())
	}
	mk(m, "q")
	if m.screen != "home" || m.info.Skills[0].Off != "" {
		t.Errorf("q again: on %s, %+v", m.screen, m.info.Skills[0])
	}
}

// A skill's screen turns it off or on, back on the list, marked.
func TestMenuSkillScreen(t *testing.T) {
	m, ran := skillsMenu()
	mk(m, "down", "down", "enter") // humanizer, off
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Home › Skills › humanizer") || !strings.Contains(v, "› Turn it on") {
		t.Fatalf("skill:\n%s", v)
	}
	mk(m, "enter")
	if m.screen != "skills" || m.info.Skills[2].Off != "" || !strings.Contains(m.View(), "not saved") || len(*ran) != 0 {
		t.Errorf("Turn it on: on %s\n%s", m.screen, m.View())
	}
}
