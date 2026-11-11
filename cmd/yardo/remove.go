package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
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

// leaving is what uninstalling ids takes out of the picks: ids, and each
// one's dependents (they stop working, so yardo stops managing them),
// by id.
func leaving(cat *catalog.Catalog, picks, ids []string) map[string][]string {
	deps := map[string][]string{}
	for _, id := range ids {
		for _, d := range neededBy(cat, picks, id) {
			if !slices.Contains(ids, d) {
				deps[id] = append(deps[id], d)
			}
		}
	}
	return deps
}

// keptAfter is what stays in use once ids and their dependents leave
// the picks: the items left, and the extra mise tools.
func keptAfter(cat *catalog.Catalog, env *steps.Env, a *answers.Answers, ids []string, deps map[string][]string) (steps.Kept, error) {
	gone := slices.Clone(ids)
	for _, d := range deps {
		gone = append(gone, d...)
	}
	stay := slices.DeleteFunc(slices.Clone(a.Picks), func(p string) bool { return slices.Contains(gone, p) })
	items, err := plan.Resolve(cat, stay, len(a.MiseExtra) > 0)
	if err != nil {
		return steps.Kept{}, err
	}
	k := steps.KeptBy(env, items)
	for _, t := range a.MiseExtra {
		if !slices.Contains(ids, "mise:"+t) {
			k.Tools[steps.MiseName(t)] = "your extra mise tools"
		}
	}
	return k, nil
}

// planLines say what uninstalling ids does, each as lines for the
// question (the board's, yardo remove's): what it does, takes off and
// keeps, what stops being managed, and an add (adds) that puts it back.
// It changes nothing.
func (o *options) planLines(ctx context.Context, cat *catalog.Catalog, a *answers.Answers, ids, adds []string) map[string][]string {
	out := map[string][]string{}
	env, err := o.env(a, nil, nil)
	if err != nil {
		for _, id := range ids {
			out[id] = []string{"Can't work out what goes: " + err.Error()}
		}
		return out
	}
	deps := leaving(cat, a.Picks, ids)
	keep, err := keptAfter(cat, env, a, ids, deps)
	for _, id := range ids {
		if t, ok := strings.CutPrefix(id, "mise:"); ok {
			out[id] = []string{"Takes off: " + t + " (mise unuse -g " + t + ")"}
			continue
		}
		var ls []string
		var p *steps.RemovePlan
		if err == nil {
			p, err = steps.PlanRemove(ctx, env, cat.Item(id), keep)
		}
		if err != nil {
			out[id] = []string{"Can't work out what goes: " + err.Error()}
			err = nil
			continue
		}
		if p.Say != "" {
			ls = append(ls, "It "+p.Say+".")
		}
		if len(p.Off) > 0 {
			ls = append(ls, "Takes off: "+strings.Join(p.Off, ", "))
		} else if p.Say == "" {
			ls = append(ls, "Nothing of it is left to take off.")
		}
		if len(p.Keep) > 0 {
			ls = append(ls, "Keeps: "+strings.Join(p.Keep, ", "))
		}
		if d := deps[id]; len(d) > 0 {
			ls = append(ls, "⚠ "+strings.Join(labels(cat, d), ", ")+" need it and will stop working; yardo stops managing them too (they stay installed).")
		}
		for _, n := range adds {
			if items, err := plan.Resolve(cat, []string{n}, false); err == nil &&
				slices.ContainsFunc(items, func(it *catalog.Item) bool { return it.ID == id }) {
				ls = append(ls, "↺ "+ui.Short(cat.Item(n).Label)+", queued to install, needs it: it's installed again.")
			}
		}
		out[id] = ls
	}
	return out
}

func labels(cat *catalog.Catalog, ids []string) []string {
	var out []string
	for _, id := range ids {
		if it := cat.Item(id); it != nil {
			out = append(out, ui.Short(it.Label))
		} else {
			out = append(out, id)
		}
	}
	return out
}

