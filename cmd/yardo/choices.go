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
// catalog's choices. A first run starts from each choice's on; later,
// from what a holds.
func choiceQuestions(cat *catalog.Catalog, a *answers.Answers, first bool) []*ui.Question {
	prompt := a.Prompt
	if prompt == "" {
		prompt = "p10k"
	}
	qs := []*ui.Question{{Title: "zsh prompt", Kind: "pick", Options: [][2]string{{"p10k", promptP10k}, {"starship", promptStarship}}, Picked: []string{prompt}}}
	for _, ch := range cat.Choices {
		q := &ui.Question{Title: ch.Title, Note: ch.Note, Kind: ch.Kind}
		for _, id := range ch.Items {
			q.Options = append(q.Options, [2]string{id, cat.Item(id).Label})
		}
		for _, o := range ch.Options {
			q.Options = append(q.Options, [2]string{o.ID, o.Title})
		}
		if first {
			q.Picked = slices.Clone(ch.On)
		} else {
			q.Picked = held(ch, a.Picks)
		}
		qs = append(qs, q)
	}
	return qs
}

// held is ch's answer as picks have it: "on" for a toggle with any of its
// items, a multi's items, a one's option with any of its items.
func held(ch catalog.Choice, picks []string) []string {
	in := func(ids []string) bool {
		return slices.ContainsFunc(ids, func(id string) bool { return slices.Contains(picks, id) })
	}
	switch ch.Kind {
	case "toggle":
		if in(ch.Items) {
			return []string{"on"}
		}
	case "multi":
		return slices.DeleteFunc(slices.Clone(ch.Items), func(id string) bool { return !slices.Contains(picks, id) })
	case "one":
		for _, o := range ch.Options {
			if in(o.Items) {
				return []string{o.ID}
			}
		}
	}
	return nil
}

// chosen are the items answers turn on, and the ones they turn off,
// choice by choice (qs as choiceQuestions made them, after the prompt).
// Later, a choice answered as picks have it changes nothing: Terraform
// tools on because tack is installed don't bring the rest.
func chosen(cat *catalog.Catalog, qs []*ui.Question, picks []string, first bool) (on, off []string) {
	for i, ch := range cat.Choices {
		q := qs[i+1]
		if !first && slices.Equal(q.Picked, held(ch, picks)) {
			continue
		}
		var want []string
		switch ch.Kind {
		case "toggle":
			if len(q.Picked) > 0 {
				want = ch.Items
			}
		case "multi":
			want = q.Picked
		case "one":
			for _, o := range ch.Options {
				if slices.Contains(q.Picked, o.ID) {
					want = o.Items
				}
			}
		}
		on = append(on, want...)
		for _, id := range ch.Held() {
			if !slices.Contains(want, id) {
				off = append(off, id)
			}
		}
	}
	return on, off
}

// choiceRun is what the choices change: the prompt, the items to add (on
// a first run, the base too) and the picks to uninstall.
type choiceRun struct {
	prompt       string
	adds, forget []string
}

// askChoices asks the choices; on a first run, every item turned on and
// the base are added, else only the change from a's picks.
func askChoices(cat *catalog.Catalog, a *answers.Answers, first bool) (*choiceRun, error) {
	qs := choiceQuestions(cat, a, first)
	if err := ui.Ask(qs); err != nil {
		return nil, err
	}
	return choiceChange(cat, a, first, qs), nil
}

// choiceChange is what answered qs change for a.
func choiceChange(cat *catalog.Catalog, a *answers.Answers, first bool, qs []*ui.Question) *choiceRun {
	on, off := chosen(cat, qs, a.Picks, first)
	r := &choiceRun{prompt: qs[0].Picked[0]}
	if first {
		on = append(cat.Base(), on...)
	}
	for _, id := range on {
		if !slices.Contains(a.Picks, id) && !slices.Contains(r.adds, id) {
			r.adds = append(r.adds, id)
		}
	}
	if !first {
		for _, id := range off {
			if slices.Contains(a.Picks, id) {
				r.forget = append(r.forget, id)
			}
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
