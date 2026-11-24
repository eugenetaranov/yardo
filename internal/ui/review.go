package ui

import (
	"context"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The Review screen: a held add-on (a skill or an MCP server from
// elsewhere) and everything to decide on it, in one place. Its header
// says what it is and the verdict; its sub-tabs are Summary (what it can
// do, what to look at, what was read), Findings (each with its code),
// Changes (an update's, against what you approved) and Files (by what
// they are to the agent). Checks, fixes, the install and the decline run
// from here, in the background: the stage strip says which check is
// running, esc stops Claude's. ←→ go along its sub-tabs, not the tabs
// at the top.

// Review is a held add-on as the Review screen shows it.
type Review struct {
	// Kind is skill or mcp; What says what it is and where it's from
	// ("skill from tt-a1i/archify @ 7c1e9a0"); Size how big ("22 MB · 441
	// files · Apache-2.0").
	Kind, Name, What, Size string
	// Verdict is block, ok, note or clean; NeedsOK: installing it takes
	// its name typed (what's to look at is OK'd).
	Verdict string
	NeedsOK bool
	// Dir is where its files are held; Main its main file there
	// (SKILL.md, spec), to open.
	Dir, Main string
	// Update: a newer version of one you approved; Existing: one that was
	// there already, off while it's checked.
	Update, Existing bool
	// Caps are what it can do (NoCaps: not worked out, an older report);
	// an update's Gained and Lost since you approved it.
	Caps, Gained, Lost []Cap
	NoCaps             bool
	// Findings are what the checks found, in the report's order.
	Findings []Finding
	// Read is what each check read.
	Read []Read
	// AI is the model Claude read it with ("" none); Careful: Claude can
	// read it again with the larger model (c).
	AI      string
	Careful bool
	// Fixes are what yardo can fix in its held copy.
	Fixes []Fix
	// Changes are an update's files, against the approved copy.
	Changes []Change
	// Files are its files' roles, path → role.
	Files map[string]string
	// Dest is where it installs ("~/.claude/skills and ~/.agents/skills").
	Dest string
	// Accepted is how many findings you OK'd when you approved it.
	Accepted int
}

// Cap is something it can do, found in its code (see addon.Cap).
// Say, if set, is it in words with its verb ("contacts x.io").
type Cap struct {
	Kind, What, Say string
	Where           []string
	Count           int
	Look            bool
}

// Read is what a check (Layer: basic, skill-veil, claude) read: Files of
// All; Skipped and Unread what it didn't, in words; Looks which files
// worth a look it read, and how; Off why it didn't run.
type Read struct {
	Layer                       string
	Files, All                  int
	Skipped, Unread, Looks, Off string
}

// Fix is a fix yardo can make to the held copy: ID as yardo names it
// (remove:<file>), What it does, Breaks what stops working.
type Fix struct{ ID, What, Breaks string }

// Change is a file an update adds, changes or removes, with its diff.
type Change struct{ Path, What, Diff string }

// ReviewEnd is how a review ended: Did installed, declined, held (left
// as it was) or quit (ctrl+c); Toast says it; Changed: it was checked
// again or fixed (its row is out of date).
type ReviewEnd struct {
	Did, Toast string
	Changed    bool
}

// Reviewer does what the Review screen asks of held add-on name (kind
// skill or mcp), each in the background; stage is told each check as it
// starts (basic, skill-veil, claude:<model>).
type Reviewer interface {
	// Report is its review as it is: checked first when it never was, or
	// by older rules.
	Report(ctx context.Context, kind, name string, stage func(string)) (Review, error)
	// Check runs every check again; careful: Claude's again, with the
	// larger model.
	Check(ctx context.Context, kind, name string, careful bool, stage func(string)) (Review, error)
	// Fix makes fix to the held copy, then checks it again.
	Fix(ctx context.Context, kind, name, fix string, stage func(string)) (Review, error)
	// Install installs it (typed: its name, when it needs your OK); held
	// is its review again when it wasn't (its files changed: checked
	// again), with what to say.
	Install(ctx context.Context, kind, name, typed string) (toast string, held *Review, err error)
	// Decline deletes it, saying so.
	Decline(ctx context.Context, kind, name string) (string, error)
	// Read is one of its held files: its content, and its place on disk.
	Read(kind, name, file string) ([]byte, string, bool)
}

// review is the screen; see newReview.
type review struct {
	kind, name string
	by         Reviewer
	// ctx is what its runs are under (the program's).
	ctx    context.Context
	rv     Review
	loaded bool
	// sub is the sub-tab on (by name); look Summary's cursor; fb the
	// Findings; chg Changes' cursor (open: its diff alone, scrolled by
	// off); role Files' cursor (open: its files listed).
	sub      string
	look     int
	fb       *findings
	chg, off int
	chgOpen  bool
	role     int
	roleOpen bool
	dialog   *Dialog
	// busy says what's running ("" nothing), stage which check; cancel
	// stops it (esc, while Claude reads); ch brings its stages and end.
	busy, stage string
	cancel      context.CancelFunc
	ch          chan tea.Msg
	toast       string
	hits        []Hit
	width       int
	height      int
	ended       bool
	end         ReviewEnd
}

// reviewStage is a check started; reviewRan a run done: the review as
// it's now (or err), stopped: esc stopped it.
type reviewStage struct {
	r *review
	s string
}

type reviewRan struct {
	r       *review
	rv      Review
	err     error
	stopped bool
	// did: an install or a decline that went (installed, declined), toast
	// saying it.
	toast, did string
}

// newReview opens name's review: it loads (checking it first when it
// must) with the Cmd returned.
func newReview(ctx context.Context, by Reviewer, kind, name string) (*review, tea.Cmd) {
	r := &review{kind: kind, name: name, by: by, ctx: ctx, sub: "Summary", rv: Review{Kind: kind, Name: name}, width: 100, height: 30}
	r.fb = newFindings(nil, r.read)
	cmd := r.run(ctx, "checking", func(ctx context.Context, stage func(string)) reviewRan {
		rv, err := by.Report(ctx, kind, name, stage)
		return reviewRan{rv: rv, err: err}
	})
	return r, cmd
}

func (r *review) read(file string) ([]byte, string, bool) { return r.by.Read(r.kind, r.name, file) }

// run does do in the background under ctx, busy saying what it is:
// its stages, then its end, come back as messages.
func (r *review) run(ctx context.Context, busy string, do func(ctx context.Context, stage func(string)) reviewRan) tea.Cmd {
	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan tea.Msg, 16)
	r.busy, r.stage, r.cancel, r.ch = busy, "", cancel, ch
	go func() {
		out := do(ctx, func(s string) {
			select {
			case ch <- reviewStage{r, s}:
			default:
			}
		})
		out.r, out.stopped = r, ctx.Err() != nil
		cancel()
		ch <- out
	}()
	return r.wait()
}

func (r *review) wait() tea.Cmd {
	ch := r.ch
	return func() tea.Msg { return <-ch }
}

// took takes a message of the review's: a stage, or a run's end.
func (r *review) took(msg tea.Msg) tea.Cmd {
	switch v := msg.(type) {
	case reviewStage:
		// A check ran: the list's row is out of date.
		r.stage, r.end.Changed = v.s, true
		return r.wait()
	case reviewRan:
		r.busy, r.stage, r.cancel = "", "", nil
		switch {
		case v.did != "":
			r.ended, r.end = true, ReviewEnd{Did: v.did, Toast: v.toast, Changed: true}
			return nil
		case v.err != nil:
			r.toast = "✗ " + v.err.Error()
			return nil
		}
		r.show(v.rv)
		r.toast = v.toast
		if v.stopped {
			r.toast = "Stopped: not checked by AI."
		}
	case editedMsg:
		r.fb.edited(v.err)
	}
	return nil
}

// show is rv on the screen: the cursors kept where they can be.
func (r *review) show(rv Review) {
	r.rv, r.loaded = rv, true
	cur, open := r.fb.cur, r.fb.open
	r.fb = newFindings(rv.Findings, r.read)
	r.fb.cur, r.fb.open = cur, open
	r.look = clamp(r.look, 0, len(r.looks())-1)
	if !slices.Contains(r.subs(), r.sub) {
		r.sub = "Summary"
	}
}

// looks are the findings to look at, by index.
func (r *review) looks() []int {
	var out []int
	for n, f := range r.rv.Findings {
		if f.Look {
			out = append(out, n)
		}
	}
	return out
}

// folded counts the notes folded: not to look at, nor explaining.
func (r *review) folded() int {
	n := 0
	for _, f := range r.rv.Findings {
		if !f.Look && !explains(f) {
			n++
		}
	}
	return n
}

// subs are the sub-tabs: Changes only for an update.
func (r *review) subs() []string {
	if r.rv.Update {
		return []string{"Summary", "Findings", "Changes", "Files"}
	}
	return []string{"Summary", "Findings", "Files"}
}

// typing: the install dialog takes what's typed.
func (r *review) typing() bool { return r.dialog != nil && r.dialog.Input }

// leave ends the review, the add-on held as it is.
func (r *review) leave() {
	n := len(r.looks())
	say := "Enter reviews it again."
	if n > 0 {
		say = fmt.Sprintf("%s to look at. Enter reviews it again.", plural(n, "thing", "things"))
	}
	r.ended = true
	r.end.Did, r.end.Toast = "held", fmt.Sprintf("%s stays held: %s", r.name, say)
}

// moveBy is a list's move (pgup, pgdn, the wheel) on the sub-tab on.
func (r *review) moveBy(n int) {
	switch r.sub {
	case "Summary":
		r.look = clamp(r.look+n, 0, len(r.looks())-1)
	case "Findings":
		r.fb.move(n)
	case "Changes":
		if r.chgOpen {
			r.off = max(0, r.off+n)
		} else {
			r.chg = clamp(r.chg+n, 0, len(r.rv.Changes)-1)
		}
	case "Files":
		r.role = clamp(r.role+n, 0, len(r.roles())-1)
	}
}

func (r *review) key(k string) tea.Cmd {
	r.toast = ""
	if r.dialog != nil {
		return dialogKey(&r.dialog, k)
	}
	up, down := k == "up" || k == "k", k == "down" || k == "j"
	subs := r.subs()
	at := max(0, slices.Index(subs, r.sub))
	switch k {
	case "left", "h":
		r.sub = subs[(at+len(subs)-1)%len(subs)]
		return nil
	case "right", "l":
		r.sub = subs[(at+1)%len(subs)]
		return nil
	}
	if r.busy != "" {
		switch {
		case k == "esc" && strings.HasPrefix(r.stage, "claude"):
			r.cancel()
			r.toast = "Stopping Claude…"
		case k == "esc":
			r.toast = "A moment: esc stops Claude once it's reading."
		case up:
			r.moveBy(-1)
		case down:
			r.moveBy(1)
		default:
			r.toast = fmt.Sprintf("A moment: %s.", r.busy)
		}
		return nil
	}
	switch k {
	case "esc", "q":
		if r.sub == "Changes" && r.chgOpen || r.sub == "Files" && r.roleOpen {
			r.chgOpen, r.roleOpen = false, false
			return nil
		}
		r.leave()
		return nil
	case "i":
		r.askInstall()
		return nil
	case "x":
		r.askFix()
		return nil
	case "d":
		r.askDecline()
		return nil
	case "r":
		return r.check(false)
	case "c":
		if !r.rv.Careful {
			r.toast = "Claude can't read it here: Claude Code isn't installed or logged in."
			return nil
		}
		return r.check(true)
	}
	switch r.sub {
	case "Summary":
		switch {
		case up:
			r.moveBy(-1)
		case down:
			r.moveBy(1)
		case (k == "enter" || k == "tab") && len(r.looks()) > 0:
			r.open(r.look)
		case k == "tab":
			r.sub = "Findings"
		}
	case "Findings":
		return r.fb.key(k)
	case "Changes":
		switch {
		case up:
			r.moveBy(-1)
		case down:
			r.moveBy(1)
		case k == "enter" && len(r.rv.Changes) > 0:
			r.chgOpen, r.off = !r.chgOpen, 0
		}
	case "Files":
		switch {
		case up && !r.roleOpen:
			r.moveBy(-1)
		case down && !r.roleOpen:
			r.moveBy(1)
		case k == "enter":
			r.roleOpen = !r.roleOpen
		}
	}
	return nil
}

// check runs every check again (careful: Claude's, carefully).
func (r *review) check(careful bool) tea.Cmd {
	by, kind, name := r.by, r.kind, r.name
	busy := "checking it again"
	if careful {
		busy = "Claude is reading it carefully"
	}
	return r.run(r.ctx, busy, func(ctx context.Context, stage func(string)) reviewRan {
		rv, err := by.Check(ctx, kind, name, careful, stage)
		return reviewRan{rv: rv, err: err}
	})
}

// askInstall asks to install it: what you OK, typing its name when the
// verdict needs that; enter or y else (the cursor on Back).
func (r *review) askInstall() {
	rv := r.rv
	if rv.Verdict == "block" {
		r.toast = "It can't be installed as it is: x fixes what yardo can, d declines."
		return
	}
	// The ones that need an OK, then the notes worth a look: seen, not
	// OK'd.
	var oks, seen []int
	kw := 0
	for _, i := range r.looks() {
		f := rv.Findings[i]
		kw = max(kw, runewidth.StringWidth(cmpOr(f.Kind, oneLine(f.Text))))
		if f.Level == "note" {
			seen = append(seen, i)
		} else {
			oks = append(oks, i)
		}
	}
	kw = min(kw, 40)
	var ls []string
	list := func(is []int) {
		for _, i := range is {
			f := rv.Findings[i]
			what := runewidth.Truncate(cmpOr(f.Kind, oneLine(f.Text)), kw, "…")
			ls = append(ls, fmt.Sprintf("  %s %s  %s", levelGlyph(f.Level), padTo(what, kw), tail(path.Base(f.where()), 40)))
		}
	}
	these := func(n int) string { return map[bool]string{true: "this", false: fmt.Sprintf("these %d", n)}[n == 1] }
	if n := len(oks); n > 0 {
		ls = append(ls, fmt.Sprintf("You're OK-ing %s, as %s now:", these(n), map[bool]string{true: "it is", false: "they are"}[n == 1]))
		list(oks)
	}
	if n := len(seen); n > 0 {
		ls = append(ls, fmt.Sprintf("and you've seen %s:", these(n)))
		list(seen)
	}
	if says := CapsSay(rv.Caps, true); says != "" {
		ls = append(ls, fmt.Sprintf("and that it %s.", says))
	}
	if len(ls) > 0 {
		ls = append(ls, "yardo records them: an update shows only what's new since.", "")
	}
	if rv.Dest != "" {
		ls = append(ls, "Goes to "+rv.Dest+".")
	}
	by, kind, name := r.by, r.kind, r.name
	install := func(typed string) tea.Cmd {
		return r.run(r.ctx, "installing", func(ctx context.Context, _ func(string)) reviewRan {
			toast, held, err := by.Install(ctx, kind, name, typed)
			if err != nil || held != nil {
				out := reviewRan{err: err, toast: toast}
				if held != nil {
					out.rv = *held
				}
				return out
			}
			return reviewRan{did: "installed", toast: toast}
		})
	}
	title := fmt.Sprintf("Install %s as it is?", name)
	if rv.NeedsOK {
		ls = append(ls, "")
		r.dialog = &Dialog{Title: title, Lines: ls, Input: true, Ask: fmt.Sprintf("Type %s to install:", name), Enter: "install",
			Bad: func(t string) string {
				if strings.TrimSpace(t) != name {
					return fmt.Sprintf("That isn't its name: type %s", name)
				}
				return ""
			},
			On: func(t string) tea.Cmd { return install(strings.TrimSpace(t)) }}
		return
	}
	r.dialog = &Dialog{Title: fmt.Sprintf("Install %s?", name), Lines: ls, YN: true, Sel: 1,
		Opts: [][2]string{{"Install (y)", "yes"}, {"Back to the review (n)", "no"}},
		On: func(v string) tea.Cmd {
			if v != "yes" {
				return nil
			}
			return install("")
		}}
}

// askDecline asks before deleting it, saying what that does here.
func (r *review) askDecline() {
	rv, name := r.rv, r.name
	var ls []string
	switch {
	case rv.Kind == "mcp" && rv.Existing:
		ls = []string{fmt.Sprintf("%s stays off: its tools aren't the ones you approved.", name), "Its settings stay; delete it from MCP servers to remove it."}
	case rv.Existing:
		ls = []string{fmt.Sprintf("%s stays off, held off where it was.", name), fmt.Sprintf("sudo yardo skills delete %s removes it.", name)}
	case rv.Update:
		ls = []string{"The update's files are deleted from this machine.", fmt.Sprintf("%s stays as you approved it.", name)}
	case rv.Kind == "mcp":
		ls = []string{"Its held spec is deleted from this machine (and an image pulled for it). Nothing was installed, so nothing else changes.", "To get it later, add it again: it's checked from scratch."}
	default:
		ls = []string{"Its held files are deleted from this machine. Nothing was installed, so nothing else changes.", "To get it later, add it again: it's checked from scratch."}
	}
	by, kind := r.by, r.kind
	r.dialog = &Dialog{Title: fmt.Sprintf("Decline %s?", name), Lines: ls, YN: true, Sel: 1,
		Opts: [][2]string{{"Decline and delete (y)", "yes"}, {"Back to the review (n)", "no"}},
		On: func(v string) tea.Cmd {
			if v != "yes" {
				return nil
			}
			return r.run(r.ctx, "declining", func(ctx context.Context, _ func(string)) reviewRan {
				toast, err := by.Decline(ctx, kind, name)
				if err != nil {
					return reviewRan{err: err}
				}
				return reviewRan{did: "declined", toast: toast}
			})
		}}
}

// askFix lists the fixes: one picked changes the held copy, every check
// runs again, and the review stays.
func (r *review) askFix() {
	if len(r.rv.Fixes) == 0 {
		r.toast = "Nothing yardo can fix in it."
		return
	}
	var opts [][2]string
	for _, f := range r.rv.Fixes {
		l := f.What
		if f.Breaks != "" {
			l += ": " + f.Breaks
		}
		opts = append(opts, [2]string{l, f.ID})
	}
	opts = append(opts, [2]string{"Back to the review", "back"})
	by, kind, name := r.by, r.kind, r.name
	r.dialog = &Dialog{Title: fmt.Sprintf("Fix %s's held copy", name),
		Lines: []string{"These change only yardo's copy. Every check runs again, then you're back here.", "Nothing is installed until you press i.", ""},
		Opts:  opts,
		On: func(v string) tea.Cmd {
			if v == "back" {
				return nil
			}
			return r.run(r.ctx, "fixing it, then checking again", func(ctx context.Context, stage func(string)) reviewRan {
				rv, err := by.Fix(ctx, kind, name, v, stage)
				return reviewRan{rv: rv, err: err, toast: "✓ Fixed and checked again: nothing is installed until you press i."}
			})
		}}
}

// OffSays is why a check didn't run, in words.
func OffSays(off string) string {
	switch off {
	case "not run":
		return "yardo didn't ask it when this was checked"
	case "not installed":
		return "it isn't installed"
	}
	return off
}

// offShort is why a check didn't run, for the strip.
func offShort(off string) string {
	switch off {
	case "not run":
		return "not asked"
	case "not installed":
		return off
	}
	return "didn't run"
}

// verdictWord is the header's verdict, in its colour.
func verdictWord(v string) string {
	mk := tui.Cur.Mark
	switch v {
	case "block":
		return red.Render(mk.Down + " Can't be installed")
	case "ok":
		return yellow.Render(mk.Warn + " Needs your OK")
	case "note":
		return muted.Render(mk.Unchecked + " Notes only")
	case "clean":
		return green.Render(mk.OK + " Nothing found")
	}
	return ""
}

// layerName is a check's name, as the screen says it.
func layerName(l, ai string) string {
	switch l {
	case "basic":
		return "yardo's rules"
	case "claude":
		if ai != "" {
			return fmt.Sprintf("Claude (%s)", ai)
		}
		return "Claude"
	}
	return l
}

// strip is the stage strip: each check done (✓), running (◐) or to come
// (·); when none runs, what each read.
func (r *review) strip() string {
	mk := tui.Cur.Mark
	if r.busy != "" && r.busy != "installing" && r.busy != "declining" {
		at := 0
		switch {
		case r.stage == "skill-veil":
			at = 1
		case r.stage == "basic":
			at = 0
		case strings.HasPrefix(r.stage, "claude"):
			at = 2
		}
		claude := "Claude"
		if _, m, ok := strings.Cut(r.stage, ":"); ok {
			claude = fmt.Sprintf("Claude (%s)", m)
		}
		var out []string
		for n, s := range []string{"rules", "skill-veil", claude} {
			switch {
			case n < at:
				out = append(out, accent.Render(mk.OK+" "+s))
			case n == at:
				out = append(out, keyOn.Render("◐ "+s))
			default:
				out = append(out, muted.Render("· "+s))
			}
		}
		return strings.Join(out, "  ")
	}
	var out []string
	for _, rd := range r.rv.Read {
		name := map[string]string{"basic": "rules", "claude": layerName("claude", r.rv.AI)}[rd.Layer]
		name = cmpOr(name, rd.Layer)
		switch {
		case rd.Off != "":
			out = append(out, muted.Render(mk.Unchecked+" "+name+": "+offShort(rd.Off)))
		case rd.Layer == "claude" && rd.Files < rd.All:
			out = append(out, accent.Render(fmt.Sprintf("%s %s %d of %d files", mk.OK, name, rd.Files, rd.All)))
		case rd.Layer == "basic":
			out = append(out, accent.Render(fmt.Sprintf("%s %s %d", mk.OK, name, rd.All)))
		default:
			out = append(out, accent.Render(mk.OK+" "+name))
		}
	}
	return strings.Join(out, "  ")
}

// head is the screen's top: what it is and its verdict, the sub-tabs
// and the stage strip, a rule.
func (r *review) head(w int) []string {
	right := verdictWord(r.rv.Verdict)
	if r.busy != "" {
		right = keyOn.Render("◐ " + r.busy + "…")
	}
	left := "  " + bold.Render(r.name)
	if about := strings.Join(slices.DeleteFunc([]string{r.rv.What, r.rv.Size}, func(s string) bool { return s == "" }), " · "); about != "" {
		left += muted.Render("  " + about)
	}
	room := w - lipgloss.Width(right) - 2
	if lipgloss.Width(left) > room {
		left = ansi.Truncate(left, max(0, room), "…")
	}
	l0 := left + strings.Repeat(" ", max(2, w-lipgloss.Width(left)-lipgloss.Width(right)-1)) + right
	tabs := " "
	for n, s := range r.subs() {
		label := s
		if s == "Findings" && r.loaded {
			label = fmt.Sprintf("Findings %d", len(r.looks()))
			if f := r.folded(); f > 0 {
				label += "+"
			}
		}
		if n > 0 {
			tabs += " "
		}
		if s == r.sub {
			tabs += tabOn.Render("[" + label + "]")
		} else {
			tabs += " " + label + " "
		}
	}
	st := r.strip()
	l1 := tabs
	if gap := w - lipgloss.Width(tabs) - lipgloss.Width(st) - 1; gap >= 2 {
		l1 += strings.Repeat(" ", gap) + st
	}
	return []string{l0, l1, muted.Render(strings.Repeat("─", w))}
}

// Body is the screen under the app's header, w by h.
func (r *review) Body(w, h int) string {
	r.hits = nil
	b := r.head(w)
	room := h - len(b)
	var in []string
	switch {
	case !r.loaded && r.busy != "":
		in = []string{"", "  " + muted.Render(fmt.Sprintf("Reading %s's report: a check runs first when there's none yet, or it's by older rules.", r.name))}
	case !r.loaded:
		in = []string{"", "  " + muted.Render("No report to show: r checks it.")}
	case r.sub == "Summary":
		in = r.summary(w, room, len(b))
	case r.sub == "Findings":
		in = r.fb.body(w, room)
	case r.sub == "Changes":
		in = r.changes(w, room)
	case r.sub == "Files":
		in = r.filesView(w, room)
	}
	b = append(b, in...)
	if len(b) > h {
		b = b[:h]
	}
	if r.dialog != nil {
		dl := dialogLines(r.dialog, w, 0, nil)
		cut := max(0, min(len(b), h-len(dl)))
		var dh []Hit
		dl = dialogLines(r.dialog, w, cut, &dh)
		b = append(b[:cut], dl...)
		r.hits = dh // the dialog's alone
	}
	return strings.Join(b, "\n")
}

// capGroups are WHAT IT CAN DO's lines: a label, the kinds of Cap on it.
var capGroups = []struct {
	label string
	kinds []string
}{
	{"Runs programs", []string{"exec"}},
	{"Network", []string{"network"}},
	{"Reads", []string{"env", "home", "secrets"}},
	{"Writes", []string{"writes"}},
	{"As the agent", []string{"agent"}},
	{"Grants · hooks", []string{"grants", "hooks", "install"}},
}

// capWord is a word of a cap's value: its style (nil: plain), glued
// to the one before (no space).
type capWord struct {
	s     string
	st    *lipgloss.Style
	glued bool
}

// capWords are caps as words: each one said (yellow when it's worth a
// look) and how many places, a dot between, then whether bundled code
// does it too.
func capWords(cs []Cap) []capWord {
	var out []capWord
	add := func(s string, st *lipgloss.Style) {
		for n, w := range strings.Fields(s) {
			out = append(out, capWord{s: w, st: st, glued: n == 0 && strings.HasPrefix(s, ";")})
		}
	}
	bundled := false
	for _, c := range cs {
		if c.What == "bundled code" {
			bundled = true
			continue
		}
		if len(out) > 0 {
			add("·", &muted)
		}
		var st *lipgloss.Style
		if c.Look {
			st = &yellow
		}
		add(c.value(), st)
		if c.Count > 1 {
			add(fmt.Sprintf("(%s)", plural(c.Count, "place", "places")), &muted)
		}
	}
	switch {
	case bundled && len(out) == 0:
		add("only in bundled code", &muted)
	case bundled:
		add("; also in bundled code", &muted)
	}
	return out
}

// capLines are caps' words in lines at most w wide, styled.
func capLines(cs []Cap, w int) []string {
	var out []string
	line, lw := "", 0
	for _, c := range capWords(cs) {
		ww := runewidth.StringWidth(c.s)
		sep := " "
		switch {
		case line == "" || c.glued:
			sep = ""
		case lw+1+ww > w:
			out, line, lw, sep = append(out, line), "", 0, ""
		}
		s := c.s
		if c.st != nil {
			s = c.st.Render(s)
		}
		line, lw = line+sep+s, lw+len(sep)+ww
	}
	return append(out, line)
}

// capValue is caps' words in a line, plain.
func capValue(cs []Cap) string {
	var b strings.Builder
	for n, c := range capWords(cs) {
		if n > 0 && !c.glued {
			b.WriteString(" ")
		}
		b.WriteString(c.s)
	}
	return b.String()
}

// CapRows are WHAT IT CAN DO in plain text: a label and what's on it,
// for the kinds it has.
func CapRows(cs []Cap) [][2]string {
	var out [][2]string
	for _, g := range capGroups {
		if of := ofKinds(cs, g.kinds); len(of) > 0 {
			out = append(out, [2]string{g.label, capValue(of)})
		}
	}
	return out
}

// value is c on its row: in words, as it says them.
func (c Cap) value() string { return cmpOr(c.Say, c.What) }

// say is c in words, its verb first ("contacts x.io").
func (c Cap) say() string {
	if c.Say != "" {
		return c.Say
	}
	verb := map[string]string{"exec": "runs", "network": "contacts", "writes": "writes", "env": "reads", "home": "uses", "secrets": "reads",
		"agent": "as the agent:", "grants": "is granted", "hooks": "hooks", "install": "runs at install"}
	return strings.TrimSpace(verb[c.Kind] + " " + c.What)
}

// CapsSay is what caps say, in words ("runs node, contacts x.io"); look:
// only those worth a look, and the programs and hosts (none in bundled
// code alone).
func CapsSay(cs []Cap, look bool) string {
	var out []string
	for _, c := range cs {
		if c.What == "bundled code" || look && !c.Look && c.Kind != "exec" && c.Kind != "network" {
			continue
		}
		out = append(out, c.say())
	}
	return strings.Join(out, "; ")
}

// ofKinds are the caps of kinds.
func ofKinds(cs []Cap, kinds []string) []Cap {
	return slices.DeleteFunc(slices.Clone(cs), func(c Cap) bool { return !slices.Contains(kinds, c.Kind) })
}

// open shows the nth to look at in Findings.
func (r *review) open(n int) {
	r.look, r.sub = n, "Findings"
	r.fb.focus(r.looks()[n])
}

// summary is the Summary, y down the body: what it can do, what to look
// at (each clicked opens it in Findings), what was read.
func (r *review) summary(w, h, y int) []string {
	rv := r.rv
	var ls []string
	sel := 0
	rows := map[int]int{} // a look's line: which it is
	lw := 18
	line := func(label, v string) { ls = append(ls, "   "+muted.Render(padTo(label, lw))+v) }
	ls = append(ls, "")
	switch {
	case rv.NoCaps:
		ls = append(ls, "  "+bold.Render("WHAT IT CAN DO")+muted.Render("  not worked out (an older report: r rescans)"))
	default:
		ls = append(ls, "  "+bold.Render("WHAT IT CAN DO")+muted.Render("  from its own code, not what it says it does"))
		for _, g := range capGroups {
			cs := ofKinds(rv.Caps, g.kinds)
			if len(cs) == 0 {
				line(g.label, muted.Render("none"))
				continue
			}
			// Wrapped under itself, whole.
			for n, l := range capLines(cs, max(10, w-lw-4)) {
				line(map[bool]string{true: g.label}[n == 0], l)
			}
		}
	}
	ls = append(ls, "")
	looks := r.looks()
	var says []string
	count := func(level, one, many string) {
		n := 0
		for _, i := range looks {
			if rv.Findings[i].Level == level {
				n++
			}
		}
		if n > 0 {
			says = append(says, plural(n, one, many))
		}
	}
	count("block", "that stops it", "that stop it")
	count("ok", "that needs your OK", "that need your OK")
	count("note", "worth a look", "worth a look")
	if f := r.folded(); f > 0 {
		says = append(says, plural(f, "note", "notes")+" folded, none of them stops it")
	}
	head := "  " + bold.Render("LOOK AT THESE")
	if len(says) > 0 {
		head += muted.Render("  " + strings.Join(says, " · "))
	}
	ls = append(ls, head+"  "+keyOn.Render("tab")+muted.Render(" findings"))
	if len(looks) == 0 {
		ls = append(ls, "   "+muted.Render("Nothing to look at."))
	}
	kw := clamp(w/4+4, 20, 34)
	ww := clamp(w/3, 20, 46)
	for n, i := range looks {
		f := rv.Findings[i]
		where := tail(f.where(), ww)
		what := cmpOr(f.Kind, oneLine(f.Text))
		rows[len(ls)] = n
		if n == r.look {
			sel = len(ls)
			plain := fmt.Sprintf("› %d %s %s %s %s", n+1, levelGlyph(f.Level), padTo(what, kw), padTo(where, ww), roleName(f.Role))
			ls = append(ls, " "+rowOn.Render(padTo(plain, w-2)))
		} else {
			ls = append(ls, fmt.Sprintf("   %d %s %s %s %s", n+1, levelMark(f.Level), bold.Render(padTo(what, kw)), keyOn.Render(padTo(where, ww)), muted.Render(roleName(f.Role))))
		}
		if f.Kind != "" {
			ls = append(ls, "       "+muted.Render(runewidth.Truncate(oneLine(f.Text), max(10, w-9), "…")))
		}
	}
	ls = append(ls, "", "  "+bold.Render("WHAT WAS READ"))
	for _, rd := range rv.Read {
		var v string
		switch {
		case rd.Off != "":
			v = yellow.Render("didn't run: " + OffSays(rd.Off))
			if rd.Off == "not run" {
				v += muted.Render(" · ") + keyOn.Render("r") + muted.Render(" checks again with it")
			}
		case rd.Layer == "basic" && rd.Files == rd.All:
			v = fmt.Sprintf("every line of all %s", plural(rd.All, "file", "files"))
		case rd.Files == rd.All:
			v = "all " + plural(rd.All, "file", "files")
		default:
			v = fmt.Sprintf("%d of its %s", rd.Files, plural(rd.All, "file", "files"))
		}
		if rd.Skipped != "" {
			v += muted.Render(" · skipped: " + rd.Skipped)
		}
		line(layerName(rd.Layer, rv.AI), v)
		// Which files worth a look it read, and how: a part a line.
		for _, part := range strings.Split(rd.Looks, "; ") {
			if part != "" {
				line("", muted.Render(runewidth.Truncate(part, max(10, w-lw-4), "…")))
			}
		}
	}
	for _, rd := range rv.Read {
		if rd.Unread == "" {
			continue
		}
		v := yellow.Render(rd.Unread)
		if rv.Careful {
			v += muted.Render(" · ") + keyOn.Render("c") + muted.Render(" read them all with the larger model")
		}
		line("Not read", v)
	}
	line("Not checked", muted.Render("what it does when it runs; what a later update brings until you update it"))
	shown, at := window(ls, sel, h)
	for k, i := range at {
		if n, ok := rows[i]; ok {
			r.hits = append(r.hits, Hit{Y0: y + k, Y1: y + k, Do: func() { r.open(n) }})
		}
	}
	return shown
}

// diffCount is a diff's lines added and removed.
func diffCount(d string) (add, del int) {
	for l := range strings.SplitSeq(d, "\n") {
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
		case strings.HasPrefix(l, "+"):
			add++
		case strings.HasPrefix(l, "-"):
			del++
		}
	}
	return add, del
}

