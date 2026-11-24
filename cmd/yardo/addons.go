package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/eugenetaranov/yardo/internal/addon"
	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/mcp"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// Skills and MCP servers from elsewhere: each is fetched at a fixed
// version into the holding area, checked, its report shown, and installed
// only when the user says so (see internal/addon).

func (k *asker) typeIn(title, desc string) (string, error) {
	if k.text != nil {
		return k.text(title, desc)
	}
	return ui.Text(title, desc)
}

func (k *asker) chooseMany(title string, opts, picked []string) ([]string, error) {
	if k.many != nil {
		return k.many(title, opts, picked)
	}
	return ui.ChooseMany(title, opts, picked)
}

// addonRun is one add-on session: the user's, with what it needs.
type addonRun struct {
	k   *asker
	env *steps.Env
	cat *catalog.Catalog
	out io.Writer
	yes bool
	// sync registers the MCP servers after name's install, saying how
	// name went (mcp-sync).
	sync func(ctx context.Context, name string)
}

func (o *options) addonRun(ctx context.Context, cat *catalog.Catalog, st *answers.Store, out io.Writer, yes bool) (*addonRun, *answers.Answers, error) {
	u, err := o.userFor(st)
	if err != nil {
		return nil, nil, err
	}
	if os.Geteuid() != 0 {
		return nil, nil, errors.New("run it with sudo: it holds add-ons where only root reads them")
	}
	a, _, err := st.Load(u)
	if err != nil {
		return nil, nil, err
	}
	a.User = u
	r, err := (&asker{cat: cat, a: a, share: o.share(), out: out}).addons(ctx)
	if err != nil {
		return nil, nil, err
	}
	r.yes = yes
	return r, a, nil
}

// addons is an add-on session for k's user, saying what it does on k.out.
func (k *asker) addons(ctx context.Context) (*addonRun, error) {
	env, err := newEnv(k.a.User, k.share, io.Discard, false)
	if err != nil {
		return nil, err
	}
	env.Share = k.share
	addon.Version = version
	u := k.a.User
	addon.Token = func() string { return ghToken(ctx, u) }
	r := &addonRun{k: k, env: env, cat: k.cat, out: k.out}
	addon.Extra = func(ctx context.Context, h *addon.Held, files map[string][]byte, earlier []addon.Finding) (fs []addon.Finding, model string) {
		r.progress(ctx, fmt.Sprintf("Claude is reading %s (%s)", h.Name, addon.QuickModel), func(ctx context.Context) {
			fs, model = addon.ReviewLayer(ctx, h, files, earlier)
		})
		return fs, model
	}
	r.sync = func(ctx context.Context, name string) {
		// Another server failing isn't this one's: only its line counts.
		_ = k.syncAll(ctx, func(l string) {
			if strings.HasPrefix(l, name+": ") {
				fmt.Fprintln(r.out, l)
			}
		})
	}
	return r, nil
}

// syncAll registers k's user's servers: mcp-sync NAME… saves the names
// as the list, so all of them, never one alone.
func (k *asker) syncAll(ctx context.Context, line func(string)) error {
	if k.mcpSync != nil {
		return k.mcpSync(mcpNames(k.a.MCPServers)...)
	}
	a := *k.a
	if a.MCPPort == 0 && k.port != nil {
		a.MCPPort = k.port()
	}
	out, err := mcpSyncCmd(ctx, k.share, &a, mcpNames(k.a.MCPServers)...).CombinedOutput()
	for l := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line(l)
	}
	return err
}

