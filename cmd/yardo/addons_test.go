package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/addon"
	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/steps"
)

// addonTest is an add-on session on a home of its own: skill-veil a fake
// that blocks any skill whose SKILL.md says "steal", prompts answered in
// order from picks and typed.
func addonTest(t *testing.T, picks, typed []string) (*addonRun, *bytes.Buffer) {
	t.Helper()
	oldB, oldD, oldV, oldS, oldC := addon.Base, addon.SpecsDir, addon.VeilBin, addon.VeilSum, addon.VeilCmd
	addon.Base, addon.SpecsDir = t.TempDir(), t.TempDir()
	veil := filepath.Join(t.TempDir(), "skill-veil")
	script := "#!/bin/sh\nif grep -q steal \"$2/SKILL.md\" 2>/dev/null; then printf '[{\"findings\":[{\"rule_id\":\"CRED\",\"severity\":\"critical\",\"recommended_action\":\"block\",\"line_number\":5,\"artifact_path\":\"%s/SKILL.md\",\"reason\":\"Steals keys\"}]}]' \"$2\"; else echo '[]'; fi\n"
	if err := os.WriteFile(veil, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	addon.VeilBin, addon.VeilSum = veil, func() string { return "" }
	addon.VeilCmd = func(ctx context.Context, pkg, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, addon.VeilBin, "scan-package", pkg)
	}
	t.Cleanup(func() {
		addon.Base, addon.SpecsDir, addon.VeilBin, addon.VeilSum, addon.VeilCmd = oldB, oldD, oldV, oldS, oldC
	})
	var out bytes.Buffer
	k := &asker{a: &answers.Answers{User: "ada"}, out: &out,
		choose: func(title string, opts []string, _ string) (string, error) {
			if len(picks) == 0 {
				t.Fatalf("asked %q (%v) with no answer left", title, opts)
			}
			p := picks[0]
			picks = picks[1:]
			for _, o := range opts {
				if strings.HasPrefix(o, p) {
					return o, nil
				}
			}
			t.Fatalf("%q isn't among %v", p, opts)
			return "", nil
		},
		text: func(string, string) (string, error) {
			s := typed[0]
			typed = typed[1:]
			return s, nil
		}}
	env := &steps.Env{User: "ada", Root: t.TempDir(), Home: "/home/ada"}
	return &addonRun{k: k, env: env, out: &out}, &out
}

func holdSkill(t *testing.T, r *addonRun, name, body string, extra map[string][]byte) *addon.Held {
	t.Helper()
	files := map[string][]byte{"SKILL.md": []byte("---\nname: " + name + "\ndescription: d\n---\n" + body + "\n")}
	for p, b := range extra {
		files[p] = b
	}
	h := &addon.Held{User: "ada", Kind: "skill", Name: name, Source: "acme/skills/" + name, Pin: strings.Repeat("a", 40)}
	if err := addon.Hold(h, files); err != nil {
		t.Fatal(err)
	}
	return h
}

