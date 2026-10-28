package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBin puts scripts named for commands in a directory, first on PATH.
func fakeBin(t *testing.T, cmds map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range cmds {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mcpSync(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	sync, _ := filepath.Abs("mcp-sync")
	c := exec.Command("sh", append([]string{sync}, args...)...)
	c.Env = env
	out, err := c.CombinedOutput()
	return string(out), err
}

// Re-run as the service user, nothing of the caller's environment crosses
// over: not the home, the catalog, the gateway's address or the editor.
func TestMCPSyncReexecIgnoresCallerEnv(t *testing.T) {
	got := filepath.Join(t.TempDir(), "sudo-args")
	bin := fakeBin(t, map[string]string{
		"id":     `[ "$1" = -un ] && echo ada; exit 0`,
		"getent": `echo "ada:x:1000:1000::/home/ada:/bin/sh"`,
		"sudo":   `printf '%s\n' "$@" > ` + got,
	})
	env := []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=/tmp/attacker", "MCP_CATALOG=/tmp/evil",
		"MCPJUNGLE_REGISTRY=http://evil.example", "VISUAL=sh -c pwn", "EDITOR=/tmp/evil/ed"}
	if out, err := mcpSync(t, env, "github"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	b, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	args := string(b)
	for _, want := range []string{"-u\nmcp-ada\n", "MCP_DIR=/home/ada/.mcp\n", "MCP_CATALOG=/usr/local/share/mcp-catalog\n", "MCPJUNGLE_REGISTRY=http://127.0.0.1:8080\n", "\ngithub\n"} {
		if !strings.Contains(args, want) {
			t.Errorf("sudo args lack %q:\n%s", want, args)
		}
	}
	for _, bad := range []string{"attacker", "evil", "pwn"} {
		if strings.Contains(args, bad) {
			t.Errorf("sudo args carry the caller's %q:\n%s", bad, args)
		}
	}
}

// A name that isn't one (a path) is refused; a spec linked outside the
// catalog isn't sourced, one linked into it is.
func TestMCPSyncNamesAndLinks(t *testing.T) {
	root := t.TempDir()
	mcp, cat, evil := filepath.Join(root, "mcp"), filepath.Join(root, "catalog"), filepath.Join(root, "evil")
	marker := filepath.Join(root, "sourced")
	for _, d := range []string{mcp + "/bad", mcp + "/good", cat, evil} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(evil, "x.spec"), []byte("touch "+marker+"\nKIND=docker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cat, "grafana.spec"), []byte("KIND=docker\nIMAGE=x\nREQUIRED=GRAFANA_URL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(evil, "x.spec"), filepath.Join(mcp, "bad", "spec")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(cat, "grafana.spec"), filepath.Join(mcp, "good", "spec")); err != nil {
		t.Fatal(err)
	}
	bin := fakeBin(t, map[string]string{"mcpjungle": "exit 0"})
	env := []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}

	for _, name := range []string{"../../tmp/evil", ".hidden", "a__b", "-x"} {
		if out, err := mcpSync(t, env, name); err == nil || !strings.Contains(out, "not a server name") {
			t.Errorf("%q: err %v, out %s", name, err, out)
		}
	}
	if out, err := mcpSync(t, env, "edit", "../x"); err == nil || !strings.Contains(out, "usage") {
		t.Errorf("edit ../x: err %v, out %s", err, out)
	}
	out, _ := mcpSync(t, env)
	if _, err := os.Stat(marker); err == nil {
		t.Errorf("a spec linked outside the catalog was sourced:\n%s", out)
	}
	if !strings.Contains(out, "bad: its spec links outside") || !strings.Contains(out, "good: waiting for credentials") {
		t.Errorf("out:\n%s", out)
	}
}

// An instance (grafana-prod, a link to grafana.spec) registers under its
// own name, its name leading the description.
func TestMCPSyncInstanceDescription(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	for _, d := range []string{mcp + "/grafana-prod", cat} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cat, "grafana.spec"), []byte("DESCRIPTION=\"Grafana: dashboards\"\nKIND=docker\nIMAGE=img\nREQUIRED=GRAFANA_URL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(cat, "grafana.spec"), filepath.Join(mcp, "grafana-prod", "spec")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcp, "grafana-prod", "env"), []byte("GRAFANA_URL=https://g\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := filepath.Join(root, "registered")
	bin := fakeBin(t, map[string]string{
		"mcpjungle": `[ "$3" = register ] && cp "$6" ` + reg + `; exit 0`,
		"docker":    "exit 0",
	})
	jq, _ := exec.LookPath("jq")
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	if out, err := mcpSync(t, env); err != nil || !strings.Contains(out, "grafana-prod: registered") {
		t.Fatalf("%v:\n%s", err, out)
	}
	b, _ := os.ReadFile(reg)
	if !strings.Contains(string(b), `"name": "grafana-prod"`) || !strings.Contains(string(b), `"description": "(grafana-prod) Grafana: dashboards"`) {
		t.Errorf("registered:\n%s", b)
	}
}

// rename takes the server off the gateway and moves its directory (its
// settings go along); remove takes it off and deletes them.
func TestMCPSyncRenameRemove(t *testing.T) {
	root := t.TempDir()
	mcp := filepath.Join(root, "mcp")
	for _, d := range []string{"jira-jira", "grafana-old"} {
		if err := os.MkdirAll(filepath.Join(mcp, d), 0o700); err != nil {
			t.Fatal(err)
		}
		putFile(t, filepath.Join(mcp, d, "env"), []byte("KEY=v\n"), 0o600)
		putFile(t, filepath.Join(mcp, d, ".registered.json"), []byte("{}"), 0o600)
	}
	putFile(t, filepath.Join(mcp, "servers"), []byte("github\njira-jira\ngrafana-old\n"), 0o600)
	log := filepath.Join(root, "mj.log")
	bin := fakeBin(t, map[string]string{"mcpjungle": `echo "$*" >> ` + log})
	env := []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}

	if out, err := mcpSync(t, env, "rename", "jira-jira", "jira"); err != nil {
		t.Fatalf("rename: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(mcp, "jira", "env")); got != "KEY=v\n" {
		t.Errorf("renamed env = %q", got)
	}
	if _, err := os.Stat(filepath.Join(mcp, "jira", ".registered.json")); err == nil {
		t.Error("the renamed server still looks registered")
	}
	if out, err := mcpSync(t, env, "rename", "grafana-old", "jira"); err == nil {
		t.Errorf("rename onto an existing server: %s", out)
	}
	if out, err := mcpSync(t, env, "remove", "grafana-old"); err != nil {
		t.Fatalf("remove: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(mcp, "grafana-old")); err == nil {
		t.Error("the removed server's directory is still there")
	}
	if got := readFile(t, filepath.Join(mcp, "servers")); got != "github\n" {
		t.Errorf("servers = %q", got)
	}
	if got := readFile(t, log); !strings.Contains(got, "deregister jira-jira") || !strings.Contains(got, "deregister grafana-old") {
		t.Errorf("mcpjungle ran %q", got)
	}
	for _, bad := range [][]string{{"remove", "../x"}, {"rename", "jira", "a/b"}, {"remove", ""}} {
		if _, err := mcpSync(t, env, bad...); err == nil {
			t.Errorf("mcp-sync %v: want an error", bad)
		}
	}
}
