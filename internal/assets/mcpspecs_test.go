package assets

import (
	"io/fs"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"testing"
)

func specNames(t *testing.T, dir string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".spec") {
			names[strings.TrimSuffix(e.Name(), ".spec")] = true
		}
	}
	return names
}

// yardo's own MCP specs sit next to the synced tack-roles ones; a name in
// both would make one silently replace the other on the VM.
func TestMCPSpecNamesDistinct(t *testing.T) {
	tack, own := specNames(t, "mcp-catalog"), specNames(t, "conf/mcp-catalog")
	for n := range own {
		if tack[n] {
			t.Errorf("MCP spec %q is in both assets/mcp-catalog (tack-roles) and conf/mcp-catalog (yardo)", n)
		}
	}
	for _, want := range []string{"github", "postgres", "playwright", "linear", "grafana", "opensearch", "figma", "supabase", "slack", "azure", "docker", "mysql", "k8s"} {
		if !own[want] {
			t.Errorf("conf/mcp-catalog has no %s.spec", want)
		}
	}
}

// Each yardo spec parses, pins its image, and its skeleton has every
// required key with a comment saying where to get it.
func TestMCPSpecsValid(t *testing.T) {
	bash := bashPath(t)
	for n := range specNames(t, "conf/mcp-catalog") {
		p := path.Join("conf/mcp-catalog", n+".spec")
		out, err := exec.Command(bash, "-c", `set -eu; . "$1"; printf '%s\n%s\n%s\n%s\n---\n' "$KIND" "${IMAGE:-}" "${URL:-}" "$REQUIRED"; skeleton`, "_", p).CombinedOutput()
		if err != nil {
			t.Errorf("%s: %v\n%s", n, err, out)
			continue
		}
		head, skel, _ := strings.Cut(string(out), "---\n")
		f := strings.Split(head, "\n")
		kind, image, url, required := f[0], f[1], f[2], strings.Fields(f[3])
		switch kind {
		case "docker":
			if i := strings.LastIndex(image, ":"); i < 0 || image[i+1:] == "latest" {
				t.Errorf("%s: image %q is not pinned to a tag", n, image)
			}
		case "http":
			if !strings.HasPrefix(url, "https://") {
				t.Errorf("%s: URL %q is not https", n, url)
			}
		default:
			t.Errorf("%s: KIND %q", n, kind)
		}
		for _, k := range required {
			if !strings.Contains(skel, "\n"+k+"=\n") && !strings.HasPrefix(skel, k+"=\n") {
				t.Errorf("%s: skeleton has no empty %s= line", n, k)
			}
		}
		if len(required) > 0 && !strings.Contains(skel, "https://") {
			t.Errorf("%s: skeleton doesn't say where to get the credential", n)
		}
	}
}

// Every spec, yardo's and the vendored ones, pins what it runs: an image
// with a tag that isn't latest, and a package run with npx -y or uvx at
// an exact version.
func TestMCPSpecsPinned(t *testing.T) {
	run := regexp.MustCompile(`(?:npx -y|uvx) +(@?[a-z0-9][a-z0-9._/-]*)(@[0-9]\S*)?`)
	for _, dir := range []string{"conf/mcp-catalog", "mcp-catalog"} {
		for n := range specNames(t, dir) {
			b, err := fs.ReadFile(files, path.Join(dir, n+".spec"))
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			if m := regexp.MustCompile(`(?m)^IMAGE=(\S+)`).FindStringSubmatch(s); m != nil {
				if i := strings.LastIndex(m[1], ":"); i < 0 || m[1][i+1:] == "latest" {
					t.Errorf("%s/%s: image %q is not pinned to a tag", dir, n, m[1])
				}
			}
			// What image_args runs is a line each: uvx, then the package.
			s = regexp.MustCompile(`echo (npx -y|uvx)\n\s*echo `).ReplaceAllString(s, "$1 ")
			for _, l := range strings.Split(s, "\n") {
				if strings.HasPrefix(strings.TrimSpace(l), "#") {
					continue
				}
				for _, m := range run.FindAllStringSubmatch(l, -1) {
					if m[2] == "" {
						t.Errorf("%s/%s: %q runs without an exact version", dir, n, m[1])
					}
				}
			}
		}
	}
}