// menuDo does what the menu asked of a skill, or of a server from
// elsewhere (act.Do: add, elsewhere, check, review, delete), its prompts
// on the terminal; it says how it went.
func (r *addonRun) menuDo(ctx context.Context, kind string, act ui.MCPAct) (string, error) {
	switch act.Do {
	case "add":
		srcs, err := r.k.typeIn("Skills to add from GitHub", "owner/repo[/path][@ref], by spaces: each is held and checked before you decide")
		if err != nil || strings.TrimSpace(srcs) == "" {
			return "Nothing added.", err
		}
		r.ensureVeil(ctx)
		var errs []error
		for _, s := range strings.Fields(srcs) {
			errs = append(errs, r.addSkills(ctx, s))
		}
		return "Done: the Skills list says what's installed and what's held.", errors.Join(errs...)
	case "elsewhere":
		src, err := r.k.typeIn("Where it's from", "a Docker image (ghcr.io/acme/mcp-x:1.2), an https URL, or a spec file's path")
		if err != nil || strings.TrimSpace(src) == "" {
			return "Nothing added.", err
		}
		r.ensureVeil(ctx)
		h, files, err := r.holdServer(ctx, strings.TrimSpace(src), "")
		if err != nil {
			return "", err
		}
		if err := addon.Hold(h, files); err != nil {
			return "", err
		}
		return fmt.Sprintf("Done: %s is on the list, installed or held.", h.Name), r.decide(ctx, h)
	case "check":
		r.ensureVeil(ctx)
		return fmt.Sprintf("Done: %s's row says how it went.", act.Entry), r.checkSkill(ctx, act.Entry)
	case "update":
		ap, ok := addon.Approved(r.env.User, kind, act.Entry)
		if !ok {
			return "", fmt.Errorf("%s wasn't added from elsewhere", act.Entry)
		}
		r.ensureVeil(ctx)
		return fmt.Sprintf("Done: %s's row says how it went.", act.Entry), r.update(ctx, ap)
	case "restore":
		return fmt.Sprintf("✓ %s is as approved again.", act.Entry), r.restoreSkill(act.Entry)
	case "recheck":
		r.ensureVeil(ctx)
		return fmt.Sprintf("Done: %s's row says how it went.", act.Entry), r.recheckTools(ctx, act.Entry)
	case "review":
		held, err := addon.List(r.env.User)
		if err != nil {
			return "", err
		}
		for _, h := range held {
			if h.Kind == kind && h.Name == act.Entry {
				return fmt.Sprintf("Done: %s's row says how it went.", h.Name), r.decide(ctx, h)
			}
		}
		return "", fmt.Errorf("nothing named %s is held", act.Entry)
	case "delete":
		if err := steps.DeleteSkill(r.env, act.Entry); err != nil {
			return "", err
		}
		_ = addon.Forget(r.env.User, "skill", act.Entry)
		return fmt.Sprintf("✓ %s deleted.", act.Entry), nil
	}
	return "", fmt.Errorf("nothing to do for %q", act.Do)
}

// ghToken is the user's GitHub token from gh, when they're logged in
// (more requests an hour), else "".
func ghToken(ctx context.Context, user string) string {
	out, err := exec.CommandContext(ctx, "runuser", "-u", user, "--", "sh", "-c", steps.UserShell, "_", "gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ensureVeil installs skill-veil (the skillveil item) when it isn't the
// pinned release: the checks run it.
func (r *addonRun) ensureVeil(ctx context.Context) {
	r.ensure(ctx, "skillveil", "", "Installing skill-veil, the scanner the checks use…",
		"skill-veil didn't install: %v (the reports say not scanned)")
}

// ensureSync brings mcp-sync up to date (the gateway's go:mcp_files): an
// older one doesn't hold servers from elsewhere to the spec approved.
func (r *addonRun) ensureSync(ctx context.Context) {
	r.ensure(ctx, "mcpjungle-install", "go:mcp_files", "Updating mcp-sync…", "mcp-sync didn't update: %v")
}

// ensure applies item id's steps (of type typ, when given) that aren't
// done, saying so; failing, it says why, and goes on.
func (r *addonRun) ensure(ctx context.Context, id, typ, doing, failed string) {
	if r.cat == nil || r.cat.Item(id) == nil {
		return
	}
	it := r.cat.Item(id)
	said := false
	for _, s := range it.Steps {
		if typ != "" && s.Type != typ {
			continue
		}
		st, err := steps.New(it.ID, s)
		if err != nil {
			return
		}
		if ok, _ := st.Check(ctx, r.env); ok {
			continue
		}
		if !said {
			fmt.Fprintln(r.out, doing)
			said = true
		}
		if err := st.Apply(ctx, r.env); err != nil {
			fmt.Fprintln(r.out, "▲ "+fmt.Sprintf(failed, err))
			return
		}
	}
}

// parseMixed parses args with fs, flags before or after the others,
// which it returns.
func parseMixed(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return rest, nil
		}
		rest, args = append(rest, fs.Arg(0)), fs.Args()[1:]
	}
}

// skillsAdd is `yardo skills add <source …> [--yes]`.
func (o *options) skillsAdd(ctx context.Context, cat *catalog.Catalog, st *answers.Store, out io.Writer, args []string) error {
	fs := flag.NewFlagSet("yardo skills add", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "install each one whose report needs no OK; leave the rest held")
	fs.BoolVar(yes, "y", false, "the same as --yes")
	srcs, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if len(srcs) == 0 {
		return errors.New("usage: sudo yardo skills add owner/repo[/path][@ref] … [--yes]")
	}
	r, _, err := o.addonRun(ctx, cat, st, out, *yes)
	if err != nil {
		return err
	}
	r.ensureVeil(ctx)
	var errs []error
	for _, s := range srcs {
		errs = append(errs, r.addSkills(ctx, s))
	}
	return errors.Join(errs...)
}

// addSkills fetches the skills at source s, holds the ones picked (all,
// with --yes) and decides on each.
func (r *addonRun) addSkills(ctx context.Context, s string) error {
	src, err := addon.ParseSource(s)
	if err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Fetching %s…\n", src)
	f, err := addon.FetchSkills(ctx, src)
	if err != nil {
		return err
	}
	names := slices.Sorted(maps.Keys(f.Skills))
	if len(names) > 1 && !r.yes {
		if names, err = r.k.chooseMany(fmt.Sprintf("%s has %d skills: which to add?", src, len(names)), names, nil); err != nil {
			return err
		}
	}
	var errs []error
	for _, n := range names {
		h := &addon.Held{User: r.env.User, Kind: "skill", Name: n, Source: src.String(), Pin: f.Commit, Odd: f.Odd[n]}
		if err := addon.Hold(h, f.Skills[n]); err != nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, r.decide(ctx, h))
	}
	return errors.Join(errs...)
}

