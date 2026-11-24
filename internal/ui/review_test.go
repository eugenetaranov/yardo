package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// fakeReviewer stands in for yardo's: rv as the report, each call
// recorded; careful checks wait to be stopped.
type fakeReviewer struct {
	mu    sync.Mutex
	rv    Review
	calls []string
}

func (f *fakeReviewer) did(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

// Report: its report is current (no check runs).
func (f *fakeReviewer) Report(context.Context, string, string, func(string)) (Review, error) {
	f.did("report")
	return f.rv, nil
}

func (f *fakeReviewer) Check(ctx context.Context, _, _ string, careful bool, stage func(string)) (Review, error) {
	if careful {
		f.did("careful")
		stage("claude:sonnet")
		<-ctx.Done()
		return f.rv, nil
	}
	f.did("check")
	stage("basic")
	stage("claude:haiku")
	return f.rv, nil
}

func (f *fakeReviewer) Fix(_ context.Context, _, _, fix string, stage func(string)) (Review, error) {
	f.did("fix " + fix)
	stage("basic")
	rv := f.rv
	rv.Fixes = nil
	return rv, nil
}

func (f *fakeReviewer) Install(_ context.Context, _, name, typed string) (string, *Review, error) {
	f.did("install " + typed)
	return "✓ " + name + " installed to ~/.agents/skills and ~/.claude/skills · recorded what you OK'd", nil, nil
}

func (f *fakeReviewer) Decline(_ context.Context, _, name string) (string, error) {
	f.did("decline")
	return name + " declined: its files are gone from this machine.", nil
}

func (f *fakeReviewer) Read(_, _, file string) ([]byte, string, bool) {
	return []byte("---\nname: archify\n---\n" + strings.Repeat("line\n", 40)), "/var/lib/yardo/quarantine/ada/7c1e/files/" + file, true
}

// fxReview is archify as yardo would see it: what it can do, three
// things to look at, one that explains, notes to fold, what was read.
func fxReview() Review {
	return Review{Kind: "skill", Name: "archify", What: "skill from tt-a1i/archify @ 7c1e9a0", Size: "22.0 MB · 441 files · Apache-2.0",
		Verdict: "ok", NeedsOK: true, Dir: "/var/lib/yardo/quarantine/ada/7c1e/files", Main: "SKILL.md",
		Caps: []Cap{{Kind: "exec", What: "node, git, open", Count: 28}, {Kind: "network", What: "tt-a1i.github.io", Look: true},
			{Kind: "network", What: "127.0.0.1"}, {Kind: "network", What: "bundled code"}, {Kind: "env", What: "environment variables", Count: 34},
			{Kind: "writes", What: "your global rules file", Look: true}, {Kind: "agent", What: "loads in every session"}},
		Findings: []Finding{
			{Level: "ok", Look: true, Kind: "Changes your agent's global rules", Role: "instructions", File: "SKILL.md", Line: 30, Text: "Tells the agent to write a line into your global rules file.", Why: []string{"SKILL.md is read in every session."}, By: "yardo's rules: writes-outside"},
			{Level: "ok", Look: true, Kind: "Contacts the network", Role: "script", File: "scripts/check-update.mjs", Line: 1397, Text: "Fetches tt-a1i.github.io/archify/stable.json each time finalize runs.", By: "capabilities: network"},
			{Level: "ok", Look: true, Kind: "Not read through", Role: "vendored", File: "modules/code-analysis/vendor/typescript.cjs", Text: "9.5 MB bundled library: Claude's review read none of it.", Next: "install it as it is, or delete it from the held copy", By: "yardo's rules: size"},
			{Level: "note", Kind: "Starts programs", Role: "script", File: "bin/preview.mjs", Line: 793, Text: "Starts node for a local preview server.", Why: []string{"What a diagram tool does."}, By: "capabilities: exec"},
			{Level: "note", Group: "Web addresses", Role: "license", File: "LICENSE", Line: 4, Text: "a web address: http://www.apache.org/licenses/", Short: "http://www.apache.org/licenses/"},
			{Level: "note", Group: "Web addresses", Role: "doc", File: "README.md", Line: 9, Text: "a web address: https://github.com/tt-a1i/archify", Short: "https://github.com/tt-a1i/archify"},
			{Level: "note", Group: "Invisible characters", Role: "vendored", File: "modules/code-analysis/vendor/deps.cjs", Line: 4982, Text: "U+200E in a string"},
		},
		Read: []Read{{Layer: "basic", Files: 441, All: 441}, {Layer: "skill-veil", Files: 441, All: 441},
			{Layer: "claude", Files: 38, All: 52, Skipped: "389 licences, assets, vendored", Looks: "3 of the 3 files worth a look; whole: SKILL.md; 40 lines around each place: scripts/check-update.mjs", Unread: "14 own files past the budget"}},
		AI: "haiku", Careful: true,
		Fixes: []Fix{{ID: "remove:modules/code-analysis/vendor/typescript.cjs", What: "Delete modules/code-analysis/vendor/typescript.cjs", Breaks: "whatever uses typescript.cjs stops working"}},
		Files: map[string]string{"SKILL.md": "instructions", "references/a.md": "instructions", "scripts/check-update.mjs": "script", "bin/preview.mjs": "script",
			"modules/code-analysis/vendor/typescript.cjs": "vendored", "README.md": "doc", "LICENSE": "license"},
		Dest: "~/.agents/skills/archify and ~/.claude/skills/archify"}
}

// fxUpdate is archify's update: a host it didn't call before, a file
// changed.
func fxUpdate() Review {
	rv := fxReview()
	rv.Update, rv.Accepted = true, 3
	rv.Gained = []Cap{{Kind: "network", What: "api.github.com", Where: []string{"scripts/check-update.mjs:1502"}, Look: true}}
	rv.Caps = append(rv.Caps, rv.Gained...)
	rv.Findings = append(rv.Findings, Finding{Level: "ok", Look: true, New: true, Kind: "Contacts the network", Role: "script", File: "scripts/check-update.mjs", Line: 1502, Text: "Asks GitHub for release notes."})
	rv.Changes = []Change{{Path: "scripts/check-update.mjs", What: "changed", Diff: "--- a/scripts/check-update.mjs\n+++ b/scripts/check-update.mjs\n@@ -1500,1 +1500,3 @@\n   const manifest = await fetchManifest(url);\n+  const notes = await fetchImpl(`https://api.github.com/repos/x/releases/latest`);\n+  result.notes = notes;\n"}}
	return rv
}

// drive runs cmd and what it leads to, giving each message to m: a
// review's run, to its end.
func drive(m tea.Model, cmd tea.Cmd) {
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			return
		}
		_, cmd = m.Update(msg)
	}
}