// diffLines are a diff's lines, w wide, coloured: + green, − red.
func diffLines(d string, w int) []string {
	var out []string
	for l := range strings.SplitSeq(strings.TrimRight(d, "\n"), "\n") {
		s := runewidth.Truncate(oneLine(l), w, "…")
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"), strings.HasPrefix(l, "@@"):
			s = muted.Render(s)
		case strings.HasPrefix(l, "+"):
			s = green.Render(s)
		case strings.HasPrefix(l, "-"):
			s = red.Render(s)
		}
		out = append(out, s)
	}
	return out
}

// changes is an update's Changes: what it can do now against what you
// approved, the findings new since, then its files' diffs.
func (r *review) changes(w, h int) []string {
	rv := r.rv
	if r.chgOpen && r.chg < len(rv.Changes) {
		c := rv.Changes[r.chg]
		ds := diffLines(c.Diff, w-4)
		r.off = clamp(r.off, 0, max(0, len(ds)-(h-2)))
		out := []string{"", fmt.Sprintf("  %s  %s", bold.Render(c.Path), muted.Render(fmt.Sprintf("%s · lines %d–%d of %d · esc back", c.What, r.off+1, min(len(ds), r.off+h-2), len(ds))))}
		for _, l := range ds[r.off:min(len(ds), r.off+h-2)] {
			out = append(out, "   "+l)
		}
		return out
	}
	ls := []string{""}
	if len(rv.Gained)+len(rv.Lost)+len(rv.Caps) > 0 {
		ls = append(ls, "  "+bold.Render("WHAT IT CAN DO, CHANGED"))
		label := func(kind string) string {
			for _, g := range capGroups {
				if slices.Contains(g.kinds, kind) {
					return g.label
				}
			}
			return kind
		}
		for _, c := range rv.Gained {
			where := ""
			if len(c.Where) > 0 {
				where = muted.Render("  new: " + c.Where[0])
			}
			ls = append(ls, yellow.Render("   + it "+c.say())+where)
		}
		for _, c := range rv.Lost {
			ls = append(ls, muted.Render("   − it no longer "+c.say()))
		}
		for _, c := range rv.Caps {
			if !slices.ContainsFunc(rv.Gained, func(g Cap) bool { return g.Kind == c.Kind && g.What == c.What }) {
				ls = append(ls, muted.Render("   = "+padTo(label(c.Kind), 16)+c.What))
			}
		}
		ls = append(ls, "")
	}
	var fresh []Finding
	for _, f := range rv.Findings {
		if f.New {
			fresh = append(fresh, f)
		}
	}
	look := 0
	for _, f := range fresh {
		if f.Look {
			look++
		}
	}
	say := fmt.Sprintf("%d to look at · %s", look, plural(len(fresh)-look, "note", "notes"))
	if rv.Accepted > 0 {
		say += fmt.Sprintf(" · the %d you OK'd stay folded", rv.Accepted)
	}
	ls = append(ls, "  "+bold.Render("NEW SINCE YOU APPROVED IT")+muted.Render("  "+say))
	for _, f := range fresh {
		ls = append(ls, fmt.Sprintf("   %s %s %s", levelMark(f.Level), padTo(cmpOr(f.Kind, oneLine(f.Text)), 30), keyOn.Render(tail(f.where(), max(10, w-40)))))
	}
	ls = append(ls, "", "  "+bold.Render("THE CHANGES")+muted.Render("  in full, file by file (enter opens one)"))
	if len(rv.Changes) == 0 {
		ls = append(ls, "   "+muted.Render("No file changed."))
	}
	sel := 0
	for n, c := range rv.Changes {
		a, d := diffCount(c.Diff)
		count := fmt.Sprintf("+%d −%d", a, d)
		mk := map[string]string{"added": "A", "removed": "D"}[c.What]
		mk = cmpOr(mk, "M")
		if n == r.chg {
			sel = len(ls)
			ls = append(ls, "  "+rowOn.Render(padTo(fmt.Sprintf(" › %s %s %s", mk, padTo(c.Path, 44), count), min(w-4, 60))))
			continue
		}
		st := yellow
		if mk == "A" {
			st = accent
		}
		ls = append(ls, fmt.Sprintf("    %s %s %s", st.Render(mk), keyOn.Render(padTo(c.Path, 44)), muted.Render(count)))
	}
	if r.chg < len(rv.Changes) {
		c := rv.Changes[r.chg]
		ls = append(ls, "", "   "+bold.Render(c.Path))
		for _, l := range diffLines(c.Diff, w-6) {
			ls = append(ls, "    "+l)
		}
	}
	shown, _ := window(ls, sel, h)
	return shown
}

