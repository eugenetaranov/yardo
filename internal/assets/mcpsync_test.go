package assets

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
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
	if os.Geteuid() == 0 {
		// mcp-sync won't run as root: these tests run it as a user, which
		// id says they are (a fake id of the test's own still goes first).
		bin := fakeBin(t, map[string]string{"id": `[ "$1" = -u ] && { echo 1000; exit 0; }; exec /usr/bin/id "$@"`})
		c.Env = slices.Clone(env)
		for n, e := range c.Env {
			if p, ok := strings.CutPrefix(e, "PATH="); ok {
				c.Env[n] = "PATH=" + strings.Replace(":"+p, ":/usr/bin", ":"+bin+":/usr/bin", 1)[1:]
			}
		}
	}
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

// Run as root (sudo mcp-sync), it acts for the user who ran it, as their
// gateway's user; with no one to act for, it reads no spec at all.
func TestMCPSyncNotAsRoot(t *testing.T) {
	got := filepath.Join(t.TempDir(), "sudo-args")
	bin := fakeBin(t, map[string]string{
		"id":     `case "$1" in -u) echo 0 ;; -un) echo root ;; mcp-ada) exit 0 ;; *) exit 1 ;; esac`,
		"getent": `echo "ada:x:1000:1000::/home/ada:/bin/sh"`,
		"sudo":   `printf '%s\n' "$@" > ` + got,
	})
	cat := t.TempDir()
	if err := os.WriteFile(filepath.Join(cat, "evil.spec"), []byte("touch "+got+".ran\nKIND=docker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + bin + ":/usr/bin:/bin", "MCP_CATALOG=" + cat, "MCP_DIR=" + t.TempDir()}
	out, err := mcpSync(t, env, "evil")
	if err == nil || !strings.Contains(out, "not as root") {
		t.Errorf("no SUDO_USER: %v: %s", err, out)
	}
	if out, err := mcpSync(t, append(env, "SUDO_USER=bob"), "evil"); err == nil || !strings.Contains(out, "bob has no MCP gateway") {
		t.Errorf("no gateway: %v: %s", err, out)
	}
	if out, err := mcpSync(t, append(env, "SUDO_USER=ada"), "evil"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if b, err := os.ReadFile(got); err != nil || !strings.Contains(string(b), "-u\nmcp-ada\n") || !strings.Contains(string(b), "MCP_DIR=/home/ada/.mcp\n") {
		t.Errorf("sudo args: %q, %v", b, err)
	}
	if _, err := os.Stat(got + ".ran"); err == nil {
		t.Error("a spec was sourced as root")
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
// there (else us-east-1), signed for it; read-only only when turned on.
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
	// A new server's env, as the skeleton makes it (its region filled in):
	// it calls AWS APIs, not read-only.
	skel, err := exec.Command("sh", "-c", `. "$1"; skeleton`, "_", filepath.Join(cat, "aws.spec")).Output()
	if err != nil {
		t.Fatal(err)
	}
	fresh := strings.Replace(string(skel), "\nAWS_REGION=\n", "\nAWS_REGION=eu-west-1\n", 1)
	for _, c := range []struct{ env, want string }{
		{fresh, `"https://aws-mcp.eu-west-1.api.aws/mcp","--region","eu-west-1","--metadata","AWS_REGION=eu-west-1"]`},
		{"AWS_REGION=eu-north-1\nREAD_OPERATIONS_ONLY=true\n", `"https://aws-mcp.us-east-1.api.aws/mcp","--region","us-east-1","--metadata","AWS_REGION=eu-north-1","--read-only"]`},
		{"AWS_REGION=eu-west-1\nREAD_OPERATIONS_ONLY=false\n", `"https://aws-mcp.eu-west-1.api.aws/mcp","--region","eu-west-1","--metadata","AWS_REGION=eu-west-1"]`},
		{"AWS_REGION=eu-north-1\nAWS_MCP_ENDPOINT_REGION=eu-central-1\n", `"https://aws-mcp.eu-central-1.api.aws/mcp","--region","eu-central-1","--metadata","AWS_REGION=eu-north-1"]`},
		{"AWS_REGION=us-east-1\n#AWS_PROFILE=x\nAWS_PROFILE=prod-ro\n", `"https://aws-mcp.us-east-1.api.aws/mcp","--region","us-east-1","--metadata","AWS_REGION=us-east-1","--profile","prod-ro"]`},
	} {
		putFile(t, filepath.Join(mcp, "aws", "env"), []byte(c.env), 0o600)
		_ = os.Remove(filepath.Join(mcp, "aws", ".registered.json"))
		if out, err := mcpSync(t, env, "aws"); err != nil {
			t.Fatalf("%v:\n%s", err, out)
		}
		out, err := exec.Command(jq, "-c", ".args", reg).Output()
		if err != nil || !strings.HasSuffix(strings.TrimSpace(string(out)), `"public.ecr.aws/mcp-proxy-for-aws/mcp-proxy-for-aws:1.8.0",`+c.want) {
			t.Errorf("%q: args %s", c.env, out)
		}
	}
}

// aws.spec's MOUNT_AWS: the home's ~/.aws, read-only, as the home's owner,
// when there is one; never a path from the env file.
func TestMCPSyncAWSMount(t *testing.T) {
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
	putFile(t, filepath.Join(mcp, "aws", "env"), []byte("AWS_REGION=us-east-1\nHOME=/etc\n"), 0o600)
	reg := filepath.Join(root, "registered")
	bin := fakeBin(t, map[string]string{"mcpjungle": `[ "$3" = register ] && cp "$6" ` + reg + `; exit 0`, "docker": "exit 0"})
	jq, _ := exec.LookPath("jq")
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	args := func() string {
		t.Helper()
		_ = os.Remove(filepath.Join(mcp, "aws", ".registered.json"))
		if out, err := mcpSync(t, env, "aws"); err != nil {
			t.Fatalf("%v:\n%s", err, out)
		}
		out, err := exec.Command(jq, "-c", ".args", reg).Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	// Root in the container either way: the image's own user can't read
	// the env file's directory (~/.mcp/aws, 700).
	if a := args(); strings.Contains(a, "/aws/.aws") || !strings.Contains(a, `"--no-healthcheck","--user","0:0","public.ecr.aws/`) {
		t.Errorf("no ~/.aws: %s", a)
	}
	mkdirAll(t, filepath.Join(root, ".aws"), 0o700)
	fi, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	ids := fmt.Sprintf("%d:%d", fi.Sys().(*syscall.Stat_t).Uid, fi.Sys().(*syscall.Stat_t).Gid)
	want := `"-v","` + mcp + `/aws:/mcp:ro","-v","` + root + `/.aws:/aws/.aws:ro","-e","HOME=/aws","--user","` + ids + `","--no-healthcheck","--user","0:0","public.ecr.aws/`
	if a := args(); !strings.Contains(a, want) || strings.Contains(a, "/etc") {
		t.Errorf("args %s\nwant %s", a, want)
	}
}

// k8s.spec's MOUNT_KUBE and MOUNT_AWS: ~/.kube and ~/.aws (EKS's
// get-token), read-only; a KUBECONFIG under ~/.kube made the container's
// path, any other left be; root in the container all the same.
func TestMCPSyncKubeMount(t *testing.T) {
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	mkdirAll(t, filepath.Join(mcp, "k8s"), 0o700)
	mkdirAll(t, filepath.Join(root, ".kube"), 0o700)
	mkdirAll(t, filepath.Join(root, ".aws"), 0o700)
	mkdirAll(t, cat, 0o755)
	spec, err := os.ReadFile("conf/mcp-catalog/k8s.spec")
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(cat, "k8s.spec"), spec, 0o644)
	reg := filepath.Join(root, "registered")
	bin := fakeBin(t, map[string]string{"mcpjungle": `[ "$3" = register ] && cp "$6" ` + reg + `; exit 0`, "docker": "exit 0"})
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	fi, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	ids := fmt.Sprintf("%d:%d", fi.Sys().(*syscall.Stat_t).Uid, fi.Sys().(*syscall.Stat_t).Gid)
	mounts := `"-v","` + root + `/.aws:/aws/.aws:ro","-v","` + root + `/.kube:/aws/.kube:ro",`
	user := `"-e","HOME=/aws","--user","` + ids + `","--user","0:0","flux159/`
	for kc, want := range map[string]string{
		"~/.kube/config/dev1":                         mounts + `"-e","KUBECONFIG=/aws/.kube/config/dev1",` + user,
		root + "/.kube/a:$HOME/.kube/b":               mounts + `"-e","KUBECONFIG=/aws/.kube/a:/aws/.kube/b",` + user,
		"/mcp/config":                                 mounts + user,
		"~/.kube/../../etc/passwd:/elsewhere/.kube/x": mounts + `"-e","KUBECONFIG=/aws/.kube/../../etc/passwd:/elsewhere/.kube/x",` + user,
	} {
		putFile(t, filepath.Join(mcp, "k8s", "env"), []byte("KUBECONFIG="+kc+"\n"), 0o600)
		_ = os.Remove(filepath.Join(mcp, "k8s", ".registered.json"))
		if out, err := mcpSync(t, env, "k8s"); err != nil {
			t.Fatalf("%v:\n%s", err, out)
		}
		out, err := exec.Command(jq, "-c", ".args", reg).Output()
		if err != nil || !strings.Contains(string(out), want) {
			t.Errorf("%s: args %s\nwant %s", kc, out, want)
		}
	}
}

// A server synced after an AWS one gets nothing of its: not ~/.aws, not
// its user (a spec's variables don't carry over).
func TestMCPSyncAWSMountNotCarried(t *testing.T) {
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	mkdirAll(t, filepath.Join(root, ".aws"), 0o700)
	mkdirAll(t, cat, 0o755)
	spec, err := os.ReadFile("conf/mcp-catalog/aws.spec")
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(cat, "aws.spec"), spec, 0o644)
	putFile(t, filepath.Join(cat, "zz.spec"), []byte("KIND=docker\nIMAGE=example/zz:1\nIMAGE_ARGS=serve\n"), 0o644)
	for n, env := range map[string]string{"aws": "AWS_REGION=us-east-1\n", "zz": "X=1\n"} {
		mkdirAll(t, filepath.Join(mcp, n), 0o700)
		putFile(t, filepath.Join(mcp, n, "env"), []byte(env), 0o600)
	}
	reg := filepath.Join(root, "registered")
	bin := fakeBin(t, map[string]string{"mcpjungle": `[ "$3" = register ] && cp "$6" "` + reg + `.$(jq -r .name "$6")"; exit 0`, "docker": "exit 0"})
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	if out, err := mcpSync(t, env, "aws", "zz"); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	for n, mounted := range map[string]bool{"aws": true, "zz": false} {
		out, err := exec.Command(jq, "-c", ".args", reg+"."+n).Output()
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		if got := strings.Contains(string(out), "/aws/.aws") && strings.Contains(string(out), "--user"); got != mounted {
			t.Errorf("%s: ~/.aws mounted %v: %s", n, got, out)
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

// client makes a laptop's token in a file the gateway reads, and says
// where it is: never the token itself.
func TestMCPSyncClient(t *testing.T) {
	root := t.TempDir()
	mcp := filepath.Join(root, "mcp")
	if err := os.MkdirAll(mcp, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcp+"/.mcpjungle.conf", []byte("access_token: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(root, "conf-seen")
	bin := fakeBin(t, map[string]string{
		"mcpjungle": `[ "$3 $4 $5" = "create mcp-client --conf" ] || exit 1; cp "$6" ` + conf,
	})
	env := []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	out, err := mcpSync(t, env, "client", "laptop")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	tokFile := mcp + "/clients/laptop.token"
	tok, err := os.ReadFile(tokFile)
	if err != nil || len(strings.TrimSpace(string(tok))) != 64 {
		t.Fatalf("token %q, %v", tok, err)
	}
	if fi, _ := os.Stat(tokFile); fi.Mode().Perm() != 0o600 {
		t.Errorf("token mode %v", fi.Mode().Perm())
	}
	if strings.Contains(out, strings.TrimSpace(string(tok))) || !strings.Contains(out, "sudo cat "+tokFile) {
		t.Errorf("out:\n%s", out)
	}
	if b, _ := os.ReadFile(conf); !strings.Contains(string(b), `"access_token_ref": {"file": "`+tokFile+`"}`) || !strings.Contains(string(b), `"allowed_servers": ["*"]`) {
		t.Errorf("client conf: %s", b)
	}
	if out, err := mcpSync(t, env, "client", "laptop"); err == nil || !strings.Contains(out, "there's a laptop already") {
		t.Errorf("again: %v: %s", err, out)
	}
}

// source says where each server runs from: an image (and, pulled here,
// its version: none for one with no labels), or a URL made from its env;
// it leaves the saved list be.
func TestMCPSyncSource(t *testing.T) {
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	for _, d := range []string{mcp + "/jira", mcp + "/seq", mcp + "/k8s", mcp + "/jenkins", cat} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		cat + "/jira.spec":    "KIND=docker\nIMAGE=img/jira:1.2\n",
		cat + "/seq.spec":     "KIND=docker\nIMAGE=img/seq:latest\n",
		cat + "/k8s.spec":     "KIND=docker\nIMAGE=img/k8s:4\n",
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
		"docker": `case "$5" in
  img/jira:1.2) echo '{"Id":"sha256:abc","Created":"2026-03-02T00:00:00Z","Config":{"Labels":{"org.opencontainers.image.version":"1.2"}}}' ;;
  img/k8s:4) echo '{"Id":"sha256:def","Created":"2026-10-26T18:10:20Z","Config":{}}' ;;
  *) exit 1 ;;
esac`,
	})
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	out, err := mcpSync(t, env, "source", "jira", "seq", "k8s", "jenkins", "nope")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := "jira\timg/jira:1.2\t1.2\tsha256:abc\t2026-03-02T00:00:00Z\nseq\timg/seq:latest\nk8s\timg/k8s:4\t\tsha256:def\t2026-10-26T18:10:20Z\njenkins\thttps://ci.example/mcp-server/mcp\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
	if b, _ := os.ReadFile(mcp + "/servers"); string(b) != "jira\n" {
		t.Errorf("servers rewritten: %q", b)
	}
}

// onOffEnv is a ~/.mcp with servers a and b of one spec, both registered
// as they are, and a gateway that logs what it's told (and fails while
// $root/down exists).
func onOffEnv(t *testing.T) (mcp, log string, env []string) {
	t.Helper()
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	mkdirAll(t, cat, 0o755)
	putFile(t, filepath.Join(cat, "svc.spec"), []byte("KIND=docker\nIMAGE=img:1\nREQUIRED=TOKEN\n"), 0o644)
	log = filepath.Join(root, "mj.log")
	bin := fakeBin(t, map[string]string{
		"mcpjungle": `[ -f ` + root + `/down ] && exit 1
case "$3" in
  list) printf '1. a\n2. b\n' ;;
  register) echo "register $(jq -r .name "$6")" >> ` + log + ` ;;
  *) echo "$3 $4 $5" >> ` + log + ` ;;
esac; exit 0`,
		"docker": "exit 0",
	})
	env = []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	for _, d := range []string{"a", "b"} {
		mkdirAll(t, filepath.Join(mcp, d), 0o700)
		if err := os.Symlink(filepath.Join(cat, "svc.spec"), filepath.Join(mcp, d, "spec")); err != nil {
			t.Fatal(err)
		}
		putFile(t, filepath.Join(mcp, d, "env"), []byte("TOKEN=1\n"), 0o600)
	}
	if out, err := mcpSync(t, env); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	_ = os.Remove(log)
	return mcp, log, env
}

func logged(t *testing.T, log string) string {
	t.Helper()
	b, _ := os.ReadFile(log)
	_ = os.Remove(log)
	return string(b)
}

// off and on: the markers, and the gateway told once; with none named,
// all of them, each back to its own choice.
func TestMCPSyncOnOff(t *testing.T) {
	mcp, log, env := onOffEnv(t)
	if out, err := mcpSync(t, env, "off", "a"); err != nil || !strings.Contains(out, "a: off") {
		t.Fatalf("off a: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(mcp, "a", "off")); err != nil {
		t.Error("no a/off")
	}
	if got := logged(t, log); got != "disable server a\n" {
		t.Errorf("off a: gateway told %q", got)
	}
	if out, err := mcpSync(t, env, "off"); err != nil || !strings.Contains(out, "all servers: off") {
		t.Fatalf("off: %v\n%s", err, out)
	}
	if got := logged(t, log); got != "disable server b\n" {
		t.Errorf("off (all): gateway told %q", got)
	}
	if out, err := mcpSync(t, env, "on"); err != nil {
		t.Fatalf("on: %v\n%s", err, out)
	}
	if got := logged(t, log); got != "enable server b\n" {
		t.Errorf("on (all): gateway told %q (a is off by itself)", got)
	}
	if _, err := mcpSync(t, env, "on", "a"); err != nil {
		t.Fatal(err)
	}
	if got := logged(t, log); got != "enable server a\n" {
		t.Errorf("on a: gateway told %q", got)
	}
	if _, err := mcpSync(t, env, "off", "../x"); err == nil {
		t.Error("off ../x: want an error")
	}
}

// The gateway down: the choice is kept, said, and made at the next sync.
func TestMCPSyncOffGatewayDown(t *testing.T) {
	mcp, log, env := onOffEnv(t)
	down := filepath.Join(filepath.Dir(mcp), "down")
	putFile(t, down, nil, 0o600)
	out, err := mcpSync(t, env, "off", "a")
	if err != nil || !strings.Contains(out, "once the gateway runs") {
		t.Fatalf("%v:\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(mcp, "a", "off")); err != nil {
		t.Error("the choice wasn't kept")
	}
	_ = os.Remove(down)
	if out, err := mcpSync(t, env); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	if got := logged(t, log); got != "disable server a\n" {
		t.Errorf("sync: gateway told %q", got)
	}
}

// A server off and registered again (its settings changed) is disabled
// again in the same sync; a sync with nothing changed tells the gateway
// nothing.
func TestMCPSyncOffSurvivesRegister(t *testing.T) {
	mcp, log, env := onOffEnv(t)
	if _, err := mcpSync(t, env, "off", "a"); err != nil {
		t.Fatal(err)
	}
	logged(t, log)
	if out, err := mcpSync(t, env); err != nil || logged(t, log) != "" {
		t.Errorf("a sync with nothing changed told the gateway something (%v):\n%s", err, out)
	}
	// Registered with another config than its settings make now.
	putFile(t, filepath.Join(mcp, "a", ".registered.json"), []byte("{}\n"), 0o600)
	out, err := mcpSync(t, env)
	if err != nil || !strings.Contains(out, "a: registered, off") {
		t.Fatalf("%v:\n%s", err, out)
	}
	if got := logged(t, log); got != "register a\ndisable server a\n" {
		t.Errorf("gateway told %q", got)
	}
}

// restart deregisters the servers, removes their containers (only theirs;
// none named: every server's) and registers them again, off ones still
// off; the saved list stays as it was.
func TestMCPSyncRestart(t *testing.T) {
	mcp, log, env := onOffEnv(t)
	rm := filepath.Join(filepath.Dir(mcp), "rm.log")
	docker := fakeBin(t, map[string]string{"docker": `case "$1" in
  ps) printf 'id-a\nid-b\nid-x\nid-n\n' ;;
  inspect) printf 'id-a ` + mcp + `/a\nid-b ` + mcp + `/b\nid-x /elsewhere/a\nid-n\n' ;;
  rm) echo "$3" >> ` + rm + ` ;;
esac; exit 0`})
	env[0] = "PATH=" + docker + ":" + strings.TrimPrefix(env[0], "PATH=")
	servers := "a\nb\n"
	putFile(t, filepath.Join(mcp, "servers"), []byte(servers), 0o600)
	if _, err := mcpSync(t, env, "off", "b"); err != nil {
		t.Fatal(err)
	}
	logged(t, log)

	out, err := mcpSync(t, env, "restart", "a")
	if err != nil || !strings.Contains(out, "a: removed container id-a") || !strings.Contains(out, "a: registered") {
		t.Fatalf("restart a (%v):\n%s", err, out)
	}
	if got := logged(t, log); got != "deregister a \nregister a\n" {
		t.Errorf("restart a: gateway told %q", got)
	}
	if got := logged(t, rm); got != "id-a\n" {
		t.Errorf("restart a removed %q", got)
	}

	out, err = mcpSync(t, env, "restart")
	if err != nil || !strings.Contains(out, "b: registered, off") {
		t.Fatalf("restart (%v):\n%s", err, out)
	}
	if got := logged(t, log); got != "deregister a \nderegister b \nregister a\nregister b\ndisable server b\n" {
		t.Errorf("restart: gateway told %q", got)
	}
	if got := logged(t, rm); got != "id-a\nid-b\n" {
		t.Errorf("restart removed %q", got)
	}
	if got := readFile(t, filepath.Join(mcp, "servers")); got != servers {
		t.Errorf("servers = %q, was %q", got, servers)
	}

	if out, err := mcpSync(t, env, "remove", "a"); err != nil {
		t.Fatalf("remove a: %v\n%s", err, out)
	}
	if got := logged(t, rm); got != "id-a\n" {
		t.Errorf("remove a removed %q", got)
	}
	if _, err := mcpSync(t, env, "restart", "../x"); err == nil {
		t.Error("restart ../x: want an error")
	}
	putFile(t, filepath.Join(filepath.Dir(mcp), "down"), nil, 0o600)
	if out, err := mcpSync(t, env, "restart", "b"); err == nil || logged(t, rm) != "" {
		t.Errorf("restart, the gateway down: want an error, nothing removed:\n%s", out)
	}
}

// status says off first, own or all; a renamed server stays off and isn't
// thought disabled on the gateway any more.
func TestMCPSyncOffStatusRename(t *testing.T) {
	mcp, _, env := onOffEnv(t)
	putFile(t, filepath.Join(mcp, "b", "env"), []byte("TOKEN=\n"), 0o600) // would be waiting
	if _, err := mcpSync(t, env, "off", "b"); err != nil {
		t.Fatal(err)
	}
	out, err := mcpSync(t, env, "status")
	if err != nil || !strings.Contains(out, "a\tok\t\n") || !strings.Contains(out, "b\toff\town\n") {
		t.Errorf("status (%v):\n%s", err, out)
	}
	if _, err := mcpSync(t, env, "off"); err != nil {
		t.Fatal(err)
	}
	out, _ = mcpSync(t, env, "status")
	if !strings.HasPrefix(out, "*\toff\t\n") || !strings.Contains(out, "a\toff\tall\n") || !strings.Contains(out, "b\toff\town\n") {
		t.Errorf("status, all off:\n%s", out)
	}
	if out, err := mcpSync(t, env, "rename", "a", "c"); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	if _, err := mcpSync(t, env, "off", "c"); err != nil {
		t.Fatal(err)
	}
	if out, err := mcpSync(t, env, "rename", "c", "d"); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(mcp, "d", "off")); err != nil {
		t.Error("the choice didn't move with the rename")
	}
	if _, err := os.Stat(filepath.Join(mcp, "d", ".disabled")); err == nil {
		t.Error("the renamed server still looks disabled on the gateway")
	}
}

// client-rm takes a client off the gateway, then deletes its token; a
// gateway that won't forget it keeps the files (the token still works,
// and it says so); no such client is refused.
func TestMCPSyncClientRm(t *testing.T) {
	root := t.TempDir()
	mcp := filepath.Join(root, "mcp")
	if err := os.MkdirAll(mcp+"/clients", 0o700); err != nil {
		t.Fatal(err)
	}
	putFile(t, mcp+"/clients/ci.token", []byte("secret\n"), 0o600)
	putFile(t, mcp+"/clients/ci.json", []byte("{}\n"), 0o600)
	seen := filepath.Join(root, "seen")
	ok := `case "$3 $4" in "list mcp-clients") echo "1. ci";; "delete mcp-client") echo "$5" >> ` + seen + `;; *) exit 1;; esac`
	env := func(fake string) []string {
		return []string{"PATH=" + fakeBin(t, map[string]string{"mcpjungle": fake}) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	}
	refuse := `case "$3 $4" in "list mcp-clients") echo "1. ci";; *) exit 1;; esac`
	if out, err := mcpSync(t, env(refuse), "client-rm", "ci"); err == nil || !strings.Contains(out, "still works") {
		t.Errorf("refused: %v: %s", err, out)
	}
	if _, err := os.Stat(mcp + "/clients/ci.token"); err != nil {
		t.Error("the token went though the gateway kept it")
	}
	out, err := mcpSync(t, env(ok), "client-rm", "ci")
	if err != nil || !strings.Contains(out, "revoked") {
		t.Fatalf("%v: %s", err, out)
	}
	if b, _ := os.ReadFile(seen); string(b) != "ci\n" {
		t.Errorf("deleted on the gateway: %q", b)
	}
	for _, f := range []string{"ci.token", "ci.json"} {
		if _, err := os.Stat(mcp + "/clients/" + f); err == nil {
			t.Errorf("%s stays", f)
		}
	}
	if out, err := mcpSync(t, env(ok), "client-rm", "ci"); err == nil || !strings.Contains(out, "no client ci") {
		t.Errorf("again: %v: %s", err, out)
	}
}

// The read-only switches as the servers get them: postgres restricted;
// opensearch not writing unless its env says so (one of before, without
// the line, too); slack's read tools only, posting kept for an env that
// allowed it, an env's own list as it is; grafana writing nothing and
// reporting nothing.
func TestMCPSpecsReadOnlyArgs(t *testing.T) {
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	mkdirAll(t, cat, 0o755)
	reg := filepath.Join(root, "registered")
	bin := fakeBin(t, map[string]string{"mcpjungle": `[ "$3" = register ] && cp "$6" ` + reg + `; exit 0`, "docker": "exit 0"})
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	args := func(name, envFile string) string {
		t.Helper()
		spec, err := os.ReadFile("conf/mcp-catalog/" + name + ".spec")
		if err != nil {
			t.Fatal(err)
		}
		putFile(t, filepath.Join(cat, name+".spec"), spec, 0o644)
		mkdirAll(t, filepath.Join(mcp, name), 0o700)
		putFile(t, filepath.Join(mcp, name, "env"), []byte(envFile), 0o600)
		_ = os.Remove(filepath.Join(mcp, name, ".registered.json"))
		if out, err := mcpSync(t, env, name); err != nil {
			t.Fatalf("%s: %v:\n%s", name, err, out)
		}
		out, err := exec.Command(jq, "-c", ".args", reg).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	skel := func(name string) string {
		t.Helper()
		out, err := exec.Command("sh", "-c", `. "$1"; skeleton`, "_", "conf/mcp-catalog/"+name+".spec").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	if a := args("postgres", "DATABASE_URI=postgresql://u:p@127.0.0.1/db\n"); !strings.HasSuffix(a, `"crystaldba/postgres-mcp:0.3.0","--access-mode=restricted"]`) {
		t.Errorf("postgres: %s", a)
	}
	os := skel("opensearch")
	if !strings.Contains(os, "\nOPENSEARCH_SETTINGS_ALLOW_WRITE=false\n") {
		t.Errorf("opensearch skeleton:\n%s", os)
	}
	for envFile, want := range map[string]string{
		strings.Replace(os, "\nOPENSEARCH_URL=\n", "\nOPENSEARCH_URL=https://s:9200\n", 1): "false",
		"OPENSEARCH_URL=https://s:9200\n":                                                  "false",
		"OPENSEARCH_URL=https://s:9200\nOPENSEARCH_SETTINGS_ALLOW_WRITE=true\n":            "true",
	} {
		if a := args("opensearch", envFile); !strings.HasSuffix(a, `-slim","env","OPENSEARCH_SETTINGS_ALLOW_WRITE=`+want+`","uvx","opensearch-mcp-server-py@0.12.0"]`) {
			t.Errorf("opensearch %q: %s", envFile, a)
		}
	}
	read := "conversations_history,conversations_replies,conversations_search_messages,conversations_unreads,channels_list,channels_me,users_search,usergroups_list,usergroups_me,saved_list"
	sl := strings.Replace(skel("slack"), "\nSLACK_MCP_XOXP_TOKEN=\n", "\nSLACK_MCP_XOXP_TOKEN=xoxp-1\n", 1)
	for envFile, want := range map[string]string{
		sl:                              read,
		"SLACK_MCP_XOXP_TOKEN=xoxp-1\n": read,
		"SLACK_MCP_XOXP_TOKEN=xoxp-1\nSLACK_MCP_ADD_MESSAGE_TOOL=C123\n":       read + ",conversations_add_message",
		"SLACK_MCP_XOXP_TOKEN=xoxp-1\nSLACK_MCP_ENABLED_TOOLS=channels_list\n": "channels_list",
	} {
		if a := args("slack", envFile); !strings.HasSuffix(a, `"--transport","stdio","--enabled-tools","`+want+`"]`) {
			t.Errorf("slack %q: %s", envFile, a)
		}
	}
	if a := args("grafana", "GRAFANA_URL=http://g\nGRAFANA_SERVICE_ACCOUNT_TOKEN=x\n"); !strings.HasSuffix(a, `"-t","stdio","--disable-write","--usage-stats=disabled"]`) {
		t.Errorf("grafana: %s", a)
	}
}

// A spec's Docker options that would let the container act on the host
// (root, in effect) keep the server off the gateway, and status says why.
func TestMCPSyncRefusesHostOptions(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	mkdirAll(t, filepath.Join(mcp, "x"), 0o700)
	mkdirAll(t, cat, 0o755)
	putFile(t, filepath.Join(mcp, "x", "env"), []byte("TOKEN=1\n"), 0o600)
	if err := os.WriteFile(filepath.Join(mcp, "servers"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := filepath.Join(root, "registered")
	bin := fakeBin(t, map[string]string{"mcpjungle": `[ "$3" = register ] && cp "$6" ` + reg + `; exit 0`, "docker": "exit 0"})
	jq, _ := exec.LookPath("jq")
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	for _, c := range []struct{ args, why string }{
		{"--privileged", "--privileged"},
		{"--network=host", "--network host"},
		{"--net host", "--net host"},
		{"--pid=host", "--pid host"},
		{"-v /:/h", "-v /:/h"},
		{"-v /var/run/docker.sock:/s", "-v /var/run/docker.sock:/s"},
		{"-v " + root + "/.ssh:/s:ro", "-v " + root + "/.ssh:/s:ro"},
		{"-v " + mcp + "/x/../../.ssh:/s", "-v " + mcp + "/x/../../.ssh:/s"},
		{"--mount type=bind,source=/etc,target=/e", "--mount type=bind,source=/etc,target=/e"},
		{"-p 80:80", "-p"},
		{"-p8080:80", "-p"},
		{"--cap-add SYS_ADMIN", "--cap-add"},
		{"--device=/dev/sda", "--device"},
		{"--security-opt seccomp=unconfined", "--security-opt seccomp=unconfined"},
		{"-e A=1 -v " + mcp + "/x/data:/data", ""},
		{"-v cache:/cache --tmpfs /tmp --network none", ""},
	} {
		putFile(t, filepath.Join(cat, "x.spec"), []byte("DESCRIPTION=X\nKIND=docker\nIMAGE=img:1\nREQUIRED=TOKEN\nDOCKER_ARGS=\""+c.args+"\"\nIMAGE_ARGS=\"--privileged\"\n"), 0o644)
		_ = os.Remove(reg)
		_ = os.Remove(filepath.Join(mcp, "x", ".registered.json"))
		st, err := mcpSync(t, env, "status")
		if err != nil {
			t.Fatalf("%s: status %v\n%s", c.args, err, st)
		}
		out, err := mcpSync(t, env)
		_, regErr := os.Stat(reg)
		if c.why == "" {
			if err != nil || regErr != nil || strings.Contains(st, "refused") {
				t.Errorf("%s: should register: %v\n%s\n%s", c.args, err, st, out)
			}
			continue
		}
		if want := "x\trefused\t" + c.why + "\n"; st != want {
			t.Errorf("%s: status %q, want %q", c.args, st, want)
		}
		if err == nil || regErr == nil || !strings.Contains(out, "x: refused") {
			t.Errorf("%s: registered or no error: %v\n%s", c.args, err, out)
		}
	}
}

// None of yardo's own servers asks for a Docker option mcp-sync refuses.
func TestMCPSyncCatalogNotRefused(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	mkdirAll(t, cat, 0o755)
	var names []string
	for _, d := range []string{"mcp-catalog", "conf/mcp-catalog"} {
		specs, _ := filepath.Glob(filepath.Join(d, "*.spec"))
		for _, p := range specs {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			name := strings.TrimSuffix(filepath.Base(p), ".spec")
			putFile(t, filepath.Join(cat, name+".spec"), b, 0o644)
			req, err := exec.Command("sh", "-c", `. "$1"; echo "${REQUIRED:-}"`, "_", p).Output()
			if err != nil {
				t.Fatal(err)
			}
			env := ""
			for _, k := range strings.Fields(string(req)) {
				env += k + "=x\n"
			}
			mkdirAll(t, filepath.Join(mcp, name), 0o700)
			putFile(t, filepath.Join(mcp, name, "env"), []byte(env), 0o600)
			names = append(names, name)
		}
	}
	if len(names) < 10 {
		t.Fatalf("only %d catalog specs", len(names))
	}
	putFile(t, filepath.Join(mcp, "servers"), []byte(strings.Join(names, "\n")+"\n"), 0o600)
	bin := fakeBin(t, map[string]string{"mcpjungle": "exit 0", "docker": "exit 0"})
	jq, _ := exec.LookPath("jq")
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	out, err := mcpSync(t, env, "status")
	if err != nil || strings.Contains(out, "\trefused\t") || strings.Count(out, "\n") < len(names) {
		t.Errorf("status: %v\n%s", err, out)
	}
}

// HOST_ACCESS counts only in yardo's own catalog specs: not in a team's
// (listed in .team-files), nor in a spec of one's own in ~/.mcp.
func TestMCPSyncHostAccessOnlyYardos(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	root := t.TempDir()
	mcp, cat := filepath.Join(root, "mcp"), filepath.Join(root, "catalog")
	mkdirAll(t, cat, 0o755)
	spec := []byte("DESCRIPTION=D\nKIND=docker\nIMAGE=img:1\nDOCKER_ARGS=\"-v /var/run/docker.sock:/var/run/docker.sock\"\nHOST_ACCESS=docker-socket\n")
	putFile(t, filepath.Join(cat, "dock.spec"), spec, 0o644)
	putFile(t, filepath.Join(cat, "team.spec"), spec, 0o644)
	putFile(t, filepath.Join(cat, ".team-files"), []byte("team.spec\n"), 0o644)
	for _, n := range []string{"dock", "team", "own", "inst"} {
		mkdirAll(t, filepath.Join(mcp, n), 0o700)
		putFile(t, filepath.Join(mcp, n, "env"), nil, 0o600)
	}
	putFile(t, filepath.Join(mcp, "own", "spec"), spec, 0o600)
	if err := os.Symlink(filepath.Join(cat, "dock.spec"), filepath.Join(mcp, "inst", "spec")); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(mcp, "servers"), []byte("dock\nteam\n"), 0o600)
	bin := fakeBin(t, map[string]string{"mcpjungle": "exit 0", "docker": "exit 0"})
	jq, _ := exec.LookPath("jq")
	env := []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + root, "MCP_DIR=" + mcp, "MCP_CATALOG=" + cat, "MCPJUNGLE_REGISTRY=http://127.0.0.1:1"}
	out, err := mcpSync(t, env, "status")
	want := "dock\tunregistered\t\ninst\tunregistered\t\nown\trefused\t-v /var/run/docker.sock:/var/run/docker.sock\nteam\trefused\t-v /var/run/docker.sock:/var/run/docker.sock\n"
	if err != nil || out != want {
		t.Errorf("status: %v\n%q\nwant\n%q", err, out, want)
	}
}
