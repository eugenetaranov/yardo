package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// Skills go to both folders, marked; yardo updates its own, and leaves
// one the user edited (or made) alone.
func TestSkills(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	e.Files = fstest.MapFS{
		"skills/jira-tickets/SKILL.md":       {Data: []byte("---\nname: jira-tickets\ndescription: d\n---\nv1\n")},
		"skills/jira-tickets/scripts/run.sh": {Data: []byte("#!/bin/sh\necho hi\n")},
		"skills/mine/SKILL.md":               {Data: []byte("---\nname: mine\ndescription: d\n---\nyardo's\n")},
	}
	st := step(t, catalog.Step{Type: "go:skills", Skills: []string{"jira-tickets", "mine"}})
	// The user's own "mine", made by hand: not yardo's to change.
	put(t, e.Path("~/.claude/skills/mine/SKILL.md"), "my own\n")
	if check(t, e, st) {
		t.Fatal("Check = true before Apply")
	}
	apply(t, e, st)
	for _, d := range SkillDirs {
		if got := read(t, e.Path(d+"/jira-tickets/SKILL.md")); !strings.HasSuffix(got, "v1\n") {
			t.Errorf("%s: %q", d, got)
		}
		if fi, err := os.Stat(e.Path(d + "/jira-tickets/scripts/run.sh")); err != nil || fi.Mode().Perm() != 0o755 {
			t.Errorf("%s: script mode %v, %v", d, fi, err)
		}
		if _, err := os.Stat(e.Path(d + "/jira-tickets/" + skillMark)); err != nil {
			t.Errorf("%s: no mark", d)
		}
	}
	if got := read(t, e.Path("~/.claude/skills/mine/SKILL.md")); got != "my own\n" {
		t.Errorf("the user's own skill was changed: %q", got)
	}
	if got := read(t, e.Path("~/.agents/skills/mine/SKILL.md")); !strings.Contains(got, "yardo's") {
		t.Errorf("~/.agents/skills/mine = %q", got)
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
	// A new version replaces yardo's copy (a file it dropped goes too)...
	e.Files = fstest.MapFS{
		"skills/jira-tickets/SKILL.md": {Data: []byte("---\nname: jira-tickets\ndescription: d\n---\nv2\n")},
		"skills/mine/SKILL.md":         {Data: []byte("---\nname: mine\ndescription: d\n---\nyardo's\n")},
	}
	// ... but not one the user edited.
	put(t, e.Path("~/.claude/skills/jira-tickets/SKILL.md"), "edited\n")
	if check(t, e, st) {
		t.Fatal("Check = true with a new version")
	}
	apply(t, e, st)
	if got := read(t, e.Path("~/.agents/skills/jira-tickets/SKILL.md")); !strings.HasSuffix(got, "v2\n") {
		t.Errorf("not updated: %q", got)
	}
	if _, err := os.Stat(e.Path("~/.agents/skills/jira-tickets/scripts/run.sh")); err == nil {
		t.Error("a file the new version dropped is still there")
	}
	if got := read(t, e.Path("~/.claude/skills/jira-tickets/SKILL.md")); got != "edited\n" {
		t.Errorf("the edited copy was overwritten: %q", got)
	}
	if !check(t, e, st) {
		t.Error("Check = false with only the user's copies left")
	}
}

// yardo's skills follow agentskills.io: name = folder, lowercase and
// hyphens, a description of at most 1024 characters, portable keys only,
// under 100 lines.
func TestShippedSkills(t *testing.T) {
	dirs, err := filepath.Glob("../assets/conf/skills/*")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no skills: %v", err)
	}
	portable := map[string]bool{"name": true, "description": true, "license": true, "compatibility": true, "metadata": true, "allowed-tools": true}
	for _, d := range dirs {
		b, err := os.ReadFile(filepath.Join(d, "SKILL.md"))
		if err != nil {
			t.Errorf("%s: %v", d, err)
			continue
		}
		s := string(b)
		front, body, ok := strings.Cut(strings.TrimPrefix(s, "---\n"), "\n---\n")
		if !strings.HasPrefix(s, "---\n") || !ok {
			t.Errorf("%s: no frontmatter", d)
			continue
		}
		keys := map[string]string{}
		for _, l := range strings.Split(front, "\n") {
			if k, v, ok := strings.Cut(l, ":"); ok && !strings.HasPrefix(l, " ") {
				keys[k] = strings.TrimSpace(v)
				if !portable[k] {
					t.Errorf("%s: frontmatter key %q isn't portable", d, k)
				}
			}
		}
		name := filepath.Base(d)
		if keys["name"] != name || strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" || strings.Contains(name, "--") {
			t.Errorf("%s: name %q", d, keys["name"])
		}
		if n := len(keys["description"]); n == 0 || n > 1024 {
			t.Errorf("%s: description is %d characters", d, n)
		}
		if n := strings.Count(body, "\n"); n > 100 {
			t.Errorf("%s: %d lines", d, n)
		}
	}
}