// roleRow is a role on Files: its files.
type roleRow struct {
	role  string
	files []string
}

// roles are the files by role, most weight first.
func (r *review) roles() []roleRow {
	by := map[string][]string{}
	for p, role := range r.rv.Files {
		by[role] = append(by[role], p)
	}
	var out []roleRow
	for _, role := range append(slices.Clone(roleOrder), slices.Sorted(maps.Keys(by))...) {
		if fs := by[role]; len(fs) > 0 && !slices.ContainsFunc(out, func(x roleRow) bool { return x.role == role }) {
			slices.Sort(fs)
			out = append(out, roleRow{role, fs})
		}
	}
	return out
}

// roleAbout says what a role is to the agent, and how it's checked.
var roleAbout = map[string][2]string{
	"instructions": {"the agent reads them as orders", "every check, Claude first"},
	"script":       {"its instructions tell the agent to run them", "every check, Claude next"},
	"code":         {"its own, imported by those", "every check; Claude as its budget allows"},
	"vendored":     {"bundled third-party code", "rules and skill-veil, as context"},
	"asset":        {"templates, fonts, images", "rules, as context"},
	"doc":          {"for people, not the agent", "rules, as context"},
	"license":      {"", "skipped"},
}

// which is a role's files in a few words: their folders, counted.
func which(fs []string, w int) string {
	dirs := map[string]int{}
	var order []string
	for _, f := range fs {
		d := path.Dir(f)
		if d == "." {
			d = f
		} else {
			d += "/"
		}
		if dirs[d] == 0 {
			order = append(order, d)
		}
		dirs[d]++
	}
	var out []string
	for _, d := range order {
		if dirs[d] > 1 {
			out = append(out, fmt.Sprintf("%s (%d)", d, dirs[d]))
		} else {
			out = append(out, d)
		}
	}
	return runewidth.Truncate(strings.Join(out, ", "), w, "…")
}

