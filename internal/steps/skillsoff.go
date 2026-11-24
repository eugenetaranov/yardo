package steps

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
)

// Turning skills off and on. A skill that's off is moved out of the skills
// folders, where every agent looks, into SkillsOff/<claude|agents>/<name>,
// and back when it's turned on: the folder is renamed, so its files, modes
// and yardo's mark stay as they were. Where the folder is, is the state:
// nothing is kept in the answers. SkillsOff/ALL says all skills are off;
// SkillsOff/own lists the ones turned off by themselves, which stay off when
// all come back on. Everything goes through an os.Root on the home, so a
// link can't take a move or a write out of it, and a skill folder that is a
// link isn't moved.

// SkillsOff is where skills turned off are held.
const SkillsOff = "~/.local/share/yardo/skills-off"

// heldDir is where a skill of skills folder d is held while it's off.
func heldDir(d string) string {
	return SkillsOff + "/" + strings.TrimPrefix(path.Dir(d), "~/.")
}

// Skill is one skill there is: in a skills folder, held off, or both.
type Skill struct {
	Name string
	// Places are the skills folders (SkillDirs) it's in; Held those it's
	// held off from.
	Places, Held []string
	// Source is yardo (as yardo wrote it), changed (yardo's, edited since)
	// or theirs (not yardo's).
	Source string
	// Off: no skills folder has it (it's held). Own: it was turned off by
	// itself, and stays off when all skills are turned on.
	Off, Own bool
}

// home opens the user's home as a root: every path below is relative to it.
func home(e *Env) (*os.Root, error) { return os.OpenRoot(e.Path("~")) }

