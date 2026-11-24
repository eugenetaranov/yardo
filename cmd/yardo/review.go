package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/addon"
	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// A held add-on's review: what the Review screen shows (reviewOf), how
// it runs its checks and decisions (heldReviewer), and the same in plain
// text, for a script or a pipe (printPlain).

// heldReviewer is the Review screen's way to r's held add-ons.
type heldReviewer struct {
	r *addonRun
	// veil: skill-veil is installed first, when it's not the pinned
	// release (the app's; a command does it before its screen).
	veil bool
	// saved is told of an MCP server installed: it saves the answers and
	// registers it, saying how that went.
	saved func(ctx context.Context, name string) string
}

func (v heldReviewer) held(kind, name string) (*addon.Held, error) {
	hs, err := addon.List(v.r.env.User)
	if err != nil {
		return nil, err
	}
	for _, h := range hs {
		if h.Kind == kind && h.Name == name {
			return h, nil
		}
	}
	return nil, fmt.Errorf("nothing named %s is held", name)
}

// check runs every check on h, telling stage each as it starts.
func (v heldReviewer) check(ctx context.Context, h *addon.Held, stage func(string)) error {
	r := v.r
	if v.veil {
		r.ensureVeil(ctx)
	}
	r.stage = stage
	defer func() { r.stage = nil }()
	if addon.Extra != nil {
		// r's, saying its stage (another session's may be set).
		addon.Extra = r.extra
	}
	old := addon.Stage
	addon.Stage = func(_ *addon.Held, layer string) { stage(layer) }
	defer func() { addon.Stage = old }()
	return h.Check(ctx, r.taken(h))
}

func (v heldReviewer) Report(ctx context.Context, kind, name string, stage func(string)) (ui.Review, error) {
	h, err := v.held(kind, name)
	if err != nil {
		return ui.Review{}, err
	}
	if !h.Report.Current() {
		// Never checked, or by older rules.
		if err := v.check(ctx, h, stage); err != nil {
			return ui.Review{}, err
		}
	}
	return reviewOf(h), nil
}

func (v heldReviewer) Check(ctx context.Context, kind, name string, careful bool, stage func(string)) (ui.Review, error) {
	h, err := v.held(kind, name)
	if err != nil {
		return ui.Review{}, err
	}
	if careful {
		stage("claude:" + addon.CarefulModel)
		err = h.Recheck(ctx)
	} else {
		err = v.check(ctx, h, stage)
	}
	if err != nil {
		return ui.Review{}, err
	}
	return reviewOf(h), nil
}

func (v heldReviewer) Fix(ctx context.Context, kind, name, fix string, stage func(string)) (ui.Review, error) {
	h, err := v.held(kind, name)
	if err != nil {
		return ui.Review{}, err
	}
	if err := h.Fix(fix); err != nil {
		return ui.Review{}, err
	}
	if err := v.check(ctx, h, stage); err != nil {
		return ui.Review{}, err
	}
	return reviewOf(h), nil
}

func (v heldReviewer) Install(ctx context.Context, kind, name, typed string) (string, *ui.Review, error) {
	h, err := v.held(kind, name)
	if err != nil {
		return "", nil, err
	}
	toast, err := v.r.install(ctx, h, typed)
	switch {
	case errors.Is(err, addon.ErrChanged):
		if err := v.check(ctx, h, func(string) {}); err != nil {
			return "", nil, err
		}
		rv := reviewOf(h)
		return "▲ Its files changed since they were checked: checked again, this is what it finds now.", &rv, nil
	case errors.Is(err, addon.ErrNeedsOK):
		return "", nil, errors.New("that isn't its name: not installed")
	case err != nil:
		return "", nil, err
	}
	if h.Kind == "mcp" && !h.Existing && v.saved != nil {
		if s := v.saved(ctx, h.Name); s != "" {
			toast = strings.TrimSuffix(toast, ".") + " · " + s
		}
	}
	return toast, nil, nil
}

