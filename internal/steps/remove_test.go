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
