package addon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Approved copies: the files of each approval, kept where only root reads
// them, to tell what an update changes and to put back a skill edited
// since it was checked.

func copyDir(user, kind, name string) string {
	return filepath.Join(Base, "approved", user, kind, name)
}

// KeepCopy keeps files as kind name's approved copy (replacing one).
func KeepCopy(user, kind, name string, files map[string][]byte) error {
	if !validUser(user) || name == "" || strings.ContainsAny(name, "/\x00") || name == "." || name == ".." {
		return fmt.Errorf("not an add-on: %q %q", user, name)
	}
	d := copyDir(user, kind, name)
	for _, p := range []string{filepath.Join(Base, "approved"), filepath.Join(Base, "approved", user), filepath.Dir(d)} {
		if err := os.Mkdir(p, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	// approved/ itself is read by the gateway's list's writer only (root):
	// 0755 there, 0700 below.
	if err := os.Chmod(filepath.Join(Base, "approved", user), 0o700); err != nil {
		return err
	}
	tmp := d + ".new"
	_ = os.RemoveAll(tmp)
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return err
	}
	for rel, b := range files {
		if !cleanRel(rel) {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("%q: not a path in an add-on", rel)
		}
		to := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(to, b, 0o600); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(d); err != nil {
		return err
	}
	return os.Rename(tmp, d)
}

// ApprovedFiles are kind name's approved copy (fs.ErrNotExist: none kept,
// as for approvals from before copies were).
func ApprovedFiles(user, kind, name string) (map[string][]byte, error) {
	d := copyDir(user, kind, name)
	if _, err := os.Lstat(d); err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	err := filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		if !e.Type().IsRegular() {
			return fmt.Errorf("%s: not a file", p)
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(d, p)
		out[filepath.ToSlash(rel)] = b
		return err
	})
	return out, err
}

// Change is a file an update adds, changes or removes, with its diff.
type Change struct {
	Path string `json:"path"`
	What string `json:"what"` // added, changed, removed
	Diff string `json:"diff,omitempty"`
}

// Changes are what's different from old to now, by path.
func Changes(old, now map[string][]byte) []Change {
	var out []Change
	for _, p := range slices.Sorted(maps.Keys(now)) {
		o, had := old[p]
		switch {
		case !had:
			out = append(out, Change{p, "added", Diff(p, nil, now[p])})
		case string(o) != string(now[p]):
			out = append(out, Change{p, "changed", Diff(p, o, now[p])})
		}
	}
	for _, p := range slices.Sorted(maps.Keys(old)) {
		if _, ok := now[p]; !ok {
			out = append(out, Change{p, "removed", Diff(p, old[p], nil)})
		}
	}
	return out
}

// Diff is a unified diff of file p from a to b (3 lines of context), or
// a line saying why there's none: a binary file, or too long to diff.
func Diff(p string, a, b []byte) string {
	if !text(a) || !text(b) {
		return "(binary: no diff)"
	}
	x, y := lines(a), lines(b)
	if len(x)*len(y) > 4_000_000 {
		return fmt.Sprintf("(too long to diff: %d lines before, %d now)", len(x), len(y))
	}
	// The longest common subsequence, from the end.
	lcs := make([][]int32, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	type op struct {
		k    byte // ' ', '-', '+'
		s    string
		i, j int // lines before it, in a and b
	}
	var ops []op
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			ops = append(ops, op{' ', x[i], i, j})
			i, j = i+1, j+1
		case j < len(y) && (i == len(x) || lcs[i][j+1] >= lcs[i+1][j]):
			ops = append(ops, op{'+', y[j], i, j})
			j++
		default:
			ops = append(ops, op{'-', x[i], i, j})
			i++
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", p, p)
	for n := 0; n < len(ops); {
		if ops[n].k == ' ' {
			n++
			continue
		}
		// A hunk: 3 lines before, through changes no more than 6 apart.
		start := max(0, n-3)
		end := n
		for end < len(ops) {
			if ops[end].k != ' ' {
				end++
				continue
			}
			k := end
			for k < len(ops) && ops[k].k == ' ' {
				k++
			}
			if k == len(ops) || k-end > 6 {
				end = min(len(ops), end+3)
				break
			}
			end = k
		}
		da, db := 0, 0
		for _, o := range ops[start:end] {
			if o.k != '+' {
				da++
			}
			if o.k != '-' {
				db++
			}
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", ops[start].i+min(1, da), da, ops[start].j+min(1, db), db)
		for _, o := range ops[start:end] {
			out.WriteByte(o.k)
			out.WriteString(o.s)
			out.WriteByte('\n')
		}
		n = end
	}
	return out.String()
}

func text(b []byte) bool { return !strings.ContainsRune(string(b), 0) }

func lines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// isScript says whether file p (content b) is a script: Claude reads an
// update's new and changed ones in full, first.
func isScript(p string, b []byte) bool {
	switch path.Ext(p) {
	case ".sh", ".bash", ".zsh", ".py", ".js", ".mjs", ".cjs", ".ts":
		return true
	}
	return strings.HasPrefix(string(b), "#!")
}

// Updates: a skill's source at the ref the user gave, an image's tag.

// Outdated is an approved add-on with a newer version.
type Outdated struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Pin    string `json:"pin"`
	Latest string `json:"latest"`
}

// OutdatedFor is how long a look for updates is kept.
var OutdatedFor = time.Hour

var commitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Updatable says what an approval would update from: a skill's source
// (not a commit the user pinned), an image's tag (not a digest). "" when
// it has nothing to update from: a spec file, a URL, a skill found here.
func Updatable(a Approval) string {
	switch a.Kind {
	case "skill":
		src, err := ParseSource(a.Source)
		if err != nil || commitRe.MatchString(src.Ref) {
			return ""
		}
		return a.Source
	case "mcp":
		if !strings.Contains(a.Pin, "@sha256:") || strings.Contains(a.Source, "@sha256:") || strings.Contains(a.Source, "://") || !imageRef.MatchString(a.Source) {
			return ""
		}
		return a.Source
	}
	return ""
}

// Latest is what a's source is now: a commit, or repo@digest.
func Latest(ctx context.Context, a Approval) (string, error) {
	from := Updatable(a)
	if from == "" {
		return "", errors.New("nothing to update from")
	}
	if a.Kind == "skill" {
		src, _ := ParseSource(from)
		return resolve(ctx, src)
	}
	// The tag's digest, from the registry, nothing pulled.
	out, err := exec.CommandContext(ctx, Docker, "buildx", "imagetools", "inspect", "--format", "{{json .Manifest}}", from).Output()
	if err != nil {
		return "", fmt.Errorf("docker buildx imagetools inspect %s: %s", from, lastLine(out, err))
	}
	var m struct {
		Digest string `json:"digest"`
	}
	if json.Unmarshal(out, &m) != nil || !strings.HasPrefix(m.Digest, "sha256:") {
		return "", fmt.Errorf("%s: no digest in the registry's answer", from)
	}
	repo, _, _ := strings.Cut(a.Pin, "@")
	return fmt.Sprintf("%s@%s", repo, m.Digest), nil
}

func outdatedFile() string { return filepath.Join(Base, "addons-outdated.json") }

type outdatedCache struct {
	At     time.Time         `json:"at"`
	Latest map[string]string `json:"latest"` // kind/name@pin → latest
}

// FindOutdated looks for updates of user's approved add-ons: kept an hour
// (fresh: looked for again). errs are the ones it couldn't tell, by name.
func FindOutdated(ctx context.Context, user string, fresh bool) ([]Outdated, map[string]error) {
	as, err := Approvals(user)
	if err != nil {
		return nil, map[string]error{"": err}
	}
	var c outdatedCache
	if b, err := os.ReadFile(outdatedFile()); err == nil && !fresh {
		_ = json.Unmarshal(b, &c)
	}
	if time.Since(c.At) > OutdatedFor || c.Latest == nil {
		c = outdatedCache{At: time.Now().UTC(), Latest: map[string]string{}}
	}
	errs := map[string]error{}
	var out []Outdated
	changed := false
	for _, a := range as {
		if Updatable(a) == "" {
			continue
		}
		key := fmt.Sprintf("%s/%s@%s", a.Kind, a.Name, a.Pin)
		l, ok := c.Latest[key]
		if !ok {
			if l, err = Latest(ctx, a); err != nil {
				errs[a.Name] = err
				continue
			}
			c.Latest[key], changed = l, true
		}
		if l != a.Pin {
			out = append(out, Outdated{a.Kind, a.Name, a.Pin, l})
		}
	}
	if changed {
		if b, err := json.Marshal(c); err == nil && os.MkdirAll(Base, 0o700) == nil {
			_ = os.WriteFile(outdatedFile(), b, 0o600)
		}
	}
	return out, errs
}

// SetTools records server name's tool list hash (a remote one's, once
// registered): mcp-sync turns it off when the list isn't that one.
func SetTools(user, name, hash string) error {
	as, err := Approvals(user)
	if err != nil {
		return err
	}
	for n := range as {
		if as[n].Kind == "mcp" && as[n].Name == name {
			as[n].Tools = hash
			return writeRecord(user, as)
		}
	}
	return fmt.Errorf("no approved server %s", name)
}

// What says what was checked, to read: a skill's source, the ref it was
// asked for and the commit; an image's tag and digest; else the hash of
// the spec (or files) checked. Commits and digests are cut to 7.
func What(kind, source, pin, hash string) string {
	short := func(s string) string { return s[:min(7, len(s))] }
	if kind == "skill" {
		src, err := ParseSource(source)
		switch {
		case err != nil && hash != "":
			return fmt.Sprintf("%s · content %s", source, short(hash))
		case err != nil:
			return source
		case commitRe.MatchString(src.Ref):
			src.Ref = ""
			return fmt.Sprintf("%s · commit %s", src, short(pin))
		}
		ref := src.Ref
		if ref == "" {
			ref = "default branch"
		}
		src.Ref = ""
		return fmt.Sprintf("%s · %s · commit %s", src, ref, short(pin))
	}
	if repo, d, ok := strings.Cut(pin, "@sha256:"); ok {
		tag := source
		switch {
		case strings.Contains(source, "@sha256:"):
			tag = repo
		case !strings.Contains(source[strings.LastIndex(source, "/")+1:], ":"):
			tag = source + ":latest"
		}
		return fmt.Sprintf("%s · sha256:%s", tag, short(d))
	}
	if hash == "" {
		return source
	}
	return fmt.Sprintf("%s · spec %s", source, short(hash))
}
