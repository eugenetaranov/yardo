package ui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// The Skills screens: the list (All skills, then each skill: where it's
// from, on or off) and one skill's actions. As on MCP servers, space marks
// a skill off or on, and a applies what's marked, in the background.

// SkillSwitcher turns skills off or on as acts say (an act's Entry is a
// skill's name; "" is all of them), then says how the skills are now (and
// whether all are off), and what happened.
type SkillSwitcher func(acts []MCPAct) (skills []SkillRow, allOff bool, toast string, err error)

// SwitchingSkills has the menu save the skills turned off and on itself,
// in the background, with f.
func (m *Menu) SwitchingSkills(f SkillSwitcher) *Menu {
	m.skSwitcher = f
	return m
}

// skillsSwitched is a save done: the skills as they are now.
type skillsSwitched struct {
	skills []SkillRow
	allOff bool
	toast  string
	err    error
}

func (m *Menu) skill() *SkillRow {
	if m.skSel < 0 || m.skSel >= len(m.info.Skills) {
		return nil
	}
	return &m.info.Skills[m.skSel]
}

// stageSkill marks act to apply (a): the screen shows it at once. Marking
// a skill back as it's saved unmarks it.
func (m *Menu) stageSkill(act MCPAct) {
	if m.skPend == nil {
		m.skPend, m.skBefore, m.skBeforeAll = map[string]string{}, slices.Clone(m.info.Skills), m.info.SkillsAllOff
	}
	m.showSkill(act)
	saved, now := m.skBeforeAll, m.info.SkillsAllOff
	if act.Entry != "" {
		i := slices.IndexFunc(m.skBefore, func(s SkillRow) bool { return s.Name == act.Entry })
		saved = i >= 0 && m.skBefore[i].Off == "own"
		now = act.Do == "off"
	}
	if now == saved {
		delete(m.skPend, act.Entry)
	} else {
		m.skPend[act.Entry] = act.Do
	}
	if len(m.skPend) == 0 {
		m.unstageSkills()
	}
}

// unstageSkills drops what's marked: the skills as they're saved.
func (m *Menu) unstageSkills() {
	if m.skPend != nil {
		m.info.Skills, m.info.SkillsAllOff = m.skBefore, m.skBeforeAll
	}
	m.skPend = nil
}

// showSkill is act on the screen, before it's saved.
func (m *Menu) showSkill(act MCPAct) {
	off := act.Do == "off"
	for n := range m.info.Skills {
		s := &m.info.Skills[n]
		how := "own"
		switch {
		case act.Entry == "":
			how = "all"
		case s.Name != act.Entry:
			continue
		}
		switch {
		case off && s.Off == "":
			s.Off = how
		case !off && (act.Entry != "" || s.Off == "all"):
			s.Off = ""
		}
	}
	if act.Entry == "" {
		m.info.SkillsAllOff = off
	}
}

// stagedSkills are the marked acts, in the order to save them: all on
// first, then each skill, then all off.
func (m *Menu) stagedSkills() []MCPAct {
	var out []MCPAct
	if m.skPend[""] == "on" {
		out = append(out, MCPAct{Do: "on"})
	}
	for _, n := range slices.Sorted(maps.Keys(m.skPend)) {
		if n != "" {
			out = append(out, MCPAct{Do: m.skPend[n], Entry: n})
		}
	}
	if m.skPend[""] == "off" {
		out = append(out, MCPAct{Do: "off"})
	}
	return out
}

// saveSkills applies what's marked, in the background: the menu stays.
func (m *Menu) saveSkills() tea.Cmd {
	switch {
	case len(m.skPend) == 0:
		m.toast = "Nothing to apply: space turns a skill off or on."
		return nil
	case m.skSwitching:
		m.toast = "Saving: a moment."
		return nil
	case m.skSwitcher == nil:
		m.toast = "Can't save from here: sudo yardo skills off|on <skill>."
		return nil
	}
	m.skSwitching = true
	acts, f := m.stagedSkills(), m.skSwitcher
	m.toast = "Saving…"
	return func() tea.Msg {
		skills, allOff, toast, err := f(acts)
		return skillsSwitched{skills, allOff, toast, err}
	}
}

