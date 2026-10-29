package steps

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
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

// state is what to do with a skill's folder: "ok" (as shipped), "write"
// (missing, or yardo's and out of date) or "theirs" (edited or made by the
// user: left alone).
func skillState(dir string, want map[string][]byte) (string, error) {
	cur, mark, err := onDisk(dir)
	switch {
	case err != nil:
		return "", err
	case cur == nil:
		return "write", nil
	case mark == "" || treeHash(cur) != mark:
		return "theirs", nil
	case mark == treeHash(want):
		return "ok", nil
	}
	return "write", nil
}

func (k skillsStep) Check(_ context.Context, e *Env) (bool, error) {
	for _, n := range k.s.Skills {
		want, err := shipped(e, n)
		if err != nil {
			return false, err
		}
		for _, d := range SkillDirs {
			st, err := skillState(e.Path(path.Join(d, n)), want)
			if err != nil || st == "write" {
				return false, err
			}
		}
	}
	return true, nil
}

func (k skillsStep) Apply(_ context.Context, e *Env) error {
	for _, n := range k.s.Skills {
		want, err := shipped(e, n)
		if err != nil {
			return err
		}
		for _, d := range SkillDirs {
			dir := e.Path(path.Join(d, n))
			st, err := skillState(dir, want)
			if err != nil {
				return err
			}
			if st != "write" {
				continue
			}
			if err := os.RemoveAll(dir); err != nil { // yardo's own, out of date
				return err
			}
			if err := k.write(e, d, n, want); err != nil {
				return err
			}
		}
	}
	return nil
}

// write puts the skill's files in d/name, marked, owned by the user.
func (skillsStep) write(e *Env, d, name string, files map[string][]byte) error {
	dir := e.Path(path.Join(d, name))
	for rel, b := range files {
		mode := fs.FileMode(0o644)
		if bytes.HasPrefix(b, []byte("#!")) {
			mode = 0o755
		}
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(rel)), b, mode); err != nil {
			return err
		}
	}
	if err := writeFile(filepath.Join(dir, skillMark), []byte(treeHash(files)+"\n"), 0o644); err != nil {
		return err
	}
	// The user's, and the folders it's in (~/.agents, ~/.agents/skills).
	for _, p := range []string{e.Path(path.Dir(d)), e.Path(d)} {
		if err := e.chown(p, e.UID, e.GID); err != nil {
			return err
		}
	}
	return filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return e.chown(p, e.UID, e.GID)
	})
}
