package main

import (
	"io"
	"slices"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// The configure screen offers what the install picks (now, before, or
// already on PATH) can use: registry logins only with Docker.
func TestConfigurable(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	onPath := map[string]bool{}
	k := &asker{cat: cat, a: &answers.Answers{}, out: io.Discard, installed: func(tool string) bool { return onPath[tool] }}
	shown := func(installs ...string) []string {
		conf := ui.CatalogEntries(cat, catalog.PhaseConfigure)
		for _, e := range conf {
			e.On = true
		}
		var ids []string
		for _, e := range k.configurable(conf, installs) {
			ids = append(ids, e.ID)
		}
		for _, e := range conf {
			if e.On != slices.Contains(ids, e.ID) {
				t.Errorf("%s: ticked %v, shown %v", e.ID, e.On, slices.Contains(ids, e.ID))
			}
		}
		return ids
	}
	got := shown()
	for _, id := range []string{"gitid", "github", "agentkeys", "dotfiles"} {
		if !slices.Contains(got, id) {
			t.Errorf("%s should always be offered: %v", id, got)
		}
	}
	for _, id := range []string{"registries", "npmrc", "tailscale", "sops", "mcpjungle"} {
		if slices.Contains(got, id) {
			t.Errorf("%s offered without its install: %v", id, got)
		}
	}
	if got := shown("docker", "node"); !slices.Contains(got, "registries") || !slices.Contains(got, "npmrc") {
		t.Errorf("picked docker and node: %v", got)
	}
	k.a.Picks = []string{"tailscale-install"} // an earlier run
	onPath["sops"] = true                      // installed by hand
	if got := shown(); !slices.Contains(got, "tailscale") || !slices.Contains(got, "sops") || slices.Contains(got, "registries") {
		t.Errorf("saved tailscale, sops on PATH: %v", got)
	}
}
