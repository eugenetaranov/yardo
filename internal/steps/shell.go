package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// The zsh themes: oh-my-zsh's default (until Powerlevel10k is cloned),
// Powerlevel10k, and none for Starship, which draws the prompt itself.
const (
	DefaultZshTheme = "robbyrussell"
	P10kTheme       = "powerlevel10k/powerlevel10k"
	StarshipTheme   = ""
	p10kDir         = "~/.oh-my-zsh/custom/themes/powerlevel10k"
)

// Prompt is the zsh prompt the user chose: "p10k" (the default) or
// "starship".
func Prompt(e *Env) string {
	if e.Answers != nil && e.Answers.Prompt == "starship" {
		return "starship"
	}
	return "p10k"
}

// Drop-ins: a line in .bashrc, and in yardo's zsh file (or, without it,
// yardo's block in .zshrc), for each file yardo keeps shell settings in.
var Dropins = []struct{ File, Line string }{
	{"~/.config/yardo/shell.sh", "[ -r ~/.config/yardo/shell.sh ] && . ~/.config/yardo/shell.sh"},
	{"~/.config/yardo/agent-env.sh", "[ -r ~/.config/yardo/agent-env.sh ] && . ~/.config/yardo/agent-env.sh"},
	{"~/.config/yardo/nvim.sh", "[ -r ~/.config/yardo/nvim.sh ] && . ~/.config/yardo/nvim.sh"},
	{"~/.config/yardo/helpers.sh", "[ -r ~/.config/yardo/helpers.sh ] && . ~/.config/yardo/helpers.sh"},
	{"~/.config/yardo/aws.sh", "[ -r ~/.config/yardo/aws.sh ] && . ~/.config/yardo/aws.sh"},
}

func init() {
	RegisterGo("zshrc", func(catalog.Step) Step { return zshrcStep{} })
	RegisterGo("zshdropin", func(s catalog.Step) Step { return zshDropinStep{line: s.Line} })
	RegisterGo("prompt", func(catalog.Step) Step { return promptStep{} })
	RegisterGo("session", func(catalog.Step) Step { return sessionStep{} })
}

// promptStep installs the chosen prompt: Powerlevel10k (its clone, and a
// ready ~/.p10k.zsh, the lean preset in yardo's style) or Starship (mise,
// and ~/.config/starship.toml), in the user's accent colour. A config of
// the user's own is left alone (ownedStep). The other prompt is left
// alone too; ~/.zshrc only loads the chosen one.
type promptStep struct{}

func (promptStep) parts(e *Env) []Step {
	if Prompt(e) == "starship" {
		return append(mustSteps("prompt", catalog.Step{Type: "mise", Tools: []string{"starship@latest"}}),
			ownedStep{src: "starship.toml", to: "~/.config/starship.toml", marker: "# yardo's Starship prompt"})
	}
	return append(mustSteps("prompt", catalog.Step{Type: "git_clone", Repo: "https://github.com/romkatv/powerlevel10k.git", To: p10kDir}),
		ownedStep{src: "p10k-yardo.zsh", to: "~/.p10k.zsh", marker: "# Powerlevel10k config written by yardo"})
}

// ownedStep writes a template where there's no file, or where the file
// still starts with marker (yardo wrote it: the accent may have changed).
// A file of the user's own (p10k configure's, or one they edited and took
// the marker off) is left alone.
type ownedStep struct{ src, to, marker string }

func (o ownedStep) file() fileStep {
	return fileStep{s: catalog.Step{Src: o.src, To: o.to}, template: true}
}

func (o ownedStep) Plan(e *Env) []Action { return o.file().Plan(e) }

// theirs reports whether the file there is the user's own.
func (o ownedStep) theirs(e *Env) bool {
	cur, err := os.ReadFile(e.Path(o.to))
	return err == nil && !bytes.HasPrefix(cur, []byte(o.marker))
}

func (o ownedStep) Check(ctx context.Context, e *Env) (bool, error) {
	if o.theirs(e) {
		return true, nil
	}
	return o.file().Check(ctx, e)
}

func (o ownedStep) Apply(ctx context.Context, e *Env) error {
	if o.theirs(e) {
		return nil
	}
	return o.file().Apply(ctx, e)
}

func (p promptStep) Plan(e *Env) []Action {
	var out []Action
	for _, st := range p.parts(e) {
		out = append(out, st.Plan(e)...)
	}
	return out
}

