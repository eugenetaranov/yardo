package steps

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

func blobSha(b []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// fakeGitHub serves a repo's tree at commit "c0" and its files, as
// GitHub's API and raw hosts do; mode overrides a path's mode.
func fakeGitHub(t *testing.T, files map[string]string, mode map[string]string) {
	t.Helper()
	type entry struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		Sha  string `json:"sha"`
		Size int    `json:"size"`
	}
	var tree []entry
	for p, c := range files {
		m, typ := "100644", "blob"
		if mode[p] != "" {
			m = mode[p]
		}
		if m == "160000" {
			typ = "commit"
		}
		tree = append(tree, entry{p, m, typ, blobSha([]byte(c)), len(c)})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/o/r/git/trees/c0":
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": tree})
		case strings.HasPrefix(r.URL.Path, "/o/r/c0/"):
			c, ok := files[strings.TrimPrefix(r.URL.Path, "/o/r/c0/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, c)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	api, raw := githubAPI, githubRaw
	githubAPI, githubRaw = srv.URL, srv.URL
	t.Cleanup(func() { githubAPI, githubRaw = api, raw })
}

// A fetched skill: only its folder's regular files (not dot-files), the
// repo's licence when it has none, and only when the hash is the lock's.
func TestFetchSkill(t *testing.T) {
	fakeGitHub(t, map[string]string{
		"LICENSE":             "Apache\n",
		"other/x.md":          "x",
		"sk/SKILL.md":         "---\nname: sk\n---\nhi\n",
		"sk/bin/run.mjs":      "#!/usr/bin/env node\n",
		"sk/.github/ci.yaml":  "x",
		"sk/assets/style.css": "body{}",
	}, map[string]string{"sk/bin/run.mjs": "100755"})
	want := map[string][]byte{"SKILL.md": []byte("---\nname: sk\n---\nhi\n"), "bin/run.mjs": []byte("#!/usr/bin/env node\n"), "assets/style.css": []byte("body{}"), "LICENSE": []byte("Apache\n")}
	s := LockedSkill{Name: "sk", Repo: "o/r", Path: "sk", Commit: "c0", LicenseFile: "LICENSE", Sha256: treeHash(want), Fetch: true}
	got, err := fetchSkill(context.Background(), s)
	if err != nil || treeHash(got) != treeHash(want) || len(got) != len(want) {
		t.Fatalf("fetch = %v, %v", keys(got), err)
	}
	s.Sha256 = strings.Repeat("0", 64)
	if _, err := fetchSkill(context.Background(), s); err == nil || !strings.Contains(err.Error(), "don't match yardo's lock") {
		t.Errorf("wrong hash: %v", err)
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A link or a submodule in the folder: nothing fetched.
func TestFetchSkillRefusesLinks(t *testing.T) {
	for _, m := range []string{"120000", "160000"} {
		fakeGitHub(t, map[string]string{"sk/SKILL.md": "x", "sk/evil": "/etc/passwd"}, map[string]string{"sk/evil": m})
		_, err := fetchSkill(context.Background(), LockedSkill{Name: "sk", Repo: "o/r", Path: "sk", Commit: "c0", LicenseFile: "LICENSE"})
		if err == nil || !strings.Contains(err.Error(), "a link or a submodule") {
			t.Errorf("%s: %v", m, err)
		}
	}
}

// The skills step installs a fetched skill like a shipped one (marked, in
// both folders), Check knows it's there without fetching, and a hash that
// doesn't match installs nothing.
func TestSkillsFetched(t *testing.T) {
	files := map[string]string{"LICENSE": "L", "sk/SKILL.md": "---\nname: sk\n---\nhi\n"}
	fakeGitHub(t, files, nil)
	hash := treeHash(map[string][]byte{"SKILL.md": []byte(files["sk/SKILL.md"]), "LICENSE": []byte("L")})
	prev := lockedSkills
	lockedSkills = func() (map[string]LockedSkill, error) {
		return map[string]LockedSkill{"sk": {Name: "sk", Repo: "o/r", Path: "sk", Commit: "c0", LicenseFile: "LICENSE", Sha256: hash, Fetch: true}}, nil
	}
	t.Cleanup(func() { lockedSkills = prev })
	e := testEnv(t, &fakeExec{})
	e.Files = fstest.MapFS{}
	st := step(t, catalog.Step{Type: "go:skills", Skills: []string{"sk"}})
	if check(t, e, st) {
		t.Fatal("Check = true before Apply")
	}
	apply(t, e, st)
	for _, d := range SkillDirs {
		if got := read(t, e.Path(d+"/sk/SKILL.md")); got != files["sk/SKILL.md"] {
			t.Errorf("%s: %q", d, got)
		}
	}
	githubAPI = "http://127.0.0.1:1" // Check doesn't fetch
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
	// A lock the files don't match: nothing written, and it says why.
	lockedSkills = func() (map[string]LockedSkill, error) {
		return map[string]LockedSkill{"sk": {Name: "sk", Repo: "o/r", Path: "sk", Commit: "c0", LicenseFile: "LICENSE", Sha256: strings.Repeat("1", 64), Fetch: true}}, nil
	}
	fakeGitHub(t, files, nil)
	e = testEnv(t, &fakeExec{})
	e.Files = fstest.MapFS{}
	if err := st.Apply(context.Background(), e); err == nil || !strings.Contains(err.Error(), "don't match") {
		t.Errorf("apply: %v", err)
	}
	if _, err := os.Stat(e.Path("~/.agents/skills/sk")); err == nil {
		t.Error("installed anyway")
	}
}

// Archify as locked, from GitHub itself (YARDO_LIVE_FETCH=1): the Go fetch
// and scripts/update-skills agree on its hash.
func TestFetchArchifyLive(t *testing.T) {
	if os.Getenv("YARDO_LIVE_FETCH") == "" {
		t.Skip("YARDO_LIVE_FETCH not set")
	}
	lock, err := lockedSkills()
	if err != nil {
		t.Fatal(err)
	}
	files, err := fetchSkill(context.Background(), lock["archify"])
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("archify: %d files", len(files))
}
