package steps

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/eugenetaranov/yardo/internal/assets"
)

// LockedSkill is a community skill in skills.lock.toml: a folder of a
// repo at a commit, its licence, and the hash of its files (treeHash).
// Fetch marks one too big to ship inside yardo: the skills step downloads
// it at the commit, and installs it only if the hash matches.
type LockedSkill struct {
	Name, Repo, Path, Commit, License string
	LicenseFile                       string `toml:"license_file"`
	Sha256                            string
	Fetch                             bool
	Files                             []string
}

// lockedSkills is the lock yardo ships, read once.
var lockedSkills = sync.OnceValues(func() (map[string]LockedSkill, error) {
	var lock struct{ Skill []LockedSkill }
	if _, err := toml.Decode(string(assets.SkillsLock()), &lock); err != nil {
		return nil, fmt.Errorf("skills.lock.toml: %w", err)
	}
	out := map[string]LockedSkill{}
	for _, s := range lock.Skill {
		out[s.Name] = s
	}
	return out, nil
})

// Where a fetched skill comes from; tests point them at a server of theirs.
var (
	githubAPI = "https://api.github.com"
	githubRaw = "https://raw.githubusercontent.com"
)

// Limits on a fetched skill, well above Archify's 23 MB in 600 files.
const (
	fetchMaxFiles = 2000
	fetchMaxBytes = 64 << 20
)

// fetchSkill downloads s's folder at its commit: the repo's file list from
// GitHub's API, then each file, each checked against the blob hash the
// list gives. Only regular files (no links, no submodules), none under a
// dot-folder (as scripts/update-skills vendors), the repo's licence as
// LICENSE when the folder has none. The files' treeHash must be the
// lock's, or nothing is returned.
func fetchSkill(ctx context.Context, s LockedSkill) (map[string][]byte, error) {
	var tree struct {
		Tree []struct {
			Path, Mode, Type, Sha string
			Size                  int64
		}
		Truncated bool
	}
	if err := getJSON(ctx, fmt.Sprintf("%s/repos/%s/git/trees/%s?recursive=1", githubAPI, s.Repo, s.Commit), &tree); err != nil {
		return nil, fmt.Errorf("skill %s: %w", s.Name, err)
	}
	if tree.Truncated {
		return nil, fmt.Errorf("skill %s: %s's file list is too long to read", s.Name, s.Repo)
	}
	type blob struct {
		rel, path, sha string
	}
	var want []blob
	var total int64
	licence := ""
	for _, t := range tree.Tree {
		if t.Path == s.LicenseFile && t.Type == "blob" {
			licence = t.Sha
		}
		rel, ok := strings.CutPrefix(t.Path, s.Path+"/")
		if !ok || t.Type == "tree" || hidden(rel) {
			continue
		}
		if t.Type != "blob" || (t.Mode != "100644" && t.Mode != "100755") {
			return nil, fmt.Errorf("skill %s: %s is a link or a submodule, which yardo doesn't install", s.Name, t.Path)
		}
		total += t.Size
		want = append(want, blob{rel, t.Path, t.Sha})
	}
	if len(want) > fetchMaxFiles || total > fetchMaxBytes {
		return nil, fmt.Errorf("skill %s: %d files, %d bytes: too big to install", s.Name, len(want), total)
	}
	if !hasLicence(want, func(b blob) string { return b.rel }) {
		if licence == "" {
			return nil, fmt.Errorf("skill %s: no %s in %s", s.Name, s.LicenseFile, s.Repo)
		}
		want = append(want, blob{"LICENSE", s.LicenseFile, licence})
	}
	files := map[string][]byte{}
	var mu sync.Mutex
	var first error
	jobs := make(chan blob)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for b := range jobs {
				data, err := getRaw(ctx, s, b.path, b.sha)
				mu.Lock()
				if err != nil && first == nil {
					first = err
				}
				files[b.rel] = data
				mu.Unlock()
			}
		})
	}
	for _, b := range want {
		jobs <- b
	}
	close(jobs)
	wg.Wait()
	if first != nil {
		return nil, fmt.Errorf("skill %s: %w", s.Name, first)
	}
	if files["SKILL.md"] == nil {
		return nil, fmt.Errorf("skill %s: no SKILL.md at %s/%s", s.Name, s.Repo, s.Path)
	}
	if got := treeHash(files); got != s.Sha256 {
		return nil, fmt.Errorf("skill %s: the files fetched for %s@%.12s don't match yardo's lock (sha256 %s, want %s): not installed", s.Name, s.Repo, s.Commit, got, s.Sha256)
	}
	return files, nil
}

// hidden says whether rel is, or is in, a dot-file or dot-folder.
func hidden(rel string) bool {
	for _, p := range strings.Split(rel, "/") {
		if strings.HasPrefix(p, ".") {
			return true
		}
	}
	return false
}

func hasLicence[T any](xs []T, rel func(T) string) bool {
	for _, x := range xs {
		if r := strings.ToUpper(rel(x)); !strings.Contains(r, "/") && (strings.HasPrefix(r, "LICENSE") || strings.HasPrefix(r, "LICENCE")) {
			return true
		}
	}
	return false
}

// getRaw downloads one file of s's repo at its commit, checked against
// its git blob hash.
func getRaw(ctx context.Context, s LockedSkill, p, sha string) ([]byte, error) {
	b, err := get(ctx, githubRaw+"/"+s.Repo+"/"+s.Commit+"/"+escapePath(p))
	if err != nil {
		return nil, err
	}
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	if hex.EncodeToString(h.Sum(nil)) != sha {
		return nil, fmt.Errorf("%s: not the file the commit has", p)
	}
	return b, nil
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(s, "%", "%25"), "#", "%23")
		parts[i] = strings.ReplaceAll(parts[i], "?", "%3F")
	}
	return path.Join(parts...)
}

func getJSON(ctx context.Context, url string, v any) error {
	b, err := get(ctx, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes+1))
	if err == nil && len(b) > fetchMaxBytes {
		err = errors.New(url + ": too big")
	}
	return b, err
}