// reviewMenu is the Skills list with archify held and a reviewer.
func reviewMenu(rv Review) (*Menu, *fakeReviewer) {
	m := testMenu(true)
	m.info.Skills = []SkillRow{{Name: "archify", Source: "held", Off: "held", Verdict: "ok", Held: "3 to look at · runs node, git, open, contacts tt-a1i.github.io"}, {Name: "pdf", Source: "yardo"}}
	m = NewMenu(m.info, "", m.items, nil, true)
	f := &fakeReviewer{rv: rv}
	m.Reviewing(context.Background(), f)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m.openTab("skills")
	return m, f
}

// rk presses keys on m, each one's run driven to its end.
func rk(m *Menu, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd := m.Update(msg)
		drive(m, cmd)
	}
}

// rview is m drawn w by h, each line checked to fit.
func rview(t *testing.T, m *Menu, w, h int) string {
	t.Helper()
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	raw := m.View()
	if n := len(strings.Split(raw, "\n")); n != h {
		t.Errorf("%dx%d: %d lines", w, h, n)
	}
	for _, l := range strings.Split(raw, "\n") {
		if ansi.StringWidth(l) > w {
			t.Errorf("%dx%d: a line %d wide: %q", w, h, ansi.StringWidth(l), ansi.Strip(l))
		}
	}
	return ansi.Strip(raw)
}

func has(t *testing.T, what, v string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(v, w) {
			t.Errorf("%s: no %q:\n%s", what, w, v)
		}
	}
}