// taken are the names h's would clash with: the skills there are, or the
// servers and catalog specs.
func (r *addonRun) taken(h *addon.Held) []string {
	if h.Kind == "skill" {
		ss, _, _ := steps.ListSkills(r.env)
		var out []string
		for _, s := range ss {
			if s.Name != h.Name || !h.Existing && !h.Update {
				out = append(out, s.Name)
			}
		}
		return out
	}
	out := mcpNames(r.k.a.MCPServers)
	for _, d := range []string{"/usr/local/share/mcp-catalog"} {
		ents, _ := os.ReadDir(d)
		for _, e := range ents {
			if n, ok := strings.CutSuffix(e.Name(), ".spec"); ok {
				out = append(out, n)
			}
		}
	}
	ents, _ := os.ReadDir(filepath.Join(r.env.Home, ".mcp"))
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	if h.Existing || h.Update {
		out = slices.DeleteFunc(out, func(n string) bool { return n == h.Name })
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// decide checks h, shows its report and does what the user picks (with
// --yes: installs it when nothing needs an OK, else leaves it held).
func (r *addonRun) decide(ctx context.Context, h *addon.Held) error {
	if h.Report.Hash == "" {
		fmt.Fprintf(r.out, "Checking %s…\n", h.Name)
		if err := h.Check(ctx, r.taken(h)); err != nil {
			return err
		}
	}
	for {
		printReport(r.out, h)
		v := h.Report.Verdict()
		if r.yes {
			switch v {
			case "block":
				fmt.Fprintf(r.out, "%s stays held: it can't be installed as it is (sudo yardo addons, then decide).\n\n", h.Name)
				return nil
			case "ok":
				fmt.Fprintf(r.out, "%s stays held: it needs your OK (sudo yardo addons review %s).\n\n", h.Name, h.Name)
				return nil
			}
			return r.install(ctx, h, "")
		}
		const install, decline, later = "Install", "Decline and remove", "Leave it held (decide later)"
		var opts []string
		if v != "block" {
			opts = append(opts, install)
		}
		fixes := map[string]string{}
		for _, f := range h.Report.Findings {
			if f.Fix != "" {
				l := "Fix: " + f.Next
				if w := f.Where(); w != "" {
					l += fmt.Sprintf(" (%s)", w)
				}
				if _, dup := fixes[l]; !dup && !slices.ContainsFunc(opts, func(o string) bool { return fixes[o] == f.Fix }) {
					fixes[l] = f.Fix
					opts = append(opts, l)
				}
			}
		}
		const careful = "Check again carefully (Claude, the larger model: slower, uses more of your plan)"
		if !hasRule(h.Report, "claude", "not-checked") || h.Report.AI != "" {
			opts = append(opts, careful)
		}
		opts = append(opts, decline, later)
		pick, err := r.k.chooseOne(fmt.Sprintf("%s (%s): what now?", h.Name, verdictSays(v)), opts, opts[0])
		if err != nil {
			return err
		}
		switch {
		case pick == install:
			typed := ""
			if v == "ok" {
				if typed, err = r.k.typeIn(fmt.Sprintf("Type %s to install it", h.Name), "Its report needs your OK: you've read what it found."); err != nil {
					return err
				}
			}
			err := r.install(ctx, h, strings.TrimSpace(typed))
			switch {
			case errors.Is(err, addon.ErrChanged):
				fmt.Fprintln(r.out, "▲ "+err.Error())
				if err := h.Check(ctx, r.taken(h)); err != nil {
					return err
				}
				continue
			case errors.Is(err, addon.ErrNeedsOK):
				fmt.Fprintln(r.out, "▲ That isn't its name: not installed.")
				continue
			}
			return err
		case fixes[pick] != "":
			if err := h.Fix(fixes[pick]); err != nil {
				return err
			}
			fmt.Fprintln(r.out, "Fixed; checking again…")
			if err := h.Check(ctx, r.taken(h)); err != nil {
				return err
			}
		case pick == careful:
			var err error
			r.progress(ctx, fmt.Sprintf("Claude is reading %s carefully (%s)", h.Name, addon.CarefulModel), func(ctx context.Context) {
				err = h.Recheck(ctx)
			})
			if err != nil {
				return err
			}
		case pick == decline:
			err := h.Decline(ctx)
			switch {
			case h.Kind == "mcp" && h.Existing:
				fmt.Fprintf(r.out, "Declined: %s stays off; its tools aren't the ones you approved.\n\n", h.Name)
			case h.Existing:
				fmt.Fprintf(r.out, "Declined: %s stays off (sudo yardo skills delete %s removes it).\n\n", h.Name, h.Name)
			case h.Update:
				fmt.Fprintf(r.out, "Declined: the update is gone; %s stays as you approved it.\n\n", h.Name)
			default:
				fmt.Fprintf(r.out, "Declined: %s is gone from this machine.\n\n", h.Name)
			}
			return err
		default:
			fmt.Fprintf(r.out, "%s stays held: sudo yardo addons review %s decides.\n\n", h.Name, h.Name)
			return nil
		}
	}
}

func verdictSays(v string) string {
	return map[string]string{"block": "can't be installed as it is", "ok": "needs your OK", "note": "notes only", "clean": "nothing found"}[v]
}

// hasRule says whether r has a finding of layer's rule.
func hasRule(r addon.Report, layer, rule string) bool {
	return slices.ContainsFunc(r.Findings, func(f addon.Finding) bool { return f.Layer == layer && f.Rule == rule })
}

// hasLayer says whether r has a finding of layer at one of levels, but
// its notes on what it couldn't read.
func hasLayer(r addon.Report, layer string, levels ...string) bool {
	return slices.ContainsFunc(r.Findings, func(f addon.Finding) bool {
		return f.Layer == layer && slices.Contains(levels, f.Level) &&
			!slices.Contains([]string{"not-checked", "too-big", "image-program", "no-source"}, f.Rule)
	})
}

// printReport shows h's report: what it is, the verdict, each finding.
func printReport(out io.Writer, h *addon.Held) {
	pin := h.Pin
	if len(pin) == 40 {
		pin = pin[:12]
	}
	switch {
	case h.Existing && h.Kind == "mcp":
		fmt.Fprintf(out, "\n%s · server from %s: its tools now\n", h.Name, h.Source)
	case h.Existing:
		fmt.Fprintf(out, "\n%s · skill %s, %s\n", h.Name, h.Source, pin)
	default:
		fmt.Fprintf(out, "\n%s · %s from %s @ %s\n", h.Name, h.Kind, h.Source, pin)
	}
	mark := map[string]string{"block": "✗", "ok": "▲", "note": "·", "clean": "✓"}
	fmt.Fprintf(out, "%s %s\n", mark[h.Report.Verdict()], strings.ToUpper(verdictSays(h.Report.Verdict())[:1])+verdictSays(h.Report.Verdict())[1:])
	for _, f := range h.Report.Findings {
		where := f.Where()
		if where != "" {
			where += ": "
		}
		fmt.Fprintf(out, "  %s %s%s [%s %s]\n", mark[f.Level], where, f.Text, f.Layer, f.Rule)
		if f.Next != "" {
			fmt.Fprintf(out, "      → %s\n", f.Next)
		}
	}
	if len(h.Report.Changes) > 0 {
		fmt.Fprintln(out, "  Changed since the version you approved:")
		for _, c := range h.Report.Changes {
			fmt.Fprintf(out, "    %s %s\n", c.What, c.Path)
			ls := strings.Split(strings.TrimRight(c.Diff, "\n"), "\n")
			for n, l := range ls {
				if n == 60 {
					fmt.Fprintf(out, "      … %d more lines\n", len(ls)-n)
					break
				}
				fmt.Fprintln(out, "      "+l)
			}
		}
	}
	switch {
	case h.Report.AI != "" && !hasLayer(h.Report, "claude", "ok", "note"):
		fmt.Fprintf(out, "  Claude (%s) read it and found nothing harmful. It can miss things, and it can be fooled.\n", h.Report.AI)
	case h.Report.AI != "":
		fmt.Fprintf(out, "  Claude (%s) read it: its findings are marked claude. It can miss things, and it can be fooled.\n", h.Report.AI)
	}
	if h.Kind == "mcp" {
		if files, err := h.Files(); err == nil {
			sp := addon.ParseSpec(string(files["spec"]))
			if !sp.Data() {
				fmt.Fprintln(out, "  The spec (lines marked > are code, not settings):")
				for n, l := range sp.Lines {
					m := " "
					if slices.Contains(sp.Code, n+1) {
						m = ">"
					}
					fmt.Fprintf(out, "   %s %3d  %s\n", m, n+1, l)
				}
			}
		}
		if h.Image && h.Report.AI == "" {
			fmt.Fprintln(out, "  Not checked: the image's own files (its layers).")
		}
	}
}

// install installs h as approved.
func (r *addonRun) install(ctx context.Context, h *addon.Held, typed string) error {
	put := func(files map[string][]byte) error { return steps.InstallSkill(r.env, h.Name, files) }
	switch {
	case h.Kind == "skill" && h.Existing:
		// It's there, held off while checked: on again, if it's as checked.
		put = func(files map[string][]byte) error {
			ss, _, err := steps.ListSkills(r.env)
			if err != nil {
				return err
			}
			for _, s := range ss {
				if s.Name != h.Name || !s.Off {
					continue
				}
				now, err := steps.SkillOnDisk(r.env, steps.HeldDir(s.Held[0]), h.Name)
				if err != nil {
					return err
				}
				if addon.Hash(now) != addon.Hash(files) {
					return fmt.Errorf("%s changed where it's held off since it was checked: it stays off (Check it again)", h.Name)
				}
				return steps.SwitchSkills(r.env, "on", []string{h.Name})
			}
			return nil
		}
	case h.Kind == "mcp" && h.Existing:
		put = func(map[string][]byte) error { return nil } // its spec stays; its tools are approved below
	case h.Kind == "mcp":
		r.ensureSync(ctx)
		put = func(files map[string][]byte) error { return r.putServer(ctx, h.Name, files["spec"]) }
	}
	files, _ := h.Files()
	if err := h.Install(ctx, r.env.User, typed, put); err != nil {
		return err
	}
	switch {
	case h.Kind == "mcp" && h.Existing:
		if err := addon.SetTools(r.env.User, h.Name, addon.SpecHash(files["tools.json"])); err != nil {
			return err
		}
		if err := r.k.mcpSyncAs(ctx, "on", h.Name); err != nil {
			return err
		}
		fmt.Fprintf(r.out, "✓ %s's tools approved: it's on again.\n\n", h.Name)
		r.sync(ctx, h.Name)
		return nil
	case h.Update:
		fmt.Fprintf(r.out, "✓ %s updated to %s and recorded.\n\n", h.Name, shortPin(h.Pin))
	default:
		fmt.Fprintf(r.out, "✓ %s installed and recorded (sudo yardo addons lists it).\n\n", h.Name)
	}
	if h.Kind == "mcp" {
		r.sync(ctx, h.Name)
		// A remote server's tools, as it registered with them, are the
		// ones approved: mcp-sync turns it off when they change.
		if sp := addon.ParseSpec(string(files["spec"])); sp.Fields["KIND"] == "http" {
			if list, err := r.k.mcpTools(ctx, h.Name); err == nil && len(list) > 2 {
				_ = addon.SetTools(r.env.User, h.Name, addon.SpecHash(list))
				// Kept with the spec: checking them again shows what changed.
				_ = addon.KeepCopy(r.env.User, "mcp", h.Name, toolFiles(files["spec"], list))
			}
		}
	}
	return nil
}

// mcpTools is server name's tool list as mcp-sync hashes it.
func (k *asker) mcpTools(ctx context.Context, name string) ([]byte, error) {
	a := *k.a
	if a.MCPPort == 0 && k.port != nil {
		a.MCPPort = k.port()
	}
	return mcpSyncCmd(ctx, k.share, &a, "tools", name).Output()
}

// update fetches ap's newer version into the holding area (the approved
// one stays installed) and decides on it.
func (r *addonRun) update(ctx context.Context, ap addon.Approval) error {
	from := addon.Updatable(ap)
	if from == "" {
		return fmt.Errorf("%s: nothing to update from (%s)", ap.Name, ap.Source)
	}
	h := &addon.Held{User: r.env.User, Kind: ap.Kind, Name: ap.Name, Source: ap.Source, Update: true}
	var files map[string][]byte
	switch ap.Kind {
	case "skill":
		src, _ := addon.ParseSource(from)
		fmt.Fprintf(r.out, "Fetching %s…\n", src)
		f, err := addon.FetchSkills(ctx, src)
		if err != nil {
			return err
		}
		if f.Commit == ap.Pin {
			fmt.Fprintf(r.out, "%s is up to date (%s).\n", ap.Name, shortPin(ap.Pin))
			return nil
		}
		name := ap.Name
		if _, ok := f.Skills[name]; !ok && len(f.Skills) == 1 {
			for n := range f.Skills {
				name = n
			}
		}
		if files = f.Skills[name]; files == nil {
			return fmt.Errorf("%s: %s has no skill %s any more", ap.Name, src, ap.Name)
		}
		h.Pin, h.Odd = f.Commit, f.Odd[name]
	case "mcp":
		fmt.Fprintf(r.out, "Pulling %s (only pulled and read: no container is made)…\n", from)
		img, err := addon.PullImage(ctx, from)
		if err != nil {
			return err
		}
		if img.Ref == ap.Pin {
			fmt.Fprintf(r.out, "%s is up to date (%s).\n", ap.Name, shortPin(ap.Pin))
			return nil
		}
		old, err := addon.ApprovedFiles(r.env.User, "mcp", ap.Name)
		if err != nil {
			return fmt.Errorf("%s: no approved copy of its spec kept: delete it and add it again", ap.Name)
		}
		sp := addon.ParseSpec(string(old["spec"]))
		sp.Set("IMAGE", img.Ref)
		files = map[string][]byte{"spec": []byte(sp.Text(fmt.Sprintf("Written by yardo for %s, from %s, updated to %s.", ap.Name, from, img.Ref)))}
		h.Pin, h.Image, h.ImageUser, h.Pulled = img.Ref, true, img.User, img.New
	}
	if err := addon.Hold(h, files); err != nil {
		return err
	}
	return r.decide(ctx, h)
}

// restoreSkill puts skill name's approved files back.
func (r *addonRun) restoreSkill(name string) error {
	files, err := addon.ApprovedFiles(r.env.User, "skill", name)
	if err != nil {
		return fmt.Errorf("%s: no approved copy kept (it was approved before yardo kept them): Check it again instead", name)
	}
	if err := steps.InstallSkill(r.env, name, files); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "✓ %s is as approved again.\n", name)
	return nil
}

// recheckTools checks server name's tools as they are now (they changed
// since it was approved: it's off), and decides on them.
func (r *addonRun) recheckTools(ctx context.Context, name string) error {
	ap, ok := addon.Approved(r.env.User, "mcp", name)
	if !ok {
		return fmt.Errorf("%s wasn't added from elsewhere: nothing to check again", name)
	}
	list, err := r.k.mcpTools(ctx, name)
	if err != nil {
		return fmt.Errorf("%s's tools: %w", name, err)
	}
	old, err := addon.ApprovedFiles(r.env.User, "mcp", name)
	if err != nil {
		return fmt.Errorf("%s: no approved copy of its spec kept: delete it and add it again", name)
	}
	h := &addon.Held{User: r.env.User, Kind: "mcp", Name: name, Source: ap.Source, Pin: ap.Pin, Existing: true, Update: true}
	if err := addon.Hold(h, toolFiles(old["spec"], list)); err != nil {
		return err
	}
	return r.decide(ctx, h)
}

// toolFiles are a remote server's files as checked: its spec, and its
// tool list as hashed and as read (indented).
func toolFiles(spec, list []byte) map[string][]byte {
	pretty, _ := json.MarshalIndent(json.RawMessage(list), "", "  ")
	return map[string][]byte{"spec": spec, "tools.json": list, "tools.txt": append(pretty, '\n')}
}

// putServer makes ~/.mcp/<name> with spec, as the gateway's user, an
// env with each REQUIRED key to fill in, and puts it on yardo's list.
func (r *addonRun) putServer(ctx context.Context, name string, spec []byte) error {
	dir := filepath.Join(r.env.Home, ".mcp", name)
	sp := addon.ParseSpec(string(spec))
	var env strings.Builder
	fmt.Fprintf(&env, "# %s's settings (sudo yardo mcp edit %s)\n", name, name)
	for _, k := range strings.Fields(sp.Fields["REQUIRED"]) {
		env.WriteString(k + "=\n")
	}
	// The spec replaces one there; an env there (its settings) stays.
	for _, w := range []struct {
		file, script string
		b            []byte
	}{
		{"spec", `umask 077; mkdir -p "$1" && cat > "$1/spec.new" && mv "$1/spec.new" "$1/spec"`, spec},
		{"env", `umask 077; [ -e "$1/env" ] || cat > "$1/env"`, []byte(env.String())},
	} {
		c := exec.CommandContext(ctx, "runuser", "-u", mcpOwner(r.env.User), "--", "sh", "-c", w.script, "_", dir)
		c.Stdin = strings.NewReader(string(w.b))
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("writing ~/.mcp/%s/%s: %w: %s", name, w.file, err, out)
		}
	}
	if !slices.Contains(r.k.a.MCPServers, name) {
		r.k.a.MCPServers = append(r.k.a.MCPServers, name)
	}
	return nil
}

