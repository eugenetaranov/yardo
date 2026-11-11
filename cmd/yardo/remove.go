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
	"path"
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

// removal is what uninstalling some items takes: Order is every item to
// uninstall, each after what needs it (extra mise tools first); Also, by
// the item asked for, the ones that go with it (they need it), and
// Forget the ones that need it but yardo can't uninstall: they're
// forgotten, staying installed.
type removal struct {
	Order        []string
	Also, Forget map[string][]string
}

// planRemoval works out what uninstalling ids (from picks) takes.
func planRemoval(cat *catalog.Catalog, picks, ids []string) removal {
	r := removal{Also: map[string][]string{}, Forget: map[string][]string{}}
	seen := map[string]bool{}
	var items []string
	for _, id := range ids {
		if strings.HasPrefix(id, "mise:") {
			r.Order = append(r.Order, id)
			continue
		}
		seen[id] = true
		items = append(items, id)
	}
	for _, id := range ids {
		if strings.HasPrefix(id, "mise:") {
			continue
		}
		for _, d := range neededBy(cat, picks, id) {
			if seen[d] {
				continue
			}
			seen[d] = true
			if it := cat.Item(d); it != nil && it.Removable() {
				r.Also[id] = append(r.Also[id], d)
				items = append(items, d)
			} else {
				r.Forget[id] = append(r.Forget[id], d)
			}
		}
	}
	// What needs another goes first: it has fewer that need it.
	count := map[string]int{}
	for _, id := range items {
		count[id] = len(neededBy(cat, picks, id))
	}
	slices.SortStableFunc(items, func(x, y string) int { return count[x] - count[y] })
	r.Order = append(r.Order, items...)
	return r
}

// forgotten are all the items r forgets.
func (r removal) forgotten() []string {
	var out []string
	for _, f := range r.Forget {
		out = append(out, f...)
	}
	return out
}

// keptAfter is what stays in use once gone leave the picks: the items
// left, and the extra mise tools.
func keptAfter(cat *catalog.Catalog, env *steps.Env, a *answers.Answers, gone []string) (steps.Kept, error) {
	stay := slices.DeleteFunc(slices.Clone(a.Picks), func(p string) bool { return slices.Contains(gone, p) })
	items, err := plan.Resolve(cat, stay, len(a.MiseExtra) > 0)
	if err != nil {
		return steps.Kept{}, err
	}
	k := steps.KeptBy(env, items)
	for _, t := range a.MiseExtra {
		if !slices.Contains(gone, "mise:"+t) {
			k.Tools[steps.MiseName(t)] = "your extra mise tools"
		}
	}
	return k, nil
}

// shortTool is a mise tool's name as people say it:
// github:eugenetaranov/ssmsearch is ssmsearch, pipx:git+https://…/rds-mysql.git
// is rds-mysql.
func shortTool(n string) string {
	if strings.HasPrefix(n, "/") || strings.HasPrefix(n, "~") || !strings.Contains(n, ":") {
		return n
	}
	return strings.TrimSuffix(path.Base(n), ".git")
}

// plans say what uninstalling ids does, for the question (the board's,
// yardo remove's): what goes and stays, what goes with each and is
// forgotten, and a queued add (adds) that brings it back. It changes
// nothing.
func (o *options) plans(ctx context.Context, cat *catalog.Catalog, a *answers.Answers, ids, adds []string) map[string]ui.UninstallPlan {
	out := map[string]ui.UninstallPlan{}
	env, err := o.env(a, nil, nil)
	if err != nil {
		for _, id := range ids {
			out[id] = ui.UninstallPlan{Err: err.Error()}
		}
		return out
	}
	r := planRemoval(cat, a.Picks, ids)
	keep, err := keptAfter(cat, env, a, append(slices.Clone(r.Order), r.forgotten()...))
	for _, id := range ids {
		if t, ok := strings.CutPrefix(id, "mise:"); ok {
			out[id] = ui.UninstallPlan{Off: []string{t + " (a mise tool)"}}
			continue
		}
		if err != nil {
			out[id] = ui.UninstallPlan{Err: err.Error()}
			continue
		}
		// What goes with it: its own and theirs.
		var u ui.UninstallPlan
		for _, x := range append([]string{id}, r.Also[id]...) {
			p, err := steps.PlanRemove(ctx, env, cat.Item(x), keep)
			if err != nil {
				u.Err = err.Error()
				break
			}
			if x == id {
				u.Say = p.Say
			}
			for _, off := range p.Off {
				if off = shortTool(off); !slices.Contains(u.Off, off) {
					u.Off = append(u.Off, off)
				}
			}
			for _, k := range p.Keep {
				if !slices.Contains(u.Stays, k) {
					u.Stays = append(u.Stays, k)
				}
			}
		}
		u.Also, u.Forgets = labels(cat, r.Also[id]), labels(cat, r.Forget[id])
		for _, n := range adds {
			if items, err := plan.Resolve(cat, []string{n}, false); err == nil &&
				slices.ContainsFunc(items, func(it *catalog.Item) bool { return it.ID == id }) {
				u.Back = append(u.Back, ui.Short(cat.Item(n).Label))
			}
		}
		out[id] = u
	}
	return out
}

