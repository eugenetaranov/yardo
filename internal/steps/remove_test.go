package steps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// Removing an item: its script first, then the packages installed and
// nothing else uses, its apt repo's files, its mise tools; what others
// use (and the base packages) stays.
func TestRemove(t *testing.T) {
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "dpkg-query" && c.Args[len(c.Args)-1] == "docker-compose-plugin" {
			return nil, errors.New("not installed")
		}
		return nil, nil
	}}
	e := testEnv(t, x)
	it := &catalog.Item{ID: "docker", Remove: "systemctl disable --now docker # {{user}}", RemoveSay: "stops Docker", Steps: []catalog.Step{
		{Type: "apt", Packages: []string{"ca-certificates", "curl"}},
		{Type: "apt_repo", Name: "docker"},
		{Type: "apt", Packages: []string{"docker-ce", "containerd.io", "docker-compose-plugin"}},
		{Type: "systemctl", Unit: "docker"},
		{Type: "mise", Tools: []string{"lazydocker@latest", "dive@latest"}},
	}}
	for _, f := range []string{"/etc/apt/sources.list.d/docker.sources", "/etc/apt/keyrings/docker.asc"} {
		if err := os.MkdirAll(filepath.Dir(e.Path(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(e.Path(f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	keep := KeptBy(e, []*catalog.Item{{ID: "k8s", Steps: []catalog.Step{{Type: "mise", Tools: []string{"dive"}}, {Type: "apt", Packages: []string{"containerd.io"}}}}})
	did, err := Remove(context.Background(), e, it, keep)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(x.commands(), "\n")
	for _, want := range []string{"systemctl disable --now docker # ada", "apt-get purge -y -q docker-ce", "mise unuse -g lazydocker"} {
		if !strings.Contains(got, want) {
			t.Errorf("ran no %q:\n%s", want, got)
		}
	}
	// Only docker-ce goes: curl and ca-certificates are the base's,
	// containerd.io and dive k8s', docker-compose-plugin isn't installed.
	var purge []string
	for _, c := range x.commands() {
		if strings.Contains(c, "apt-get purge") {
			purge = append(purge, c)
		}
	}
	if len(purge) != 1 || !strings.HasSuffix(purge[0], "purge -y -q docker-ce") || strings.Contains(got, "unuse -g dive") {
		t.Errorf("removed what stays: %q\n%s", purge, got)
	}
	if _, err := os.Stat(e.Path("/etc/apt/keyrings/docker.asc")); err == nil {
		t.Error("the repo's key is still there")
	}
	if len(did) == 0 {
		t.Error("said nothing removed")
	}
	// Not removable: an error, nothing run.
	x.ran = nil
	if _, err := Remove(context.Background(), e, &catalog.Item{ID: "x", Steps: []catalog.Step{{Type: "run", Cmd: "true"}}}, keep); err == nil || len(x.ran) > 0 {
		t.Errorf("removed an item it can't: %v %v", err, x.ran)
	}
}

func writeAt(t *testing.T, e *Env, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(e.Path(p)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.Path(p), []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readAt(t *testing.T, e *Env, p string) string {
	t.Helper()
	b, err := os.ReadFile(e.Path(p))
	if err != nil {
		return "<gone>"
	}
	return string(b)
}

// Removing undoes yardo's own steps: its files (when unchanged), its rc
// lines, its block, its link; what another item uses stays, and the
// plan says so.
func TestRemoveUndoes(t *testing.T) {
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "mise" && len(c.Args) > 0 && c.Args[0] == "ls" {
			return []byte(`{"aws-cli":[{"version":"2"}]}`), nil
		}
		return nil, nil
	}}
	e := testEnv(t, x)
	src := "[ -r ~/.config/yardo/aws.sh ] && . ~/.config/yardo/aws.sh"
	it := &catalog.Item{ID: "aws", Label: "AWS tools (aws CLI)", Steps: []catalog.Step{
		{Type: "mise", Tools: []string{"aws-cli@latest", "uv@latest"}},
		{Type: "apt", Packages: []string{"curl", "fzf", "mariadb-client"}},
		{Type: "file", Src: "tmux.conf", To: "~/.config/yardo/aws.sh"},
		{Type: "line", To: "~/.bashrc", Line: src},
		{Type: "line", To: "~/.zshrc", Line: src, IfExists: true},
		{Type: "block", Src: "tmux.conf", To: "~/.config/git/ignore"},
		{Type: "link", Src: "/opt/aws/bin/aws", To: "/usr/local/bin/aws"},
	}}
	writeAt(t, e, "~/.config/yardo/aws.sh", "set -g mouse on\n")
	writeAt(t, e, "~/.bashrc", "export A=1\n\n"+src+"\n")
	writeAt(t, e, "~/.zshrc", src+"\nexport Z=1\n")
	writeAt(t, e, "~/.config/git/ignore", BlockBegin+"\nset -g mouse on\n"+BlockEnd+"\n*.swp\n")
	if err := os.MkdirAll(filepath.Dir(e.Path("/usr/local/bin/aws")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/opt/aws/bin/aws", e.Path("/usr/local/bin/aws")); err != nil {
		t.Fatal(err)
	}
	keep := KeptBy(e, []*catalog.Item{{ID: "helpers", Label: "Shell helpers (git, fzf)", Steps: []catalog.Step{{Type: "apt", Packages: []string{"fzf"}}}}})
	p, err := PlanRemove(context.Background(), e, it, keep)
	if err != nil {
		t.Fatal(err)
	}
	off, kept := strings.Join(p.Off, "|"), strings.Join(p.Keep, "|")
	for _, want := range []string{"~/.config/yardo/aws.sh", "a line in ~/.bashrc", "a line in ~/.zshrc", "yardo's block in ~/.config/git/ignore", "/usr/local/bin/aws", "mariadb-client", "aws-cli"} {
		if !strings.Contains(off, want) {
			t.Errorf("plan takes off no %q: %s", want, off)
		}
	}
	// uv isn't in mise (not installed), curl is the base's: neither is
	// said; fzf stays for Shell helpers.
	if strings.Contains(off, "uv") || strings.Contains(off, "curl") || strings.Contains(off, "fzf") {
		t.Errorf("plan takes off what it shouldn't: %s", off)
	}
	if kept != "fzf (Shell helpers uses it)" {
		t.Errorf("kept %q", kept)
	}
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for f, want := range map[string]string{
		"~/.config/yardo/aws.sh": "<gone>",
		"~/.bashrc":              "export A=1\n",
		"~/.zshrc":               "export Z=1\n",
		"~/.config/git/ignore":   "*.swp\n",
	} {
		if got := readAt(t, e, f); got != want {
			t.Errorf("%s: %q, want %q", f, got, want)
		}
	}
	if exists(e.Path("/usr/local/bin/aws")) {
		t.Error("the link is still there")
	}
	// Again: nothing left to take off.
	x.ran = nil
	p, err = PlanRemove(context.Background(), e, it, keep)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Off) != 2 { // mariadb-client and aws-cli: the fakes still say they're there
		t.Errorf("second plan takes off %q", p.Off)
	}
}

// What the user changed stays: a file edited by hand, a clone with local
// changes; a file yardo replaced gets its original back; a unit another
// item runs stays up.
func TestRemoveKeepsTheUsers(t *testing.T) {
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "git" {
			return []byte(" M init.lua\n"), nil
		}
		return nil, nil
	}}
	e := testEnv(t, x)
	it := &catalog.Item{ID: "x", Steps: []catalog.Step{
		{Type: "file", Src: "tmux.conf", To: "~/.tmux.conf"},
		{Type: "file", Src: "tmux.conf", To: "/etc/x.conf", KeepOrig: true},
		{Type: "git_clone", Repo: "https://example.com/r", To: "~/.r"},
		{Type: "systemctl", Unit: "shared"},
		{Type: "systemctl", Unit: "own"},
	}}
	writeAt(t, e, "~/.tmux.conf", "set -g mouse off\n")
	writeAt(t, e, "/etc/x.conf", "set -g mouse on\n")
	writeAt(t, e, "/etc/x.conf.orig", "the distro's\n")
	writeAt(t, e, "~/.r/init.lua", "x")
	keep := KeptBy(e, []*catalog.Item{{ID: "y", Label: "Y", Steps: []catalog.Step{{Type: "systemctl", Unit: "shared"}}}})
	p, err := PlanRemove(context.Background(), e, it, keep)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.Keep, "|"); got != "the shared service (Y uses it)|~/.r (has local changes)|~/.tmux.conf (edited by hand)" {
		t.Errorf("kept %q", got)
	}
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := readAt(t, e, "/etc/x.conf"); got != "the distro's\n" {
		t.Errorf("/etc/x.conf: %q, want the original", got)
	}
	if got := readAt(t, e, "~/.tmux.conf"); got != "set -g mouse off\n" {
		t.Errorf("the edited file went: %q", got)
	}
	cmds := strings.Join(x.commands(), "\n")
	if !strings.Contains(cmds, "systemctl disable --now own") || strings.Contains(cmds, "disable --now shared") {
		t.Errorf("units:\n%s", cmds)
	}
}

func TestWithoutLine(t *testing.T) {
	for in, want := range map[string]string{
		"a\n\nL\n":    "a\n",
		"L\nb\n":      "b\n",
		"a\nL\nb\n":   "a\nb\n",
		"a\n\nL\n\nb": "a\n\nb",
		"a\n":         "a\n",
	} {
		if got := string(WithoutLine([]byte(in), "L")); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