// mcpAdd is `yardo mcp add <image|https URL|spec file> [--name N] [--yes]`.
func (o *options) mcpAdd(ctx context.Context, cat *catalog.Catalog, st *answers.Store, out io.Writer, args []string) error {
	fs := flag.NewFlagSet("yardo mcp add", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "install it when its report needs no OK; else leave it held")
	name := fs.String("name", "", "the server's name (asked when not given)")
	srcs, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if len(srcs) != 1 {
		return errors.New("usage: sudo yardo mcp add <image | https URL | spec file> [--name NAME] [--yes]")
	}
	r, a, err := o.addonRun(ctx, cat, st, out, *yes)
	if err != nil {
		return err
	}
	r.ensureVeil(ctx)
	h, files, err := r.holdServer(ctx, srcs[0], *name)
	if err != nil {
		return err
	}
	if err := addon.Hold(h, files); err != nil {
		return err
	}
	if err := r.decide(ctx, h); err != nil {
		return err
	}
	return st.Save(a)
}

// holdServer fetches what src is (a spec file, an https URL, an image)
// and makes the held server, asking what a form needs.
func (r *addonRun) holdServer(ctx context.Context, src, name string) (*addon.Held, map[string][]byte, error) {
	h := &addon.Held{User: r.env.User, Kind: "mcp", Source: src}
	if b, err := os.ReadFile(src); err == nil {
		h.Pin = "spec sha256:" + addon.SpecHash(b)[:12]
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(src), ".spec")
		}
		h.Name = name
		if img := addon.ParseSpec(string(b)).Fields["IMAGE"]; img != "" {
			h.Image = true
		}
		return h, map[string][]byte{"spec": b}, nil
	}
	f := addon.Form{Name: name}
	switch {
	case strings.Contains(src, "://"):
		u, err := addon.CheckURL(src)
		if err != nil {
			return nil, nil, err
		}
		f.URL, h.Pin = u, u
	default:
		fmt.Fprintf(r.out, "Pulling %s (only pulled and read: no container is made)…\n", src)
		img, err := addon.PullImage(ctx, src)
		if err != nil {
			return nil, nil, err
		}
		f.Image, h.Pin, h.Image, h.ImageUser, h.Pulled = img.Ref, img.Ref, true, img.User, img.New
	}
	if err := r.form(&f, src); err != nil {
		return nil, nil, err
	}
	h.Name = f.Name
	sp := addon.FormSpec(f)
	return h, map[string][]byte{"spec": []byte(sp.Text(fmt.Sprintf("Written by yardo for %s, from %s.", f.Name, src)))}, nil
}