func (p promptStep) Check(ctx context.Context, e *Env) (bool, error) {
	return checkAll(ctx, e, p.parts(e))
}

func (p promptStep) Apply(ctx context.Context, e *Env) error {
	return applyAll(ctx, e, p.parts(e), false)
}

// yardo's zsh setup is its own file, loaded from a marked block first in
// the user's ~/.zshrc; yardo never touches another line of .zshrc.
const (
	zshrc      = "~/.zshrc"
	yardoZshrc = "~/.config/yardo/zshrc"
	// ZshBlockBegin and ZshBlockEnd mark yardo's block in ~/.zshrc.
	ZshBlockBegin = "# >>> yardo (managed: sudo yardo rewrites this block; your lines go below)"
	ZshBlockEnd   = "# <<< yardo"
	zshSourceLine = "[ -r ~/.config/yardo/zshrc ] && source ~/.config/yardo/zshrc"
	// zshrcSaved is where a .zshrc an older yardo wrote is kept.
	zshrcSaved = "~/.zshrc.before-yardo-split"
)

// zshBlock is yardo's block holding lines.
func zshBlock(lines []string) string {
	return fmt.Sprintf("%s\n%s\n%s\n", ZshBlockBegin, strings.Join(lines, "\n"), ZshBlockEnd)
}

// dropinLines are the Dropins lines of the files that exist.
func dropinLines(e *Env) []string {
	var out []string
	for _, d := range Dropins {
		if _, err := os.Stat(e.Path(d.File)); err == nil {
			out = append(out, d.Line)
		}
	}
	return out
}

// yardoWrote reports whether a .zshrc is one an older yardo wrote whole
// (from zshrc.tmpl, which always had LastMark), not yet split.
func yardoWrote(b []byte) bool {
	return bytes.Contains(b, []byte(LastMark)) && !bytes.Contains(b, []byte(ZshBlockBegin))
}

// zshrcPlan is what .zshrc should be with block in it: the block first,
// the user's lines kept, the drop-in lines an older yardo appended taken
// out; a .zshrc an older yardo wrote becomes the block alone (saved
// first, see zshrcApply).
func zshrcPlan(cur []byte, block string) []byte {
	if yardoWrote(cur) {
		return []byte(block)
	}
	for _, d := range Dropins {
		cur = WithoutLine(cur, d.Line)
	}
	return WithBlock(cur, ZshBlockBegin, ZshBlockEnd, block)
}

// zshrcLink is where ~/.zshrc links to ("" when it's no link): a dotfiles
// repo's, which yardo doesn't write through.
func zshrcLink(e *Env) string {
	p := e.Path(zshrc)
	fi, err := os.Lstat(p)
	if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		return ""
	}
	t, err := os.Readlink(p)
	if err != nil {
		return p
	}
	if !filepath.IsAbs(t) {
		t = filepath.Join(filepath.Dir(p), t)
	}
	return t
}

// zshrcCheck reports whether .zshrc has block as zshrcPlan puts it (a
// linked one: whether it loads yardo's lines at all).
func zshrcCheck(e *Env, block string) (bool, error) {
	if zshrcLink(e) != "" {
		cur, err := os.ReadFile(e.Path(zshrc))
		if err != nil {
			return true, nil // a broken link: nothing to do here
		}
		for _, l := range strings.Split(strings.TrimSuffix(block, "\n"), "\n")[1:] {
			if l != ZshBlockEnd && !bytes.Contains(cur, []byte(l)) {
				return false, nil
			}
		}
		return true, nil
	}
	cur, err := os.ReadFile(e.Path(zshrc))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return err == nil && bytes.Equal(cur, zshrcPlan(cur, block)), nil
}