// Enter on a held skill opens its review at once: what it is and its
// verdict, the sub-tabs and what each check read; Summary says what it
// can do, the three to look at, numbered, what was read and what wasn't,
// and how many notes are folded.
func TestReviewSummary(t *testing.T) {
	m, f := reviewMenu(fxReview())
	v := rview(t, m, 120, 36)
	has(t, "list", v, "▲ held: needs your OK", "3 to look at · runs node, git, open")
	rk(m, "enter")
	if m.screen != "review" || !slices.Equal(f.calls, []string{"report"}) {
		t.Fatalf("enter: on %s, %v", m.screen, f.calls)
	}
	for _, sz := range [][2]int{{120, 36}, {80, 24}} {
		v := rview(t, m, sz[0], sz[1])
		has(t, "summary", v, "Skills › archify", "archify  skill from tt-a1i/archify @ 7c1e9a0", "▲ Needs your OK", "[Summary]", "Findings 3+", "Files",
			"tt-a1i.github.io", "LOOK AT THESE", "› 1 ▲ Changes your agent's gl", "2 ▲ Contacts the network", "i install…")
		if sz[0] == 120 {
			has(t, "summary", v, "WHAT IT CAN DO", "tt-a1i.github.io · 127.0.0.1; also in bundled code", "Runs programs", "node, git, open (28 places)", "✓ rules 441", "✓ Claude (haiku) 38 of 52 files", "3 that need your OK · 3 notes folded, none of them stops it",
				"WHAT WAS READ", "every line of all 441 files", "38 of its 52 files · skipped: 389 licences", "3 of the 3 files worth a look\n", "whole: SKILL.md\n", "40 lines around each place: scripts/check-update.mjs", "Not read", "14 own files past the budget",
				"c read them all with the larger model", "Not checked", "Grants · hooks", "none", "esc leave held")
			t.Logf("Summary, 120x36:\n%s", v)
		}
		if strings.Contains(v, "Changes  ") || strings.Contains(v, "[Changes]") {
			t.Errorf("a Changes tab on a new add-on:\n%s", v)
		}
	}
}

// ←→ go along the sub-tabs (not the tabs at the top); enter on an item
// of Summary opens it in Findings; Files has the roles and where it's
// held.
func TestReviewSubTabs(t *testing.T) {
	m, _ := reviewMenu(fxReview())
	rk(m, "enter", "down", "enter")
	v := rview(t, m, 120, 36)
	has(t, "findings", v, "[Findings 3+]", "› ▲ Contacts the network", "/var/lib/yardo/quarantine/ada/7c1e/files/scripts/check-update.mjs:", "script it runs", "Context (1)", "▸ Web addresses (2)")
	t.Logf("Findings, 120x36:\n%s", v)
	rk(m, "right")
	v = rview(t, m, 120, 36)
	has(t, "files", v, "[Files]", "instructions", "the agent reads them as orders", "vendored", "Held at  /var/lib/yardo/quarantine/ada/7c1e/files", "sudo vi -R /var/lib/yardo/quarantine/ada/7c1e/files/SKILL.md")
	rk(m, "right")
	if m.screen != "review" || m.rv.sub != "Summary" {
		t.Errorf("→ past Files: %s %s", m.screen, m.rv.sub)
	}
	rk(m, "left", "left")
	if m.rv.sub != "Findings" {
		t.Errorf("← ←: %s", m.rv.sub)
	}
}

// An update has Changes: what it can do that's new (+), what's new
// since you approved it, and its diff, enter opening it alone.
func TestReviewChanges(t *testing.T) {
	m, _ := reviewMenu(fxUpdate())
	rk(m, "enter", "right", "right")
	v := rview(t, m, 120, 36)
	has(t, "changes", v, "[Changes]", "WHAT IT CAN DO, CHANGED", "+ it contacts api.github.com  new: scripts/check-update.mjs:1502", "= Runs programs",
		"NEW SINCE YOU APPROVED IT", "1 to look at · 0 notes · the 3 you OK'd stay folded", "THE CHANGES", "M scripts/check-update.mjs", "+2 −0", "+  const notes = await fetchImpl")
	rk(m, "enter")
	if v := rview(t, m, 120, 36); !strings.Contains(v, "esc back") || strings.Contains(v, "THE CHANGES") {
		t.Errorf("its diff alone:\n%s", v)
	}
	rk(m, "esc")
	if m.screen != "review" || m.rv.chgOpen {
		t.Errorf("esc from the diff: %s", m.screen)
	}
}