func labels(cat *catalog.Catalog, ids []string) []string {
	var out []string
	for _, id := range ids {
		if it := cat.Item(id); it != nil {
			out = append(out, ui.Short(it.Label))
		} else {
			out = append(out, strings.TrimPrefix(id, "mise:"))
		}
	}
	return out
}

// uninstalled is how an uninstall went: a line per item, and the ids
// that went and that failed.
type uninstalled struct {
	Lines        []string
	Done, Failed []string
}

// Title says it in a few words: "Done: 2 uninstalled, 1 failed".
func (u uninstalled) Title() string {
	s := fmt.Sprintf("Done: %d uninstalled", len(u.Done))
	if len(u.Failed) > 0 {
		s += fmt.Sprintf(", %d failed", len(u.Failed))
	}
	return s
}

// uninstall removes ids (the board's, yardo remove's), and what needs
// them, each after what needs it; what needs them that yardo can't
// uninstall is forgotten. A failed item stays installed, and the others
// still run.
func (o *options) uninstall(ctx context.Context, cat *catalog.Catalog, st *answers.Store, a *answers.Answers, ids []string, stdout io.Writer) (uninstalled, error) {
	var res uninstalled
	logf, err := os.OpenFile(o.log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return res, err
	}
	defer logf.Close()
	env, err := o.env(a, nil, logf)
	if err != nil {
		return res, err
	}
	r := planRemoval(cat, a.Picks, ids)
	fail := func(id, name string, err error) {
		res.Lines = append(res.Lines, "✗ "+name+": the uninstall stopped ("+err.Error()+"). Retry picks up where it stopped. Log: "+o.log)
		res.Failed = append(res.Failed, id)
	}
	purged := false
	for n, id := range r.Order {
		fmt.Fprintf(logf, "=== yardo %s, uninstall %s, user %s\n", version, id, a.User)
		if t, ok := strings.CutPrefix(id, "mise:"); ok {
			fmt.Fprintf(stdout, "Uninstalling %s…\n", t)
			if _, err := env.RunLogged(ctx, steps.Cmd{Name: "mise", Args: []string{"unuse", "-g", t}, AsUser: true}); err != nil {
				fail(id, t, err)
				continue
			}
			a.MiseExtra = slices.DeleteFunc(a.MiseExtra, func(x string) bool { return x == t })
			res.Lines, res.Done = append(res.Lines, "✓ "+t+" uninstalled."), append(res.Done, id)
			if err := st.Save(a); err != nil {
				return res, err
			}
			continue
		}
		it := cat.Item(id)
		if it == nil {
			return res, fmt.Errorf("%q is not in the catalog", id)
		}
		name := ui.Short(it.Label)
		fmt.Fprintf(stdout, "Uninstalling %s…\n", name)
		// What's still to go stays out of what's kept: two items that share
		// a package, both going, take it off.
		keep, err := keptAfter(cat, env, a, append(slices.Clone(r.Order[n:]), r.forgotten()...))
		if err != nil {
			return res, err
		}
		p, err := steps.PlanRemove(ctx, env, it, keep)
		if err == nil {
			_, err = p.Run(ctx)
		}
		if err != nil {
			fail(id, name, err)
			continue
		}
		purged = purged || slices.ContainsFunc(p.Off, func(x string) bool { return !strings.ContainsAny(x, "/ ") })
		a.Uninstalled(id)
		line := "✓ " + name + " uninstalled."
		if kept := slices.DeleteFunc(slices.Clone(p.Keep), func(k string) bool {
			return !strings.Contains(k, "by hand") && !strings.Contains(k, "local changes") && !strings.Contains(k, "your keys")
		}); len(kept) > 0 {
			line += " Kept " + strings.Join(kept, ", ") + "."
		}
		if f := r.Forget[id]; len(f) > 0 {
			a.Unpick(f)
			line += " Forgot " + strings.Join(labels(cat, f), ", ") + ": they needed it, and stay installed."
		}
		res.Lines, res.Done = append(res.Lines, line), append(res.Done, id)
		if err := st.Save(a); err != nil {
			return res, err
		}
	}
	if purged {
		if n := autoremovable(ctx, env); n > 0 {
			res.Lines = append(res.Lines, fmt.Sprintf("%d packages nothing needs now: sudo apt autoremove frees them.", n))
		}
	}
	return res, nil
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

// partsOf are the picked items that are part of id (its login).
func partsOf(cat *catalog.Catalog, picks []string, id string) []string {
	var out []string
	for _, p := range picks {
		if it := cat.Item(p); it != nil && it.PartOf == id {
			out = append(out, p)
		}
	}
	return out
}

// exitErr is an error with its own exit code.
type exitErr struct {
	code int
	err  error
}

func (e *exitErr) Error() string { return e.err.Error() }
func (e *exitErr) Unwrap() error { return e.err }

// resolve finds what name means for yardo remove: an extra mise tool, or
// an item by its label's short name or id (any case), a tool before its
// parts (netbird is NetBird, not its login). It says why when it can't
// be uninstalled.
func resolve(cat *catalog.Catalog, a *answers.Answers, name string) (string, error) {
	n := strings.ToLower(strings.TrimPrefix(name, "mise:"))
	for _, t := range a.MiseExtra {
		if strings.ToLower(t) == n || strings.ToLower(steps.MiseName(t)) == n {
			return "mise:" + t, nil
		}
	}
	var it *catalog.Item
	for _, parts := range []bool{false, true} {
		for _, x := range cat.Items {
			if !x.Hidden() && (x.PartOf != "") == parts && (strings.ToLower(x.ID) == n || strings.ToLower(ui.Short(x.Label)) == n) {
				it = x
				break
			}
		}
		if it != nil {
			break
		}
	}
	switch {
	case it == nil:
		return "", fmt.Errorf("%s: no such item (yardo --list lists them)", name)
	case !slices.Contains(a.Picks, it.ID):
		return "", fmt.Errorf("%s isn't installed", name)
	case !it.Removable():
		why := it.KeepSay
		if why == "" {
			why = "its own installer or settings set it up"
		}
		return "", fmt.Errorf("yardo can't uninstall %s: %s", ui.Short(it.Label), why)
	}
	return it.ID, nil
}

// removeCmd is yardo remove ITEM…: it says what each would take off,
// keep and take with it, asks, then uninstalls them. Exit 2: a name it
// can't uninstall (nothing is done); 1: an uninstall failed.
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
		return &exitErr{2, fmt.Errorf("%w; nothing uninstalled", errors.Join(bad...))}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	plans := o.plans(ctx, cat, a, ids, nil)
	for _, id := range ids {
		fmt.Fprintf(stdout, "Uninstall %s\n", labels(cat, []string{id})[0])
		for _, l := range plans[id].Lines(true) {
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
			fmt.Fprintln(stdout, "Nothing uninstalled.")
			return nil
		}
	}
	res, err := o.uninstall(ctx, cat, st, a, ids, stdout)
	for _, l := range res.Lines {
		fmt.Fprintln(stdout, l)
	}
	if err != nil {
		return err
	}
	if len(res.Failed) > 0 {
		return &exitErr{1, errors.New("not everything was uninstalled")}
	}
	return nil
}