// skillsSaved takes a save's outcome: the skills as they are now, or as
// they were before, when it failed before reading them.
func (m *Menu) skillsSaved(v skillsSwitched) {
	m.skSwitching = false
	switch {
	case v.err != nil && v.skills == nil:
		m.unstageSkills()
		m.toast = "✗ " + v.err.Error()
	case v.err != nil:
		m.skPend = nil
		m.info.Skills, m.info.SkillsAllOff, m.toast = v.skills, v.allOff, "✗ "+v.err.Error()
	default:
		m.skPend = nil
		m.info.Skills, m.info.SkillsAllOff, m.toast = v.skills, v.allOff, v.toast
	}
	m.normalize()
}

// skillAct is space's act on the list's row sel (-1: All skills), or why
// there's none.
func (m *Menu) skillAct(sel int) (MCPAct, string) {
	if sel < 0 {
		if m.info.SkillsAllOff {
			return MCPAct{Do: "on"}, ""
		}
		return MCPAct{Do: "off"}, ""
	}
	s := m.info.Skills[sel]
	switch {
	case m.info.SkillsAllOff:
		return MCPAct{}, "All skills are off: turn them on first (the All skills row)."
	case s.Off != "":
		return MCPAct{Do: "on", Entry: s.Name}, ""
	}
	return MCPAct{Do: "off", Entry: s.Name}, ""
}

// skillActions are a skill's screen's rows: turn it off or on, back.
func (m *Menu) skillActions(s *SkillRow) []mcpAction {
	if s == nil {
		return nil
	}
	act, _ := m.skillAct(slices.IndexFunc(m.info.Skills, func(r SkillRow) bool { return r.Name == s.Name }))
	var out []mcpAction
	switch act.Do {
	case "off":
		out = append(out, mcpAction{"off", "Turn it off", "out of every agent's reach, kept to turn on again"})
	case "on":
		out = append(out, mcpAction{"on", "Turn it on", "back where it was"})
	}
	return append(out, mcpAction{"back", "Back", ""})
}

func (m *Menu) skillsKey(k string, up, down bool) tea.Cmd {
	switch m.screen {
	case "skills":
		n := len(m.info.Skills)
		leaving := m.leaving
		m.leaving = false
		switch {
		case (k == "esc" || k == "q") && len(m.skPend) > 0 && !leaving:
			m.leaving = true
			m.toast = plural(len(m.skPend), "change isn't applied", "changes aren't applied") + ": a applies, q again leaves without them."
		case k == "esc" || k == "q":
			m.unstageSkills()
			m.back()
		case k == "a":
			return m.saveSkills()
		case up:
			m.sel = max(-1, m.sel-1)
		case down:
			m.sel = min(n-1, m.sel+1)
		case k == " " || (k == "enter" && m.sel < 0):
			if m.skSwitching || m.sel >= n {
				return nil
			}
			act, why := m.skillAct(m.sel)
			if why != "" {
				m.toast = why
			}
			if act.Do != "" {
				m.stageSkill(act)
			}
		case k == "enter":
			m.skSel = m.sel
			m.screen, m.sel = "skills:skill", 0
		}
	case "skills:skill":
		as := m.skillActions(m.skill())
		switch {
		case k == "esc" || k == "q":
			m.screen, m.sel = "skills", m.skSel
		case up:
			m.sel = max(0, m.sel-1)
		case down:
			m.sel = min(len(as)-1, m.sel+1)
		case k == "enter" && m.sel < len(as):
			if a := as[m.sel].id; a != "back" {
				m.stageSkill(MCPAct{Do: a, Entry: m.skill().Name})
			}
			m.screen, m.sel = "skills", m.skSel
		}
	}
	return nil
}

// skillWord is how a skill is: on, or off (by itself, or with all).
func skillWord(s SkillRow) string {
	switch s.Off {
	case "own":
		return accent.Render("○ off")
	case "all":
		return accent.Render("○ off") + muted.Render(" · with all skills")
	}
	return green.Render("● on")
}

// skillFrom says where a skill is from.
func skillFrom(s SkillRow) string {
	switch s.Source {
	case "yardo":
		return "yardo's"
	case "changed":
		return "yardo's, edited"
	}
	return "not yardo's"
}

