package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

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
			fmt.Fprintln(stdout, skillSays(s, all))
		}
	}
	if len(args) > 0 && done == nil {
		fmt.Fprintln(stdout, "New agent sessions see the change.")
	}
	return done
}

// skillSays is a skill's line: on or off, and where it's from.
func skillSays(s steps.Skill, all bool) string {
	word := "● " + s.Name + ": on"
	switch {
	case s.Off && all && !s.Own:
		word = "○ " + s.Name + ": off, with all skills"
	case s.Off:
		word = "○ " + s.Name + ": off"
	}
	if !s.Off && len(s.Held) > 0 {
		word += " (a copy is held off too: " + strings.Join(s.Held, ", ") + ")"
	}
	return word + " · " + skillFrom(s.Source)
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

// skillRows are env's user's skills, as the Skills screen shows them.
func skillRows(env *steps.Env) ([]ui.SkillRow, bool, error) {
	skills, all, err := steps.ListSkills(env)
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
		out = append(out, r)
	}
	return out, all, err
}

// skillWhere says which agents have a skill, or where it's held off.
func skillWhere(s steps.Skill) string {
	who := func(ds []string) string {
		var w []string
		for _, d := range ds {
			if d == "~/.claude/skills" {
				w = append(w, "Claude Code ("+d+")")
			} else {
				w = append(w, "the other agents ("+d+")")
			}
		}
		return strings.Join(w, " and ")
	}
	if s.Off {
		return "Held off in " + steps.SkillsOff + ", for " + who(s.Held)
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
			what = append(what, who+" "+act.Do)
		}
		rows, all, rerr := skillRows(env)
		if rerr != nil {
			return nil, false, "", errors.Join(err, rerr)
		}
		return rows, all, "✓ " + strings.Join(what, ", ") + ". New agent sessions see the change.", err
	}
}