// filesView is Files: the files by role, where they're held, and how
// to open them.
func (r *review) filesView(w, h int) []string {
	rv := r.rv
	ls := []string{""}
	rs := r.roles()
	sel := 0
	if len(rs) == 0 {
		ls = append(ls, "  "+muted.Render("Its files' roles weren't worked out (an older report: r rescans)."))
	} else {
		ls = append(ls, "  "+muted.Render(fmt.Sprintf("%s by what they are to the agent: what decides how much a finding matters", plural(len(rv.Files), "file", "files"))), "")
		ww := max(20, w-17-8-40)
		ls = append(ls, muted.Render(fmt.Sprintf("   %s %s  %s  how it's checked", padTo("role", 16), padTo("count", 5), padTo("which", ww))))
		for n, x := range rs {
			about := roleAbout[x.role]
			row := fmt.Sprintf("%s %5d  %s  ", padTo(roleName(x.role), 16), len(x.files), padTo(which(x.files, ww), ww))
			if n == r.role {
				sel = len(ls)
				ls = append(ls, "  "+rowOn.Render("›"+row)+muted.Render(about[1]))
			} else {
				ls = append(ls, "   "+row+muted.Render(about[1]))
			}
			if about[0] != "" {
				ls = append(ls, "   "+muted.Render(padTo("", 17)+about[0]))
			}
			if n == r.role && r.roleOpen {
				for _, f := range x.files {
					ls = append(ls, "      "+keyOn.Render(runewidth.Truncate(f, w-8, "…")))
				}
			}
		}
	}
	ls = append(ls, "")
	if rv.Dir != "" {
		// In full, wrapped: to copy.
		long := func(label, v string) {
			for n, c := range chunk(v, max(10, w-13)) {
				ls = append(ls, "  "+muted.Render(padTo(map[bool]string{true: label}[n == 0], 9))+keyOn.Render(c))
			}
		}
		long("Held at", rv.Dir)
		if rv.Main != "" {
			long("Open it", fmt.Sprintf("sudo vi -R %s/%s", rv.Dir, rv.Main))
		}
	}
	shown, _ := window(ls, sel, h)
	return shown
}

