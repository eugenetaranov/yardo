package main

import (
	"bytes"
	"os/user"
	"slices"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// answer sets the question for choice id (the prompt is "prompt").
func answer(t *testing.T, cat *catalog.Catalog, qs []*ui.Question, id string, picked ...string) {
	t.Helper()
	if id == "prompt" {
		qs[0].Picked = picked
		return
	}
	for i, ch := range cat.Choices {
		if ch.ID == id {
			qs[i+1].Picked = picked
			return
		}
	}
	t.Fatalf("no choice %q", id)
}

// A first run: each choice's on, and the base, all added.
func TestChoicesFirstRun(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &answers.Answers{}
	qs := choiceQuestions(cat, a)
	if qs[0].Picked[0] != "p10k" {
		t.Errorf("prompt starts on %v", qs[0].Picked)
	}
	answer(t, cat, qs, "prompt", "starship")
	r := choiceChange(cat, a, qs)
	if r.prompt != "starship" {
		t.Errorf("prompt %q", r.prompt)
	}
	for _, id := range append(cat.Base(), "docker", "mcpjungle", "claude", "github") {
		if !slices.Contains(r.adds, id) {
			t.Errorf("first run doesn't add %s: %v", id, r.adds)
		}
	}
	// The languages too: each is ticked on its own.
	for _, id := range []string{"python", "node", "go", "k8s", "terraform", "aws", "codex", "gitlab", "tailscale", "java", "podman"} {
		if slices.Contains(r.adds, id) {
			t.Errorf("first run adds %s, off by default", id)
		}
	}
}

// A machine set up before the base: what's new in it is added, once.
func TestBaseBackfill(t *testing.T) {
	state := t.TempDir()
	me, _ := user.Current()
	if err := answers.Write(state, &answers.Answers{User: me.Username, Picks: []string{"zsh"}}); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if err := cli([]string{"--yes", "--dry-run", "--state", state}, &out, &errb); err != nil {
		t.Fatalf("run: %v\n%s", err, errb.String())
	}
	if !strings.Contains(errb.String(), "new in every machine's base, installing: ") || !strings.Contains(errb.String(), "Neovim") {
		t.Errorf("no note:\n%s", errb.String())
	}
	if !strings.Contains(out.String(), "neovim") {
		t.Errorf("the plan has no neovim:\n%s", out.String())
	}
}

// v0.9.0's base had NFS + SMB clients: a machine it came to stops
// managing them, and installs the base it picked (v0.9.0 didn't), nothing
// it uninstalled.
func TestBaseChangeFrom1(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &answers.Answers{Base: 1, Picks: []string{"zsh", "netfs", "mosh"}}
	var w bytes.Buffer
	if add := baseChange(cat, a, &w); !slices.Equal(add, []string{"zsh", "mosh"}) || !slices.Equal(a.Picks, []string{"zsh", "mosh"}) || a.Base != catalog.BaseRev {
		t.Errorf("add %v, picks %v, base %d", add, a.Picks, a.Base)
	}
	if !strings.Contains(w.String(), "NFS + SMB clients is no longer") {
		t.Errorf("note: %q", w.String())
	}
	// Before any base: all of it but NFS, to install.
	a = &answers.Answers{Picks: []string{"zsh"}}
	if add := baseChange(cat, a, &w); slices.Contains(add, "netfs") || !slices.Contains(add, "mosh") || slices.Contains(add, "zsh") {
		t.Errorf("add %v", add)
	}
}
