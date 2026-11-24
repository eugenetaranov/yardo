package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// skillsHome makes skills in the home: name → the skills folders it's in.
func skillsHome(t *testing.T, e *Env, skills map[string][]string) {
	t.Helper()
	for n, ds := range skills {
		for _, d := range ds {
			put(t, e.Path(fmt.Sprintf("%s/%s/SKILL.md", d, n)), fmt.Sprintf("---\nname: %s\n---\n%s\n", n, n))
			put(t, e.Path(fmt.Sprintf("%s/%s/scripts/run.sh", d, n)), "#!/bin/sh\n")
			if err := os.Chmod(e.Path(fmt.Sprintf("%s/%s/scripts/run.sh", d, n)), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func listed(t *testing.T, e *Env) map[string]Skill {
	t.Helper()
	ss, _, err := ListSkills(e)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Skill{}
	for _, s := range ss {
		out[s.Name] = s
	}
	return out
}

func there(e *Env, p string) bool {
	_, err := os.Lstat(e.Path(p))
	return err == nil
}

// Off moves a skill out of where agents look, on puts it back where it
// was, byte for byte, modes and all.
func TestSkillsOffOn(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	skillsHome(t, e, map[string][]string{"tdd": SkillDirs, "my-notes": {"~/.claude/skills"}})
	if err := SwitchSkills(e, "off", []string{"tdd", "my-notes"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"~/.agents/skills/tdd", "~/.claude/skills/tdd", "~/.claude/skills/my-notes"} {
		if there(e, p) {
			t.Errorf("%s still there", p)
		}
	}
	if !there(e, SkillsOff+"/claude/tdd/SKILL.md") || !there(e, SkillsOff+"/agents/tdd/SKILL.md") {
		t.Error("tdd isn't held")
	}
	ls := listed(t, e)
	if !ls["tdd"].Off || !ls["tdd"].Own || len(ls["tdd"].Held) != 2 || !ls["my-notes"].Off {
		t.Errorf("listed: %+v", ls)
	}
	if err := SwitchSkills(e, "on", []string{"tdd", "my-notes"}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, e.Path("~/.agents/skills/tdd/SKILL.md")); !strings.HasSuffix(got, "tdd\n") {
		t.Errorf("tdd = %q", got)
	}
	if fi, err := os.Stat(e.Path("~/.claude/skills/tdd/scripts/run.sh")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("mode: %v %v", fi, err)
	}
	if !there(e, "~/.claude/skills/my-notes") || there(e, "~/.agents/skills/my-notes") {
		t.Error("my-notes isn't back where it was, only")
	}
	if there(e, SkillsOff+"/own") {
		t.Error("own still lists a skill")
	}
	if ls := listed(t, e); ls["tdd"].Off || len(ls["tdd"].Places) != 2 {
		t.Errorf("listed: %+v", ls["tdd"])
	}
}

// All off and back on: each skill as it was left by itself.
func TestSkillsAllOff(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	skillsHome(t, e, map[string][]string{"tdd": SkillDirs, "humanizer": SkillDirs})
	if err := SwitchSkills(e, "off", []string{"humanizer"}); err != nil {
		t.Fatal(err)
	}
	if err := SwitchSkills(e, "off", nil); err != nil {
		t.Fatal(err)
	}
	if there(e, "~/.claude/skills/tdd") || !SkillsAllOff(e) {
		t.Fatal("tdd isn't off with all")
	}
	if err := SwitchSkills(e, "on", []string{"tdd"}); err == nil || !strings.Contains(err.Error(), "turn them all on first") {
		t.Errorf("one on while all off: %v", err)
	}
	// One that appears meanwhile goes off with the rest.
	skillsHome(t, e, map[string][]string{"grill-me": {"~/.claude/skills"}})
	if ls := listed(t, e); !ls["grill-me"].Off || ls["grill-me"].Own {
		t.Errorf("grill-me: %+v", ls["grill-me"])
	}
	if err := SwitchSkills(e, "on", nil); err != nil {
		t.Fatal(err)
	}
	ls := listed(t, e)
	if ls["tdd"].Off || ls["grill-me"].Off || !ls["humanizer"].Off || !ls["humanizer"].Own {
		t.Errorf("after all on: %+v", ls)
	}
}

// A folder of the same name put back meanwhile stays; the held copy too.
func TestSkillsClash(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	skillsHome(t, e, map[string][]string{"tdd": SkillDirs})
	if err := SwitchSkills(e, "off", []string{"tdd"}); err != nil {
		t.Fatal(err)
	}
	put(t, e.Path("~/.claude/skills/tdd/SKILL.md"), "another\n")
	err := SwitchSkills(e, "on", []string{"tdd"})
	if err == nil || !strings.Contains(err.Error(), "~/.claude/skills/tdd is in the way") {
		t.Fatalf("err = %v", err)
	}
	if got := read(t, e.Path("~/.claude/skills/tdd/SKILL.md")); got != "another\n" {
		t.Errorf("the new folder changed: %q", got)
	}
	if !there(e, SkillsOff+"/claude/tdd") || !there(e, SkillsOff+"/agents/tdd") || there(e, "~/.agents/skills/tdd") {
		t.Error("the held copy was moved")
	}
}

// A link as a skill's folder isn't moved; a folder without SKILL.md isn't
// a skill; an unknown name lists the ones there are.
func TestSkillsOffOdd(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	skillsHome(t, e, map[string][]string{"tdd": {"~/.claude/skills"}})
	if err := os.MkdirAll(e.Path("~/.claude/skills/empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(e.Root, "etc", "x")
	put(t, target+"/SKILL.md", "x\n")
	if err := os.Symlink(target, e.Path("~/.claude/skills/linked")); err != nil {
		t.Fatal(err)
	}
	ls := listed(t, e)
	if len(ls) != 1 || ls["tdd"].Source != "theirs" {
		t.Errorf("listed: %+v", ls)
	}
	if err := SwitchSkills(e, "off", []string{"nope"}); err == nil || !strings.Contains(err.Error(), "the skills are tdd") {
		t.Errorf("unknown: %v", err)
	}
	if err := SwitchSkills(e, "off", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target + "/SKILL.md"); err != nil || !there(e, "~/.claude/skills/linked") {
		t.Error("the link was moved")
	}
}

// The skills step keeps an off skill off: an update goes to the held copy,
// a new skill goes held while all are off, and uninstalling removes held
// copies.
func TestSkillsStepWhileOff(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	v := func(s string) fstest.MapFS {
		return fstest.MapFS{
			"skills/jira-tickets/SKILL.md": {Data: []byte(fmt.Sprintf("---\nname: jira-tickets\ndescription: d\n---\n%s\n", s))},
			"skills/tdd/SKILL.md":          {Data: []byte("---\nname: tdd\ndescription: d\n---\nt\n")},
		}
	}
	e.Files = v("v1")
	st := step(t, catalog.Step{Type: "go:skills", Skills: []string{"jira-tickets"}})
	apply(t, e, st)
	if ls := listed(t, e); ls["jira-tickets"].Source != "yardo" {
		t.Errorf("source: %+v", ls["jira-tickets"])
	}
	if err := SwitchSkills(e, "off", []string{"jira-tickets"}); err != nil {
		t.Fatal(err)
	}
	e.Files = v("v2")
	if check(t, e, st) {
		t.Fatal("Check = true with a new version held")
	}
	apply(t, e, st)
	for _, d := range SkillDirs {
		if there(e, d+"/jira-tickets") {
			t.Errorf("%s: turned back on by a run", d)
		}
		if got := read(t, e.Path(heldDir(d)+"/jira-tickets/SKILL.md")); !strings.HasSuffix(got, "v2\n") {
			t.Errorf("%s: the held copy = %q", d, got)
		}
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
	// All off: a new skill goes straight to where it's held.
	if err := SwitchSkills(e, "off", nil); err != nil {
		t.Fatal(err)
	}
	tdd := step(t, catalog.Step{Type: "go:skills", Skills: []string{"tdd"}})
	apply(t, e, tdd)
	if there(e, "~/.claude/skills/tdd") || !there(e, SkillsOff+"/claude/tdd/SKILL.md") {
		t.Error("tdd was installed on while all are off")
	}
	// Uninstalling removes the held copies.
	p := &RemovePlan{}
	if err := st.(Undoer).PlanUndo(context.Background(), e, p); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if there(e, SkillsOff+"/claude/jira-tickets") || there(e, SkillsOff+"/agents/jira-tickets") {
		t.Error("held copies stayed after uninstall")
	}
}
