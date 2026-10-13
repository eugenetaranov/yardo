package ui

import (
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/eugenetaranov/yardo/internal/assets"
	"github.com/eugenetaranov/yardo/internal/catalog"
)

// everything is the picker's search set: the catalog and mise's registry.
func everything(t *testing.T) []*Entry {
	t.Helper()
	c, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return Dedupe(append(CatalogEntries(c), LoadMise(string(assets.MiseRegistry()))...))
}

func ids(es []*Entry, n int) []string {
	var out []string
	for i, e := range es {
		if i == n {
			break
		}
		out = append(out, e.ID)
	}
	return out
}

func has(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// The cases of picker_test.py, so the Go ranking matches the bash picker's.
func TestSearch(t *testing.T) {
	items := everything(t)
	top := ids(Search(items, "zsh"), 5)
	// One zsh item: its parts are hidden.
	if top[0] != "zsh" || slices.ContainsFunc(top, func(id string) bool { return id == "zshcfg" || id == "p10k" }) {
		t.Errorf("zsh top 5 = %v", top)
	}
	res := Search(items, "zsh")
	firstMise := -1
	for i, e := range res {
		if e.Tab == MiseTab {
			firstMise = i
			break
		}
	}
	if firstMise < 0 || !has(ids(res, firstMise), "zsh") {
		t.Errorf("catalog items should come before mise tools: %v", ids(res, 10))
	}
	cases := []struct {
		q    string
		n    int
		want string
	}{
		{"k9s", 1, "k8s"}, {"find", 40, "mise:fd"}, {"starship", 1, "zsh"}, {"starship", 3, "mise:starship"},
		{"strshp", 3, "mise:starship"}, {"lzgt", 5, "lazygit"},
	}
	for _, c := range cases {
		if !has(ids(Search(items, c.q), c.n), c.want) {
			t.Errorf("%q: %s not in the top %d: %v", c.q, c.want, c.n, ids(Search(items, c.q), c.n))
		}
	}
	if n := len(Search(items, "starsh")); n >= 10 {
		t.Errorf("descriptions aren't fuzzy-matched: starsh gave %d results", n)
	}
	// Subsequence matches count only when nothing has the query in it:
	// "ssh" lists what mentions SSH, the git host items near the top, not
	// every label with s…s…h in it ("zsh config").
	ssh := Search(items, "ssh")
	for _, want := range []string{"github", "gitlab", "bitbucket"} {
		if !has(ids(ssh, 5), want) {
			t.Errorf("ssh: %s not in the top 5: %v", want, ids(ssh, 5))
		}
	}
	for _, e := range ssh {
		if !strict(e, "ssh") {
			t.Errorf("ssh: %s matches only as a subsequence", e.ID)
		}
	}
	if n := len(Search(items, "qqqqzzzz")); n != 0 {
		t.Errorf("no match expected, got %d", n)
	}
}

func TestNarrow(t *testing.T) {
	if got := Narrow(":cherry_blossom: A fuzzy finder 🚀 for おはよう you"); got != "A fuzzy finder for you" {
		t.Errorf("Narrow = %q", got)
	}
	if got := Narrow("Kubernetes — ⎈ helm…"); got != "Kubernetes — ⎈ helm…" {
		t.Errorf("Narrow = %q", got)
	}
}

func TestDedupe(t *testing.T) {
	names := map[string]bool{}
	for _, e := range everything(t) {
		names[e.ID] = true
	}
	for _, covered := range []string{"terraform", "k9s", "kubectl", "glances", "helmfile", "aider-chat", "neovim"} {
		if names["mise:"+covered] {
			t.Errorf("mise:%s should be hidden (a catalog item installs it)", covered)
		}
	}
	if !names["mise:starship"] {
		t.Error("mise:starship missing")
	}
}

func TestMiseTools(t *testing.T) {
	cases := map[string][]string{
		"mise use -g uv pipx:aider-chat[uvx_args=--python=3.12] aqua:block/goose": {"aider-chat", "goose", "uv"},
		"apt prometheus-node-exporter · mise pipx:glances":                        {"glances"},
		"apt install mise": nil,
	}
	for src, want := range cases {
		var got []string
		for t := range MiseTools(src) {
			got = append(got, t)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("MiseTools(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestLoadMise(t *testing.T) {
	got := LoadMise("op\t1password\tPassword manager. Second\tsentence.\n\tskipped\tx\nbare\t\t\n")
	if len(got) != 2 || got[0].ID != "mise:op" || got[0].Label != "op — Password manager" || !reflect.DeepEqual(got[0].Names, []string{"op", "1password"}) || got[1].Label != "bare" {
		t.Errorf("LoadMise = %+v %+v", got[0], got)
	}
}