// uninstall removes ids (the board's d, yardo remove), one after another:
// what needs one stops being managed too, staying installed. It returns a
// line per item saying how it went, and whether any failed; a failed
// item stays picked, and the others still run.
func (o *options) uninstall(ctx context.Context, cat *catalog.Catalog, st *answers.Store, a *answers.Answers, ids []string, stdout io.Writer) ([]string, bool, error) {
	logf, err := os.OpenFile(o.log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, false, err
	}
	defer logf.Close()
	env, err := o.env(a, nil, logf)
	if err != nil {
		return nil, false, err
	}
	var said []string
	failed, purged := false, false
	for n, id := range ids {
		fmt.Fprintf(logf, "=== yardo %s, uninstall %s, user %s\n", version, id, a.User)
		if t, ok := strings.CutPrefix(id, "mise:"); ok {
			fmt.Fprintf(stdout, "Uninstalling %s…\n", t)
			if _, err := env.RunLogged(ctx, steps.Cmd{Name: "mise", Args: []string{"unuse", "-g", t}, AsUser: true}); err != nil {
				said, failed = append(said, "✗ Uninstalling "+t+" failed: "+err.Error()+" (see "+o.log+")"), true
				continue
			}
			a.MiseExtra = slices.DeleteFunc(a.MiseExtra, func(x string) bool { return x == t })
			said = append(said, "✓ Uninstalled "+t+".")
			if err := st.Save(a); err != nil {
				return said, failed, err
			}
			continue
		}
		it := cat.Item(id)
		if it == nil {
			return said, failed, fmt.Errorf("%q is not in the catalog", id)
		}
		name := ui.Short(it.Label)
		fmt.Fprintf(stdout, "Uninstalling %s…\n", name)
		// What's still to go stays out of what's kept: two items that share
		// a package, both going, take it off.
		rest := ids[n:]
		deps := leaving(cat, a.Picks, rest)
		keep, err := keptAfter(cat, env, a, rest, deps)
		if err != nil {
			return said, failed, err
		}
		p, err := steps.PlanRemove(ctx, env, it, keep)
		if err == nil {
			_, err = p.Run(ctx)
		}
		if err != nil {
			said, failed = append(said, "✗ Uninstalling "+name+" failed: "+err.Error()+" (see "+o.log+")"), true
			continue
		}
		purged = purged || slices.ContainsFunc(p.Off, func(x string) bool { return !strings.Contains(x, "/") && !strings.Contains(x, " ") })
		a.Unpick(append([]string{id}, deps[id]...))
		if err := st.Save(a); err != nil {
			return said, failed, err
		}
		line := "✓ Uninstalled " + name + "."
		if kept := slices.DeleteFunc(slices.Clone(p.Keep), func(k string) bool {
			return !strings.HasSuffix(k, "(edited by hand)") && !strings.HasSuffix(k, "(has local changes)")
		}); len(kept) > 0 {
			line += " Kept " + strings.Join(kept, ", ") + "."
		}
		if d := deps[id]; len(d) > 0 {
			line += " yardo stopped managing " + strings.Join(labels(cat, d), ", ") + " (they needed it)."
		}
		said = append(said, line)
	}
	if purged {
		if n := autoremovable(ctx, env); n > 0 {
			said = append(said, fmt.Sprintf("sudo apt autoremove frees %d more package(s) nothing needs now.", n))
		}
	}
	return said, failed, nil
}

// autoremovable is how many packages apt would autoremove.
func autoremovable(ctx context.Context, env *steps.Env) int {
	out, err := env.Exec.Run(ctx, steps.Cmd{Name: "apt-get", Args: []string{"-s", "autoremove"}, Env: steps.AptEnv})
	if err != nil {
		return 0
	}
	n := 0
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "Remv ") {
			n++
		}
	}
	return n
}

// exitErr is an error with its own exit code.
type exitErr struct {
	code int
	err  error
}

func (e *exitErr) Error() string { return e.err.Error() }
func (e *exitErr) Unwrap() error { return e.err }

