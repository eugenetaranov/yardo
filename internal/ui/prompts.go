package ui

import (
	"errors"

	"github.com/charmbracelet/huh"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// ErrQuit is returned when the user quits a prompt (esc / ctrl-c).
var ErrQuit = errors.New("cancelled")

func run(f huh.Field) error {
	err := huh.NewForm(huh.NewGroup(f)).WithTheme(tui.Theme()).WithShowHelp(false).Run()
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

// Ask asks qs, a page each (shift+tab goes back), and fills in their
// Picked.
func Ask(qs []*Question) error {
	var groups []*huh.Group
	ones := map[*Question]*string{}
	toggles := map[*Question]*bool{}
	for _, q := range qs {
		var f huh.Field
		switch q.Kind {
		case "toggle":
			v := len(q.Picked) > 0
			toggles[q] = &v
			f = huh.NewConfirm().Title(q.Title).Description(q.Note).Affirmative("Yes").Negative("No").Value(&v)
		case "one", "pick":
			v := ""
			if len(q.Picked) > 0 {
				v = q.Picked[0]
			}
			ones[q] = &v
			var o []huh.Option[string]
			if q.Kind == "one" {
				o = append(o, huh.NewOption("None", ""))
			}
			for _, kv := range q.Options {
				o = append(o, huh.NewOption(kv[1], kv[0]))
			}
			f = huh.NewSelect[string]().Title(q.Title).Description(q.Note).Options(o...).Value(&v)
		default:
			var o []huh.Option[string]
			for _, kv := range q.Options {
				o = append(o, huh.NewOption(kv[1], kv[0]))
			}
			f = huh.NewMultiSelect[string]().Title(q.Title).Description(q.Note).Options(o...).Value(&q.Picked)
		}
		groups = append(groups, huh.NewGroup(f))
	}
	err := huh.NewForm(groups...).WithTheme(tui.Theme()).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrQuit
	}
	for q, v := range toggles {
		q.Picked = nil
		if *v {
			q.Picked = []string{"on"}
		}
	}
	for q, v := range ones {
		q.Picked = nil
		if *v != "" {
			q.Picked = []string{*v}
		}
	}
	return err
}
