package steps

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// Git 2.54+: the hook script and its config-based hook; older git: no
// hook, and a note.
func TestGitAttrib(t *testing.T) {
	script, err := os.ReadFile("../assets/conf/git-no-agent-trailer")
	if err != nil {
		t.Fatal(err)
	}
	version := "git version 2.54.0\n"
	cfg := map[string]string{}
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		switch {
		case c.Name == "git" && c.Args[0] == "--version":
			return []byte(version), nil
		case c.Name == "git" && c.Args[2] == "--get":
			if v, ok := cfg[c.Args[3]]; ok {
				return []byte(v + "\n"), nil
			}
			return nil, exec.ErrNotFound
		case c.Name == "git" && c.Args[2] == "--replace-all":
			cfg[c.Args[3]] = c.Args[4]
		}
		return nil, nil
	}}
	e := testEnv(t, x)
	e.Files = fstest.MapFS{"git-no-agent-trailer": {Data: script}}
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	st := step(t, catalog.Step{Type: "go:gitattrib"})
	if check(t, e, st) {
		t.Fatal("Check = true before Apply")
	}
	apply(t, e, st)
	if got := read(t, e.Path(attribFile)); got != string(script) {
		t.Errorf("hook script not written")
	}
	if fi, _ := os.Stat(e.Path(attribFile)); fi.Mode().Perm() != 0o755 {
		t.Errorf("mode %v", fi.Mode())
	}
	if cfg["hook.yardo-no-agent-trailer.event"] != "commit-msg" || cfg["hook.yardo-no-agent-trailer.command"] != "/home/ada/.config/yardo/git/no-agent-trailer" {
		t.Errorf("config = %v", cfg)
	}
	if strings.Contains(strings.Join(x.commands(), "\n"), "core.hooksPath") {
		t.Error("set core.hooksPath")
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
	// Older git off Ubuntu: nothing set, a note says why.
	version = "git version 2.43.0\n"
	e2 := testEnv(t, x)
	e2.Files, e2.Note, e2.Distro = e.Files, e.Note, "debian"
	cfg = map[string]string{}
	if check(t, e2, st) {
		t.Error("old git: Check = true before noting")
	}
	apply(t, e2, st)
	if len(cfg) != 0 || len(notes) != 1 || !strings.Contains(notes[0], "older than 2.54") {
		t.Errorf("old git: cfg %v, notes %v", cfg, notes)
	}
	if !check(t, e2, st) {
		t.Error("old git: Check = false once noted")
	}
	if strings.Contains(strings.Join(x.commands(), "\n"), "apt-get") {
		t.Error("debian: apt-get ran")
	}

	// Older git on Ubuntu: the git-core PPA's git, then the hook.
	version = "git version 2.43.0\n"
	cfg = map[string]string{}
	x3 := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		switch {
		case c.Name == "gpg":
			return []byte("fpr:::::::::" + gitPPAFpr + ":\n"), nil
		case c.Name == "apt-get" && slices.Contains(c.Args, "--only-upgrade"):
			version = "git version 2.56.0\n"
		}
		return x.reply(c)
	}}
	e3 := testEnv(t, x3)
	e3.Files, e3.Note = e.Files, e.Note
	e3.Download = func(context.Context, string) ([]byte, error) { return []byte("KEY"), nil }
	apply(t, e3, st)
	if got := read(t, e3.Path("/etc/apt/sources.list.d/git-core.sources")); !strings.Contains(got, "git-core/ppa/ubuntu\nSuites: noble\n") {
		t.Errorf("ubuntu: sources = %q", got)
	}
	if cfg["hook.yardo-no-agent-trailer.event"] != "commit-msg" || len(notes) != 1 {
		t.Errorf("ubuntu: cfg %v, notes %v", cfg, notes)
	}
	if !check(t, e3, st) {
		t.Error("ubuntu: Check = false after Apply")
	}
}

// The script drops only an agent's lines, and trailing blank lines they
// leave; a message without them is untouched.
func TestNoAgentTrailer(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	script, _ := filepath.Abs("../assets/conf/git-no-agent-trailer")
	for in, want := range map[string]string{
		"feat: x\n\nwhy\n\nCo-authored-by: Ada <ada@example.com>\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\nCo-authored-by: Codex <noreply@openai.com>\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)\n": "feat: x\n\nwhy\n\nCo-authored-by: Ada <ada@example.com>\n",
		"fix: y\n\nCo-authored-by: Cursor <cursoragent@cursor.com>\n":                         "fix: y\n",
		"docs: z\n\nGenerated with [Codex](https://openai.com/codex/).\n":                       "docs: z\n",
		"chore: claude is a name in the text\n\nCo-authored-by: Claudette <c@example.com>\n": "chore: claude is a name in the text\n\nCo-authored-by: Claudette <c@example.com>\n",
	} {
		f := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
		if err := os.WriteFile(f, []byte(in), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("sh", script, f).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		got, _ := os.ReadFile(f)
		if string(got) != want {
			t.Errorf("%q →\n%q\nwant\n%q", in, got, want)
		}
	}
}