// i asks for its name typed: a typo says so and stays; its name
// installs, and the list says what happened (the machine read again).
func TestReviewInstall(t *testing.T) {
	m, f := reviewMenu(fxReview())
	rk(m, "enter", "i")
	v := rview(t, m, 120, 36)
	has(t, "install", v, "Install archify as it is?", "You're OK-ing these 3, as they are now:", "▲ Changes your agent's global rules  SKILL.md:30",
		"▲ Contacts the network               check-update.mjs:1397",
		"and that it runs node, git, open; contacts tt-a1i.github.io; contacts 127.0.0.1; writes your global rules file.",
		"yardo records them: an update shows only what's new since.", "Goes to ~/.agents/skills/archify and ~/.claude/skills/archify.", "Type archify to install:", "enter install", "esc back")
	rk(m, "a", "r", "c", "h", "f", "i", "y", "enter")
	v = rview(t, m, 120, 36)
	has(t, "a typo", v, "That isn't its name: type archify")
	if m.rv == nil || m.rv.dialog == nil || slices.Contains(f.calls, "install archify") {
		t.Fatalf("a typo installed, or closed: %v", f.calls)
	}
	rk(m, "a", "r", "c", "h", "i", "f", "x", "backspace", "y", "enter")
	if !slices.Contains(f.calls, "install archify") || m.rv != nil || !m.ended {
		t.Fatalf("its name: %v, %+v", f.calls, m.result)
	}
	if r := m.result; r.Kind != MenuReviewed || r.ID != "skills" || r.MCP != (MCPAct{Do: "installed", Entry: "archify"}) || !strings.HasPrefix(r.Toast, "✓ archify installed to") {
		t.Errorf("result: %+v", r)
	}
	// Notes only: y installs, the cursor on Back first.
	rv := fxReview()
	rv.Verdict, rv.NeedsOK = "note", false
	m, f = reviewMenu(rv)
	rk(m, "enter", "i")
	if d := m.rv.dialog; d == nil || d.Input || d.Opts[d.Sel][1] != "no" {
		t.Fatalf("notes only: %+v", d)
	}
	rk(m, "y")
	if !slices.Contains(f.calls, "install ") {
		t.Errorf("y: %v", f.calls)
	}
	// Blocked: no i, and i says why.
	rv.Verdict = "block"
	m, _ = reviewMenu(rv)
	rk(m, "enter", "i")
	v = rview(t, m, 120, 36)
	if m.rv.dialog != nil || strings.Contains(v, "i install") || !strings.Contains(v, "It can't be installed as it is") || !strings.Contains(v, "✗ Can't be installed") {
		t.Errorf("blocked:\n%s", v)
	}
}

// esc leaves it held: back on the list, saying so, nothing read again;
// d asks, y declines, and the list is read again.
func TestReviewLeaveDecline(t *testing.T) {
	m, _ := reviewMenu(fxReview())
	rk(m, "enter", "esc")
	if m.screen != "skills" || m.ended || m.toast != "archify stays held: 3 things to look at. Enter reviews it again." {
		t.Errorf("esc: on %s, %q", m.screen, m.toast)
	}
	v := rview(t, m, 120, 36)
	has(t, "held", v, "archify stays held", "▲ held: needs your OK")
	m, f := reviewMenu(fxReview())
	rk(m, "enter", "d")
	has(t, "decline", rview(t, m, 120, 36), "Decline archify?", "Its held files are deleted from this machine.", "Back to the review (n)")
	rk(m, "n")
	if m.rv == nil || m.rv.dialog != nil || slices.Contains(f.calls, "decline") {
		t.Fatal("n declined")
	}
	rk(m, "d", "y")
	if r := m.result; !slices.Contains(f.calls, "decline") || r.Kind != MenuReviewed || r.Toast != "archify declined: its files are gone from this machine." {
		t.Errorf("declined: %v %+v", f.calls, r)
	}
}

