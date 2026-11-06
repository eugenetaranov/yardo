package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/plan"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// neededBy are the picks other than id whose plan has id in it: they
// stop working without it.
func neededBy(cat *catalog.Catalog, picks []string, id string) []string {
	var out []string
	for _, p := range picks {
		if p == id {
			continue
		}
		if items, err := plan.Resolve(cat, []string{p}, false); err == nil &&
			slices.ContainsFunc(items, func(it *catalog.Item) bool { return it.ID == id && !it.Always }) {
			out = append(out, p)
		}
	}
	return out
}

// removeItem uninstalls id (from the board's d): what needs it stops being
// managed too, staying installed. It says how it went, for the board; a
// failed removal is said, not returned.
func (o *options) removeItem(ctx context.Context, cat *catalog.Catalog, st *answers.Store, a *answers.Answers, id string, stdout io.Writer) (string, error) {
	logf, err := os.OpenFile(o.log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", err
	}
	defer logf.Close()
	env, err := o.env(a, nil, logf)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(logf, "=== yardo %s, remove %s, user %s\n", version, id, a.User)
	if t, ok := strings.CutPrefix(id, "mise:"); ok {
		fmt.Fprintf(stdout, "Removing %s…\n", t)
		if _, err := env.RunLogged(ctx, steps.Cmd{Name: "mise", Args: []string{"unuse", "-g", t}, AsUser: true}); err != nil {
			return "✗ Removing " + t + " failed: " + err.Error() + " (see " + o.log + ")", nil
		}
		a.MiseExtra = slices.DeleteFunc(a.MiseExtra, func(x string) bool { return x == t })
		return "✓ Removed " + t + ".", st.Save(a)
	}
	it := cat.Item(id)
	if it == nil {
		return "", fmt.Errorf("%q is not in the catalog", id)
	}
	deps := neededBy(cat, a.Picks, id)
	stay := slices.DeleteFunc(slices.Clone(a.Picks), func(p string) bool { return p == id || slices.Contains(deps, p) })
	kept, err := plan.Resolve(cat, stay, len(a.MiseExtra) > 0)
	if err != nil {
		return "", err
	}
	name := ui.Short(it.Label)
	fmt.Fprintf(stdout, "Removing %s…\n", name)
	if _, err := steps.Remove(ctx, env, it, steps.KeptBy(env, kept)); err != nil {
		return "✗ Removing " + name + " failed: " + err.Error() + " (see " + o.log + ")", nil
	}
	a.Unpick(append([]string{id}, deps...))
	if err := st.Save(a); err != nil {
		return "", err
	}
	said := "✓ Removed " + name + "."
	if len(deps) > 0 {
		var names []string
		for _, d := range deps {
			names = append(names, ui.Short(cat.Item(d).Label))
		}
		said += " yardo stopped managing " + strings.Join(names, ", ") + " (they needed it)."
	}
	return said, nil
}