// zshrcApply puts block first in .zshrc (see zshrcPlan). A .zshrc an
// older yardo wrote is saved once as .zshrc.before-yardo-split; one of the
// user's own that loads oh-my-zsh itself, and one that links into a
// dotfiles repo, get a note.
func zshrcApply(e *Env, block string) error {
	p := e.Path(zshrc)
	if t := zshrcLink(e); t != "" {
		e.note(fmt.Sprintf("~/.zshrc links to %s, which yardo doesn't change: to load yardo's zsh setup, put these lines first in it:\n%s", t, block))
		return nil
	}
	cur, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	want := zshrcPlan(cur, block)
	switch {
	case yardoWrote(cur):
		saved := e.Path(zshrcSaved)
		if !exists(saved) {
			if err := writeFile(saved, cur, 0o644); err != nil {
				return err
			}
			if err := e.chown(saved, e.UID, e.GID); err != nil {
				return err
			}
			e.note("yardo's zsh setup moved to ~/.config/yardo/zshrc, and ~/.zshrc is yours now: your lines there stay. The old ~/.zshrc is saved as ~/.zshrc.before-yardo-split: copy any lines you'd added to it into ~/.zshrc.")
		}
	case !bytes.Contains(cur, []byte(ZshBlockBegin)) && bytes.Contains(cur, []byte("oh-my-zsh.sh")):
		e.note("~/.zshrc loads oh-my-zsh, and so does yardo's block now at its top: oh-my-zsh loads twice. Take out your oh-my-zsh lines (export ZSH, plugins=, source $ZSH/oh-my-zsh.sh) to keep yardo's, or remove the zsh config to keep yours.")
	}
	if err := writeFile(p, want, 0o644); err != nil {
		return err
	}
	return e.chown(p, e.UID, e.GID)
}

// zshrcStep writes yardo's zsh setup, ~/.config/yardo/zshrc, from
// zshrc.tmpl (the theme is the chosen prompt's, Powerlevel10k once it's
// cloned; the drop-ins of the files that exist are sourced at its end),
// and keeps the block that loads it first in ~/.zshrc.
type zshrcStep struct{}

func (zshrcStep) Plan(*Env) []Action {
	return []Action{{"file", yardoZshrc + " (from zshrc.tmpl)"}, {"block", zshrc + " (yardo's block, loading it)"}}
}

func (zshrcStep) want(e *Env) ([]byte, error) {
	theme := DefaultZshTheme
	if Prompt(e) == "starship" {
		theme = StarshipTheme
	} else if _, err := os.Stat(e.Path(p10kDir)); err == nil {
		theme = P10kTheme
	}
	b, err := fileStep{s: catalog.Step{Src: "zshrc.tmpl", Vars: map[string]string{"yardo_zsh_theme": theme}}, template: true}.content(e)
	if err != nil {
		return nil, err
	}
	for _, l := range dropinLines(e) {
		b = WithLine(b, l, "")
	}
	return b, nil
}

var zshrcBlock = zshBlock([]string{zshSourceLine})

func (z zshrcStep) Check(_ context.Context, e *Env) (bool, error) {
	want, err := z.want(e)
	if err != nil {
		return false, err
	}
	if cur, err := os.ReadFile(e.Path(yardoZshrc)); err != nil || !bytes.Equal(cur, want) {
		return false, nil
	}
	return zshrcCheck(e, zshrcBlock)
}

func (z zshrcStep) Apply(_ context.Context, e *Env) error {
	want, err := z.want(e)
	if err != nil {
		return err
	}
	s := catalog.Step{To: yardoZshrc}
	p := e.Path(yardoZshrc)
	if err := mkdirFor(e, p, s); err != nil {
		return err
	}
	if err := writeFile(p, want, 0o644); err != nil {
		return err
	}
	if err := e.chown(p, e.UID, e.GID); err != nil {
		return err
	}
	return zshrcApply(e, zshrcBlock)
}

// PlanUndo plans taking yardo's block out of ~/.zshrc and deleting
// yardo's zsh file; the rest of .zshrc stays.
func (zshrcStep) PlanUndo(_ context.Context, e *Env, p *RemovePlan) error {
	if path := e.Path(zshrc); zshrcLink(e) == "" {
		if cur, err := os.ReadFile(path); err == nil && bytes.Contains(cur, []byte(ZshBlockBegin)) {
			p.off("yardo's block in "+zshrc, func(context.Context) error {
				return rewrite(path, func(b []byte, _ string) []byte { return WithoutBlock(b, ZshBlockBegin, ZshBlockEnd) })("")
			})
		}
	}
	if path := e.Path(yardoZshrc); exists(path) {
		p.off(yardoZshrc, func(context.Context) error { return rmIfThere(path) })
	}
	return nil
}

// zshDropinStep keeps zsh loading yardo's drop-ins, for the items that
// add one (shell niceties, helpers, Neovim, AWS, agent keys): with
// yardo's zsh setup there, it's re-rendered (it sources them); without
// it, yardo's block in ~/.zshrc holds their lines. No .zshrc (a bash
// user): nothing to do. line is its item's drop-in line, for its undo.
type zshDropinStep struct{ line string }

