package addon

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type entry struct {
	name string
	kind byte
	body string
}

func tarball(t *testing.T, top string, es []entry) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, e := range es {
		h := &tar.Header{Name: top + "/" + e.name, Typeflag: e.kind, Mode: 0o644, Size: int64(len(e.body))}
		if e.kind == tar.TypeSymlink {
			h.Linkname, h.Size = "/etc/passwd", 0
		}
		if e.kind == tar.TypeDir {
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			_, _ = tw.Write([]byte(e.body))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}

// fakeGitHub answers a ref with sha, and the tarball at sha with tgz.
func fakeGitHub(t *testing.T, sha string, tgz []byte) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/acme/skills/commits/"):
			if r.Header.Get("Accept") != "application/vnd.github.sha" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(sha))
		case r.URL.Path == "/acme/skills/tar.gz/"+sha:
			_, _ = w.Write(tgz)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	oldA, oldC := GitHubAPI, Codeload
	GitHubAPI, Codeload = srv.URL, srv.URL
	t.Cleanup(func() { GitHubAPI, Codeload = oldA, oldC })
}

// A branch is resolved to its commit, which is what's fetched; each
// SKILL.md folder under the path is a skill offered; a link is named,
// not fetched.
func TestFetchSkills(t *testing.T) {
	sha := strings.Repeat("e", 40)
	fakeGitHub(t, sha, tarball(t, "skills-"+sha, []entry{
		{"README.md", tar.TypeReg, "top"},
		{"skills/", tar.TypeDir, ""},
		{"skills/a/SKILL.md", tar.TypeReg, "---\nname: a\n---\n"},
		{"skills/a/ref.md", tar.TypeReg, "more"},
		{"skills/a/.yardo-skill", tar.TypeReg, "fake mark"},
		{"skills/b/SKILL.md", tar.TypeReg, "---\nname: b\n---\n"},
		{"skills/b/key", tar.TypeSymlink, ""},
		{"skills/c/SKILL.md", tar.TypeReg, "---\nname: c\n---\n"},
	}))
	src, err := ParseSource("acme/skills/skills@main")
	if err != nil {
		t.Fatal(err)
	}
	f, err := FetchSkills(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if f.Commit != sha || len(f.Skills) != 3 {
		t.Fatalf("commit %s, skills %v", f.Commit, f.Skills)
	}
	if string(f.Skills["a"]["ref.md"]) != "more" || f.Skills["a"][".yardo-skill"] != nil || f.Skills["a"]["README.md"] != nil {
		t.Errorf("a: %q", f.Skills["a"])
	}
	if len(f.Odd["b"]) != 1 || f.Odd["b"][0] != "key" || f.Skills["b"]["key"] != nil {
		t.Errorf("b's link: %v %q", f.Odd, f.Skills["b"])
	}
	// One skill at a path: named after its folder.
	f, err = FetchSkills(context.Background(), Source{Owner: "acme", Repo: "skills", Path: "skills/c"})
	if err != nil || len(f.Skills) != 1 || f.Skills["c"] == nil {
		t.Errorf("one: %v %v", err, f)
	}
}

// An entry that leaves the archive refuses it all.
func TestFetchSkillsDotDot(t *testing.T) {
	sha := strings.Repeat("f", 40)
	fakeGitHub(t, sha, tarball(t, "skills-"+sha, []entry{{"a/SKILL.md", tar.TypeReg, "x"}, {"a/../../etc/cron.d/x", tar.TypeReg, "x"}}))
	if _, err := FetchSkills(context.Background(), Source{Owner: "acme", Repo: "skills"}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("err = %v", err)
	}
}

func TestParseSource(t *testing.T) {
	for in, want := range map[string]Source{
		"acme/skills":      {Owner: "acme", Repo: "skills"},
		"acme/skills/a@v1": {Owner: "acme", Repo: "skills", Path: "a", Ref: "v1"},
		"https://github.com/acme/skills/tree/main/x/y": {Owner: "acme", Repo: "skills", Path: "x/y", Ref: "main"},
	} {
		got, err := ParseSource(in)
		if err != nil || got != want {
			t.Errorf("%s: %+v %v", in, got, err)
		}
	}
	for _, bad := range []string{"acme", "acme/skills/../x", "a b/c", "acme/skills@x y"} {
		if _, err := ParseSource(bad); err == nil {
			t.Errorf("%q: parsed", bad)
		}
	}
}
