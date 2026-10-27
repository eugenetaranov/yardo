package plan

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

func cat(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func resolve(t *testing.T, picks ...string) []string {
	t.Helper()
	items, err := Resolve(cat(t), picks, false)
	if err != nil {
		t.Fatal(err)
	}
	return IDs(items)
}

func TestNeedsBeforeAndCatalogOrder(t *testing.T) {
	got := resolve(t, "zsh")
	for _, pair := range [][2]string{{"zshbin", "zshcfg"}, {"zshcfg", "prompt"}, {"prompt", "zshrc"}, {"zshcfg", "fzftab"}, {"cli", "fzftab"}, {"fzftab", "zsh"}, {"mise", "prompt"}} {
		if i, j := index(got, pair[0]), index(got, pair[1]); i < 0 || j < 0 || i > j {
			t.Errorf("%s should come before %s: %v", pair[0], pair[1], got)
		}
	}
}

// Every plan gets the baseline; an empty one stays empty.
func TestAlwaysItems(t *testing.T) {
	got := resolve(t, "lazygit")
	for _, id := range []string{"essentials", "autoupdates", "sshharden", "fail2ban", "devlimits", "zram", "journald", "tzlocale"} {
		if !contains(got, id) {
			t.Errorf("%s missing: %v", id, got)
		}
	}
	if items, err := Resolve(cat(t), nil, false); err != nil || len(items) != 0 {
		t.Errorf("no picks: %v, %v", IDs(items), err)
	}
}

func TestMiseFirst(t *testing.T) {
	got := resolve(t, "terraform", "lazygit")
	if got[0] != "mise" || !contains(got, "terraform") {
		t.Errorf("plan = %v, want mise first", got)
	}
	items, _ := Resolve(cat(t), []string{"zram"}, true)
	if IDs(items)[0] != "mise" {
		t.Errorf("extra mise tools should pull in mise: %v", IDs(items))
	}
}

// npmrc (Logins & keys) needs node (Languages): it goes after.
func TestCrossTabNeeds(t *testing.T) {
	got := resolve(t, "npmrc")
	if index(got, "node") > index(got, "npmrc") || index(got, "mise") > index(got, "node") {
		t.Errorf("plan = %v, want mise, node, npmrc in that order", got)
	}
}

// Installing comes first: every install item before every configure item,
// and dotfiles last of all.
func TestInstallThenConfigure(t *testing.T) {
	items, err := Resolve(cat(t), []string{"dotfiles", "registries", "github", "tailscale", "npmrc", "docker", "sops", "mcpjungle", "agentcfg", "claude"}, false)
	if err != nil {
		t.Fatal(err)
	}
	ids := IDs(items)
	n := Configuring(items)
	for i, it := range items {
		if (i < n) == (it.Configure() || it.Last) {
			t.Errorf("%s at %d, configuring starts at %d: %v", it.ID, i, n, ids)
		}
	}
	if ids[len(ids)-1] != "dotfiles" {
		t.Errorf("dotfiles should be last: %v", ids)
	}
	if index(ids, "gitkey") < index(ids, "docker") {
		t.Errorf("gitkey (configure) should come after the installs: %v", ids)
	}
}

// The configure half of a split item kept the old id: an old pick still
// installs both halves.
func TestSplitItemsPullInTheirInstall(t *testing.T) {
	for cfg, inst := range map[string]string{"tailscale": "tailscale-install", "sops": "sops-install", "mcpjungle": "mcpjungle-install"} {
		got := resolve(t, cfg)
		if i, j := index(got, inst), index(got, cfg); i < 0 || i > j {
			t.Errorf("%s should pull in %s before it: %v", cfg, inst, got)
		}
	}
}

func TestUnknownPick(t *testing.T) {
	if _, err := Resolve(cat(t), []string{"nope"}, false); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Errorf("err = %v", err)
	}
}

func contains(l []string, s string) bool { return index(l, s) >= 0 }

func index(l []string, s string) int {
	for i, x := range l {
		if x == s {
			return i
		}
	}
	return -1
}

// registries goes after the GitHub login when both are installed (ghcr.io
// can use gh's token), and doesn't pull it in.
func TestAfterOrdersWithoutPulling(t *testing.T) {
	c, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := Resolve(c, []string{"registries", "github"}, false)
	if err != nil {
		t.Fatal(err)
	}
	ids := " " + strings.Join(IDs(items), " ") + " "
	if strings.Index(ids, " github ") > strings.Index(ids, " registries ") {
		t.Errorf("order %s: want github before registries", ids)
	}
	items, _ = Resolve(c, []string{"registries"}, false)
	if strings.Contains(" "+strings.Join(IDs(items), " ")+" ", " github ") {
		t.Errorf("registries pulled in github: %v", IDs(items))
	}
}

// dotfiles goes after everything, catalog.d's items included.
func TestDotfilesLast(t *testing.T) {
	local := fstest.MapFS{"team.toml": {Data: []byte("[[item]]\nid = \"teamtools\"\ntab = \"dev\"\nlabel = \"Team\"\n[[item.step]]\ntype = \"apt\"\npackages = [\"jq\"]\n")}}
	c, err := catalog.Load(local, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := Resolve(c, []string{"dotfiles", "teamtools", "zsh", "gitid"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if ids := IDs(items); ids[len(ids)-1] != "dotfiles" {
		t.Errorf("order = %v", ids)
	}
}

// Extra mise tools bring mise and its build tools.
func TestExtraMiseBringsBuildTools(t *testing.T) {
	c, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := Resolve(c, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(IDs(items), " "); !strings.Contains(got, MiseID) || !strings.Contains(got, MiseBuildID) {
		t.Errorf("items = %s", got)
	}
	items, _ = Resolve(c, []string{"terraform"}, false)
	if strings.Contains(strings.Join(IDs(items), " "), MiseBuildID) {
		t.Error("build tools without extra mise tools")
	}
}
