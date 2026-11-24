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