// rel is p (~/x) relative to the home.
func rel(p string) string { return strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/") }

// isSkill: dir/name in the home is a folder (not a link) with a SKILL.md.
func isSkill(r *os.Root, dir, name string) bool {
	fi, err := r.Lstat(rel(path.Join(dir, name)))
	if err != nil || !fi.IsDir() {
		return false
	}
	fi, err = r.Lstat(rel(path.Join(dir, name, "SKILL.md")))
	return err == nil && fi.Mode().IsRegular()
}

// present: something (a folder, a file, a link) is at dir/name.
func present(r *os.Root, dir, name string) bool {
	_, err := r.Lstat(rel(path.Join(dir, name)))
	return err == nil
}

// SkillsAllOff says whether all skills are turned off.
func SkillsAllOff(e *Env) bool {
	_, err := os.Lstat(e.Path(SkillsOff + "/ALL"))
	return err == nil
}

// ownOff are the skills turned off by themselves.
func ownOff(e *Env) []string {
	b, err := os.ReadFile(e.Path(SkillsOff + "/own"))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Fields(string(b)) {
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// skillOff says whether yardo should keep skill name out of the skills
// folders it's missing from: all skills are off, or it's held off.
func skillOff(e *Env, name string) bool {
	if SkillsAllOff(e) {
		return true
	}
	for _, d := range SkillDirs {
		if _, err := os.Lstat(e.Path(path.Join(heldDir(d), name))); err == nil {
			return true
		}
	}
	return false
}

// skillTarget is the folder skill name of skills folder d is written to:
// d itself, or, when the skill is off and d hasn't got it, where it's held.
func skillTarget(e *Env, d, name string) string {
	if _, err := os.Lstat(e.Path(path.Join(d, name))); err == nil || !skillOff(e, name) {
		return d
	}
	return heldDir(d)
}

// ListSkills are the skills there are, by name, and whether all are off.
// While all are off, a skill that has appeared in a skills folder since is
// moved off first, as all are.
func ListSkills(e *Env) ([]Skill, bool, error) {
	all := SkillsAllOff(e)
	if all {
		if err := holdAll(e); err != nil {
			return nil, true, err
		}
	}
	r, err := home(e)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, all, nil
		}
		return nil, all, err
	}
	defer func() { _ = r.Close() }()
	own := ownOff(e)
	by := map[string]*Skill{}
	get := func(n string) *Skill {
		if by[n] == nil {
			by[n] = &Skill{Name: n}
		}
		return by[n]
	}
	for _, d := range SkillDirs {
		for _, dir := range []string{d, heldDir(d)} {
			ents, _ := fs.ReadDir(r.FS(), rel(dir))
			for _, de := range ents {
				n := de.Name()
				if !isSkill(r, dir, n) {
					continue
				}
				s := get(n)
				if dir == d {
					s.Places = append(s.Places, d)
				} else {
					s.Held = append(s.Held, d)
				}
				if s.Source == "" {
					s.Source = skillSource(e.Path(path.Join(dir, n)))
				}
			}
		}
	}
	var out []Skill
	for _, n := range slices.Sorted(maps.Keys(by)) {
		s := by[n]
		s.Off = len(s.Places) == 0
		s.Own = s.Off && slices.Contains(own, n)
		out = append(out, *s)
	}
	return out, all, nil
}

// skillSource is where the skill in dir is from: yardo, changed or theirs.
func skillSource(dir string) string {
	files, mark, err := onDisk(dir)
	switch {
	case err != nil || mark == "":
		return "theirs"
	case treeHash(files) != mark:
		return "changed"
	}
	return "yardo"
}

// SwitchSkills turns skills off or on (do), names or, when none is named,
// all of them. Turning one on while all are off isn't possible; a folder
// in the way of one coming back keeps it off, and says so.
func SwitchSkills(e *Env, do string, names []string) error {
	if do != "off" && do != "on" {
		return fmt.Errorf("skills: %q is neither off nor on", do)
	}
	skills, all, err := ListSkills(e)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		if do == "off" {
			if err := writeOff(e, "ALL", nil); err != nil {
				return err
			}
			return holdAll(e)
		}
		if err := os.Remove(e.Path(SkillsOff + "/ALL")); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		own := ownOff(e)
		var errs []error
		for _, s := range skills {
			if !slices.Contains(own, s.Name) {
				errs = append(errs, release(e, s))
			}
		}
		return errors.Join(errs...)
	}
	var there []string
	for _, s := range skills {
		there = append(there, s.Name)
	}
	for _, n := range names {
		if !slices.Contains(there, n) {
			if len(there) == 0 {
				return fmt.Errorf("no skill %s: there are no skills", n)
			}
			return fmt.Errorf("no skill %s: the skills are %s", n, strings.Join(there, ", "))
		}
	}
	if do == "on" && all {
		return errors.New("all skills are off: turn them all on first (sudo yardo skills on)")
	}
	own := ownOff(e)
	var errs []error
	for _, n := range names {
		s := skills[slices.IndexFunc(skills, func(s Skill) bool { return s.Name == n })]
		if do == "off" {
			own = append(own, n)
			errs = append(errs, hold(e, s))
			continue
		}
		if err := release(e, s); err != nil {
			errs = append(errs, err)
			continue
		}
		own = slices.DeleteFunc(own, func(o string) bool { return o == n })
	}
	slices.Sort(own)
	own = slices.Compact(own)
	if err := writeOff(e, "own", own); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// holdAll moves every skill in a skills folder off.
func holdAll(e *Env) error {
	r, err := home(e)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() { _ = r.Close() }()
	var errs []error
	for _, d := range SkillDirs {
		ents, _ := fs.ReadDir(r.FS(), rel(d))
		for _, de := range ents {
			if isSkill(r, d, de.Name()) {
				errs = append(errs, hold(e, Skill{Name: de.Name(), Places: []string{d}}))
			}
		}
	}
	return errors.Join(errs...)
}

// hold moves s out of the skills folders it's in. One already held from
// the same folder is in the way: that place is left as it is.
func hold(e *Env, s Skill) error {
	r, err := home(e)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	var errs []error
	for _, d := range s.Places {
		if !isSkill(r, d, s.Name) {
			continue // a link, or gone: not moved
		}
		h := heldDir(d)
		if present(r, h, s.Name) {
			errs = append(errs, fmt.Errorf("skill %s: %s/%s is held off already: %s/%s stays", s.Name, h, s.Name, d, s.Name))
			continue
		}
		if err := userDirs(e, r, h); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := r.Rename(rel(path.Join(d, s.Name)), rel(path.Join(h, s.Name))); err != nil {
			errs = append(errs, fmt.Errorf("skill %s: %w", s.Name, err))
		}
	}
	return errors.Join(errs...)
}

// release puts s back in the skills folders it was held from: all or none.
// Something of the same name in one of them is in the way, and stays.
func release(e *Env, s Skill) error {
	r, err := home(e)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for _, d := range s.Held {
		if present(r, d, s.Name) {
			return fmt.Errorf("skill %s stays off: %s/%s is in the way (another copy was put there since)", s.Name, d, s.Name)
		}
	}
	for _, d := range s.Held {
		if err := userDirs(e, r, d); err != nil {
			return err
		}
		if err := r.Rename(rel(path.Join(heldDir(d), s.Name)), rel(path.Join(d, s.Name))); err != nil {
			return fmt.Errorf("skill %s: %w", s.Name, err)
		}
	}
	return nil
}

// writeOff writes SkillsOff/name: ALL (names nil), or own's list (removed
// when empty).
func writeOff(e *Env, name string, names []string) error {
	r, err := home(e)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	p := rel(SkillsOff + "/" + name)
	if name == "own" && len(names) == 0 {
		if err := r.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := userDirs(e, r, SkillsOff); err != nil {
		return err
	}
	var b []byte
	if len(names) > 0 {
		b = []byte(strings.Join(names, "\n") + "\n")
	}
	if err := r.WriteFile(p, b, 0o644); err != nil {
		return err
	}
	if e.Chown {
		return r.Lchown(p, e.UID, e.GID)
	}
	return nil
}

// userDirs makes folder p (~/x/y) in the home, each part it makes the
// user's.
func userDirs(e *Env, r *os.Root, p string) error {
	cur := ""
	for _, part := range strings.Split(rel(p), "/") {
		cur = path.Join(cur, part)
		err := r.Mkdir(cur, 0o755)
		switch {
		case errors.Is(err, fs.ErrExist):
			continue
		case err != nil:
			return err
		case e.Chown:
			if err := r.Lchown(cur, e.UID, e.GID); err != nil {
				return err
			}
		}
	}
	return nil
}
