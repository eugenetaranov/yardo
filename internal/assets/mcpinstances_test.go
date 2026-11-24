package assets

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// The bash side of another instance of a server (grafana:prod): its keys
// asked under its own name, its own env from the spec's skeleton, and a
// spec link to the catalog's so mcp-sync knows what it is.
func TestMCPInstancesBash(t *testing.T) {
	home, cat := t.TempDir(), t.TempDir()
	putFile(t, filepath.Join(cat, "grafana.spec"),
		[]byte("DESCRIPTION=Grafana\nskeleton() { echo '# Grafana'; echo 'GRAFANA_URL='; echo 'GRAFANA_API_KEY='; }\n"), 0o644)
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("spec_file() { [ -f \"%s/$1.spec\" ] && echo \"%s", cat, cat) + `/$1.spec"; }
home_of() { echo "$H"; }
header() { :; }
faint() { :; }
is_secret() { case "$1" in *KEY*) return 0 ;; esac; return 1; }
# The answers: the value typed is the header's last word, upper-cased.
gum() { local h=; while [ $# -gt 0 ]; do [ "$1" = --header ] && h=$2; shift; done; echo "${h##* }" | tr a-z A-Z; }
install() { local a=(); while [ $# -gt 0 ]; do case "$1" in -d) ;; -m|-o|-g) shift ;; *) a+=("$1") ;; esac; shift; done; mkdir -p "${a[@]}"; }
declare -A MCP_KEYS=()
CATALOG=` + cat + ` A_USER=` + u.Username + `
A_MCP_SERVERS="grafana grafana:prod" RUN_MCP_SERVERS="grafana:prod"
ask_mcp_keys
mcp_dirs "$H/.mcp" ` + g.Name + `
echo "names: $(mcp_names)"`
	out := credShell(t, home, body, "spec_read", "spec_skeleton", "spec_field", "mcp_spec", "mcp_name", "mcp_names", "ask_mcp_keys", "mcp_dirs", "mcp_from_logins", "login_fill", "tomlget")
	if !strings.Contains(out, "names: grafana grafana-prod\n") {
		t.Errorf("out = %q", out)
	}
	if got := readFile(t, filepath.Join(home, ".mcp/grafana-prod/env")); got != "# Grafana\nGRAFANA_URL=GRAFANA_URL\nGRAFANA_API_KEY=GRAFANA_API_KEY\n" {
		t.Errorf("instance env = %q", got)
	}
	if got := readFile(t, filepath.Join(home, ".mcp/grafana/env")); strings.Contains(got, "=GRAFANA") {
		t.Errorf("the instance's keys went to grafana's env: %q", got)
	}
	if got, err := os.Readlink(filepath.Join(home, ".mcp/grafana-prod/spec")); err != nil || got != cat+"/grafana.spec" {
		t.Errorf("spec link = %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".mcp/grafana/spec")); err == nil {
		t.Error("the catalog's own grafana got a spec link")
	}
}

// Servers take the logins already made: gh's token for GitHub, jiractl's
// server, user and token for jira and confluence. Empty keys are filled and
// marked, marked ones kept current, the rest left alone.
func TestMCPFromLoginsBash(t *testing.T) {
	home := t.TempDir()
	mcp := filepath.Join(home, ".mcp")
	envs := map[string]string{
		"github":     "# GitHub\nGITHUB_PERSONAL_ACCESS_TOKEN=\n",
		"github-ghe": "GITHUB_PERSONAL_ACCESS_TOKEN=\nGITHUB_HOST=https://ghe.example\n",
		"github-own": "GITHUB_PERSONAL_ACCESS_TOKEN=github_pat_mine\n",
		"github-old": "GITHUB_PERSONAL_ACCESS_TOKEN=gho_old\n# yardo fills GITHUB_PERSONAL_ACCESS_TOKEN from gh\n",
		"jira":       "JIRA_URL=\nJIRA_USERNAME=\nJIRA_API_TOKEN=\n",
		"jira-work":  "JIRA_URL=\nJIRA_USERNAME=\nJIRA_API_TOKEN=\n",
		"confluence": "CONFLUENCE_URL=\nCONFLUENCE_USERNAME=\nCONFLUENCE_API_TOKEN=\n",
	}
	for n, e := range envs {
		mkdirAll(t, filepath.Join(mcp, n), 0o700)
		putFile(t, filepath.Join(mcp, n, "env"), []byte(e), 0o600)
	}
	putFile(t, filepath.Join(home, ".jiractl.toml"), []byte("server = \"https://acme.atlassian.net/\"\nproject = \"OPS\"\n\n[issue_defaults]\nserver = \"no\"\n"), 0o600)
	mkdirAll(t, filepath.Join(home, ".config/jiractl"), 0o700)
	putFile(t, filepath.Join(home, ".config/jiractl/credentials"), []byte("username = 'ada@acme.com'\ntoken = \"atl-1\"\n"), 0o600)
	body := `gh() { [ "$1 $2" = "auth token" ] && echo gho_new; }
A_MCP_SERVERS="github github:ghe github:own github:old jira jira:work confluence"
GH_IN=1
echo "changed: $(mcp_from_logins "$H/.mcp" | tr '\n' ' ')"
echo "again: $(mcp_from_logins "$H/.mcp" | tr '\n' ' ')"`
	out := credShell(t, home, body, "mcp_name", "mcp_from_logins", "login_fill", "tomlget")
	if want := "changed: github github-old jira confluence \nagain: \n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	for n, want := range map[string]string{
		"github":     "# GitHub\nGITHUB_PERSONAL_ACCESS_TOKEN=gho_new\n# yardo fills GITHUB_PERSONAL_ACCESS_TOKEN from gh\n",
		"github-ghe": envs["github-ghe"],
		"github-own": envs["github-own"],
		"github-old": "GITHUB_PERSONAL_ACCESS_TOKEN=gho_new\n# yardo fills GITHUB_PERSONAL_ACCESS_TOKEN from gh\n",
		"jira": "JIRA_URL=https://acme.atlassian.net\nJIRA_USERNAME=ada@acme.com\nJIRA_API_TOKEN=atl-1\n" +
			"# yardo fills JIRA_URL from jiractl\n# yardo fills JIRA_USERNAME from jiractl\n# yardo fills JIRA_API_TOKEN from jiractl\n",
		"jira-work": envs["jira-work"],
		"confluence": "CONFLUENCE_URL=https://acme.atlassian.net/wiki\nCONFLUENCE_USERNAME=ada@acme.com\nCONFLUENCE_API_TOKEN=atl-1\n" +
			"# yardo fills CONFLUENCE_URL from jiractl\n# yardo fills CONFLUENCE_USERNAME from jiractl\n# yardo fills CONFLUENCE_API_TOKEN from jiractl\n",
	} {
		if got := readFile(t, filepath.Join(mcp, n, "env")); got != want {
			t.Errorf("%s env = %q, want %q", n, got, want)
		}
	}

	// Not logged in, a server of its own: nothing to take, nothing changed.
	putFile(t, filepath.Join(home, ".jiractl.toml"), []byte("server = \"https://jira.acme.internal\"\n"), 0o600)
	putFile(t, filepath.Join(mcp, "confluence", "env"), []byte("CONFLUENCE_URL=\n"), 0o600)
	out = credShell(t, home, `A_MCP_SERVERS="github confluence"
mcp_from_logins "$H/.mcp"; echo done`, "mcp_name", "mcp_from_logins", "login_fill", "tomlget")
	if out != "confluence\ndone\n" {
		t.Errorf("out = %q", out)
	}
	if got := readFile(t, filepath.Join(mcp, "confluence", "env")); strings.Contains(got, "CONFLUENCE_URL=http") {
		t.Errorf("a Server/DC Jira's URL went to Confluence: %q", got)
	}
}

func TestValidLabelBash(t *testing.T) {
	body := `for l in prod eu-1 a1 "" -x x- a--b Prod "a b" ../x aaaaaaaaaaaaaaaaaaaaaaaaa; do valid_label "$l" && echo "ok $l" || echo "no $l"; done`
	out := credShell(t, t.TempDir(), body, "valid_label")
	want := "ok prod\nok eu-1\nok a1\nno \nno -x\nno x-\nno a--b\nno Prod\nno a b\nno ../x\nno aaaaaaaaaaaaaaaaaaaaaaaaa\n"
	if out != want {
		t.Errorf("valid_label:\n%s\nwant\n%s", out, want)
	}
}
