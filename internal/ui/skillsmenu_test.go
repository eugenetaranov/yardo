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
	for _, want := range []string{"Home › Skills", "All skills", "2 of 3", "archify", "not checked", "grill-me", "yardo's", "humanizer", "edited since", "○ off", "For Claude Code"} {
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

// n adds from GitHub; a skill not checked offers Check it and Delete it;
// a held one only Review it, and space can't switch it.
func TestMenuSkillsElsewhere(t *testing.T) {
	m, _ := skillsMenu()
	mk(m, "n")
	if r := m.result; r.Kind != MenuSkills || r.MCP.Do != "add" {
		t.Errorf("n: %+v", r)
	}
	m, _ = skillsMenu()
	mk(m, "enter") // archify, not checked
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Check it") || !strings.Contains(v, "Delete it") {
		t.Fatalf("not checked:\n%s", v)
	}
	mk(m, "down", "enter")
	if r := m.result; r.Kind != MenuSkills || r.MCP != (MCPAct{Do: "check", Entry: "archify"}) {
		t.Errorf("Check it: %+v", r)
	}
	m, _ = skillsMenu()
	mk(m, "enter", "down", "down", "enter")
	if m.dialog == nil || !strings.Contains(m.View(), "Delete archify?") {
		t.Fatalf("Delete it:\n%s", m.View())
	}
	mk(m, "y")
	if r := m.result; r.Kind != MenuSkills || r.MCP != (MCPAct{Do: "delete", Entry: "archify"}) {
		t.Errorf("delete: %+v", r)
	}
	m, _ = skillsMenu()
	m.info.Skills = append(m.info.Skills, SkillRow{Name: "zz-new", Source: "held", Off: "held"})
	mk(m, "down", "down", "down", " ")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "▲ held: waiting for you") || !strings.Contains(v, "enter reviews it") || len(m.skPend) != 0 {
		t.Errorf("held, space:\n%s", v)
	}
	mk(m, "enter")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Review it") || strings.Contains(v, "Turn it") {
		t.Errorf("held:\n%s", v)
	}
	mk(m, "enter")
	if r := m.result; r.MCP != (MCPAct{Do: "review", Entry: "zz-new"}) {
		t.Errorf("review: %+v", r)
	}
}

// A checked skill changed since offers Restore and Check it again; one
// with an update offers Update it; the home's Skills row says so.
func TestMenuSkillsDriftUpdate(t *testing.T) {
	m := testMenu(true)
	m.info.Skills = []SkillRow{
		{Name: "a-drift", Source: "drifted", Checked: "2026-11-24"},
		{Name: "b-new", Source: "checked", Checked: "2026-11-24", Update: strings.Repeat("c", 40)},
	}
	m = NewMenu(m.info, "", m.items, nil, true)
	i := slices.IndexFunc(m.rows(), func(r menuRow) bool { return r.id == "skills" })
	if i < 0 || m.rows()[i].note != "▲ changed since checked: a-drift" {
		t.Errorf("home's row: %+v", m.rows())
	}
	onRow(m, "skills")
	mk(m, "enter")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "▲ changed") || !strings.Contains(v, "update available") {
		t.Errorf("list:\n%s", v)
	}
	mk(m, "enter")
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "Restore it") || !strings.Contains(v, "Check it again") {
		t.Fatalf("drifted:\n%s", v)
	}
	mk(m, "down", "enter") // Turn it off, then Restore it
	if r := m.result; r.Kind != MenuSkills || r.MCP != (MCPAct{Do: "restore", Entry: "a-drift"}) {
		t.Errorf("restore: %+v", r)
	}
	m.result = MenuResult{}
	m.screen, m.sel = "skills", 1
	mk(m, "enter")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Update it") || !strings.Contains(v, "to cccccccccccc") {
		t.Fatalf("update:\n%s", v)
	}
	mk(m, "down", "enter")
	if r := m.result; r.MCP != (MCPAct{Do: "update", Entry: "b-new"}) {
		t.Errorf("update: %+v", r)
	}
}