// Keys are the footer's: the sub-tab's own, then the actions.
func (r *review) Keys() []Key {
	if r.dialog != nil {
		return dialogKeys(r.dialog)
	}
	var keys []Key
	switch r.sub {
	case "Summary":
		keys = []Key{{Key: "↑↓", What: "item"}, {Key: "enter", What: "its code"}}
	case "Findings":
		keys = []Key{{Key: "↑↓", What: "finding"}, {Key: "enter", What: "open in " + editor() + " / unfold"}, {Key: "J K", What: "scroll code"}}
	case "Changes":
		keys = []Key{{Key: "↑↓", What: "file"}, {Key: "enter", What: "its diff"}}
	case "Files":
		keys = []Key{{Key: "↑↓", What: "role"}, {Key: "enter", What: "its files"}}
	}
	keys = append(keys, Key{Key: "←→", What: "sub-tabs", Keep: true})
	if r.busy != "" {
		if strings.HasPrefix(r.stage, "claude") {
			keys = append(keys, Key{Key: "esc", What: "stop Claude", Keep: true})
		}
		return keys
	}
	if r.loaded && r.rv.Verdict != "block" {
		keys = append(keys, Key{Key: "i", What: "install…", Keep: true})
	}
	if len(r.rv.Fixes) > 0 {
		keys = append(keys, Key{Key: "x", What: "fix"})
	}
	keys = append(keys, Key{Key: "r", What: "rescan"})
	if r.rv.Careful {
		keys = append(keys, Key{Key: "c", What: "read carefully"})
	}
	return append(keys, Key{Key: "d", What: "decline", Keep: true}, Key{Key: "esc", What: "leave held", Keep: true})
}

