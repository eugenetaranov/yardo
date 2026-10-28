package run

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/plan"
	"github.com/eugenetaranov/yardo/internal/steps"
)

// DryRun prints what installing items would do, item by item, and changes
// nothing.
func DryRun(w io.Writer, e *steps.Env, items []*catalog.Item, extraMise []string) error {
	fmt.Fprintf(w, "For user: %s\n", e.User)
	extra := func() {
		if len(extraMise) > 0 {
			fmt.Fprintf(w, "Extra mise tools\n  %-6s %s\n", "mise", strings.Join(steps.LockedTools(e, extraMise), " "))
		}
	}
	n := plan.Configuring(items)
	if n > 0 || len(extraMise) > 0 {
		fmt.Fprintln(w, "== Install")
	}
	for i, x := range build(items) {
		if x.err != nil {
			return fmt.Errorf("%s: %w", x.item.ID, x.err)
		}
		if i == n {
			extra()
			fmt.Fprintln(w, "== Configure")
		}
		fmt.Fprintf(w, "%s\n", label(x.item))
		for _, st := range x.steps {
			for _, a := range st.Plan(e) {
				fmt.Fprintf(w, "  %-6s %s\n", a.Kind, a.Target)
			}
		}
	}
	if n == len(items) {
		extra()
	}
	return nil
}

// Status is one item's verify result.
type Status struct {
	Item *catalog.Item
	// State is "ok"; "missing" (none of its steps done); "partial" (some
	// done); "down" (set up, but a service of it isn't running); "secret"
	// (set up, but values left empty: Inputs); or "unchecked" (an item still
	// run by the bash installer, which has no check).
	State string
	// Detail is the first step not done, or the service that's down.
	Detail string
	// Done of Total steps have nothing to do.
	Done, Total int
	// Inputs are the values left empty ("file: KEY").
	Inputs []string
}

// Problem reports whether the state needs fixing (a secret to fill in or
// an unchecked item doesn't).
func (s Status) Problem() bool {
	return s.State == "missing" || s.State == "partial" || s.State == "down"
}

// Verify checks each item's steps without changing anything.
func Verify(ctx context.Context, e *steps.Env, items []*catalog.Item) []Status {
	defer e.CacheChecks()()
	var out []Status
	for _, x := range build(items) {
		st := Status{Item: x.item, State: "ok"}
		if x.err != nil {
			st.State, st.Detail = "missing", x.err.Error()
			out = append(out, st)
			continue
		}
		down, failed, legacy := "", 0, false
		for _, s := range x.steps {
			if _, ok := s.(steps.LegacyStep); ok {
				legacy = true
				continue
			}
			if f, ok := s.(steps.Follower); ok && f.Follows() {
				continue
			}
			if a, ok := s.(steps.Always); ok && a.Always() {
				continue
			}
			st.Total++
			if done, err := s.Check(ctx, e); err == nil && done {
				st.Done++
				continue
			}
			failed++
			if sv, ok := s.(steps.Service); ok {
				if d, name := sv.Down(ctx, e); d {
					// Set up but not running: down, unless something else is missing.
					if down == "" {
						down = name + " isn't running"
					}
					st.Done++
					continue
				}
			}
			if st.Detail == "" {
				if acts := s.Plan(e); len(acts) > 0 {
					st.Detail = acts[0].String()
				}
			}
		}
		downOnly := down != "" && st.Done == st.Total
		if downOnly {
			st.Done-- // the service's step isn't done
		}
		for _, s := range x.steps {
			if in, ok := s.(steps.Inputs); ok {
				st.Inputs = append(st.Inputs, in.Missing(ctx, e)...)
			}
		}
		switch {
		case legacy && failed == 0:
			st.State = "unchecked"
		case downOnly:
			st.State, st.Detail = "down", down
		case failed > 0 && st.Done > 0:
			st.State = "partial"
		case failed > 0:
			st.State = "missing"
		case len(st.Inputs) > 0:
			st.State, st.Detail = "secret", st.Inputs[0]
		}
		out = append(out, st)
	}
	return out
}

// Summary writes what is installed, grouped by how, with how to manage it
// (~/yardo.txt).
func Summary(w io.Writer, items []*catalog.Item, extraMise []string, now time.Time) {
	var miseL, aptL, otherL []string
	row := func(it *catalog.Item) string { return fmt.Sprintf("  %-40s %s", label(it), it.Describe()) }
	for _, it := range items {
		if it.Hidden() {
			continue
		}
		kinds := map[string]bool{}
		for _, s := range it.Steps {
			kinds[s.Type] = true
		}
		switch {
		case len(kinds) == 1 && kinds["mise"]:
			miseL = append(miseL, row(it))
		case len(kinds) == 1 && kinds["apt"], strings.HasPrefix(it.Describe(), "apt install "):
			aptL = append(aptL, row(it))
		default:
			otherL = append(otherL, row(it))
		}
	}
	if len(extraMise) > 0 {
		miseL = append(miseL, fmt.Sprintf("  %-40s mise use -g %s", "Extra mise tools", strings.Join(extraMise, " ")))
	}
	fmt.Fprintf(w, "yardo — what is installed and how to manage it (updated %s)\n\n", now.UTC().Format("2006-01-02"))
	if len(miseL) > 0 {
		fmt.Fprintln(w, "Installed with mise (per user, ~/.config/mise/config.toml):")
		fmt.Fprintln(w, strings.Join(miseL, "\n"))
		fmt.Fprintln(w, "  update all: mise up · list: mise ls · pin: mise use -g <tool>@<version> · remove: mise rm -g <tool>")
		fmt.Fprintln(w)
	}
	if len(aptL) > 0 {
		fmt.Fprintln(w, "Installed with apt (system):")
		fmt.Fprintln(w, strings.Join(aptL, "\n"))
		fmt.Fprintln(w, "  update: sudo apt update && sudo apt upgrade · remove: sudo apt remove <package>")
		fmt.Fprintln(w)
	}
	if len(otherL) > 0 {
		fmt.Fprintln(w, "Vendor repos, release binaries, scripts and settings:")
		fmt.Fprintln(w, strings.Join(otherL, "\n"))
		fmt.Fprintln(w, "  vendor apt repos update with apt upgrade; claude and omp update themselves.")
		fmt.Fprintln(w)
	}
	if u := Unverified(items); len(u) > 0 {
		fmt.Fprintln(w, "Installed without a pinned checksum (what was fetched wasn't checked against a known hash):")
		for _, x := range u {
			fmt.Fprintf(w, "  %-40s %s\n", label(x.Item), x.Reason)
		}
		fmt.Fprintln(w)
	}
}

// UnverifiedItem is an item with a download that isn't pinned, and why.
type UnverifiedItem struct {
	Item   *catalog.Item
	Reason string
}

// Unverified lists the items with an unverified download.
func Unverified(items []*catalog.Item) []UnverifiedItem {
	var out []UnverifiedItem
	for _, it := range items {
		var why []string
		for _, s := range it.Steps {
			if s.Unverified != "" {
				why = append(why, s.Unverified)
			}
		}
		if len(why) > 0 {
			out = append(out, UnverifiedItem{it, strings.Join(why, "; ")})
		}
	}
	return out
}