// skillsKeys are a Skills screen's keys.
func (m *Menu) skillsKeys() []Key {
	if m.screen == "skills:skill" {
		return []Key{{Key: "↑↓", What: "choose"}, {Key: "enter", What: "do it"}}
	}
	apply := "apply"
	if n := len(m.skPend); n > 0 {
		apply = fmt.Sprintf("apply %d", n)
	}
	return []Key{{Key: "↑↓", What: "choose"}, {Key: "space", What: "on/off"}, {Key: "a", What: apply, On: len(m.skPend) > 0}, {Key: "enter", What: "open"}}
}

// skillsPath is where a Skills screen is, from the home.
func (m *Menu) skillsPath() string {
	if s := m.skill(); m.screen == "skills:skill" && s != nil {
		return "Skills › " + s.Name
	}
	return "Skills"
}

// skillsView is the Skills screens' lines; room is how many lines the
// screen has for them. Its rows' hits are recorded from body line top.
func (m *Menu) skillsView(w, room int) []string {
	top := 1 // under the body's blank first line
	var b []string
	add := func(s ...string) { b = append(b, s...) }
	if m.screen == "skills:skill" {
		s := m.skill()
		if s == nil {
			return nil
		}
		add("  "+bold.Render(s.Name)+muted.Render(" · "+skillFrom(*s)), "  "+skillWord(*s), "  "+muted.Render(s.Where), "")
		for n, a := range m.skillActions(s) {
			m.hit(top+len(b), 0, 0, m.selects(n))
			if n == m.sel {
				add("  " + rowOn.Render(padTo(" › "+a.label, 24)) + "  " + muted.Render(a.note))
			} else {
				add("     " + padTo(a.label, 21) + "  " + muted.Render(a.note))
			}
		}
		return b
	}
	ss := m.info.Skills
	if len(ss) == 0 {
		add("  No skills yet. Tools › AI adds yardo's; an agent's own installer adds its.", "")
		return b
	}
	var lines []string
	var rowOf []int
	row := func(n int, l string) {
		for len(rowOf) < len(lines) {
			rowOf = append(rowOf, -2)
		}
		rowOf = append(rowOf, n)
		lines = append(lines, l)
	}
	on := 0
	for _, s := range ss {
		if s.Off == "" {
			on++
		}
	}
	word := green.Render("● on") + muted.Render(fmt.Sprintf(" · %d of %d", on, len(ss)))
	if m.info.SkillsAllOff {
		word = accent.Render("○ off") + muted.Render(" · space turns them back on")
	}
	if _, marked := m.skPend[""]; marked {
		word += accent.Render("  · not saved")
	}
	if m.sel < 0 {
		row(-1, "  "+rowOn.Render(padTo(" › All skills", 25))+"  "+word)
	} else {
		row(-1, "     "+padTo("All skills", 22)+"  "+word)
	}
	lines = append(lines, "")
	selLine := 0
	for n, s := range ss {
		cols := padTo(skillFrom(s), 16) + "  " + skillWord(s)
		name := padTo(s.Name, 22)
		if m.info.SkillsAllOff {
			cols, name = muted.Render(padTo(skillFrom(s), 16))+"  "+skillWord(s), muted.Render(name)
		}
		if _, marked := m.skPend[s.Name]; marked {
			cols += accent.Render("  · not saved")
		}
		if n == m.sel {
			selLine = len(lines)
			row(n, "  "+rowOn.Render(padTo(" › "+s.Name, 25))+"  "+cols)
		} else {
			row(n, "     "+name+"  "+cols)
		}
	}
	if len(m.skPend) > 0 {
		lines = append(lines, "", "  "+accentBd.Render("a applies "+plural(len(m.skPend), "change", "changes"))+"  "+muted.Render("q leaves without them"))
	}
	for len(rowOf) < len(lines) {
		rowOf = append(rowOf, -2)
	}
	about := "Turns every skill off at once, and back on: each as you left it."
	if m.sel >= 0 && m.sel < len(ss) {
		about = ss[m.sel].Where
	}
	n := room - len(b) - 4
	start := windowStart(len(lines), selLine, n)
	for c, l := range window(lines, selLine, n) {
		if r := rowOf[start+c]; r > -2 {
			m.hit(top+len(b), 0, 0, m.selects(r))
		}
		add(l)
	}
	add("", "     "+muted.Render(about))
	add("", "  "+muted.Render(strings.Join([]string{"Off moves a skill out of every agent's reach, kept to turn on again.", "New agent sessions see the change."}, " ")))
	return b
}