// Help are its keys, for the keys screen.
func (r *review) Help() []Key {
	return []Key{{Key: "←→ h l", What: "Summary, Findings, Changes (an update's), Files"},
		{Key: "↑↓", What: "move: an item, a finding, a file, a role"},
		{Key: "enter tab", What: "on Summary: the item in Findings; on Findings: open its file in " + editor() + " (read only), or unfold a group"},
		{Key: "J K", What: "scroll the code"},
		{Key: "i", What: "install it: what you OK is recorded"},
		{Key: "x", What: "fix what yardo can in its held copy, then check again"},
		{Key: "r", What: "rescan: every check again, on it as it is"},
		{Key: "c", What: "Claude reads it again, carefully (the larger model)"},
		{Key: "d", What: "decline: its held files are deleted"},
		{Key: "esc", What: "leave it held (while Claude reads: stop it)"}}
}

// Path is where it is: its list, then its name.
func (r *review) Path() string {
	if r.kind == "mcp" {
		return "MCP servers › " + r.name
	}
	return "Skills › " + r.name
}

func (r *review) Back() string {
	if r.dialog != nil {
		return ""
	}
	return "back"
}

func (r *review) Hits() []Hit   { return r.hits }
func (r *review) Toast() string { return r.toast }
func (r *review) Typing() bool  { return r.typing() }

