package addon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// base points the package at a fresh state directory.
func base(t *testing.T) {
	t.Helper()
	old, oldS := Base, SpecsDir
	Base, SpecsDir = t.TempDir(), t.TempDir()
	t.Cleanup(func() { Base, SpecsDir = old, oldS })
	VeilBin = filepath.Join(t.TempDir(), "none")
}

func hold(t *testing.T, h *Held, files map[string][]byte) *Held {
	t.Helper()
	if h.User == "" {
		h.User = "ada"
	}
	if err := Hold(h, files); err != nil {
		t.Fatal(err)
	}
	return h
}

// The holding area is root's alone: folders 0700, files 0600 (never
// executable, a script's #! notwithstanding); removed, nothing is left.
func TestHold(t *testing.T) {
	base(t)
	h := hold(t, &Held{Kind: "skill", Name: "a", Source: "acme/s/a", Pin: strings.Repeat("a", 40)},
		map[string][]byte{"SKILL.md": []byte("---\nname: a\n---\n"), "scripts/run.sh": []byte("#!/bin/sh\n")})
	_ = filepath.Walk(filepath.Join(Base, "quarantine"), func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if fi.IsDir() {
			want = 0o700
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s: %v, want %v", p, fi.Mode().Perm(), want)
		}
		return nil
	})
	hs, err := List("ada")
	if err != nil || len(hs) != 1 || hs[0].Name != "a" || hs[0].ID != h.ID {
		t.Fatalf("list: %v %+v", err, hs)
	}
	files, err := hs[0].Files()
	if err != nil || string(files["scripts/run.sh"]) != "#!/bin/sh\n" {
		t.Errorf("files: %v %q", err, files)
	}
	if err := hs[0].Remove(); err != nil {
		t.Fatal(err)
	}
	if ents, _ := os.ReadDir(filepath.Join(Base, "quarantine", "ada")); len(ents) != 0 {
		t.Errorf("left: %v", ents)
	}
	if err := Hold(&Held{User: "../x"}, nil); err == nil {
		t.Error("a user that's a path: held")
	}
	if err := Hold(&Held{User: "ada"}, map[string][]byte{"../x": nil}); err == nil {
		t.Error("a file outside: held")
	}
}

// Path is a held file's place on disk, to open; none for one outside.
func TestHeldPath(t *testing.T) {
	base(t)
	h := hold(t, &Held{Kind: "skill", Name: "a", Source: "acme/s/a", Pin: strings.Repeat("a", 40)},
		map[string][]byte{"SKILL.md": []byte("---\nname: a\n---\n"), "scripts/run.sh": []byte("#!/bin/sh\n")})
	p := h.Path("scripts/run.sh")
	if b, err := os.ReadFile(p); err != nil || string(b) != "#!/bin/sh\n" || !filepath.IsAbs(p) {
		t.Errorf("%s: %v %q", p, err, b)
	}
	if d := h.Path(""); d != filepath.Dir(filepath.Dir(p)) {
		t.Errorf("its folder: %s", d)
	}
	for _, f := range []string{"../meta.json", "../../b/files/x", "/etc/passwd", "a/../../x", ".", "..", "./SKILL.md"} {
		if got := h.Path(f); got != "" {
			t.Errorf("Path(%q) = %s", f, got)
		}
	}
	if got := (&Held{User: "ada", ID: ".."}).Path("SKILL.md"); got != "" {
		t.Errorf("an ID that's a path: %s", got)
	}
}