// form asks what yardo can't read from an image or a URL: the name, what
// it's for, the settings it needs, and (an image) whether it needs the
// network.
func (r *addonRun) form(f *addon.Form, src string) error {
	if r.yes {
		if f.Name == "" {
			return errors.New("--yes needs --name")
		}
		f.Description = "added from " + src
		return nil
	}
	var err error
	if f.Name == "" {
		if f.Name, err = r.k.typeIn("Its name", "lowercase letters, digits, hyphens: its tools show as <name>__<tool>"); err != nil {
			return err
		}
	}
	if f.Description, err = r.k.typeIn("What it's for", "one line, for agents choosing its tools"); err != nil {
		return err
	}
	keys, err := r.k.typeIn("The settings it needs", "names of its keys, by spaces (GITHUB_TOKEN …): they go in its env file; blank for none")
	if err != nil {
		return err
	}
	for _, k := range strings.Fields(keys) {
		f.Required = append(f.Required, strings.ToUpper(k))
	}
	if f.Image != "" {
		net, err := r.k.chooseOne("Does it need the network?", []string{"No: it works on what it's given", "Yes: it calls a service"}, "")
		if err != nil {
			return err
		}
		f.Network = strings.HasPrefix(net, "Yes")
	}
	f.Name = strings.TrimSpace(f.Name)
	return nil
}