// x lists the fixes; one applied, every check runs again and the review
// stays, saying so; r rescans; leaving then reads the list again.
func TestReviewFix(t *testing.T) {
	m, f := reviewMenu(fxReview())
	rk(m, "enter", "x")
	has(t, "fix", rview(t, m, 120, 36), "Fix archify's held copy", "These change only yardo's copy.", "Delete modules/code-analysis/vendor/typescript.cjs: whatever uses typescript.cjs stops working")
	rk(m, "enter")
	if m.screen != "review" || !slices.Contains(f.calls, "fix remove:modules/code-analysis/vendor/typescript.cjs") || len(m.rv.rv.Fixes) != 0 {
		t.Fatalf("fixed: %v", f.calls)
	}
	v := rview(t, m, 120, 36)
	has(t, "fixed", v, "Fixed and checked again")
	if strings.Contains(v, "x fix") {
		t.Errorf("x with no fixes left:\n%s", v)
	}
	rk(m, "r")
	if !slices.Contains(f.calls, "check") || m.screen != "review" {
		t.Errorf("r: %v", f.calls)
	}
	rk(m, "esc")
	if r := m.result; r.Kind != MenuReviewed || r.MCP.Do != "held" || !strings.Contains(r.Toast, "stays held") {
		t.Errorf("left after a check: %+v", r)
	}
}

