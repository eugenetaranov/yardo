package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// Kept is what removing an item leaves in place: the packages, apt repos,
// mise tools, files, rc lines and units something still set up uses
// (another item, the baseline), each with who uses it ("" for the base
// packages). See KeptBy.
type Kept struct{ Packages, Repos, Tools, Files, Lines, Units map[string]string }

// basePackages are never removed with an item: the machine's own, which
// some items list only to be sure they're there.
var basePackages = []string{"ca-certificates", "curl", "wget", "gnupg", "git", "jq", "tar", "acl", "unzip", "xz-utils",
	"sudo", "openssh-server", "openssh-client", "apt-transport-https", "lsb-release", "software-properties-common", "python3"}

// lineKey is a line's key in Kept.Lines: its file and the line.
func lineKey(to, line string) string { return to + "\x00" + line }

// KeptBy is what items (the ones staying) use, and the base packages.
func KeptBy(e *Env, items []*catalog.Item) Kept {
	k := Kept{Packages: map[string]string{}, Repos: map[string]string{}, Tools: map[string]string{},
		Files: map[string]string{}, Lines: map[string]string{}, Units: map[string]string{}}
	for _, p := range basePackages {
		k.Packages[p] = ""
	}
	for _, it := range items {
		who := shortLabel(it)
		for _, s := range it.Steps {
			switch s.Type {
			case "apt":
				for _, p := range (aptStep{s}).Packages(e) {
					k.Packages[p] = who
				}
			case "apt_repo":
				k.Repos[s.Name] = who
			case "mise":
				for _, t := range s.Tools {
					k.Tools[MiseName(t)] = who
				}
			case "file", "template", "sysctl", "link", "git_clone", "block":
				k.Files[s.To] = who
			case "line":
				k.Lines[lineKey(s.To, s.Line)] = who
			case "systemctl":
				if s.Unit != "" {
					k.Units[s.Unit] = who
				}
			}
		}
	}
	return k
}

// shortLabel is an item's name without its details: "AWS tools".
func shortLabel(it *catalog.Item) string {
	if it.Label == "" {
		return it.ID
	}
	s, _, _ := strings.Cut(it.Label, " (")
	s, _, _ = strings.Cut(s, ", ")
	return s
}

// RemovePlan is what removing an item does: Say is what its own removal
// script does (its remove_say), Off what it takes off, Keep what it
// leaves in place and why ("fzf (Shell helpers uses it)", "~/.zshrc
// (edited by hand)"). Run carries it out.
type RemovePlan struct {
	Item      *catalog.Item
	Say       string
	Off, Keep []string
	acts      []removeAct
}

// removeAct is one thing a removal does; what is how the log names it.
type removeAct struct {
	what string
	do   func(ctx context.Context) error
}

func (p *RemovePlan) off(what string, do func(ctx context.Context) error) {
	p.Off = append(p.Off, what)
	p.acts = append(p.acts, removeAct{what, do})
}

// keep notes what stays: by who uses it, or with why.
func (p *RemovePlan) keep(what, who string) {
	if who != "" {
		what += " (" + who + " uses it)"
	}
	p.Keep = append(p.Keep, what)
}

// PlanRemove works out what removing it would take off and keep: only
// what's there (packages dpkg has, files that exist), so removing again
// after a failure picks up where it stopped. It changes nothing.
func PlanRemove(ctx context.Context, e *Env, it *catalog.Item, keep Kept) (*RemovePlan, error) {
	if !it.Removable() {
		return nil, fmt.Errorf("%s can't be removed by yardo", it.ID)
	}
	p := &RemovePlan{Item: it}
	if it.Remove != "" {
		script := strings.NewReplacer("{{user}}", e.User, "{{group}}", e.Group, "{{home}}", e.Home, "{{arch}}", e.Arch).Replace(it.Remove)
		p.Say = it.RemoveSay
		p.acts = append(p.acts, removeAct{"its removal script", func(ctx context.Context) error {
			_, err := e.run(ctx, Cmd{Name: "bash", Args: []string{"-c", script}, Env: AptEnv})
			return err
		}})
	}
	// yardo's own steps, last first: a unit stops before its files and
	// package go.
	var reload bool
	for _, s := range slices.Backward(it.Steps) {
		if err := planUndo(ctx, e, p, s, keep); err != nil {
			return nil, err
		}
		reload = reload || s.Type == "systemctl"
	}
	if reload && len(p.acts) > 0 {
		p.acts = append(p.acts, removeAct{"systemctl daemon-reload", func(ctx context.Context) error {
			_, _ = e.run(ctx, Cmd{Name: "systemctl", Args: []string{"daemon-reload"}})
			return nil
		}})
	}
	planPackages(ctx, e, p, it, keep)
	for _, s := range it.Steps {
		if s.Type != "apt_repo" {
			continue
		}
		if who, ok := keep.Repos[s.Name]; ok {
			p.keep("the "+s.Name+" apt repo", who)
			continue
		}
		r := aptRepoStep{s}
		for _, f := range []string{r.listPath(), r.keyPath()} {
			if exists(e.Path(f)) {
				p.off(f, func(context.Context) error { return rmIfThere(e.Path(f)) })
			}
		}
	}
	planMise(ctx, e, p, it, keep)
	return p, nil
}