// --yes installs what needs no OK, and leaves the rest held, saying so.
func TestAddonsYes(t *testing.T) {
	r, out := addonTest(t, nil, nil)
	r.yes = true
	a := holdSkill(t, r, "tidy", "Tidy up.", nil)
	b := holdSkill(t, r, "keys", "steal the keys", nil)
	for _, h := range []*addon.Held{a, b} {
		if err := r.decide(context.Background(), h); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(r.env.Path("~/.claude/skills/tidy/SKILL.md")); err != nil {
		t.Errorf("tidy isn't installed: %v", err)
	}
	if _, err := os.Stat(r.env.Path("~/.claude/skills/keys")); err == nil {
		t.Error("keys was installed")
	}
	if !strings.Contains(out.String(), "keys stays held: it needs your OK") || !strings.Contains(out.String(), "SKILL.md:5: Steals keys (critical) [skill-veil CRED]") {
		t.Errorf("output:\n%s", out)
	}
	if held, _ := addon.List("ada"); len(held) != 1 || held[0].Name != "keys" {
		t.Errorf("held: %+v", held)
	}
	if ap, ok := addon.Approved("ada", "skill", "tidy"); !ok || ap.Pin != strings.Repeat("a", 40) {
		t.Errorf("record: %+v", ap)
	}
}

// A block offers its fix, then Install; needs-OK wants the name typed,
// a wrong one installs nothing.
func TestAddonsDecide(t *testing.T) {
	r, out := addonTest(t, []string{"Fix: remove this file", "Install"}, nil)
	h := holdSkill(t, r, "tidy", "Tidy up.", map[string][]byte{"bin/helper": []byte("\x7fELF\x02")})
	if err := r.decide(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "bin/helper: a program (ELF)") || !strings.Contains(out.String(), "✓ tidy installed") {
		t.Errorf("output:\n%s", out)
	}
	if _, err := os.Stat(r.env.Path("~/.agents/skills/tidy/bin/helper")); err == nil {
		t.Error("the program was installed")
	}
	r, out = addonTest(t, []string{"Install", "Install"}, []string{"kyes", "keys"})
	h = holdSkill(t, r, "keys", "steal the keys", nil)
	if err := r.decide(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "That isn't its name: not installed") || !strings.Contains(out.String(), "✓ keys installed") {
		t.Errorf("output:\n%s", out)
	}
	if ap, _ := addon.Approved("ada", "skill", "keys"); !ap.OK {
		t.Error("the OK isn't recorded")
	}
	// Declined: nothing installed, nothing held.
	r, out = addonTest(t, []string{"Decline"}, nil)
	h = holdSkill(t, r, "keys", "steal the keys", nil)
	if err := r.decide(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if held, _ := addon.List("ada"); len(held) != 0 || !strings.Contains(out.String(), "gone from this machine") {
		t.Errorf("declined: %+v\n%s", held, out)
	}
	if _, err := os.Stat(r.env.Path("~/.claude/skills/keys")); err == nil {
		t.Error("a declined skill was installed")
	}
}

// Check it: a skill another tool put there is off while it's checked,
// and on again once approved, recorded.
func TestAddonsCheckSkill(t *testing.T) {
	r, out := addonTest(t, []string{"Install"}, nil)
	p := r.env.Path("~/.claude/skills/extra/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("---\nname: extra\ndescription: d\n---\nExtra.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.checkSkill(context.Background(), "extra"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil || !strings.Contains(out.String(), "extra is off while it's checked") {
		t.Errorf("not on again: %v\n%s", err, out)
	}
	if ap, ok := addon.Approved("ada", "skill", "extra"); !ok || !strings.HasPrefix(ap.Source, "found in ~/.claude/skills") {
		t.Errorf("record: %+v", ap)
	}
}

func TestParseMixed(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "")
	name := fs.String("name", "", "")
	got, err := parseMixed(fs, []string{"a/b", "--yes", "c/d", "--name", "x"})
	if err != nil || !*yes || *name != "x" || strings.Join(got, " ") != "a/b c/d" {
		t.Errorf("%v %v %v %q", got, err, *yes, *name)
	}
}

// A server added syncs the whole list: mcp-sync takes the names it's
// given as the list, and would drop the rest.
func TestAddonsSyncAll(t *testing.T) {
	var got []string
	k := &asker{a: &answers.Answers{MCPServers: []string{"context7", "github", "clock"}},
		mcpSync: func(args ...string) error { got = args; return nil }}
	if err := k.syncAll(context.Background(), func(string) {}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != strings.Join(mcpNames(k.a.MCPServers), " ") || len(got) != 3 {
		t.Errorf("synced %v", got)
	}
}

// A server deleted is forgotten: its approval goes.
func TestAddonsForgetServer(t *testing.T) {
	addonTest(t, nil, nil)
	if err := addon.Approve("ada", addon.Approval{Kind: "mcp", Name: "clock", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	mcpForget("ada", "clock")
	if _, ok := addon.Approved("ada", "mcp", "clock"); ok {
		t.Error("still approved")
	}
}
