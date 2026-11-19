package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// The prompts the zsh item offers, as askPrompt and the choices say them.
const (
	promptP10k     = "Powerlevel10k (lean, one line, icons; p10k configure restyles it)"
	promptStarship = "Starship (one line, fast; ~/.config/starship.toml)"
)

// choiceQuestions are the first run's questions: the prompt, then the
// catalog's choices, each starting from its on.
func choiceQuestions(cat *catalog.Catalog, a *answers.Answers) []*ui.Question {
	prompt := a.Prompt
	if prompt == "" {
		prompt = "p10k"
	}
	qs := []*ui.Question{{Title: "zsh prompt", Kind: "pick", Options: [][2]string{{"p10k", promptP10k}, {"starship", promptStarship}}, Picked: []string{prompt}}}
	for _, ch := range cat.Choices {
		q := &ui.Question{Title: ch.Title, Note: ch.Note, Kind: ch.Kind, Picked: slices.Clone(ch.On)}
		for _, id := range ch.Items {
			q.Options = append(q.Options, [2]string{id, cat.Item(id).Label})
		}
		for _, o := range ch.Options {
			q.Options = append(q.Options, [2]string{o.ID, o.Title})
		}
		qs = append(qs, q)
	}
	return qs
}

// chosen are the items the answers turn on, choice by choice (qs as
// choiceQuestions made them, after the prompt).
func chosen(cat *catalog.Catalog, qs []*ui.Question) []string {
	var on []string
	for i, ch := range cat.Choices {
		q := qs[i+1]
		switch ch.Kind {
		case "toggle":
			if len(q.Picked) > 0 {
				on = append(on, ch.Items...)
			}
		case "multi":
			on = append(on, q.Picked...)
		case "one":
			for _, o := range ch.Options {
				if slices.Contains(q.Picked, o.ID) {
					on = append(on, o.Items...)
				}
			}
		}
	}
	return on
}

// choiceRun is what the first run's choices make: the prompt, and the
// items to add, the base too. Later, Tools adds and uninstalls, and
// Settings changes the prompt.
type choiceRun struct {
	prompt string
	adds   []string
}

// askChoices asks the first run's choices.
func askChoices(cat *catalog.Catalog, a *answers.Answers) (*choiceRun, error) {
	qs := choiceQuestions(cat, a)
	if err := ui.Ask(qs); err != nil {
		return nil, err
	}
	return choiceChange(cat, a, qs), nil
}

// choiceChange is what answered qs add for a: the base, then what they
// turn on, but what a has already.
func choiceChange(cat *catalog.Catalog, a *answers.Answers, qs []*ui.Question) *choiceRun {
	r := &choiceRun{prompt: qs[0].Picked[0]}
	for _, id := range append(cat.Base(), chosen(cat, qs)...) {
		if !slices.Contains(a.Picks, id) && !slices.Contains(r.adds, id) {
			r.adds = append(r.adds, id)
		}
	}
	return r
}

// baseChange brings a's picks up to the catalog's base, once: what's new
// in it is added (and returned, to install), and NFS + SMB clients, in
// v0.9.0's base only, stops being managed. v0.9.0 added its base without
// installing it, so from there the base picked is returned too. An item
// uninstalled after its base came in stays uninstalled.
func baseChange(cat *catalog.Catalog, a *answers.Answers, w io.Writer) []string {
	var add []string
	if a.Base < 1 {
		add = slices.DeleteFunc(cat.Base(), func(id string) bool { return slices.Contains(a.Picks, id) })
	}
	if a.Base == 1 && slices.Contains(a.Picks, "netfs") {
		a.Unpick([]string{"netfs"})
		fmt.Fprintln(w, "yardo: NFS + SMB clients is no longer in every machine's base: yardo stops checking it (installed, it stays; Tools adds it back).")
	}
	if len(add) > 0 {
		a.Pick(add)
		fmt.Fprintf(w, "yardo: new in every machine's base, installing: %s\n", strings.Join(labels(cat, add), ", "))
	}
	if a.Base == 1 {
		add = slices.DeleteFunc(cat.Base(), func(id string) bool { return !slices.Contains(a.Picks, id) })
	}
	a.Base = catalog.BaseRev
	return add
}