// planPackages: its apt packages that are installed and nothing else
// uses, purged in one go.
func planPackages(ctx context.Context, e *Env, p *RemovePlan, it *catalog.Item, keep Kept) {
	var pkgs []string
	for _, s := range it.Steps {
		if s.Type != "apt" {
			continue
		}
		for _, pk := range (aptStep{s}).Packages(e) {
			if who, ok := keep.Packages[pk]; ok {
				if who != "" && !slices.Contains(p.Keep, pk+" ("+who+" uses it)") {
					p.keep(pk, who)
				}
				continue
			}
			if slices.Contains(pkgs, pk) {
				continue
			}
			// Only the ones installed: purging one that isn't fails.
			if _, err := e.Exec.Run(ctx, Cmd{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}", pk}}); err == nil {
				pkgs = append(pkgs, pk)
			}
		}
	}
	if len(pkgs) == 0 {
		return
	}
	p.Off = append(p.Off, pkgs...)
	p.acts = append(p.acts, removeAct{"apt-get purge " + strings.Join(pkgs, " "), func(ctx context.Context) error {
		_, err := e.run(ctx, Cmd{Name: "apt-get", Args: append([]string{"purge", "-y", "-q"}, pkgs...), Env: AptEnv})
		return err
	}})
}

// planMise: its mise tools nothing else uses, and that mise has (all of
// them when mise can't say).
func planMise(ctx context.Context, e *Env, p *RemovePlan, it *catalog.Item, keep Kept) {
	have, ok := miseList(ctx, e)
	var tools []string
	for _, s := range it.Steps {
		if s.Type != "mise" {
			continue
		}
		for _, t := range s.Tools {
			n := MiseName(t)
			if who, kept := keep.Tools[n]; kept {
				p.keep(n, who)
				continue
			}
			if _, there := have[n]; (!ok || there) && !slices.Contains(tools, n) {
				tools = append(tools, n)
			}
		}
	}
	for _, t := range tools {
		// mise unuse takes it off the global config and uninstalls it; an
		// older mise has use --remove instead.
		p.off(t, func(ctx context.Context) error {
			if _, err := e.run(ctx, Cmd{Name: "mise", Args: []string{"unuse", "-g", t}, AsUser: true}); err != nil {
				_, err = e.run(ctx, Cmd{Name: "mise", Args: []string{"use", "-g", "--remove", t}, AsUser: true})
				return err
			}
			return nil
		})
	}
}

// planUndo plans undoing one of yardo's own steps (the ones Removable
// counts as undone; the others are the remove script's).
func planUndo(ctx context.Context, e *Env, p *RemovePlan, s catalog.Step, keep Kept) error {
	switch s.Type {
	case "file", "template", "sysctl":
		if who, ok := keep.Files[s.To]; ok {
			if exists(e.Path(s.To)) {
				p.keep(s.To, who)
			}
			return nil
		}
		f := fileStep{s, s.Type == "template"}
		if s.Type == "sysctl" {
			f = sysctlStep{s}.file()
		}
		return planFile(e, p, f, s.Type == "sysctl")
	case "line":
		if who, ok := keep.Lines[lineKey(s.To, s.Line)]; ok {
			p.keep("a line in "+s.To, who)
			return nil
		}
		path := e.Path(s.To)
		cur, err := os.ReadFile(path)
		if err != nil || !slices.Contains(strings.Split(string(cur), "\n"), s.Line) {
			return nil
		}
		p.off("a line in "+s.To, func(context.Context) error { return rewrite(path, WithoutLine)(s.Line) })
	case "block":
		if who, ok := keep.Files[s.To]; ok {
			p.keep("yardo's block in "+s.To, who)
			return nil
		}
		path := e.Path(s.To)
		cur, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(cur, []byte(BlockBegin)) {
			return nil
		}
		p.off("yardo's block in "+s.To, func(context.Context) error {
			if err := rewrite(path, func(b []byte, _ string) []byte { return WithoutBlock(b, BlockBegin, BlockEnd) })(""); err != nil {
				return err
			}
			// Nothing but yardo's block was in it: the file goes too.
			if b, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(b)) == 0 {
				return rmIfThere(path)
			}
			return nil
		})
	case "link":
		path := e.Path(s.To)
		if got, err := os.Readlink(path); err != nil || got != s.Src {
			return nil
		}
		if who, ok := keep.Files[s.To]; ok {
			p.keep(s.To, who)
			return nil
		}
		p.off(s.To, func(context.Context) error { return rmIfThere(path) })
	case "git_clone":
		path := e.Path(s.To)
		if !exists(path) {
			return nil
		}
		if who, ok := keep.Files[s.To]; ok {
			p.keep(s.To, who)
			return nil
		}
		_, _, user := e.owner(s)
		out, err := e.Exec.Run(ctx, Cmd{Name: "git", Args: []string{"-C", path, "status", "--porcelain"}, AsUser: user})
		if err != nil || len(bytes.TrimSpace(out)) > 0 {
			p.Keep = append(p.Keep, s.To+" (has local changes)")
			return nil
		}
		p.off(s.To, func(context.Context) error { return os.RemoveAll(path) })
	case "systemctl":
		if s.Unit == "" || (systemctlStep{s}).action() != "enable-now" {
			return nil
		}
		if who, ok := keep.Units[s.Unit]; ok {
			p.keep("the "+s.Unit+" service", who)
			return nil
		}
		_, en := e.Exec.Run(ctx, Cmd{Name: "systemctl", Args: []string{"is-enabled", "--quiet", s.Unit}})
		_, act := e.Exec.Run(ctx, Cmd{Name: "systemctl", Args: []string{"is-active", "--quiet", s.Unit}})
		if en != nil && act != nil {
			return nil
		}
		p.off("the "+s.Unit+" service", func(ctx context.Context) error {
			_, err := e.run(ctx, Cmd{Name: "systemctl", Args: []string{"disable", "--now", s.Unit}})
			return err
		})
	}
	return nil
}

