package steps

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

func init() { RegisterGo("skills", func(s catalog.Step) Step { return skillsStep{s} }) }

// skillsStep installs agent skills (agentskills.io: a folder with a
// SKILL.md) from conf/skills/<name>, for every agent: ~/.agents/skills is
// read by Codex, Gemini CLI, OpenCode, Goose, pi and omp, ~/.claude/skills
// by Claude Code. yardo marks each folder it writes (skillMark: the hash
// of what it wrote) and changes only folders that still match: one the
// user edited, or made, stays theirs.
type skillsStep struct{ s catalog.Step }

// SkillDirs are where skills go, one folder per skill in each.
var SkillDirs = []string{"~/.agents/skills", "~/.claude/skills"}

const skillMark = ".yardo-skill"

// skillName is a name agentskills.io allows: lowercase letters, digits and
// single hyphens, 1–64 characters. It's a folder yardo writes as root, so
// nothing else (a "..", a "/") may get that far.
var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidSkillName says whether n is a skill name yardo installs.
func ValidSkillName(n string) bool { return len(n) <= 64 && skillName.MatchString(n) }

func checkNames(names []string) error {
	for _, n := range names {
		if !ValidSkillName(n) {
			return fmt.Errorf("skill %q: not a valid name (lowercase letters, digits and hyphens, at most 64)", n)
		}
	}
	return nil
}

// skillLink is the first link in the way of skill name in d: ~/.agents or
// ~/.claude, the skills folder, the skill's folder, or anything in it.
// yardo writes as root: a link there could send its writes anywhere
// (~/.agents/skills → /etc/systemd/system), so it writes nothing there.
func skillLink(e *Env, d, name string) string {
	for _, p := range []string{path.Dir(d), d, path.Join(d, name)} {
		if fi, err := os.Lstat(e.Path(p)); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
			return p
		}
	}
	found := ""
	dir := e.Path(path.Join(d, name))
	_ = filepath.WalkDir(dir, func(p string, de fs.DirEntry, err error) error {
		if err == nil && de.Type()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(dir, p)
			found = path.Join(d, name, filepath.ToSlash(rel))
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func (k skillsStep) Plan(*Env) []Action {
	var out []Action
	for _, n := range k.s.Skills {
		out = append(out, Action{"file", strings.Join(SkillDirs, ", ") + ": " + n + "/SKILL.md"})
	}
	return out
}

// shipped are a skill's files (path within its folder → content).
func shipped(e *Env, name string) (map[string][]byte, error) {
	root := path.Join("skills", name)
	files := map[string][]byte{}
	err := fs.WalkDir(e.Files, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(e.Files, p)
		files[strings.TrimPrefix(p, root+"/")] = b
		return err
	})
	if err == nil && files["SKILL.md"] == nil {
		err = errors.New("skill " + name + " has no SKILL.md")
	}
	return files, err
}

// treeHash is the hash of files: their paths and contents, in order.
func treeHash(files map[string][]byte) string {
	h := sha256.New()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		h.Write([]byte(n + "\x00"))
		h.Write(files[n])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// onDisk are the files in dir (but the mark), and its mark; nil when dir
// isn't there.
func onDisk(dir string) (map[string][]byte, string, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, "", nil
	}
	files := map[string][]byte{}
	mark := ""
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		if rel == skillMark {
			mark = strings.TrimSpace(string(b))
		} else {
			files[filepath.ToSlash(rel)] = b
		}
		return err
	})
	return files, mark, err
}

// wantHash is the treeHash of skill name as yardo installs it: the copy
// shipped inside yardo, or, for one fetched at install, its lock's.
func wantHash(e *Env, name string) (string, error) {
	if lock, err := lockedSkills(); err == nil && lock[name].Fetch {
		return lock[name].Sha256, nil
	}
	files, err := shipped(e, name)
	if err != nil {
		return "", err
	}
	return treeHash(files), nil
}

// skillFiles are skill name's files to install: shipped inside yardo, or
// fetched at the lock's commit (and checked against its hash).
func skillFiles(ctx context.Context, e *Env, name string) (map[string][]byte, error) {
	if lock, err := lockedSkills(); err == nil && lock[name].Fetch {
		return fetchSkill(ctx, lock[name])
	}
	return shipped(e, name)
}

