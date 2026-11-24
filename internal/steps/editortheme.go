package steps

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

func init() {
	RegisterGo("editortheme", func(catalog.Step) Step { return editorThemeStep{} })
}

// EditorThemes are the colour schemes Settings offers for Vim and Neovim:
// Nvim and Vim are each editor's name for it. Most ship with both editors;
// Tokyo Night, Kanagawa, Rose Pine and Gruvbox are Neovim plugins, and Vim
// takes the nearest one it has.
var EditorThemes = []struct{ Key, Name, Nvim, Vim string }{
	{"tokyonight", "Tokyo Night", "tokyonight-night", "sorbet"},
	{"catppuccin", "Catppuccin", "catppuccin", "catppuccin"},
	{"kanagawa", "Kanagawa", "kanagawa-wave", "habamax"},
	{"rose-pine", "Rose Pine", "rose-pine", "catppuccin"},
	{"gruvbox", "Gruvbox", "gruvbox", "retrobox"},
	{"retrobox", "Retrobox", "retrobox", "retrobox"},
	{"habamax", "Habamax", "habamax", "habamax"},
	{"sorbet", "Sorbet", "sorbet", "sorbet"},
	{"unokai", "Unokai", "unokai", "unokai"},
	{"wildcharm", "Wildcharm", "wildcharm", "wildcharm"},
	{"zaibatsu", "Zaibatsu", "zaibatsu", "zaibatsu"},
}

// EditorThemeFile names the editor theme for Neovim (its init.lua) and Vim
// (~/.vim/plugin/yardo-theme.vim), which read it as they start: a "theme"
// line with the key, then a "nvim" and a "vim" line with each one's name.
const EditorThemeFile = "~/.config/yardo/editor-theme"

// EditorTheme is the editor theme the file names ("tokyonight", Neovim's
// own, when there's none).
func EditorTheme(e *Env) string {
	b, err := os.ReadFile(e.Path(EditorThemeFile))
	if err != nil {
		return EditorThemes[0].Key
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		if k, ok := strings.CutPrefix(strings.TrimSpace(line), "theme "); ok {
			for _, t := range EditorThemes {
				if t.Key == strings.TrimSpace(k) {
					return t.Key
				}
			}
		}
	}
	return EditorThemes[0].Key
}

// EditorThemeName is key's name, as Settings shows it.
func EditorThemeName(key string) string {
	for _, t := range EditorThemes {
		if t.Key == key {
			return t.Name
		}
	}
	return key
}

// SetEditorTheme writes key's names to the editor theme file, as the
// user's; an unknown key is Tokyo Night.
func SetEditorTheme(e *Env, key string) error {
	t := EditorThemes[0]
	for _, c := range EditorThemes {
		if c.Key == key {
			t = c
		}
	}
	p := e.Path(EditorThemeFile)
	if err := mkdirFor(e, p, catalog.Step{To: EditorThemeFile}); err != nil {
		return err
	}
	body := fmt.Sprintf("# yardo's Settings › Look › Editor theme writes this; Vim and Neovim\n# read it as they start.\ntheme %s\nnvim %s\nvim %s\n", t.Key, t.Nvim, t.Vim)
	if err := writeFile(p, []byte(body), 0o644); err != nil {
		return err
	}
	return e.chown(p, e.UID, e.GID)
}

// editorThemeStep keeps the editor theme file as Settings or a profile
// last set it (Answers.EditorTheme); with nothing set, there's no file and
// Neovim keeps Tokyo Night, Vim its own colours.
type editorThemeStep struct{}

func (editorThemeStep) Plan(e *Env) []Action {
	if e.Answers == nil || e.Answers.EditorTheme == "" {
		return nil
	}
	return []Action{{"file", fmt.Sprintf("%s (%s)", EditorThemeFile, e.Answers.EditorTheme)}}
}

func (editorThemeStep) Check(_ context.Context, e *Env) (bool, error) {
	if e.Answers == nil || e.Answers.EditorTheme == "" {
		return true, nil
	}
	_, err := os.Stat(e.Path(EditorThemeFile))
	return err == nil && EditorTheme(e) == e.Answers.EditorTheme, nil
}

// PlanUndo leaves the file: it's the user's choice, and does nothing
// without the editors' configs.
func (editorThemeStep) PlanUndo(context.Context, *Env, *RemovePlan) error { return nil }

func (editorThemeStep) Apply(_ context.Context, e *Env) error {
	if e.Answers == nil || e.Answers.EditorTheme == "" {
		return nil
	}
	return SetEditorTheme(e, e.Answers.EditorTheme)
}