// addonsCmd is `yardo addons [review NAME]`: what's held and approved,
// or the decision on one held.
func (o *options) addonsCmd(ctx context.Context, cat *catalog.Catalog, st *answers.Store, out io.Writer, args []string) error {
	r, a, err := o.addonRun(ctx, cat, st, out, false)
	if err != nil {
		return err
	}
	held, err := addon.List(r.env.User)
	if err != nil {
		return err
	}
	const usage = "usage: sudo yardo addons [review NAME | outdated | update [NAME…] [--yes] | recheck SERVER [--yes]]"
	if len(args) > 0 {
		switch args[0] {
		case "review":
			if len(args) != 2 {
				return errors.New(usage)
			}
			for _, h := range held {
				if h.Name == args[1] {
					if err := r.decide(ctx, h); err != nil {
						return err
					}
					return st.Save(a)
				}
			}
			return fmt.Errorf("nothing named %s is held", args[1])
		case "outdated":
			outdated, errs := addon.FindOutdated(ctx, r.env.User, true)
			for _, u := range outdated {
				fmt.Fprintf(out, "%-24s %-5s %s → %s\n", u.Name, u.Kind, shortPin(u.Pin), shortPin(u.Latest))
			}
			for n, err := range errs {
				fmt.Fprintf(out, "%-24s can't tell: %v\n", n, err)
			}
			if len(outdated)+len(errs) == 0 {
				fmt.Fprintln(out, "Every add-on from elsewhere is up to date.")
			}
			return nil
		case "update":
			fs := flag.NewFlagSet("yardo addons update", flag.ContinueOnError)
			yes := fs.Bool("yes", false, "install each update whose report needs no OK; leave the rest held")
			names, err := parseMixed(fs, args[1:])
			if err != nil {
				return err
			}
			r.yes = *yes
			outdated, errs := addon.FindOutdated(ctx, r.env.User, true)
			for n, err := range errs {
				fmt.Fprintf(out, "▲ %s: can't tell: %v\n", n, err)
			}
			r.ensureVeil(ctx)
			var all []error
			did := 0
			for _, u := range outdated {
				if len(names) > 0 && !slices.Contains(names, u.Name) {
					continue
				}
				ap, _ := addon.Approved(r.env.User, u.Kind, u.Name)
				all = append(all, r.update(ctx, ap))
				did++
			}
			if did == 0 {
				fmt.Fprintln(out, "Nothing to update.")
			}
			return errors.Join(append(all, st.Save(a))...)
		case "recheck":
			fs := flag.NewFlagSet("yardo addons recheck", flag.ContinueOnError)
			yes := fs.Bool("yes", false, "turn it on again when its report needs no OK; else leave it held")
			names, err := parseMixed(fs, args[1:])
			if err != nil {
				return err
			}
			if len(names) != 1 {
				return errors.New(usage)
			}
			r.yes = *yes
			r.ensureVeil(ctx)
			return r.recheckTools(ctx, names[0])
		}
		return errors.New(usage)
	}
	approved, err := addon.Approvals(r.env.User)
	if err != nil {
		return err
	}
	if len(held)+len(approved) == 0 {
		fmt.Fprintln(out, "No add-ons from elsewhere: sudo yardo skills add owner/repo, sudo yardo mcp add <image|URL|spec>.")
		return nil
	}
	if len(held) > 0 {
		fmt.Fprintln(out, "Held (waiting for you: sudo yardo addons review NAME):")
		for _, h := range held {
			fmt.Fprintf(out, "  %-24s %-5s %s @ %s · %s\n", h.Name, h.Kind, h.Source, shortPin(h.Pin), verdictSays(h.Report.Verdict()))
		}
	}
	if len(approved) > 0 {
		fmt.Fprintln(out, "Approved and installed:")
		for _, ap := range approved {
			ok := ""
			if ap.OK {
				ok = " · with your OK"
			}
			fmt.Fprintf(out, "  %-24s %-5s %s @ %s · %s by %s%s\n", ap.Name, ap.Kind, ap.Source, shortPin(ap.Pin), ap.At.Format("2006-01-02"), ap.By, ok)
		}
	}
	return nil
}