func (v heldReviewer) Decline(ctx context.Context, kind, name string) (string, error) {
	h, err := v.held(kind, name)
	if err != nil {
		return "", err
	}
	err = h.Decline(ctx)
	return declineSays(h), err
}

func (v heldReviewer) Read(kind, name, file string) ([]byte, string, bool) {
	h, err := v.held(kind, name)
	if err != nil {
		return nil, "", false
	}
	p := h.Path(file)
	if p == "" {
		return nil, "", false
	}
	b, err := os.ReadFile(p)
	return b, p, err == nil
}

// declineSays is what declining h did.
func declineSays(h *addon.Held) string {
	switch {
	case h.Kind == "mcp" && h.Existing:
		return fmt.Sprintf("%s declined: it stays off; its tools aren't the ones you approved.", h.Name)
	case h.Existing:
		return fmt.Sprintf("%s declined: it stays off (sudo yardo skills delete %s removes it).", h.Name, h.Name)
	case h.Update:
		return fmt.Sprintf("%s's update declined: it's gone; %s stays as you approved it.", h.Name, h.Name)
	}
	return fmt.Sprintf("%s declined: its files are gone from this machine.", h.Name)
}

// review shows h's Review screen (h checked first when it must be),
// saying how it ended.
func (r *addonRun) review(ctx context.Context, h *addon.Held) (ui.ReviewEnd, error) {
	by := heldReviewer{r: r}
	if r.k.held != nil {
		return r.k.held(by, h.Kind, h.Name)
	}
	return ui.ReviewScreen(ctx, by, h.Kind, h.Name)
}

