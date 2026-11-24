package addon

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Where skills are fetched from; tests point them at a server of theirs.
var (
	GitHubAPI = "https://api.github.com"
	Codeload  = "https://codeload.github.com"
	// Token is a GitHub token to send (gh's, for the rate limit), or "".
	Token = func() string { return "" }
)

// Limits on a fetched source (the basic checks hold each skill to less).
const (
	maxFetch = 64 << 20
	maxFiles = 5000
)

// Source is a GitHub source of skills: owner/repo[/path][@ref].
type Source struct{ Owner, Repo, Path, Ref string }

var ghName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ParseSource reads s (owner/repo[/path][@ref], or the same as a
// github.com URL).
func ParseSource(s string) (Source, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "https://"), "github.com/")
	s, ref, _ := strings.Cut(s, "@")
	parts := strings.Split(strings.Trim(s, "/"), "/")
	if len(parts) < 2 || !ghName.MatchString(parts[0]) || !ghName.MatchString(parts[1]) {
		return Source{}, fmt.Errorf("%q: give owner/repo[/path][@ref]", s)
	}
	src := Source{Owner: parts[0], Repo: strings.TrimSuffix(parts[1], ".git"), Ref: ref}
	if len(parts) > 2 {
		// A browser URL's tree/<branch>/ isn't part of the path.
		rest := parts[2:]
		if len(rest) >= 2 && (rest[0] == "tree" || rest[0] == "blob") && src.Ref == "" {
			src.Ref, rest = rest[1], rest[2:]
		}
		src.Path = strings.Join(rest, "/")
		if src.Path != "" && !cleanRel(src.Path) {
			return Source{}, fmt.Errorf("%q: not a path in the repo", src.Path)
		}
	}
	if strings.ContainsAny(src.Ref, " \t\n?#") {
		return Source{}, fmt.Errorf("%q: not a ref", src.Ref)
	}
	return src, nil
}

func (s Source) String() string {
	out := fmt.Sprintf("%s/%s", s.Owner, s.Repo)
	if s.Path != "" {
		out += "/" + s.Path
	}
	if s.Ref != "" {
		out += "@" + s.Ref
	}
	return out
}

// Fetched are the skills a source holds at its commit.
type Fetched struct {
	Commit string
	// Skills are each folder with a SKILL.md (its name: the folder's, or
	// the repo's at the top), path → content.
	Skills map[string]map[string][]byte
	// Odd are, per skill, entries that aren't regular files.
	Odd map[string][]string
}

// FetchSkills resolves src's ref to a commit, downloads the repo at that
// commit and keeps the skills under src.Path: regular files only, read in
// Go, nothing run. An entry with an absolute path or a ".." is refused.
func FetchSkills(ctx context.Context, src Source) (*Fetched, error) {
	commit, err := resolve(ctx, src)
	if err != nil {
		return nil, err
	}
	body, err := open(ctx, fmt.Sprintf("%s/%s/%s/tar.gz/%s", Codeload, src.Owner, src.Repo, commit), "")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	defer func() { _ = body.Close() }()
	files, odd, err := untar(io.LimitReader(body, maxFetch+1), src.Path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	f := &Fetched{Commit: commit, Skills: map[string]map[string][]byte{}, Odd: map[string][]string{}}
	for p := range files {
		if path.Base(p) != "SKILL.md" {
			continue
		}
		d := path.Dir(p)
		name := path.Base(d)
		if d == "." {
			name, d = path.Base(cmpOr(src.Path, src.Repo)), ""
		}
		f.Skills[name] = under(files, d)
		for _, o := range odd {
			if rel, ok := inside(o, d); ok {
				f.Odd[name] = append(f.Odd[name], rel)
			}
		}
	}
	if len(f.Skills) == 0 {
		return nil, fmt.Errorf("%s: no SKILL.md at %.12s", src, commit)
	}
	return f, nil
}

// resolve is the commit src's ref (else the default branch) is at.
func resolve(ctx context.Context, src Source) (string, error) {
	ref := src.Ref
	if ref == "" {
		ref = "HEAD"
	}
	sha, err := get(ctx, fmt.Sprintf("%s/repos/%s/%s/commits/%s", GitHubAPI, src.Owner, src.Repo, ref), "application/vnd.github.sha", 1<<10)
	if err != nil {
		return "", fmt.Errorf("%s: resolving %s: %w", src, ref, err)
	}
	commit := strings.TrimSpace(string(sha))
	if !commitRe.MatchString(commit) {
		return "", fmt.Errorf("%s: %s isn't a commit GitHub can name", src, ref)
	}
	return commit, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// inside is p relative to folder d ("" is the top), when it's in it.
func inside(p, d string) (string, bool) {
	if d == "" {
		return p, true
	}
	return strings.CutPrefix(p, d+"/")
}

// under are files' files in folder d, relative to it; yardo's own mark
// isn't one a skill can bring.
func under(files map[string][]byte, d string) map[string][]byte {
	out := map[string][]byte{}
	for p, b := range files {
		if rel, ok := inside(p, d); ok && rel != ".yardo-skill" {
			out[rel] = b
		}
	}
	return out
}

// untar reads a GitHub tarball (one top folder), keeping what's under
// keep: regular files by path, the rest (links, devices) named in odd.
func untar(r io.Reader, keep string) (map[string][]byte, []string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, nil, err
	}
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	var odd []string
	total := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		_, p, _ := strings.Cut(h.Name, "/") // the top folder: repo-commit/
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			continue
		}
		if !cleanRel(p) {
			return nil, nil, fmt.Errorf("the archive has %q, a path outside it: refused", h.Name)
		}
		if keep != "" {
			rel, ok := inside(p, keep)
			if !ok {
				continue
			}
			p = rel
		}
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		default:
			odd = append(odd, p)
			continue
		}
		if len(files) >= maxFiles {
			return nil, nil, fmt.Errorf("more than %d files: too big", maxFiles)
		}
		b, err := io.ReadAll(io.LimitReader(tr, maxFetch+1))
		if err != nil {
			return nil, nil, err
		}
		if total += len(b); total > maxFetch {
			return nil, nil, errors.New("more than 64 MB: too big")
		}
		files[p] = b
	}
	slices.Sort(odd)
	return files, odd, nil
}

func get(ctx context.Context, url, accept string, limit int64) ([]byte, error) {
	body, err := open(ctx, url, accept)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	return io.ReadAll(io.LimitReader(body, limit))
}

func open(ctx context.Context, url, accept string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if t := Token(); t != "" && strings.HasPrefix(url, GitHubAPI) {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("GitHub said %s: its limit without a login is 60 requests an hour (gh auth login raises it)", resp.Status)
		}
		return nil, fmt.Errorf("GitHub said %s", resp.Status)
	}
	return resp.Body, nil
}
