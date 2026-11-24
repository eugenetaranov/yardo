package addon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDocker is a docker that records its calls (read by the func it
// returns) and inspects as inspect says.
func fakeDocker(t *testing.T, inspect string) func() string {
	t.Helper()
	d := t.TempDir()
	log := filepath.Join(d, "calls")
	body := fmt.Sprintf("#!/bin/sh\necho \"$*\" >> %s\ncase \"$1 $2\" in 'image inspect') printf '%%s' '%s' ;; esac\n", log, inspect)
	if err := os.WriteFile(filepath.Join(d, "docker"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	old := Docker
	Docker = filepath.Join(d, "docker")
	t.Cleanup(func() { Docker = old })
	return func() string { b, _ := os.ReadFile(log); return string(b) }
}

// An image is pulled and inspected, pinned to its digest; nothing else
// is asked of docker (no create, no run).
func TestPullImage(t *testing.T) {
	digest := "sha256:" + strings.Repeat("d", 64)
	calls := fakeDocker(t, fmt.Sprintf("{\"RepoDigests\":[\"ghcr.io/acme/mcp-x@%s\"],\"Config\":{\"User\":\"\"}}", digest))
	img, err := PullImage(context.Background(), "ghcr.io/acme/mcp-x:1.2")
	if err != nil {
		t.Fatal(err)
	}
	if img.Ref != "ghcr.io/acme/mcp-x@"+digest || img.User != "" || img.New {
		t.Errorf("image: %+v", img)
	}
	for _, l := range strings.Split(strings.TrimSpace(calls()), "\n") {
		if !strings.HasPrefix(l, "pull ") && !strings.HasPrefix(l, "image inspect ") {
			t.Errorf("docker %s", l)
		}
	}
	fakeDocker(t, `{"RepoDigests":[],"Config":{}}`)
	if _, err := PullImage(context.Background(), "local:dev"); err == nil || !strings.Contains(err.Error(), "no digest") {
		t.Errorf("no digest: %v", err)
	}
	if _, err := PullImage(context.Background(), "x; rm -rf /"); err == nil {
		t.Error("a name with a command: pulled")
	}
	if _, err := CheckURL("http://example.com/mcp"); err == nil || !strings.Contains(err.Error(), "must be https") {
		t.Errorf("http: %v", err)
	}
	if u, err := CheckURL("https://example.com/mcp"); err != nil || u != "https://example.com/mcp" {
		t.Errorf("https: %q %v", u, err)
	}
}