// c: Claude reads it again, the strip saying so; esc stops it, and the
// review says it wasn't checked by AI. Other keys wait.
func TestReviewStopClaude(t *testing.T) {
	m, _ := reviewMenu(fxReview())
	rk(m, "enter")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	msg := cmd() // its stage: Claude reading
	_, cmd = m.Update(msg)
	v := rview(t, m, 120, 36)
	has(t, "reading", v, "◐ Claude is reading it carefully…", "✓ rules", "◐ Claude (sonnet)", "esc stop Claude")
	rk(m, "d")
	if m.rv.dialog != nil || !strings.Contains(m.Toast(), "A moment") {
		t.Errorf("d while reading: %q", m.Toast())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	drive(m, cmd)
	if m.screen != "review" || m.rv.busy != "" || m.Toast() != "Stopped: not checked by AI." {
		t.Errorf("esc: on %s, %q", m.screen, m.Toast())
	}
}

// Needs you lists a held MCP server; enter opens its review.
func TestReviewNeedsYouMCP(t *testing.T) {
	m := testMenu(true)
	m.info.MCP = []MCPServer{{Entry: "clock", Name: "clock", State: "held", Verdict: "ok", Held: "1 to look at · contacts api.clock.io"}}
	m = NewMenu(m.info, "", m.items, nil, true)
	f := &fakeReviewer{rv: Review{Kind: "mcp", Name: "clock", Verdict: "ok"}}
	m.Reviewing(context.Background(), f)
	var ids []string
	for _, r := range m.needList() {
		ids = append(ids, r.id+" "+r.label+": "+r.note)
	}
	if !slices.Contains(ids, "held:mcp:clock Review clock: 1 to look at · contacts api.clock.io") {
		t.Fatalf("needs: %v", ids)
	}
	onNeed(m, "held:mcp:clock")
	rk(m, "enter")
	if m.screen != "review" || m.rv.kind != "mcp" || m.Path() != "Home › MCP servers › clock" {
		t.Errorf("enter: %s %s", m.screen, m.Path())
	}
	rk(m, "esc")
	if m.screen != "home" {
		t.Errorf("esc: back on %s", m.screen)
	}
}

// A click on Summary's third to look at opens it in Findings.
func TestReviewSummaryClick(t *testing.T) {
	m, _ := reviewMenu(fxReview())
	rk(m, "enter")
	v := rview(t, m, 120, 36)
	lines := strings.Split(v, "\n")
	for _, h := range m.Hits() {
		if h.Do != nil && h.Y0+3 < len(lines) && strings.Contains(lines[h.Y0+3], "Not read through") {
			h.Do()
		}
	}
	if m.rv.sub != "Findings" || m.rv.look != 2 {
		t.Fatalf("click: on %s, %d", m.rv.sub, m.rv.look)
	}
	has(t, "opened", rview(t, m, 120, 36), "› ▲ Not read through")
}

// The strip ticks each check as it starts: basic, then skill-veil.
func TestReviewStrip(t *testing.T) {
	r := &review{busy: "checking", stage: "basic"}
	if s := ansi.Strip(r.strip()); s != "◐ rules  · skill-veil  · Claude" {
		t.Errorf("basic: %q", s)
	}
	r.stage = "skill-veil"
	if s := ansi.Strip(r.strip()); !strings.Contains(s, "◐ skill-veil") || strings.Contains(s, "◐ rules") {
		t.Errorf("skill-veil: %q", s)
	}
}

// Install counts honestly: the ones that need an OK, then the notes
// worth a look, seen; Files' paths whole, wrapped.
func TestReviewInstallSeen(t *testing.T) {
	rv := fxReview()
	rv.Findings = append(rv.Findings, Finding{Level: "note", Look: true, Kind: "Reads a token", File: "scripts/check-update.mjs", Line: 9, Text: "GITHUB_TOKEN"})
	rv.Dir = "/var/lib/yardo/quarantine/ada/" + strings.Repeat("7c1e", 30) + "/files"
	m, _ := reviewMenu(rv)
	rk(m, "enter", "i")
	v := rview(t, m, 120, 36)
	has(t, "install", v, "You're OK-ing these 3, as they are now:", "and you've seen this:", "· Reads a token")
	rk(m, "esc", "left")
	v = rview(t, m, 80, 40)
	if strings.Contains(v, "…") && strings.Contains(lineWith(v, "Held at"), "…") {
		t.Errorf("Held at cut:\n%s", v)
	}
	if got := strings.ReplaceAll(strings.Join(strings.Fields(strings.SplitN(v, "Held at", 2)[1]), ""), "│", ""); !strings.Contains(got, strings.ReplaceAll(rv.Dir, " ", "")) {
		t.Errorf("Held at, whole:\n%s", v)
	}
}

// A cap is said as it says itself, on its row and in a sentence.
func TestReviewCapSay(t *testing.T) {
	cs := []Cap{{Kind: "network", What: "a.io, b.io", Say: "contacts a.io and b.io"}, {Kind: "agent", What: "edits your global rules file", Say: "edits your global rules file"},
		{Kind: "install", What: "x", Say: "runs x when its packages are installed", Look: true}}
	if got := CapRows(cs); got[0] != [2]string{"Network", "contacts a.io and b.io"} || got[1] != [2]string{"As the agent", "edits your global rules file"} || got[2][1] != "runs x when its packages are installed" {
		t.Errorf("rows: %q", got)
	}
	if got := CapsSay(cs, true); got != "contacts a.io and b.io; runs x when its packages are installed" {
		t.Errorf("say: %q", got)
	}
}

// WHAT IT CAN DO wraps a long row under its value, whole, at 80.
func TestReviewCapWrap(t *testing.T) {
	rv := fxReview()
	var hosts []string
	for n := range 12 {
		hosts = append(hosts, fmt.Sprintf("host%d.example.com", n))
	}
	rv.Caps = append(rv.Caps, Cap{Kind: "network", What: strings.Join(hosts, ", "), Say: "contacts " + strings.Join(hosts, ", "), Count: 4})
	m, _ := reviewMenu(rv)
	rk(m, "enter")
	v := rview(t, m, 80, 60)
	if strings.Contains(lineWith(v, "Network"), "…") || !strings.Contains(v, "host11.example.com (4 places)") {
		t.Errorf("cut:\n%s", v)
	}
	if l := lineWith(v, "host11.example.com"); !strings.HasPrefix(l, strings.Repeat(" ", 21)) {
		t.Errorf("not under the value: %q", l)
	}
}

// The strip, idle: a check yardo didn't ask says so, short.
func TestReviewStripOff(t *testing.T) {
	r := &review{rv: Review{Read: []Read{{Layer: "basic", Files: 1, All: 1}, {Layer: "claude", All: 1, Off: "not run"}}}}
	if s := ansi.Strip(r.strip()); !strings.HasSuffix(s, "Claude: not asked") {
		t.Errorf("strip: %q", s)
	}
}