// A fix changes the held files; installing then wants a check first.
// Blocks refuse, an OK needs the name typed; installed, the approval is
// recorded and nothing is held.
func TestFixCheckInstall(t *testing.T) {
	base(t)
	ctx := context.Background()
	h := hold(t, &Held{Kind: "skill", Name: "tidy", Source: "acme/s/tidy", Pin: strings.Repeat("b", 40)},
		map[string][]byte{"SKILL.md": []byte("---\nname: tidy\ndescription: d\n---\nTidy up.\n"), "bin/helper": []byte("\x7fELF\x02")})
	if err := h.Check(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if v := h.Report.Verdict(); v != "block" || h.Report.Findings[0].Fix != "remove:bin/helper" {
		t.Fatalf("verdict %s: %+v", v, h.Report.Findings)
	}
	put := func(map[string][]byte) error { return nil }
	if err := h.Install(ctx, "ada", "", put); err == nil || !strings.Contains(err.Error(), "can't be installed as it is") {
		t.Errorf("installed with a block: %v", err)
	}
	if err := h.Fix(h.Report.Findings[0].Fix); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(ctx, "ada", "", put); !errors.Is(err, ErrChanged) {
		t.Errorf("installed before a check: %v", err)
	}
	if err := h.Check(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if v := h.Report.Verdict(); v != "note" {
		t.Fatalf("after the fix: %s %+v", v, h.Report.Findings)
	}
	// Changed while held: no install.
	if err := os.WriteFile(filepath.Join(h.dir(), "files", "SKILL.md"), []byte("---\nname: tidy\n---\nnow evil\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(ctx, "ada", "", put); !errors.Is(err, ErrChanged) {
		t.Errorf("a changed file: %v", err)
	}
	_ = h.Check(ctx, nil)
	// Checked by older rules: checked again first, the files the same.
	h.Report.Rules--
	if err := h.Install(ctx, "ada", "", put); !errors.Is(err, ErrChanged) || h.Report.Current() {
		t.Errorf("older rules: %v", err)
	}
	_ = h.Check(ctx, nil)
	var got map[string][]byte
	if err := h.Install(ctx, "ada", "", func(f map[string][]byte) error { got = f; return nil }); err != nil {
		t.Fatal(err)
	}
	if string(got["SKILL.md"]) != "---\nname: tidy\n---\nnow evil\n" || got["bin/helper"] != nil {
		t.Errorf("put %q", got)
	}
	a, ok := Approved("ada", "skill", "tidy")
	if !ok || a.Pin != strings.Repeat("b", 40) || a.Hash != h.Report.Hash || a.By != "ada" || a.Source != "acme/s/tidy" || a.Yardo == "" || len(a.Findings) == 0 {
		t.Errorf("record: %+v", a)
	}
	if hs, _ := List("ada"); len(hs) != 0 {
		t.Errorf("still held: %+v", hs)
	}
	fi, err := os.Stat(recordFile("ada"))
	if err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("record file: %v %v", fi, err)
	}
	if err := Forget("ada", "skill", "tidy"); err != nil {
		t.Fatal(err)
	}
	if _, ok := Approved("ada", "skill", "tidy"); ok {
		t.Error("forgotten, still recorded")
	}
}

// Needs OK: only the name typed installs; an MCP spec's approval is in
// the list mcp-sync reads.
func TestInstallNeedsOK(t *testing.T) {
	base(t)
	ctx := context.Background()
	spec := []byte("DESCRIPTION=\"x\"\nKIND=http\nURL=https://x.example/mcp\nskeleton() { echo hi; }\n")
	h := hold(t, &Held{Kind: "mcp", Name: "xmcp", Source: "spec", Pin: "https://x.example/mcp"}, map[string][]byte{"spec": spec})
	if err := h.Check(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if v := h.Report.Verdict(); v != "ok" {
		t.Fatalf("verdict %s: %+v", v, h.Report.Findings)
	}
	put := func(map[string][]byte) error { return nil }
	if err := h.Install(ctx, "ada", "xmc", put); !errors.Is(err, ErrNeedsOK) {
		t.Errorf("a wrong name: %v", err)
	}
	if err := h.Install(ctx, "ada", "xmcp", put); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(SpecsFile("ada"))
	if string(b) != fmt.Sprintf("xmcp %s\n", SpecHash(spec)) {
		t.Errorf("specs list: %q", b)
	}
	if a, _ := Approved("ada", "mcp", "xmcp"); !a.OK {
		t.Error("the typed OK isn't recorded")
	}
}

// Declining removes the files and the image pulled.
func TestDecline(t *testing.T) {
	base(t)
	calls := fakeDocker(t, "")
	// An image the user had (or a spec's, not pulled) stays.
	h := hold(t, &Held{Kind: "mcp", Name: "x", Image: true, Pin: "ghcr.io/acme/x@sha256:" + strings.Repeat("c", 64)}, map[string][]byte{"spec": []byte("KIND=docker\n")})
	if err := h.Decline(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := calls(); got != "" {
		t.Errorf("docker calls: %s", got)
	}
	h = hold(t, &Held{Kind: "mcp", Name: "x", Image: true, Pulled: true, Pin: "ghcr.io/acme/x@sha256:" + strings.Repeat("c", 64)}, map[string][]byte{"spec": []byte("KIND=docker\n")})
	if err := h.Decline(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hs, _ := List("ada"); len(hs) != 0 {
		t.Error("still held")
	}
	if got := calls(); !strings.Contains(got, "image rm ghcr.io/acme/x@sha256:") {
		t.Errorf("docker calls: %s", got)
	}
}

// Holding one again replaces the copy held before; others stay.
func TestHoldReplaces(t *testing.T) {
	base(t)
	for _, h := range []*Held{
		{User: "ada", Kind: "skill", Name: "tidy"},
		{User: "ada", Kind: "mcp", Name: "tidy"},
		{User: "ada", Kind: "skill", Name: "tidy", Pin: "new"},
	} {
		if err := Hold(h, map[string][]byte{"SKILL.md": []byte("x")}); err != nil {
			t.Fatal(err)
		}
	}
	held, _ := List("ada")
	if len(held) != 2 {
		t.Fatalf("held: %d", len(held))
	}
	for _, h := range held {
		if h.Kind == "skill" && h.Pin != "new" {
			t.Errorf("the old copy stayed: %+v", h)
		}
	}
}

// A report says what each layer read; Claude's and skill-veil's say why
// they didn't.
func TestCheckCoverage(t *testing.T) {
	base(t)
	h := hold(t, &Held{Kind: "skill", Name: "tidy", Source: "acme/s/tidy", Pin: strings.Repeat("b", 40)},
		map[string][]byte{"SKILL.md": []byte("---\nname: tidy\ndescription: d\n---\nTidy up.\n"), "scripts/x.mjs": []byte("x\n")})
	if err := h.Check(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	want := []string{"basic: 2 of 2 files", "skill-veil: off (not installed)", "claude: off (not run)"}
	if got := h.Report.ReadLines(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("read:\n%s", strings.Join(got, "\n"))
	}
	if h.Report.Files["scripts/x.mjs"] != "script" || !h.Report.Quiet() {
		t.Errorf("files %v, quiet %v: %+v", h.Report.Files, h.Report.Quiet(), h.Report.Findings)
	}
}

// Layers that found the same line are one finding: the worst, the others
// named in Also.
func TestMerge(t *testing.T) {
	fs := merge([]Finding{
		{Layer: "basic", Rule: "sudo", Level: "note", File: "a.sh", Line: 2},
		{Layer: "skill-veil", Rule: "R", Level: "ok", File: "a.sh", Line: 2},
		{Layer: "claude", Rule: "other", Level: "note", File: "a.sh", Line: 2},
		{Layer: "basic", Rule: "url", Level: "note", File: "a.sh", Line: 2}, // one line, one finding
		{Layer: "claude", Rule: "other", Level: "note", File: "a.sh", Line: 3},
	}, map[string]string{"a.sh": "script"})
	if len(fs) != 2 || fs[0].Layer != "skill-veil" || strings.Join(fs[0].Also, ",") != "basic,claude" || fs[0].Role != "script" || fs[0].ID == "" {
		t.Errorf("merged: %+v", fs)
	}
}