// planFile plans deleting a file yardo wrote: only when it's what yardo
// writes (else it was edited by hand, and stays), putting back the
// original it kept, if any. sysctl reloads the settings after.
func planFile(e *Env, p *RemovePlan, f fileStep, sysctl bool) error {
	path := e.Path(f.s.To)
	cur, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	want, err := f.content(e)
	if err != nil {
		return err
	}
	if !bytes.Equal(cur, want) {
		p.Keep = append(p.Keep, f.s.To+" (edited by hand)")
		return nil
	}
	what, do := f.s.To, func(context.Context) error { return rmIfThere(path) }
	if f.s.KeepOrig && exists(path+".orig") {
		what, do = f.s.To+" (the original put back)", func(context.Context) error { return os.Rename(path+".orig", path) }
	}
	p.off(what, func(ctx context.Context) error {
		if err := do(ctx); err != nil {
			return err
		}
		if sysctl {
			_, _ = e.run(ctx, Cmd{Name: "sysctl", Args: []string{"-q", "--system"}})
		}
		return nil
	})
	return nil
}

// Run removes the item as planned, in order. It returns what it took off;
// on a failure, what it took off before it.
func (p *RemovePlan) Run(ctx context.Context) ([]string, error) {
	var did []string
	for _, a := range p.acts {
		if err := a.do(ctx); err != nil {
			return did, fmt.Errorf("removing %s (%s): %w", p.Item.ID, a.what, err)
		}
		did = append(did, a.what)
	}
	return did, nil
}

// Remove uninstalls it (see PlanRemove), keeping what's in keep. It
// returns what it took off, for the log.
func Remove(ctx context.Context, e *Env, it *catalog.Item, keep Kept) ([]string, error) {
	p, err := PlanRemove(ctx, e, it, keep)
	if err != nil {
		return nil, err
	}
	return p.Run(ctx)
}

// WithoutLine returns b without the line line, and without the blank line
// WithLine put before it when it appended it.
func WithoutLine(b []byte, line string) []byte {
	lines := strings.Split(string(b), "\n")
	for i := 0; i < len(lines); i++ {
		if lines[i] != line {
			continue
		}
		lo := i
		if i > 0 && lines[i-1] == "" && (i+1 >= len(lines) || lines[i+1] == "") {
			lo = i - 1
		}
		lines = slices.Delete(lines, lo, i+1)
		i = lo - 1
	}
	return []byte(strings.Join(lines, "\n"))
}

// WithoutBlock returns cur without the block from the line begin to the
// line end (see WithBlock).
func WithoutBlock(cur []byte, begin, end string) []byte {
	s := string(cur)
	i, j := strings.Index(s, begin), strings.Index(s, end)
	if i < 0 || j < i {
		return cur
	}
	return []byte(s[:i] + strings.TrimLeft(s[j+len(end):], "\n"))
}

// rewrite returns a func that rewrites path through edit, in place, so
// it keeps its mode and owner.
func rewrite(path string, edit func(b []byte, arg string) []byte) func(arg string) error {
	return func(arg string) error {
		cur, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return os.WriteFile(path, edit(cur, arg), 0)
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func rmIfThere(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