func shortPin(p string) string {
	if len(p) == 40 {
		return p[:12]
	}
	if i := strings.Index(p, "@sha256:"); i >= 0 && len(p) > i+20 {
		return p[:i+20]
	}
	return p
}

// checkSkill is Check it, for a skill that isn't yardo's: held off while
// its copy is checked, then decided on.
func (r *addonRun) checkSkill(ctx context.Context, name string) error {
	ss, _, err := steps.ListSkills(r.env)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(ss, func(s steps.Skill) bool { return s.Name == name })
	if i < 0 {
		return fmt.Errorf("no skill %s", name)
	}
	s := ss[i]
	from := append(slices.Clone(s.Places), s.Held...)
	dir := from[0]
	if len(s.Places) == 0 {
		dir = steps.HeldDir(dir)
	}
	files, err := steps.SkillOnDisk(r.env, dir, name)
	if err != nil {
		return err
	}
	if !s.Off {
		if err := steps.SwitchSkills(r.env, "off", []string{name}); err != nil {
			return err
		}
		fmt.Fprintf(r.out, "%s is off while it's checked.\n", name)
	}
	h := &addon.Held{User: r.env.User, Kind: "skill", Name: name, Source: "found in " + dir, Pin: "as found, sha256 " + addon.Hash(files)[:12], Existing: true}
	if ap, ok := addon.Approved(r.env.User, "skill", name); ok {
		// Checked before, changed since: these files, from where it was.
		h.Source, h.Pin, h.Update = ap.Source, ap.Pin, true
	}
	if err := addon.Hold(h, files); err != nil {
		return err
	}
	return r.decide(ctx, h)
}