func (r *review) Init() tea.Cmd { return nil }

// View is the review drawn alone, as ReviewScreen draws it.
func (r *review) View() string { return standalone(r, HomeInfo{}, nil, "", r.width, r.height) }

// Update is the review alone (ReviewScreen's; the menu calls key).
func (r *review) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = v.Width, v.Height
	case tea.KeyMsg:
		return r, r.key(v.String())
	case MoveMsg:
		r.moveBy(v.N)
	case reviewStage, reviewRan, editedMsg:
		return r, r.took(msg)
	}
	return r, nil
}

// reviewProg is the Review screen as a program of its own (sudo yardo
// addons review NAME, on a terminal).
type reviewProg struct {
	r    *review
	init tea.Cmd
	out  bool
}

func (p *reviewProg) Init() tea.Cmd { return p.init }

func (p *reviewProg) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v, ok := msg.(tea.KeyMsg); ok {
		switch k := v.String(); {
		case k == "ctrl+c":
			if p.r.cancel != nil {
				p.r.cancel()
			}
			p.r.ended, p.r.end = true, ReviewEnd{Did: "quit", Changed: p.r.end.Changed}
			return p, tea.Quit
		case !p.r.typing() && (k == "pgup" || k == "pgdown"):
			n := max(1, p.r.height-8)
			if k == "pgup" {
				n = -n
			}
			p.r.moveBy(n)
			return p, nil
		}
	}
	_, cmd := p.r.Update(msg)
	if p.r.ended {
		return p, tea.Quit
	}
	return p, cmd
}

