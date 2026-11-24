package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/addon"
	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// skillsCmd is `yardo skills [on|off] [skill …]`: the skills and how each
// is, or turning some (none named: all) off or on, then how they are.
func (o *options) skillsCmd(stdout io.Writer, st *answers.Store, args []string) error {
	u, err := o.userFor(st)
	if err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return errors.New("run it with sudo: sudo yardo skills " + strings.Join(args, " "))
	}
	env, err := newEnv(u, o.share(), io.Discard, false)
	if err != nil {
		return err
	}
	return skillsDo(stdout, env, args)
}

// skillsDo does skillsCmd's args for env's user: what it turned off or on
// is printed as it is now, even when some of it failed (the error says
// what).
func skillsDo(stdout io.Writer, env *steps.Env, args []string) error {
	var names []string
	var done error
	if len(args) > 0 {
		if args[0] != "on" && args[0] != "off" {
			return fmt.Errorf("skills: unknown %q (skills [on|off] [skill …])", args[0])
		}
		names = args[1:]
		if done = steps.SwitchSkills(env, args[0], names); done != nil && strings.HasPrefix(done.Error(), "no skill ") {
			return done
		}
	}
	skills, all, err := steps.ListSkills(env)
	if err != nil {
		return errors.Join(done, err)
	}
	if len(skills) == 0 {
		fmt.Fprintln(stdout, "No skills yet: Tools › AI adds yardo's; agents' own installers add theirs.")
		return done
	}
	if all {
		fmt.Fprintln(stdout, "○ All skills are off (sudo yardo skills on).")
	}
	for _, s := range skills {
		if len(names) == 0 || slices.Contains(names, s.Name) {
			l := skillSays(s, all)
			if ap, ok := addon.Approved(env.User, "skill", s.Name); ok && s.Source == "yardo" {
				l = skillChecked(l, ap)
				if !skillAsApproved(env, s, ap) {
					l += fmt.Sprintf("\n  ▲ changed since it was checked: sudo yardo skills restore %s puts the approved files back, sudo yardo skills check %s approves these", s.Name, s.Name)
				}
			} else if v := skillVersion(env, s); v != "" {
				l += " · " + v
			}
			fmt.Fprintln(stdout, l)
		}
	}
	if len(args) > 0 && done == nil {
		fmt.Fprintln(stdout, "New agent sessions see the change.")
	}
	return done
}

// skillSays is a skill's line: on or off, and where it's from.
func skillSays(s steps.Skill, all bool) string {
	word := fmt.Sprintf("● %s: on", s.Name)
	switch {
	case s.Off && all && !s.Own:
		word = fmt.Sprintf("○ %s: off, with all skills", s.Name)
	case s.Off:
		word = fmt.Sprintf("○ %s: off", s.Name)
	}
	if !s.Off && len(s.Held) > 0 {
		word += fmt.Sprintf(" (a copy is held off too: %s)", strings.Join(s.Held, ", "))
	}
	return fmt.Sprintf("%s · %s", word, skillFrom(s.Source))
}

// skillChecked is skillSays for a skill from elsewhere, approved as ap.
func skillChecked(line string, ap addon.Approval) string {
	from, _ := strings.CutSuffix(line, " · yardo's")
	return fmt.Sprintf("%s · checked %s: %s", from, ap.At.Format("2006-01-02"), addon.What(ap.Kind, ap.Source, ap.Pin, ap.Hash))
}

// skillFrom says where a skill is from.
func skillFrom(source string) string {
	switch source {
	case "yardo":
		return "yardo's"
	case "changed":
		return "yardo's, changed since"
	}
	return "not yardo's"
}

// skillRows are env's user's skills, as the Skills screen shows them:
// those from elsewhere marked checked (by the approvals) and, last, those
// held for the user's decision.
func skillRows(env *steps.Env) ([]ui.SkillRow, bool, error) {
	skills, all, err := steps.ListSkills(env)
	approved := map[string]addon.Approval{}
	if as, aerr := addon.Approvals(env.User); aerr == nil {
		for _, a := range as {
			if a.Kind == "skill" {
				approved[a.Name] = a
			}
		}
	}
	held := map[string]*addon.Held{}
	if hs, herr := addon.List(env.User); herr == nil {
		for _, h := range hs {
			if h.Kind == "skill" {
				held[h.Name] = h
			}
		}
	}
	out := []ui.SkillRow{}
	for _, s := range skills {
		r := ui.SkillRow{Name: s.Name, Source: s.Source}
		switch {
		case s.Off && all && !s.Own:
			r.Off = "all"
		case s.Off:
			r.Off = "own"
		}
		r.Where = skillWhere(s)
		if a, ok := approved[s.Name]; ok {
			what := addon.What(a.Kind, a.Source, a.Pin, a.Hash)
			r.Source, r.Checked = "checked", a.At.Format("2006-01-02")
			r.Where += " · " + what
			if !skillAsApproved(env, s, a) {
				r.Source = "drifted"
				r.Where = fmt.Sprintf("Changed since it was checked (%s): Restore puts the approved files back, Check it again approves these", what)
			}
		} else if v := skillVersion(env, s); v != "" {
			r.Where += " · " + v
		}
		if h := held[s.Name]; h != nil {
			r.Source, r.Off = "held", "held"
			r.Where = "Off while it's checked: " + verdictSays(h.Report.Verdict())
			delete(held, s.Name)
		}
		out = append(out, r)
	}
	for _, h := range held {
		out = append(out, ui.SkillRow{Name: h.Name, Source: "held", Off: "held",
			Where: fmt.Sprintf("%s: %s", addon.What(h.Kind, h.Source, h.Pin, ""), verdictSays(h.Report.Verdict()))})
	}
	slices.SortStableFunc(out, func(a, b ui.SkillRow) int { return strings.Compare(a.Name, b.Name) })
	return out, all, err
}

