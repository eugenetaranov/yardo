package ui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

// LastRun is how the last run went, kept for its screen: when, whether
// it worked, each step's result line (a failed one's last output lines
// too), the notes and hints it printed after, and the log's path.
type LastRun struct {
	When   string
	OK     bool
	Lines  []RunLine
	Notes  []string
	Hints  []string
	Failed []string
	Log    string
}

// RunLine is one step's result: Status (ok, done, fail, skip), its Text,
// and for a failure the Tail of what it printed.
type RunLine struct {
	Status, Text string
	Tail         []string
}

// say is how the run went, in a few words.
func (l *LastRun) say() string {
	if l.OK {
		return "✓ it worked"
	}
	return "✗ " + plural(len(l.Failed), "step", "steps") + " failed: " + strings.Join(l.Failed, ", ")
}

// AddKey is a public key a run made, to add where Where says (a line per
// host it's for); Path is its file.
type AddKey struct {
	Pub, Path string
	Where     []string
}

// For is what the key is for: its first host.
func (k AddKey) For() string {
	if len(k.Where) == 0 {
		return "new"
	}
	h, _, _ := strings.Cut(k.Where[0], ":")
	return h
}

// lastRunLines are the Last run screen's lines, w wide.
func (m *Menu) lastRunLines(w int) []string {
	l := m.info.LastRun
	if l == nil {
		return nil
	}
	out := []string{"  " + bold.Render(l.When) + "  " + map[bool]string{true: green.Render(l.say()), false: yellow.Render(l.say())}[l.OK], ""}
	for _, r := range l.Lines {
		g, st := "✓", green
		switch r.Status {
		case "fail":
			g, st = "✗", red
		case "skip":
			g, st = "–", muted
		case "done":
			g, st = "·", muted
		}
		out = append(out, "  "+st.Render(g)+" "+runewidth.Truncate(r.Text, max(10, w-6), "…"))
		for _, t := range r.Tail {
			out = append(out, "      "+muted.Render(runewidth.Truncate(t, max(10, w-8), "…")))
		}
	}
	section := func(title string, ls []string) {
		if len(ls) == 0 {
			return
		}
		out = append(out, "", "  "+bold.Render(title))
		for _, x := range ls {
			for _, wl := range wrap(Unmark(x), max(10, w-6)) {
				out = append(out, "    "+wl)
			}
		}
	}
	section("Notes", l.Notes)
	section("Hints", l.Hints)
	if l.Log != "" {
		out = append(out, "", "  "+muted.Render(fmt.Sprintf("Everything it ran, with its output: %s", l.Log)))
	}
	return out
}
