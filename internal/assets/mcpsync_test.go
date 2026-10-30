package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// aws.spec's image_args: the endpoint in AWS_REGION when the server runs
// there (else us-east-1), signed for it; read-only unless turned off.
func TestMCPSyncAWSArgs(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	mkdirAll(t, filepath.Join(mcp, "aws"), 0o700)
	mkdirAll(t, cat, 0o755)
	spec, err := os.ReadFile("conf/mcp-catalog/aws.spec")
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(cat, "aws.spec"), spec, 0o644)
	reg := filepath.Join(root, "registered")
	bin := fakeBin(t, map[string]string{"mcpjungle": `[ "$3" = register ] && cp "$6" ` + reg + `; exit 0`, "docker": "exit 0"})
	jq, _ := exec.LookPath("jq")
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	for _, c := range []struct{ env, want string }{
		{"AWS_REGION=eu-north-1\nREAD_OPERATIONS_ONLY=true\n", `"https://aws-mcp.us-east-1.api.aws/mcp","--region","us-east-1","--metadata","AWS_REGION=eu-north-1","--read-only"]`},
		{"AWS_REGION=eu-west-1\nREAD_OPERATIONS_ONLY=false\n", `"https://aws-mcp.eu-west-1.api.aws/mcp","--region","eu-west-1","--metadata","AWS_REGION=eu-west-1"]`},
		{"AWS_REGION=eu-north-1\nAWS_MCP_ENDPOINT_REGION=eu-central-1\n", `"https://aws-mcp.eu-central-1.api.aws/mcp","--region","eu-central-1","--metadata","AWS_REGION=eu-north-1","--read-only"]`},
	} {
		putFile(t, filepath.Join(mcp, "aws", "env"), []byte(c.env), 0o600)
		_ = os.Remove(filepath.Join(mcp, "aws", ".registered.json"))
		if out, err := mcpSync(t, env, "aws"); err != nil {
			t.Fatalf("%v:\n%s", err, out)
		}
		out, err := exec.Command(jq, "-c", ".args", reg).Output()
		if err != nil || !strings.HasSuffix(strings.TrimSpace(string(out)), `"public.ecr.aws/mcp-proxy-for-aws/mcp-proxy-for-aws:1.7.0",`+c.want) {
			t.Errorf("%q: args %s", c.env, out)
		}
	}
}

// status says each server's state and changes nothing; a failed
// registration's reason is kept until it registers, and settings edited
// after it make it worth trying again.
func TestMCPSyncStatus(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	mkdirAll(t, cat, 0o755)
	putFile(t, filepath.Join(cat, "svc.spec"), []byte("DESCRIPTION=Svc\nKIND=docker\nIMAGE=img:1\nREQUIRED=TOKEN\n"), 0o644)
	for _, d := range []string{"a", "b", "c"} {
		mkdirAll(t, filepath.Join(mcp, d), 0o700)
		if err := os.Symlink(filepath.Join(cat, "svc.spec"), filepath.Join(mcp, d, "spec")); err != nil {
			t.Fatal(err)
		}
	}
	putFile(t, filepath.Join(mcp, "a", "env"), []byte("TOKEN=1\n"), 0o600)
	putFile(t, filepath.Join(mcp, "b", "env"), []byte("TOKEN=\n"), 0o600)
	putFile(t, filepath.Join(mcp, "c", "env"), []byte("TOKEN=bad\n"), 0o600)
	// The gateway refuses c's config; it lists what's registered.
	reg := filepath.Join(root, "live")
	bin := fakeBin(t, map[string]string{
		"mcpjungle": `case "$3" in
  list) [ -f ` + reg + ` ] && cat ` + reg + `; exit 0 ;;
  register) grep -q '"name": "c"' "$6" && { echo "Error: c: connection refused"; exit 1; }; grep -o '"name": "[a-z]*"' "$6" | cut -d'"' -f4 | sed 's/^/1. /' >> ` + reg + `; exit 0 ;;
esac; exit 0`,
		"docker": "exit 0",
	})
	jq, _ := exec.LookPath("jq")
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	status := func() string {
		t.Helper()
		out, err := mcpSync(t, env, "status")
		if err != nil {
			t.Fatalf("status: %v\n%s", err, out)
		}
		return out
	}
	if got, want := status(), "a\tunregistered\t\nb\twaiting\tTOKEN\nc\tunregistered\t\n"; got != want {
		t.Errorf("before:\n%q\nwant\n%q", got, want)
	}
	if _, err := os.Stat(filepath.Join(mcp, "servers")); err == nil {
		t.Error("status saved a server list")
	}
	if out, err := mcpSync(t, env); err == nil || !strings.Contains(out, "c: registration failed") {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got, want := status(), "a\tok\t\nb\twaiting\tTOKEN\nc\tfailed\tError: c: connection refused\n"; got != want {
		t.Errorf("after:\n%q\nwant\n%q", got, want)
	}
	// Edited after it failed: worth trying again.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(mcp, "c", "env"), later, later); err != nil {
		t.Fatal(err)
	}
	if got := status(); !strings.Contains(got, "c\tchanged\t") {
		t.Errorf("edited after failing:\n%s", got)
	}
	// a's settings changed since it registered.
	putFile(t, filepath.Join(mcp, "a", "env"), []byte("TOKEN=2\n"), 0o600)
	putFile(t, filepath.Join(cat, "svc.spec"), []byte("DESCRIPTION=Svc2\nKIND=docker\nIMAGE=img:1\nREQUIRED=TOKEN\n"), 0o644)
	if got := status(); !strings.Contains(got, "a\tchanged\t") {
		t.Errorf("a changed:\n%s", got)
	}
	// No gateway: said first, the rest still from what's on disk.
	bin2 := fakeBin(t, map[string]string{"mcpjungle": "exit 1", "docker": "exit 0"})
	env[0] = "PATH=" + bin2 + ":" + filepath.Dir(jq) + ":/usr/bin:/bin"
	if got := status(); !strings.HasPrefix(got, "-\tdown\t") {
		t.Errorf("down:\n%s", got)
	}
}

// source says where each server runs from: an image (and, pulled here,
// its version), or a URL made from its env; it leaves the saved list be.
func TestMCPSyncSource(t *testing.T) {
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	for _, d := range []string{mcp + "/jira", mcp + "/seq", mcp + "/jenkins", cat} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		cat + "/jira.spec":    "KIND=docker\nIMAGE=img/jira:1.2\n",
		cat + "/seq.spec":     "KIND=docker\nIMAGE=img/seq:latest\n",
		cat + "/jenkins.spec": "KIND=http\nURL_VAR=JENKINS_URL\nURL_SUFFIX=/mcp-server/mcp\n",
		mcp + "/jenkins/env":  "JENKINS_URL=https://ci.example/\n",
		mcp + "/servers":      "jira\n",
	}
	for f, body := range files {
		if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := fakeBin(t, map[string]string{
		"docker": `[ "$5" = img/jira:1.2 ] || exit 1; printf '1.2\tsha256:abc\t2026-03-02T00:00:00Z\n'`,
	})
	env := []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	out, err := mcpSync(t, env, "source", "jira", "seq", "jenkins", "nope")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := "jira\timg/jira:1.2\t1.2\tsha256:abc\t2026-03-02T00:00:00Z\nseq\timg/seq:latest\njenkins\thttps://ci.example/mcp-server/mcp\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
	if b, _ := os.ReadFile(mcp + "/servers"); string(b) != "jira\n" {
		t.Errorf("servers rewritten: %q", b)
	}
}
