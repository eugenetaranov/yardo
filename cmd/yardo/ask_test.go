package main

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/plan"
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
		Forgets:  []string{"tmux", "mise:starship"},
		Settings: map[string]string{"mcp": "vpn", "ports": "all", "accent": "pistachio"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.picks, []string{"zsh", "terraform"}) || !slices.Equal(c.dropped, []string{"tmux"}) || !slices.Equal(c.miseExtra, []string{"pulumi"}) {
		t.Errorf("picks %v dropped %v extra %v", c.picks, c.dropped, c.miseExtra)
	}
	if a.MCPListen != "vpn" || !k.listenSet || a.DockerPorts != "" || a.Accent != "pistachio" {
		t.Errorf("settings: mcp %q ports %q", a.MCPListen, a.DockerPorts)
	}
	if len(c.items) == 0 || !slices.ContainsFunc(c.items, func(it *catalog.Item) bool { return it.ID == "terraform" }) {
		t.Errorf("items = %v", c.items)
	}
}

// The review shows the answers the questions took: the git name, which
// keys were given (never the key).
func TestReviewAnswers(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &answers.Answers{User: "ada", TZ: "Europe/Berlin", Git: answers.Git{Name: "Ada Lovelace", Email: "ada@example.com"}}
	var out bytes.Buffer
	k := &asker{cat: cat, a: a, out: &out, installed: func(string) bool { return false }}
	c := k.boardChoices(ui.BoardResult{Adds: []string{"zsh", "gitid"}})
	if c.items, err = plan.Resolve(cat, c.picks, false); err != nil {
		t.Fatal(err)
	}
	c.secrets.Cred["agentkeys ANTHROPIC_API_KEY"] = "sk-ant-secret"
	k.review(c, "Install", "Quit")
	v := out.String()
	for _, want := range []string{"Git: Ada Lovelace <ada@example.com>", "Timezone: Europe/Berlin", "Prompt: Powerlevel10k", "Keys given: ANTHROPIC_API_KEY"} {
		if !strings.Contains(v, want) {
			t.Errorf("review lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "sk-ant-secret") {
		t.Errorf("the key shows:\n%s", v)
	}
}

// A first run: the choices, the questions, then the review; Install is
// ready to run, with nothing more to ask.
func TestFirstRunOrder(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &answers.Answers{User: "ada", TZ: "UTC", Git: answers.Git{Name: "Ada", Email: "ada@x"}}
	var order []string
	k := &asker{cat: cat, a: a, out: io.Discard, installed: func(string) bool { return false },
		pickChoices: func(qs []*ui.Question) (*choiceRun, error) {
			order = append(order, "choices")
			return &choiceRun{prompt: "starship", adds: []string{"go"}}, nil
		},
		reviewOf: func(c *choices, options ...string) string {
			order = append(order, "review:"+strings.Join(options, ","))
			return "Install"
		}}
	c, adds, pick, err := k.firstRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, " ") != "choices review:Install,Customize…,Quit" || pick != "Install" || !slices.Equal(adds, []string{"go"}) || a.Prompt != "starship" {
		t.Errorf("order %v pick %q adds %v prompt %q", order, pick, adds, a.Prompt)
	}
	if c == nil || len(c.items) == 0 || !slices.Contains(c.picks, "go") {
		t.Errorf("choices: %+v", c)
	}
	// Customize… after git was asked, then more on the board: git isn't
	// asked again (here, asking would fail: no terminal).
	k.asked = []string{"gitid", "go"}
	c2, err := k.fromBoard(context.Background(), ui.BoardResult{Kind: ui.BoardApply, Adds: []string{"gitid", "go", "java"}}, true)
	if err != nil || !slices.Contains(c2.picks, "java") {
		t.Errorf("customize: %v, %v", c2, err)
	}
}
