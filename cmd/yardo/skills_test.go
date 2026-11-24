package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/steps"
)

func skillsEnv(t *testing.T, names ...string) *steps.Env {
	t.Helper()
	e := &steps.Env{Root: t.TempDir(), Home: "/home/ada"}
	for _, n := range names {
		for _, d := range steps.SkillDirs {
			p := e.Path(d + "/" + n + "/SKILL.md")
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("---\nname: "+n+"\n---\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return e
}

// yardo skills lists; on and off switch one or all; an unknown name fails,
// naming the skills there are.
func TestSkillsCmd(t *testing.T) {
	e := skillsEnv(t, "tdd", "grill-me")
	run := func(args ...string) (string, error) {
		var b bytes.Buffer
		err := skillsDo(&b, e, args)
		return b.String(), err
	}
	out, err := run()
	if err != nil || !strings.Contains(out, "● grill-me: on · not yardo's") || !strings.Contains(out, "● tdd: on") {
		t.Errorf("list: %v\n%s", err, out)
	}
	out, err = run("off", "tdd")
	if err != nil || !strings.Contains(out, "○ tdd: off") || strings.Contains(out, "grill-me") {
		t.Errorf("off tdd: %v\n%s", err, out)
	}
	out, err = run("off")
	if err != nil || !strings.Contains(out, "All skills are off") || !strings.Contains(out, "○ grill-me: off, with all skills") || !strings.Contains(out, "○ tdd: off ·") {
		t.Errorf("off all: %v\n%s", err, out)
	}
	if _, err = run("on", "grill-me"); err == nil || !strings.Contains(err.Error(), "turn them all on first") {
		t.Errorf("one on while all off: %v", err)
	}
	out, err = run("on")
	if err != nil || !strings.Contains(out, "● grill-me: on") || !strings.Contains(out, "○ tdd: off") {
		t.Errorf("on all: %v\n%s", err, out)
	}
	if _, err = run("off", "nope"); err == nil || !strings.Contains(err.Error(), "the skills are grill-me, tdd") {
		t.Errorf("unknown: %v", err)
	}
	if _, err = run("sideways"); err == nil {
		t.Error("an unknown word: want an error")
	}
}
