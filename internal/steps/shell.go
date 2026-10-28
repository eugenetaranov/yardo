package steps

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"

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

// Drop-ins: a line in .zshrc and .bashrc for each file yardo keeps
// shell settings in.
var Dropins = []struct{ File, Line string }{
	{"~/.config/yardo/shell.sh", "[ -r ~/.config/yardo/shell.sh ] && . ~/.config/yardo/shell.sh"},
	{"~/.config/yardo/agent-env.sh", "[ -r ~/.config/yardo/agent-env.sh ] && . ~/.config/yardo/agent-env.sh"},
	{"~/.config/yardo/nvim.sh", "[ -r ~/.config/yardo/nvim.sh ] && . ~/.config/yardo/nvim.sh"},
}

func init() {
	RegisterGo("zshrc", func(catalog.Step) Step { return zshrcStep{} })
	RegisterGo("prompt", func(catalog.Step) Step { return promptStep{} })
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

// zshrcStep writes ~/.zshrc from zshrc.tmpl: the theme is the chosen
// prompt's (Powerlevel10k once it's cloned), and the drop-ins of the files
// that exist are added back (the template replaced them).
type zshrcStep struct{}

const zshrc = "~/.zshrc"

func (zshrcStep) Plan(*Env) []Action { return []Action{{"file", zshrc + " (from zshrc.tmpl)"}} }

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
	for _, d := range Dropins {
		if _, err := os.Stat(e.Path(d.File)); err == nil {
			b = WithLine(b, d.Line, "")
		}
	}
	return b, nil
}

func (z zshrcStep) Check(_ context.Context, e *Env) (bool, error) {
	want, err := z.want(e)
	if err != nil {
		return false, err
	}
	cur, err := os.ReadFile(e.Path(zshrc))
	return err == nil && bytes.Equal(cur, want), nil
}

func (z zshrcStep) Apply(_ context.Context, e *Env) error {
	want, err := z.want(e)
	if err != nil {
		return err
	}
	p := e.Path(zshrc)
	// The user's own .zshrc is kept once, as .zshrc.orig.
	if cur, err := os.ReadFile(p); err == nil {
		if _, err := os.Stat(p + ".orig"); errors.Is(err, fs.ErrNotExist) {
			if err := writeFile(p+".orig", cur, 0o644); err != nil {
				return err
			}
			if err := e.chown(p+".orig", e.UID, e.GID); err != nil {
				return err
			}
		}
	}
	if err := writeFile(p, want, 0o644); err != nil {
		return err
	}
	return e.chown(p, e.UID, e.GID)
}