func (zshDropinStep) Plan(*Env) []Action {
	return []Action{{"block", zshrc + " (yardo's block, loading its shell files)"}}
}

// full reports whether yardo's zsh setup is there (or was, in a .zshrc
// it wrote whole): the zshrc step's work then.
func (zshDropinStep) full(e *Env) bool {
	if exists(e.Path(yardoZshrc)) {
		return true
	}
	cur, err := os.ReadFile(e.Path(zshrc))
	return err == nil && zshrcLink(e) == "" && yardoWrote(cur)
}

func (z zshDropinStep) Check(ctx context.Context, e *Env) (bool, error) {
	if !exists(e.Path(zshrc)) {
		return true, nil
	}
	if z.full(e) {
		return zshrcStep{}.Check(ctx, e)
	}
	lines := dropinLines(e)
	if len(lines) == 0 {
		return true, nil
	}
	return zshrcCheck(e, zshBlock(lines))
}

func (z zshDropinStep) Apply(ctx context.Context, e *Env) error {
	if !exists(e.Path(zshrc)) {
		return nil
	}
	if z.full(e) {
		return zshrcStep{}.Apply(ctx, e)
	}
	lines := dropinLines(e)
	if len(lines) == 0 {
		return nil
	}
	return zshrcApply(e, zshBlock(lines))
}

// PlanUndo plans taking its line out of yardo's zsh file, or out of
// yardo's block in ~/.zshrc (a re-run adds it back only while its file
// is there).
func (z zshDropinStep) PlanUndo(_ context.Context, e *Env, p *RemovePlan) error {
	if z.line == "" {
		return nil
	}
	for _, f := range []string{yardoZshrc, zshrc} {
		path := e.Path(f)
		if f == zshrc && zshrcLink(e) != "" {
			continue
		}
		cur, err := os.ReadFile(path)
		if err != nil || !slices.Contains(strings.Split(string(cur), "\n"), z.line) {
			continue
		}
		p.off("a line in "+f, func(context.Context) error { return rewrite(path, WithoutLine)(z.line) })
	}
	return nil
}

// SessionFile holds the session an ssh login gets (tmux, zellij or off;
// missing: asked at login): session.zsh writes it when the question is
// answered, and yardo when Settings or a profile set it.
const SessionFile = "~/.config/yardo/session"

// Session is the session an ssh login gets, as the file says: "tmux",
// "zellij", "off", or "ask" when nothing is set.
func Session(e *Env) string {
	b, err := os.ReadFile(e.Path(SessionFile))
	switch v := strings.TrimSpace(string(b)); {
	case err != nil:
		return "ask"
	case v == "tmux", v == "zellij", v == "off":
		return v
	}
	return "ask"
}

// SetSession writes v (tmux, zellij or off) to the session file, as the
// user's; "ask" (or "") removes it, so the next login asks.
func SetSession(e *Env, v string) error {
	p := e.Path(SessionFile)
	if v == "" || v == "ask" {
		return rmIfThere(p)
	}
	if err := mkdirFor(e, p, catalog.Step{To: SessionFile}); err != nil {
		return err
	}
	if err := writeFile(p, []byte(v+"\n"), 0o644); err != nil {
		return err
	}
	return e.chown(p, e.UID, e.GID)
}

// sessionStep keeps the session file as Settings or a profile last set
// it (Answers.Session); with nothing set there, it's the login's to
// answer, and left alone.
type sessionStep struct{}

func (sessionStep) Plan(e *Env) []Action {
	if e.Answers == nil || e.Answers.Session == "" {
		return nil
	}
	return []Action{{"file", fmt.Sprintf("%s (%s)", SessionFile, e.Answers.Session)}}
}

func (sessionStep) Check(_ context.Context, e *Env) (bool, error) {
	return e.Answers == nil || e.Answers.Session == "" || Session(e) == e.Answers.Session, nil
}

// PlanUndo leaves the session file: it's the user's answer, and does
// nothing once session.zsh is gone.
func (sessionStep) PlanUndo(context.Context, *Env, *RemovePlan) error { return nil }

func (sessionStep) Apply(_ context.Context, e *Env) error {
	if e.Answers == nil || e.Answers.Session == "" {
		return nil
	}
	return SetSession(e, e.Answers.Session)
}
