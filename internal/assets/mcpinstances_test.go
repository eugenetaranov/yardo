package assets

import (
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
	body := `spec_file() { [ -f "` + cat + `/$1.spec" ] && echo "` + cat + `/$1.spec"; }
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
	out := credShell(t, home, body, "spec_skeleton", "spec_field", "mcp_spec", "mcp_name", "mcp_names", "ask_mcp_keys", "mcp_dirs")
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

func TestValidLabelBash(t *testing.T) {
	body := `for l in prod eu-1 a1 "" -x x- a--b Prod "a b" ../x aaaaaaaaaaaaaaaaaaaaaaaaa; do valid_label "$l" && echo "ok $l" || echo "no $l"; done`
	out := credShell(t, t.TempDir(), body, "valid_label")
	want := "ok prod\nok eu-1\nok a1\nno \nno -x\nno x-\nno a--b\nno Prod\nno a b\nno ../x\nno aaaaaaaaaaaaaaaaaaaaaaaaa\n"
	if out != want {
		t.Errorf("valid_label:\n%s\nwant\n%s", out, want)
	}
}
