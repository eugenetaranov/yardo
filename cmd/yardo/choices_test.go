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

// A first run: each choice's on, and the base, all added; nothing to
// uninstall.
func TestChoicesFirstRun(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &answers.Answers{}
	qs := choiceQuestions(cat, a, true)
	if qs[0].Picked[0] != "p10k" {
		t.Errorf("prompt starts on %v", qs[0].Picked)
	}
	answer(t, cat, qs, "prompt", "starship")
	r := choiceChange(cat, a, true, qs)
	if r.prompt != "starship" || len(r.forget) != 0 {
		t.Errorf("prompt %q, forget %v", r.prompt, r.forget)
	}
	for _, id := range append(cat.Base(), "python", "node", "go", "docker", "mcpjungle", "claude", "github") {
		if !slices.Contains(r.adds, id) {
			t.Errorf("first run doesn't add %s: %v", id, r.adds)
		}
	}
	for _, id := range []string{"k8s", "terraform", "aws", "codex", "gitlab", "tailscale", "java", "podman"} {
		if slices.Contains(r.adds, id) {
			t.Errorf("first run adds %s, off by default", id)
		}
	}
}

// Later: the answers start from the picks, and only the change is made:
// turned off is uninstalled, turned on added; the base isn't touched.
func TestChoicesChange(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &answers.Answers{Prompt: "starship", Picks: []string{"zsh", "k8s", "k8stools", "aws", "tailscale-install", "tailscale", "claude"}}
	qs := choiceQuestions(cat, a, false)
	if qs[0].Picked[0] != "starship" {
		t.Errorf("prompt starts on %v", qs[0].Picked)
	}
	for id, want := range map[string]string{"k8s": "on", "cloud": "aws", "vpn": "tailscale", "agents": "claude", "lang": "", "githost": ""} {
		if got := strings.Join(held(*choiceByID(cat, id), a.Picks), " "); got != want {
			t.Errorf("%s holds %q, want %q", id, got, want)
		}
	}
	// Unchanged, nothing changes, though a toggle's other items aren't
	// installed (k8s on, with k8s alone).
	a.Picks = slices.DeleteFunc(a.Picks, func(id string) bool { return id == "k8stools" })
	if r := choiceChange(cat, a, false, choiceQuestions(cat, a, false)); len(r.adds)+len(r.forget) != 0 {
		t.Errorf("unchanged: adds %v forget %v", r.adds, r.forget)
	}
	a.Picks = append(a.Picks, "k8stools")
	answer(t, cat, qs, "k8s")
	answer(t, cat, qs, "cloud", "aws", "doctl")
	answer(t, cat, qs, "vpn", "netbird")
	r := choiceChange(cat, a, false, qs)
	if !slices.Equal(r.adds, []string{"doctl", "netbird-install", "netbird"}) {
		t.Errorf("adds %v", r.adds)
	}
	if !slices.Equal(r.forget, []string{"k8s", "k8stools", "tailscale-install", "tailscale"}) {
		t.Errorf("forget %v", r.forget)
	}
}

func choiceByID(cat *catalog.Catalog, id string) *catalog.Choice {
	for i := range cat.Choices {
		if cat.Choices[i].ID == id {
			return &cat.Choices[i]
		}
	}
	return nil
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
