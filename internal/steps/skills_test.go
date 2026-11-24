package steps

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/BurntSushi/toml"

	"github.com/eugenetaranov/yardo/internal/assets"
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
	vendored := map[string]bool{}
	for _, s := range skillsLock(t) {
		vendored[s.Name] = true
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
				if !portable[k] && !vendored[filepath.Base(d)] {
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
		if n := strings.Count(body, "\n"); n > 100 && !vendored[name] {
			t.Errorf("%s: %d lines", d, n)
		}
	}
}

func skillsLock(t *testing.T) []LockedSkill {
	t.Helper()
	var lock struct{ Skill []LockedSkill }
	if _, err := toml.DecodeFile("../assets/skills.lock.toml", &lock); err != nil {
		t.Fatal(err)
	}
	return lock.Skill
}

// Each vendored skill is as the lock says (scripts/update-skills made it,
// unedited), in the binary too, under a licence yardo can ship, with its
// licence file; a folder that isn't yardo's own is in the lock.
func TestSkillsLock(t *testing.T) {
	licences := map[string]bool{"MIT": true, "Apache-2.0": true, "BSD-2-Clause": true, "BSD-3-Clause": true, "ISC": true}
	e := &Env{Files: assets.Conf()}
	locked := map[string]bool{}
	for _, s := range skillsLock(t) {
		locked[s.Name] = true
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(s.Commit) || s.Repo == "" || s.Path == "" {
			t.Errorf("%s: commit %q, repo %q, path %q", s.Name, s.Commit, s.Repo, s.Path)
		}
		if !licences[s.License] {
			t.Errorf("%s: licence %q", s.Name, s.License)
		}
		if s.Fetch {
			// Fetched at install: a hash to check it by, a licence file to
			// fall back on, and nothing vendored.
			if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(s.Sha256) || s.LicenseFile == "" {
				t.Errorf("%s: sha256 %q, license_file %q", s.Name, s.Sha256, s.LicenseFile)
			}
			if _, err := os.Stat("../assets/conf/skills/" + s.Name); err == nil {
				t.Errorf("%s: fetched at install, but vendored too", s.Name)
			}
			continue
		}
		files, _, err := onDisk("../assets/conf/skills/" + s.Name)
		if err != nil || files == nil {
			t.Errorf("%s: not vendored: %v", s.Name, err)
			continue
		}
		if got := treeHash(files); got != s.Sha256 {
			t.Errorf("%s: sha256 %s, lock says %s (run scripts/update-skills %s)", s.Name, got, s.Sha256, s.Name)
		}
		if embedded, err := shipped(e, s.Name); err != nil || treeHash(embedded) != s.Sha256 {
			t.Errorf("%s: the embedded copy differs (go:embed leaves out a file?): %v", s.Name, err)
		}
		if _, ok := files["LICENSE"]; !ok {
			if _, ok := files["LICENSE.txt"]; !ok {
				t.Errorf("%s: no LICENSE", s.Name)
			}
		}
	}
	dirs, _ := filepath.Glob("../assets/conf/skills/*")
	for _, d := range dirs {
		b, _ := os.ReadFile(filepath.Join(d, "SKILL.md"))
		if n := filepath.Base(d); !locked[n] && !strings.Contains(string(b), "source: yardo") {
			t.Errorf("%s: neither yardo's (metadata source: yardo) nor in skills.lock.toml", n)
		}
	}
}

// A link where a skill goes (the skills folder, the skill's folder, a
// file in it) isn't written through: yardo writes as root, and the link
// could point anywhere. The run says so; the other folder still gets it.
func TestSkillsLinks(t *testing.T) {
	files := fstest.MapFS{"skills/tdd/SKILL.md": {Data: []byte("---\nname: tdd\ndescription: d\n---\nv1\n")}}
	st := step(t, catalog.Step{Type: "go:skills", Skills: []string{"tdd"}})
	for _, c := range []struct{ name, link, at string }{
		{"skills folder", "~/.agents/skills", "~/.agents/skills"},
		{"agents folder", "~/.agents", "~/.agents"},
		{"skill folder", "~/.agents/skills/tdd", "~/.agents/skills/tdd"},
		{"file", "~/.agents/skills/tdd/SKILL.md", "~/.agents/skills/tdd/SKILL.md"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := testEnv(t, &fakeExec{})
			e.Files = files
			var notes []string
			e.Note = func(s string) { notes = append(notes, s) }
			target := filepath.Join(e.Root, "etc", "target")
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
			link := e.Path(c.link)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(c.link, "SKILL.md") {
				target = filepath.Join(target, "config")
				put(t, target, "secret\n")
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			apply(t, e, st)
			if got := read(t, e.Path("~/.claude/skills/tdd/SKILL.md")); !strings.HasSuffix(got, "v1\n") {
				t.Errorf("~/.claude/skills/tdd = %q", got)
			}
			if strings.HasSuffix(c.link, "SKILL.md") {
				if got := read(t, target); got != "secret\n" {
					t.Errorf("wrote through the link: %q", got)
				}
			} else if ents, _ := os.ReadDir(target); len(ents) != 0 {
				t.Errorf("wrote through the link: %v", ents)
			}
			if len(notes) != 1 || !strings.Contains(notes[0], c.at+" is a link") {
				t.Errorf("notes = %q", notes)
			}
			if !check(t, e, st) {
				t.Error("Check = false: the linked place keeps asking")
			}
		})
	}
}

// A skill's name is checked before anything is written: it's a folder
// yardo makes as root.
func TestSkillNames(t *testing.T) {
	for _, n := range []string{"../x", "x/y", "X", "-a", "a-", "a--b", "", strings.Repeat("a", 65)} {
		e := testEnv(t, &fakeExec{})
		e.Files = fstest.MapFS{"skills/" + n + "/SKILL.md": {Data: []byte("x")}}
		st := step(t, catalog.Step{Type: "go:skills", Skills: []string{n}})
		if err := st.Apply(t.Context(), e); err == nil || !strings.Contains(err.Error(), "not a valid name") {
			t.Errorf("%q: %v", n, err)
		}
		if _, err := os.Stat(e.Path("~/.agents")); err == nil {
			t.Errorf("%q: wrote something", n)
		}
	}
	for _, n := range []string{"tdd", "git-commits", "a1", strings.Repeat("a", 64)} {
		if !ValidSkillName(n) {
			t.Errorf("%q refused", n)
		}
	}
}