// state is what to do with a skill's folder: "ok" (as shipped), "write"
// (missing, or yardo's and out of date) or "theirs" (edited or made by the
// user: left alone).
func skillState(dir, want string) (string, error) {
	cur, mark, err := onDisk(dir)
	switch {
	case err != nil:
		return "", err
	case cur == nil:
		return "write", nil
	case mark == "" || treeHash(cur) != mark:
		return "theirs", nil
	case mark == want:
		return "ok", nil
	}
	return "write", nil
}

func (k skillsStep) Check(_ context.Context, e *Env) (bool, error) {
	if err := checkNames(k.s.Skills); err != nil {
		return false, err
	}
	for _, n := range k.s.Skills {
		want, err := wantHash(e, n)
		if err != nil {
			return false, err
		}
		for _, d := range SkillDirs {
			if skillLink(e, d, n) != "" {
				continue // not yardo's to write: Apply says so
			}
			st, err := skillState(e.Path(path.Join(d, n)), want)
			if err != nil || st == "write" {
				return false, err
			}
		}
	}
	return true, nil
}

func (k skillsStep) Apply(ctx context.Context, e *Env) error {
	if err := checkNames(k.s.Skills); err != nil {
		return err
	}
	for _, n := range k.s.Skills {
		want, err := wantHash(e, n)
		if err != nil {
			return err
		}
		var files map[string][]byte // fetched only when written
		for _, d := range SkillDirs {
			if l := skillLink(e, d, n); l != "" {
				e.note(fmt.Sprintf("%s is a link: yardo didn't write skill %s through it (put a folder there to have it)", l, n))
				continue
			}
			st, err := skillState(e.Path(path.Join(d, n)), want)
			if err != nil {
				return err
			}
			if st != "write" {
				continue
			}
			if files == nil {
				if files, err = skillFiles(ctx, e, n); err != nil {
					return err
				}
			}
			if err := k.write(e, d, n, files); err != nil {
				return err
			}
		}
	}
	return nil
}

// PlanUndo plans deleting the skill folders yardo wrote that still are
// as it wrote them; an edited one stays, and a folder of the user's own
// (no mark) isn't yardo's to mention.
func (k skillsStep) PlanUndo(_ context.Context, e *Env, p *RemovePlan) error {
	for _, n := range k.s.Skills {
		for _, d := range SkillDirs {
			dir := e.Path(path.Join(d, n))
			cur, mark, err := onDisk(dir)
			switch {
			case err != nil:
				return err
			case cur == nil || mark == "":
			case skillLink(e, d, n) != "":
				p.Keep = append(p.Keep, path.Join(d, n)+" (a link is in the way)")
			case treeHash(cur) != mark:
				p.Keep = append(p.Keep, path.Join(d, n)+" (edited by hand)")
			default:
				p.off(path.Join(d, n), func(context.Context) error { return os.RemoveAll(dir) })
			}
		}
	}
	return nil
}

// write puts the skill's files in d/name (replacing yardo's old copy),
// marked, owned by the user. Everything goes through an os.Root on d, so
// neither a link nor a ".." in a file's path takes a write out of it.
func (skillsStep) write(e *Env, d, name string, files map[string][]byte) error {
	// The folders it's in (~/.agents, ~/.agents/skills) are the user's.
	if err := os.MkdirAll(e.Path(path.Dir(path.Dir(d))), 0o755); err != nil {
		return err
	}
	for _, p := range []string{path.Dir(d), d} {
		if err := os.Mkdir(e.Path(p), 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		if err := e.chown(e.Path(p), e.UID, e.GID); err != nil {
			return err
		}
	}
	root, err := os.OpenRoot(e.Path(d))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := root.RemoveAll(name); err != nil {
		return err
	}
	put := func(rel string, b []byte, mode fs.FileMode) error {
		p := path.Join(name, rel)
		if err := root.MkdirAll(path.Dir(p), 0o755); err != nil {
			return err
		}
		return root.WriteFile(p, b, mode)
	}
	for rel, b := range files {
		mode := fs.FileMode(0o644)
		if bytes.HasPrefix(b, []byte("#!")) {
			mode = 0o755
		}
		if err := put(rel, b, mode); err != nil {
			return err
		}
	}
	if err := put(skillMark, []byte(treeHash(files)+"\n"), 0o644); err != nil {
		return err
	}
	if !e.Chown {
		return nil
	}
	return fs.WalkDir(root.FS(), name, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return root.Lchown(p, e.UID, e.GID)
	})
}