func (p *reviewProg) View() string {
	if p.out {
		return ""
	}
	return p.r.View()
}

// ReviewScreen shows held add-on name's review on its own, until it's
// installed, declined or left; it says how it ended.
func ReviewScreen(ctx context.Context, by Reviewer, kind, name string) (ReviewEnd, error) {
	r, cmd := newReview(ctx, by, kind, name)
	p := &reviewProg{r: r, init: cmd}
	_, err := tea.NewProgram(p, tea.WithAltScreen()).Run()
	p.out = true
	if err != nil {
		return ReviewEnd{Did: "quit"}, err
	}
	if !r.ended {
		r.end.Did = "quit"
	}
	return r.end, nil
}

// Reviewing has the menu open held add-ons' reviews in place, their
// checks and decisions run by r, under ctx.
func (m *Menu) Reviewing(ctx context.Context, r Reviewer) *Menu {
	m.reviewer, m.rvCtx = r, ctx
	return m
}

// openReview opens held add-on name's review (kind skill or mcp); with
// no reviewer, the menu is left to review it on the terminal.
func (m *Menu) openReview(kind, name string) tea.Cmd {
	if m.reviewer == nil {
		k := MenuSkills
		if kind == "mcp" {
			k = MenuMCP
		}
		return m.quit(MenuResult{Kind: k, MCP: MCPAct{Do: "review", Entry: name}})
	}
	if len(m.pend)+len(m.skPend) > 0 {
		m.toast = "Apply first (a), or turn back what you changed."
		return nil
	}
	r, cmd := newReview(m.rvCtx, m.reviewer, kind, name)
	m.rv, m.rvFrom, m.screen, m.dialog = r, m.screen, "review", nil
	return cmd
}

// reviewed is after a review's key or message: once it's ended, back
// where it was opened from, saying how it went; the machine is read
// again when the add-on changed.
func (m *Menu) reviewed(cmd tea.Cmd) tea.Cmd {
	r := m.rv
	if r == nil || !r.ended {
		return cmd
	}
	m.rv, m.screen = nil, cmpOr(m.rvFrom, "home")
	if r.end.Did == "held" && !r.end.Changed {
		m.toast = r.end.Toast
		return cmd
	}
	return m.quit(MenuResult{Kind: MenuReviewed, ID: m.screen, MCP: MCPAct{Do: r.end.Did, Entry: r.name}, Toast: r.end.Toast})
}
