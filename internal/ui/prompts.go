package ui

import (
	"errors"
	"slices"

	bkey "github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// ErrQuit is returned when the user quits a prompt (esc / ctrl-c).
var ErrQuit = errors.New("cancelled")

// keys are huh's, with esc cancelling a prompt as it does every screen
// (huh's own is ctrl+c only).
func keys() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = bkey.NewBinding(bkey.WithKeys("ctrl+c", "esc"))
	return km
}

func run(f huh.Field) error {
	err := huh.NewForm(huh.NewGroup(f)).WithTheme(tui.Theme()).WithKeyMap(keys()).WithShowHelp(false).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrQuit
	}
	return err
}

// Input asks for a line of text, prefilled with value.
func Input(title, desc, value string) (string, error) {
	v := value
	err := run(huh.NewInput().Title(title).Description(desc).Value(&v))
	return v, err
}

// Password asks for a secret, without echo; empty keeps nothing.
func Password(title, placeholder string) (string, error) {
	var v string
	err := run(huh.NewInput().Title(title).Placeholder(placeholder).EchoMode(huh.EchoModePassword).Value(&v))
	return v, err
}

// Choose asks for one of opts, starting on selected.
func Choose(title string, opts []string, selected string) (string, error) {
	v := selected
	o := make([]huh.Option[string], len(opts))
	for i, s := range opts {
		o[i] = huh.NewOption(s, s)
	}
	err := run(huh.NewSelect[string]().Title(title).Options(o...).Value(&v))
	return v, err
}

// Confirm asks yes or no.
func Confirm(title, desc string, def bool) (bool, error) {
	v := def
	err := run(huh.NewConfirm().Title(title).Description(desc).Value(&v))
	return v, err
}

// Question is one of Ask's: Kind "toggle" (yes or no: Picked is ["on"]
// or empty), "multi" (any of Options), "one" (one of Options, its key, or
// none) or "pick" (one of Options, no none). Options are key and label.
type Question struct {
	Title, Note, Kind string
	Options           [][2]string
	Picked            []string
}

// Ask asks qs on one checklist, and fills in their Picked; esc leaves
// them as they were.
func Ask(qs []*Question) error {
	saved := make([][]string, len(qs))
	for i, q := range qs {
		saved[i] = slices.Clone(q.Picked)
	}
	c := newChecklist(qs)
	_, err := tea.NewProgram(c, tea.WithAltScreen()).Run()
	c.out = true
	if err == nil && c.quit {
		err = ErrQuit
	}
	if err != nil {
		for i, q := range qs {
			q.Picked = saved[i]
		}
	}
	return err
}