// mcpForget drops a deleted server's approval.
func mcpForget(user, entry string) {
	if _, name, err := mcp.Instance(entry); err == nil {
		_ = addon.Forget(user, "mcp", name)
	}
}

// progress runs do, saying what it's at and the seconds gone, a line
// redrawn each second on a terminal; esc (or ctrl-c) there cancels do's
// context.
func (r *addonRun) progress(ctx context.Context, stage string, do func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fd := int(os.Stdin.Fd())
	if r.out != os.Stdout || !term.IsTerminal(fd) {
		fmt.Fprintln(r.out, stage+"…")
		do(ctx)
		return
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintln(r.out, stage+"…")
		do(ctx)
		return
	}
	done := make(chan struct{})
	stopped := false
	var wg sync.WaitGroup
	wg.Go(func() {
		start := time.Now()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			fmt.Fprintf(r.out, "\r\033[K  %s… %ds · esc stops it", stage, int(time.Since(start).Seconds()))
			// A key, polled so nothing is left reading after: esc stops.
			if n, _ := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, 0); n > 0 {
				b := make([]byte, 16)
				// esc alone (an arrow key's sequence starts with one too).
				if k, _ := os.Stdin.Read(b); k == 1 && b[0] == 0x1b || k > 0 && b[0] == 0x03 {
					stopped = true
					cancel()
				}
			}
			select {
			case <-done:
				fmt.Fprint(r.out, "\r\033[K")
				return
			case <-tick.C:
			}
		}
	})
	do(ctx)
	close(done)
	wg.Wait()
	_ = term.Restore(fd, old)
	if stopped {
		fmt.Fprintln(r.out, "Stopped: not checked by AI.")
	}
}