// onTerminal: r says what it does on a terminal, to answer there (not
// --plain).
func (r *addonRun) onTerminal() bool {
	return r.k.held != nil || !r.plain && r.out == io.Writer(os.Stdout) && isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

// uiCaps are cs as the screen has them.
func uiCaps(cs []addon.Cap) []ui.Cap {
	var out []ui.Cap
	for _, c := range cs {
		out = append(out, ui.Cap{Kind: c.Kind, What: c.What, Say: c.Say(), Where: c.Where, Count: c.Count, Look: c.Look})
	}
	return out
}

// look: f is worth the user's eyes, as rep's Looks are (not one OK'd in
// the version approved).
func look(rep addon.Report, f addon.Finding) bool {
	return f.Look || f.Level == "block" || f.Level == "ok" && !slices.Contains(rep.Accepted, f.ID)
}

// uiFinding is f as the screen shows it: of its kind as the report
// names it (a note's line without what its heading says), who found it
// with the others that did too.
func uiFinding(h *addon.Held, f addon.Finding, files map[string][]byte) ui.Finding {
	by := strings.Join(append([]string{layerSays(f)}, f.Also...), " · ")
	u := ui.Finding{Level: f.Level, Group: layerSays(f), File: f.File, Line: f.Line, Text: f.Text, Next: f.Next, By: by,
		Kind: f.Kind, Role: f.Role, Why: f.Why, Look: look(h.Report, f)}
	if nk, ok := noteKinds[f.Rule]; ok && f.Level == "note" && f.Layer == "basic" {
		u.Group, u.Short = nk[0], strings.TrimPrefix(f.Text, nk[1])
	}
	u.Group = cmp.Or(f.Kind, u.Group)
	if len(u.Why) == 0 {
		u.Why = why(f, files)
	}
	if f.More > 0 {
		u.Text += fmt.Sprintf(" (%s in this file)", plural(f.More, "more place", "more places"))
	}
	// New since approved: above a note and not OK'd then (notes never are).
	u.New = h.Update && f.Level != "note" && f.ID != "" && !slices.Contains(h.Report.Accepted, f.ID)
	return u
}

// reviewOf is h as the Review screen shows it.
func reviewOf(h *addon.Held) ui.Review {
	rep := h.Report
	files, _ := h.Files()
	rv := ui.Review{Kind: h.Kind, Name: h.Name, What: heldWhat(h), Size: sizeSay(files), Verdict: rep.Verdict(), NeedsOK: rep.NeedsOK(),
		Dir: h.Path(""), Update: h.Update, Existing: h.Existing, AI: rep.AI,
		Caps: uiCaps(rep.Caps), Gained: uiCaps(rep.Gained), Lost: uiCaps(rep.Lost), NoCaps: len(rep.Caps)+len(rep.Coverage) == 0,
		Files: rep.Files, Accepted: len(rep.Accepted), Careful: !hasRule(rep, "claude", "not-checked") || rep.AI != ""}
	for _, m := range []string{"SKILL.md", "spec"} {
		if _, ok := files[m]; ok {
			rv.Main = m
			break
		}
	}
	for _, f := range rep.Findings {
		rv.Findings = append(rv.Findings, uiFinding(h, f, files))
	}
	seen := map[string]bool{}
	for _, f := range rep.Findings {
		if f.Fix != "" && f.Level != "note" && !seen[f.Fix] {
			seen[f.Fix] = true
			what, breaks := fixWhat(f.Fix)
			rv.Fixes = append(rv.Fixes, ui.Fix{ID: f.Fix, What: what, Breaks: breaks})
		}
	}
	for _, c := range rep.Changes {
		rv.Changes = append(rv.Changes, ui.Change{Path: c.Path, What: c.What, Diff: c.Diff})
	}
	for _, c := range rep.Coverage {
		rv.Read = append(rv.Read, ui.Read{Layer: c.Layer, Files: c.Files, All: c.All, Skipped: c.Skipped, Unread: c.Unread, Looks: c.Looks, Off: c.Off})
	}
	if len(rv.Read) == 0 {
		rv.Read = readOf(rep, len(files))
	}
	switch {
	case h.Kind == "skill" && h.Existing:
		rv.Dest = "back on, where it was"
	case h.Kind == "skill":
		var ds []string
		for _, d := range steps.SkillDirs {
			ds = append(ds, d+"/"+h.Name)
		}
		rv.Dest = strings.Join(ds, " and ")
	case h.Existing:
		rv.Dest = "nowhere new: its tools are approved, and it's on again"
	default:
		rv.Dest = fmt.Sprintf("~/.mcp/%s, registered with the gateway", h.Name)
	}
	return rv
}

// readOf is what each check read, for a report that doesn't say (one
// by an older yardo): the rules every file; skill-veil and Claude, all
// of them unless they didn't run.
func readOf(rep addon.Report, n int) []ui.Read {
	out := []ui.Read{{Layer: "basic", Files: n, All: n}}
	veil := ui.Read{Layer: "skill-veil", Files: n, All: n}
	claude := ui.Read{Layer: "claude", Files: n, All: n}
	for _, f := range rep.Findings {
		switch {
		case f.Layer == "skill-veil" && f.Rule == "not-scanned":
			veil.Off = strings.TrimPrefix(f.Text, "not scanned by skill-veil: ")
		case f.Layer == "claude" && f.Rule == "not-checked":
			claude.Off = strings.TrimPrefix(f.Text, "Not checked by AI: ")
		}
	}
	if rep.AI == "" && claude.Off == "" {
		claude.Off = "not run"
	}
	return append(out, veil, claude)
}

// heldWhat says what h is and where it's from.
func heldWhat(h *addon.Held) string {
	switch {
	case h.Existing && h.Kind == "mcp":
		return fmt.Sprintf("server from %s: its tools now", h.Source)
	case h.Existing:
		return fmt.Sprintf("skill %s, %s", h.Source, shortPin(h.Pin))
	case h.Kind == "mcp":
		return fmt.Sprintf("MCP server from %s @ %s", h.Source, shortPin(h.Pin))
	}
	return fmt.Sprintf("skill from %s @ %s", h.Source, shortPin(h.Pin))
}

// sizeSay is how big files are, how many, and their licence when one
// says which.
func sizeSay(files map[string][]byte) string {
	n := 0
	for _, b := range files {
		n += len(b)
	}
	out := []string{bytesSay(n), plural(len(files), "file", "files")}
	if l := licence(files); l != "" {
		out = append(out, l)
	}
	return strings.Join(out, " · ")
}

func bytesSay(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return plural(n, "byte", "bytes")
}

// licence is what a LICENSE file at the top says it is, roughly.
func licence(files map[string][]byte) string {
	for _, n := range []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "COPYING"} {
		b := files[n]
		if b == nil {
			continue
		}
		b = b[:min(len(b), 2000)]
		for _, l := range [][2]string{{"Apache License", "Apache-2.0"}, {"MIT License", "MIT"}, {"Permission is hereby granted, free of charge", "MIT"},
			{"GNU AFFERO GENERAL PUBLIC LICENSE", "AGPL"}, {"GNU LESSER GENERAL PUBLIC LICENSE", "LGPL"}, {"GNU GENERAL PUBLIC LICENSE", "GPL"},
			{"Mozilla Public License", "MPL-2.0"}, {"BSD", "BSD"}, {"ISC License", "ISC"}} {
			if bytes.Contains(b, []byte(l[0])) {
				return l[1]
			}
		}
	}
	return ""
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// heldSay is h's report in a line, for its row: how many things to look
// at, and what it does that's worth saying.
func heldSay(h *addon.Held) string {
	rep := h.Report
	if rep.Hash == "" {
		return "not checked yet: enter checks it"
	}
	var out []string
	switch n := len(rep.Looks()); {
	case rep.Verdict() == "block":
		out = append(out, fmt.Sprintf("can't be installed as it is: %d to look at", n))
	case n > 0:
		out = append(out, fmt.Sprintf("%d to look at", n))
	default:
		out = append(out, verdictSays(rep.Verdict()))
	}
	if s := ui.CapsSay(uiCaps(rep.Caps), true); s != "" {
		out = append(out, s)
	}
	return strings.Join(out, " · ")
}

// verdictLine is the verdict, with its mark, and how many to look at.
func verdictLine(rep addon.Report) string {
	n := len(rep.Looks())
	switch rep.Verdict() {
	case "block":
		return fmt.Sprintf("✗ Can't be installed as it is: %d to look at", n)
	case "ok":
		return fmt.Sprintf("▲ Needs your OK: %d to look at", n)
	case "note":
		return "· Notes only: nothing stops it"
	}
	return "✓ Nothing found"
}

// printPlain is h's review in plain text: what it is, the verdict, what
// it can do and what was read, each thing to look at, the notes folded
// (notes: each one), an update's changes, and the commands that decide.
func printPlain(out io.Writer, h *addon.Held, notes bool) {
	rv := reviewOf(h)
	rep := h.Report
	fmt.Fprintf(out, "%s · %s · %s\n", h.Name, rv.What, rv.Size)
	fmt.Fprintln(out, verdictLine(rep))
	fmt.Fprintln(out)
	if rows := ui.CapRows(rv.Caps); len(rows) > 0 {
		for n, r := range rows {
			fmt.Fprintf(out, "%-9s%-15s%s\n", map[bool]string{true: "Can do"}[n == 0], r[0], r[1])
		}
	} else if rv.NoCaps {
		fmt.Fprintln(out, "Can do   not worked out (an older report: sudo yardo addons rescan "+h.Name+")")
	}
	if len(rv.Gained)+len(rv.Lost) > 0 {
		var ch []string
		for _, c := range rv.Gained {
			ch = append(ch, "+ it "+ui.CapsSay([]ui.Cap{c}, false))
		}
		for _, c := range rv.Lost {
			ch = append(ch, "− it no longer "+ui.CapsSay([]ui.Cap{c}, false))
		}
		fmt.Fprintf(out, "Since    %s\n", strings.Join(ch, " · "))
	}
	var read []string
	for _, r := range rv.Read {
		name := map[string]string{"basic": "rules", "claude": "Claude"}[r.Layer]
		name = cmp.Or(name, r.Layer)
		if r.Layer == "claude" && rep.AI != "" {
			name += " (" + rep.AI + ")"
		}
		switch {
		case r.Off == "not run":
			read = append(read, fmt.Sprintf("%s: didn't run (%s: sudo yardo addons rescan %s)", name, ui.OffSays(r.Off), h.Name))
		case r.Off != "":
			read = append(read, fmt.Sprintf("%s: didn't run (%s)", name, ui.OffSays(r.Off)))
		case r.Files == r.All:
			read = append(read, fmt.Sprintf("%s: %s", name, plural(r.All, "file", "files")))
		default:
			read = append(read, fmt.Sprintf("%s: %d of %d files", name, r.Files, r.All))
		}
		if r.Looks != "" {
			read[len(read)-1] += " (" + r.Looks + ")"
		}
		if r.Unread != "" {
			read[len(read)-1] += ", not " + r.Unread
		}
	}
	fmt.Fprintf(out, "Read     %s\n", strings.Join(read, " · "))
	n, oks := 0, 0
	var fold []addon.Finding
	files, _ := h.Files()
	for _, f := range rep.Findings {
		if !look(rep, f) {
			fold = append(fold, f)
			continue
		}
		n++
		if f.Level != "note" {
			oks++
		}
		u := uiFinding(h, f, files)
		mark := map[string]string{"block": "✗", "ok": "▲"}[f.Level]
		fmt.Fprintf(out, "\n%d %s %s  %s\n", n, cmp.Or(mark, "·"), cmp.Or(f.Kind, f.Text), cmp.Or(f.Where(), h.Name))
		if f.Kind != "" {
			fmt.Fprintf(out, "    %s\n", f.Text)
		}
		for _, l := range u.Why {
			fmt.Fprintf(out, "    %s\n", l)
		}
		if f.Next != "" {
			fmt.Fprintf(out, "    What you can do: %s.\n", strings.TrimSuffix(f.Next, "."))
		}
		fmt.Fprintf(out, "    (%s)\n", u.By)
	}
	if len(fold) > 0 {
		var order []string
		count := map[string]int{}
		for _, f := range fold {
			g := uiFinding(h, f, files).Group
			if count[g] == 0 {
				order = append(order, g)
			}
			count[g]++
		}
		slices.SortStableFunc(order, func(a, b string) int { return count[b] - count[a] })
		var says []string
		for _, g := range order {
			says = append(says, addon.KindCount(g, count[g]))
		}
		fmt.Fprintf(out, "\n%s folded: %s.\n", plural(len(fold), "note", "notes"), strings.Join(says, ", "))
		if notes {
			printNotes(out, fold)
		} else {
			fmt.Fprintf(out, "Every one: sudo yardo addons review %s --notes\n", h.Name)
		}
	}
	if len(rep.Changes) > 0 {
		fmt.Fprintln(out, "\nChanged since the version you approved:")
		for _, c := range rep.Changes {
			fmt.Fprintf(out, "  %s %s\n", c.What, c.Path)
			for l := range strings.SplitSeq(strings.TrimRight(c.Diff, "\n"), "\n") {
				fmt.Fprintln(out, "    "+l)
			}
		}
	}
	if h.Kind == "mcp" {
		sp := addon.ParseSpec(string(files["spec"]))
		if !sp.Data() {
			fmt.Fprintln(out, "\nThe spec (lines marked > are code, not settings):")
			for n, l := range sp.Lines {
				m := " "
				if slices.Contains(sp.Code, n+1) {
					m = ">"
				}
				fmt.Fprintf(out, " %s %3d  %s\n", m, n+1, l)
			}
		}
	}
	if rv.Dir != "" {
		fmt.Fprintf(out, "Its files: %s\n", rv.Dir)
	}
	fmt.Fprintln(out, "\nDecide:")
	cmds := [][2]string{}
	switch {
	case rep.Verdict() == "block":
	case rep.NeedsOK():
		cmds = append(cmds, [2]string{fmt.Sprintf("sudo yardo addons install %s --ok %s", h.Name, h.Name), map[bool]string{true: "install, OK-ing the one that needs it", false: fmt.Sprintf("install, OK-ing the %d that need it", oks)}[oks == 1]})
	default:
		cmds = append(cmds, [2]string{"sudo yardo addons install " + h.Name, "install it"})
	}
	for _, f := range rv.Fixes {
		cmds = append(cmds, [2]string{fmt.Sprintf("sudo yardo addons fix %s %s", h.Name, f.ID), strings.ToLower(f.What[:1]) + f.What[1:] + ", then check again"})
	}
	cmds = append(cmds, [2]string{"sudo yardo addons decline " + h.Name, "delete it"})
	w := 0
	for _, c := range cmds {
		w = max(w, len(c[0]))
	}
	for _, c := range cmds {
		fmt.Fprintf(out, "  %-*s   %s\n", w, c[0], c[1])
	}
}

// heldErr is the exit a review left held ends with: 3 when it can't be
// installed as it is, else 2 (it needs your OK).
func heldErr(h *addon.Held) error {
	if h.Report.Verdict() == "block" {
		return &exitErr{3, fmt.Errorf("%s stays held: it can't be installed as it is", h.Name)}
	}
	return &exitErr{2, fmt.Errorf("%s stays held: it needs your decision", h.Name)}
}

// fixWhat is what a fix yardo makes does, and what stops working.
func fixWhat(fix string) (string, string) {
	what, arg, _ := strings.Cut(fix, ":")
	switch what {
	case "remove":
		return "Delete " + arg, "whatever uses " + path.Base(arg) + " stops working"
	case "drop-key":
		return fmt.Sprintf("Take %s out of its front matter", arg), "what it set is gone"
	case "drop-opt":
		return fmt.Sprintf("Take the option %s out", arg), "it runs without it"
	}
	return "Fix: " + fix, ""
}

// heldNames are the names held, to say what there is.
func heldNames(hs []*addon.Held) string {
	return strings.Join(slices.Sorted(maps.Keys(func() map[string]bool {
		out := map[string]bool{}
		for _, h := range hs {
			out[h.Name] = true
		}
		return out
	}())), ", ")
}

const addonsUsage = "usage: sudo yardo addons [review NAME [--plain] [--notes] | install NAME [--ok NAME] | decline NAME | fix NAME FIX | rescan NAME [--careful] | outdated | update [NAME…] [--yes] | recheck SERVER [--yes]]"

// heldCmd is `yardo addons review|install|decline|fix|rescan NAME …` on
// one of held: review shows it (on a terminal, its Review screen; else
// in plain text) and exits 0 installed or declined, 2 held (it needs
// your OK), 3 held (it can't be installed as it is), 130 cancelled.
func (r *addonRun) heldCmd(ctx context.Context, held []*addon.Held, args []string) error {
	fs := flag.NewFlagSet("yardo addons "+args[0], flag.ContinueOnError)
	plain := fs.Bool("plain", false, "print the review, even on a terminal")
	notes := fs.Bool("notes", false, "print every note, not only how many of each kind")
	ok := fs.String("ok", "", "the add-on's name: you OK what its review says to look at")
	careful := fs.Bool("careful", false, "Claude reads it again with the larger model")
	rest, err := parseMixed(fs, args[1:])
	if err != nil {
		return err
	}
	if len(rest) == 0 || len(rest) > 1 && args[0] != "fix" || args[0] == "fix" && len(rest) != 2 {
		return errors.New(addonsUsage)
	}
	i := slices.IndexFunc(held, func(h *addon.Held) bool { return h.Name == rest[0] })
	if i < 0 {
		if len(held) == 0 {
			return fmt.Errorf("nothing named %s is held: nothing is", rest[0])
		}
		return fmt.Errorf("nothing named %s is held (held: %s)", rest[0], heldNames(held))
	}
	h := held[i]
	r.plain = r.plain || *plain || *notes
	switch args[0] {
	case "review":
		if !r.onTerminal() {
			if err := r.current(ctx, h); err != nil {
				return err
			}
			printPlain(r.out, h, *notes)
			return heldErr(h)
		}
		r.ensureVeil(ctx)
		end, err := r.review(ctx, h)
		if end.Toast != "" {
			fmt.Fprintln(r.out, end.Toast)
		}
		switch {
		case err != nil:
			return err
		case end.Did == "quit":
			return &exitErr{130, errors.New("cancelled")}
		case end.Did == "held":
			if h, err = addon.Load(h.User, h.ID); err != nil {
				return err
			}
			return heldErr(h)
		}
		return nil
	case "install":
		if err := r.current(ctx, h); err != nil {
			return err
		}
		switch {
		case h.Report.Verdict() == "block":
			return &exitErr{3, fmt.Errorf("%s can't be installed as it is: sudo yardo addons review %s says why, and what yardo can fix", h.Name, h.Name)}
		case h.Report.NeedsOK() && *ok != h.Name:
			return &exitErr{2, fmt.Errorf("%s needs your OK: read sudo yardo addons review %s, then install it with --ok %s", h.Name, h.Name, h.Name)}
		}
		toast, err := r.install(ctx, h, *ok)
		if errors.Is(err, addon.ErrChanged) {
			if err := h.Check(ctx, r.taken(h)); err != nil {
				return err
			}
			return &exitErr{2, fmt.Errorf("%s's files changed since they were checked: checked again, not installed (sudo yardo addons review %s)", h.Name, h.Name)}
		}
		if err == nil {
			fmt.Fprintln(r.out, toast)
		}
		return err
	case "decline":
		err := h.Decline(ctx)
		fmt.Fprintln(r.out, declineSays(h))
		return err
	case "fix":
		if !slices.ContainsFunc(h.Report.Findings, func(f addon.Finding) bool { return f.Fix == rest[1] }) {
			return fmt.Errorf("%s: no fix %s in its review (sudo yardo addons review %s lists them)", h.Name, rest[1], h.Name)
		}
		if err := h.Fix(rest[1]); err != nil {
			return err
		}
		fmt.Fprintf(r.out, "Fixed; checking %s again…\n", h.Name)
		if err := h.Check(ctx, r.taken(h)); err != nil {
			return err
		}
	case "rescan":
		r.ensureVeil(ctx)
		if *careful {
			var err error
			r.progress(ctx, fmt.Sprintf("Claude is reading %s carefully (%s)", h.Name, addon.CarefulModel), func(ctx context.Context) {
				err = h.Recheck(ctx)
			})
			if err != nil {
				return err
			}
		} else {
			fmt.Fprintf(r.out, "Checking %s again…\n", h.Name)
			if err := h.Check(ctx, r.taken(h)); err != nil {
				return err
			}
		}
	}
	printPlain(r.out, h, *notes)
	return nil
}

// reviewer is the app's way to held add-ons: their reviews run in it,
// nothing said on the terminal; an MCP server installed is saved on the
// answers and registered. nil when there's no add-on session (not root).
func (o *options) reviewer(ctx context.Context, cat *catalog.Catalog, st *answers.Store, a *answers.Answers) ui.Reviewer {
	if os.Geteuid() != 0 {
		return nil
	}
	r, err := (&asker{cat: cat, a: a, share: o.share(), out: io.Discard, port: func() int { p, _ := st.Port(a.User); return p }}).addons(ctx)
	if err != nil {
		return nil
	}
	r.sync = func(context.Context, string) string { return "" } // saved registers it
	return heldReviewer{r: r, veil: true, saved: func(ctx context.Context, name string) string {
		if err := st.Save(a); err != nil {
			return "✗ not saved: " + err.Error()
		}
		_ = o.mcpSyncNow(ctx, a, io.Discard)
		s, _ := o.mcpStatus(ctx, a)
		v, ok := s[name]
		return mcpSays(name, mcpSpecOf(a.MCPServers, name), v, ok)
	}}
}