// skillWhere says which agents have a skill, or where it's held off.
func skillWhere(s steps.Skill) string {
	who := func(ds []string) string {
		var w []string
		for _, d := range ds {
			if d == "~/.claude/skills" {
				w = append(w, fmt.Sprintf("Claude Code (%s)", d))
			} else {
				w = append(w, fmt.Sprintf("the other agents (%s)", d))
			}
		}
		return strings.Join(w, " and ")
	}
	if s.Off {
		return fmt.Sprintf("Held off in %s, for %s", steps.SkillsOff, who(s.Held))
	}
	say := "For " + who(s.Places)
	if len(s.Held) > 0 {
		say += "; a copy is held off too"
	}
	return say
}

// skillSwitcher applies the skills turned off and on on the menu, in the
// background, then reads them again. One that fails stops the rest.
func (o *options) skillSwitcher(a *answers.Answers) ui.SkillSwitcher {
	return func(acts []ui.MCPAct) ([]ui.SkillRow, bool, string, error) {
		env, err := newEnv(a.User, o.share(), io.Discard, false)
		if err != nil {
			return nil, false, "", err
		}
		var what []string
		for _, act := range acts {
			var names []string
			who := "all skills"
			if act.Entry != "" {
				names, who = []string{act.Entry}, act.Entry
			}
			if err = steps.SwitchSkills(env, act.Do, names); err != nil {
				break
			}
			what = append(what, fmt.Sprintf("%s %s", who, act.Do))
		}
		rows, all, rerr := skillRows(env)
		if rerr != nil {
			return nil, false, "", errors.Join(err, rerr)
		}
		return rows, all, fmt.Sprintf("✓ %s. New agent sessions see the change.", strings.Join(what, ", ")), err
	}
}

// skillAsApproved says whether skill s's files are the ones approved as a.
func skillAsApproved(env *steps.Env, s steps.Skill, a addon.Approval) bool {
	dirs := slices.Clone(s.Places)
	for _, h := range s.Held {
		dirs = append(dirs, steps.HeldDir(h))
	}
	if len(dirs) == 0 {
		return false
	}
	for _, d := range dirs {
		files, err := steps.SkillOnDisk(env, d, s.Name)
		if err != nil || addon.Hash(files) != a.Hash {
			return false
		}
	}
	return true
}

// skillVersion is a skill's version (its SKILL.md's, else the yardo that
// installed it, for one of yardo's) and the hash of its files, cut to 7.
func skillVersion(env *steps.Env, s steps.Skill) string {
	dirs := slices.Clone(s.Places)
	for _, h := range s.Held {
		dirs = append(dirs, steps.HeldDir(h))
	}
	if len(dirs) == 0 {
		return ""
	}
	files, err := steps.SkillOnDisk(env, dirs[0], s.Name)
	if err != nil || len(files) == 0 {
		return ""
	}
	hash := addon.Hash(files)[:7]
	v := frontVersion(files["SKILL.md"])
	switch {
	case v == "" && s.Source == "yardo":
		v = "yardo " + version
	case v == "" && s.Source == "changed":
		v = fmt.Sprintf("yardo %s, edited", version)
	case v == "":
		return "content " + hash
	}
	return fmt.Sprintf("%s · %s", v, hash)
}

// frontVersion is the version a SKILL.md's front matter gives (version:,
// or under metadata:), "" for none.
func frontVersion(b []byte) string {
	rest, ok := strings.CutPrefix(string(b), "---\n")
	if !ok {
		return ""
	}
	front, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		return ""
	}
	for _, l := range strings.Split(front, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "version:"); ok {
			if v = strings.Trim(strings.TrimSpace(v), `"'`); v != "" {
				return v
			}
		}
	}
	return ""
}
