package main

import (
	"context"
	"io"
	"slices"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// What the board queued becomes the run's choices: picks added and
// removed (removed ones dropped), extra mise tools, access settings.
func TestFromBoard(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &answers.Answers{User: "ada", Picks: []string{"zsh", "tmux"}, MiseExtra: []string{"starship"}, TZ: "UTC"}
	k := &asker{cat: cat, a: a, out: io.Discard, installed: func(string) bool { return false }}
	c, err := k.fromBoard(context.Background(), ui.BoardResult{Kind: ui.BoardApply,
		Adds:     []string{"terraform", "mise:pulumi"},
		Removes:  []string{"tmux", "mise:starship"},
		Settings: map[string]string{"mcp": "vpn", "ports": "all", "api": "lan", "accent": "pistachio"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.picks, []string{"zsh", "terraform"}) || !slices.Equal(c.dropped, []string{"tmux"}) || !slices.Equal(c.miseExtra, []string{"pulumi"}) {
		t.Errorf("picks %v dropped %v extra %v", c.picks, c.dropped, c.miseExtra)
	}
	if a.MCPListen != "vpn" || !k.listenSet || a.DockerPorts != "" || a.DockerAPI != "lan" || a.Accent != "pistachio" {
		t.Errorf("settings: mcp %q ports %q api %q", a.MCPListen, a.DockerPorts, a.DockerAPI)
	}
	if len(c.items) == 0 || !slices.ContainsFunc(c.items, func(it *catalog.Item) bool { return it.ID == "terraform" }) {
		t.Errorf("items = %v", c.items)
	}
}