// resolve finds what name means for yardo remove: an item by id or by
// its label's short name (any case), an extra mise tool, or a login's
// client (remove_with). It says why when it can't be removed.
func resolve(cat *catalog.Catalog, a *answers.Answers, name string) (string, error) {
	n := strings.ToLower(strings.TrimPrefix(name, "mise:"))
	for _, t := range a.MiseExtra {
		if strings.ToLower(t) == n || strings.ToLower(steps.MiseName(t)) == n {
			return "mise:" + t, nil
		}
	}
	var it *catalog.Item
	for _, x := range cat.Items {
		if !x.Hidden() && (strings.ToLower(x.ID) == n || strings.ToLower(ui.Short(x.Label)) == n) {
			it = x
			break
		}
	}
	switch {
	case it == nil:
		return "", fmt.Errorf("%s: no such item (yardo --list lists them)", name)
	case !slices.Contains(a.Picks, it.ID):
		return "", fmt.Errorf("%s isn't installed", name)
	case it.RemoveWith != "":
		return resolve(cat, a, it.RemoveWith)
	case !it.Removable():
		why := it.KeepSay
		if why == "" {
			why = "its own installer or settings set it up"
		}
		return "", fmt.Errorf("yardo won't uninstall %s: %s", ui.Short(it.Label), why)
	}
	return it.ID, nil
}

// removeCmd is yardo remove ITEM…: it says what each would take off,
// keep and stop, asks, then uninstalls them. Exit 2: a name it can't
// remove (nothing is done); 1: an uninstall failed.
func (o *options) removeCmd(cat *catalog.Catalog, st *answers.Store, args []string, stdout, stderr io.Writer) error {
	sfs := flag.NewFlagSet("yardo remove", flag.ContinueOnError)
	sfs.SetOutput(stderr)
	sfs.BoolVar(&o.yes, "yes", o.yes, "don't ask")
	sfs.BoolVar(&o.yes, "y", o.yes, "short for --yes")
	sfs.BoolVar(&o.dryRun, "dry-run", o.dryRun, "say what it would do; change nothing")
	sfs.BoolVar(&o.dryRun, "n", o.dryRun, "short for --dry-run")
	// Flags may come after the names too: yardo remove aws --dry-run.
	var names []string
	for rest := args; len(rest) > 0; {
		if err := sfs.Parse(rest); err != nil {
			return &exitErr{2, err}
		}
		if sfs.NArg() == 0 {
			break
		}
		names, rest = append(names, sfs.Arg(0)), sfs.Args()[1:]
	}
	if len(names) == 0 {
		return &exitErr{2, errors.New("which items? yardo remove ITEM… (yardo --list lists them)")}
	}
	u, err := o.userFor(st)
	if err != nil {
		return err
	}
	a, _, err := st.Load(u)
	if err != nil {
		return err
	}
	a.User = u
	var ids []string
	var bad []error
	for _, n := range names {
		id, err := resolve(cat, a, n)
		if err != nil {
			bad = append(bad, err)
		} else if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(bad) > 0 {
		return &exitErr{2, fmt.Errorf("%w; nothing removed", errors.Join(bad...))}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	plans := o.planLines(ctx, cat, a, ids, nil)
	for _, id := range ids {
		name := strings.TrimPrefix(id, "mise:")
		if it := cat.Item(id); it != nil {
			name = ui.Short(it.Label)
		}
		fmt.Fprintf(stdout, "✗ Uninstall %s\n", name)
		for _, l := range plans[id] {
			fmt.Fprintf(stdout, "    %s\n", l)
		}
	}
	if o.dryRun {
		return nil
	}
	if !o.yes {
		fmt.Fprint(stdout, "Uninstall? [y/N] ")
		in := o.in
		if in == nil {
			in = os.Stdin
		}
		l, _ := bufio.NewReader(in).ReadString('\n')
		if s := strings.ToLower(strings.TrimSpace(l)); s != "y" && s != "yes" {
			fmt.Fprintln(stdout, "Nothing removed.")
			return nil
		}
	}
	said, failed, err := o.uninstall(ctx, cat, st, a, ids, stdout)
	for _, l := range said {
		fmt.Fprintln(stdout, l)
	}
	if err != nil {
		return err
	}
	if failed {
		return &exitErr{1, errors.New("not everything was uninstalled")}
	}
	return nil
}
