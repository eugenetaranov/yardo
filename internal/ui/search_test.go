package ui

import (
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// everything is the catalog, both screens.
func everything(t *testing.T) []*Entry {
	t.Helper()
	c, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return append(CatalogEntries(c, catalog.PhaseInstall), CatalogEntries(c, catalog.PhaseConfigure)...)
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

func TestSearch(t *testing.T) {
	items := everything(t)
	top := ids(Search(items, "zsh"), 5)
	// One zsh item: its parts are hidden.
	if top[0] != "zsh" || slices.ContainsFunc(top, func(id string) bool { return id == "zshcfg" || id == "p10k" }) {
		t.Errorf("zsh top 5 = %v", top)
	}
	cases := []struct {
		q    string
		n    int
		want string
	}{
		{"k9s", 1, "k8s"}, {"starship", 1, "zsh"}, {"lzgt", 5, "lazygit"}, {"act", 3, "github"},
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
	// A longer query isn't matched as a subsequence: pulumi isn't
	// p…u…l…u…m…i in the admin tools' label.
	for _, q := range []string{"pulumi", "pytest"} {
		if got := ids(Search(items, q), 5); len(got) != 0 {
			t.Errorf("%q: want no match (not in the catalog), got %v", q, got)
		}
	}
}

func TestValidExtra(t *testing.T) {
	for q, want := range map[string]bool{
		"pulumi": true, "pipx:pytest": true, "github:tackhq/tack@1.1.4": true, "node@22": true,
		"-rf": false, "a b": false, "x;rm": false, "": false, "$(id)": false,
	} {
		if ValidExtra(q) != want {
			t.Errorf("ValidExtra(%q) = %v", q, !want)
		}
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
