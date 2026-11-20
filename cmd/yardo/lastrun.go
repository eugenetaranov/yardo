package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/run"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// lastRun is how a run went, kept for the home's Last run: when, whether
// it worked, each step's result line (a failure's last output lines with
// it), the notes and hints printed after, and what failed.
type lastRun struct {
	When   time.Time  `json:"when"`
	OK     bool       `json:"ok"`
	Lines  []run.Line `json:"lines"`
	Notes  []string   `json:"notes,omitempty"`
	Hints  []string   `json:"hints,omitempty"`
	Failed []string   `json:"failed,omitempty"`
}

// lastRunFile is where user's last run is kept: <state>/<user>/.lastrun.
func lastRunFile(state, user string) string { return filepath.Join(state, user, ".lastrun") }

func (l *lastRun) save(state, user string) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lastRunFile(state, user)), 0o700); err != nil {
		return err
	}
	return os.WriteFile(lastRunFile(state, user), append(b, '\n'), 0o600)
}

// failTails gives each failed step's line in lines the tail of what it
// printed (res.Errors, by item): the line named by the item's label.
func failTails(cat *catalog.Catalog, lines []run.Line, res *run.Result) []run.Line {
	for _, id := range res.Failed {
		msg := res.Errors[id]
		_, tail, _ := strings.Cut(msg, "\n")
		if tail == "" {
			continue
		}
		name := strings.TrimPrefix(id, "mise:")
		if it := cat.Item(id); it != nil {
			name = it.Label
		}
		for n := range lines {
			if lines[n].Status == "fail" && strings.Contains(lines[n].Text, name) && len(lines[n].Tail) == 0 {
				lines[n].Tail = strings.Split(strings.TrimRight(tail, "\n"), "\n")
				break
			}
		}
	}
	return lines
}

// loadLastRun is user's last run, for the home (nil: none yet).
func loadLastRun(state, user, log string, now time.Time) *ui.LastRun {
	b, err := os.ReadFile(lastRunFile(state, user))
	if err != nil {
		return nil
	}
	var l lastRun
	if json.Unmarshal(b, &l) != nil {
		return nil
	}
	out := &ui.LastRun{When: when(l.When, now), OK: l.OK, Notes: l.Notes, Hints: l.Hints, Failed: l.Failed, Log: log}
	for _, x := range l.Lines {
		out.Lines = append(out.Lines, ui.RunLine{Status: x.Status, Text: x.Text, Tail: x.Tail})
	}
	return out
}

// when is t for Last run: today at 14:05, yesterday at 09:12, or the day.
func when(t, now time.Time) string {
	y, m, d := t.Date()
	switch ny, nm, nd := now.Date(); {
	case y == ny && m == nm && d == nd:
		return "today at " + t.Format("15:04")
	case now.Sub(t) < 48*time.Hour && now.AddDate(0, 0, -1).Day() == d:
		return "yesterday at " + t.Format("15:04")
	}
	return t.Format("Mon 2 Jan, 15:04")
}

// addKey is one line of the keys file: a public key a run made, for host,
// to add where where says.
type addKey struct{ host, where, path, pub string }

// readKeys are the keys file's keys, one per public key (its hosts
// together), in the order they came.
func readKeys(state string) []ui.AddKey {
	var out []ui.AddKey
	for _, k := range keyLines(state) {
		i := slices.IndexFunc(out, func(x ui.AddKey) bool { return x.Pub == k.pub })
		if i < 0 {
			out = append(out, ui.AddKey{Pub: k.pub, Path: k.path})
			i = len(out) - 1
		}
		if w := k.host + ": " + k.where; !slices.Contains(out[i].Where, w) {
			out[i].Where = append(out[i].Where, w)
		}
	}
	return out
}

// keyLines are the keys file's lines.
func keyLines(state string) []addKey {
	b, err := os.ReadFile(filepath.Join(state, keysFile))
	if err != nil {
		return nil
	}
	var out []addKey
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.SplitN(l, "\t", 4)
		if len(f) != 4 || f[3] == "" {
			continue
		}
		out = append(out, addKey{f[0], f[1], f[2], f[3]})
	}
	return out
}

// dropKey takes pub (each of its lines) off the keys file: added, or
// dismissed.
func dropKey(state, pub string) error {
	var keep []string
	for _, k := range keyLines(state) {
		if k.pub != pub {
			keep = append(keep, strings.Join([]string{k.host, k.where, k.path, k.pub}, "\t"))
		}
	}
	path := filepath.Join(state, keysFile)
	if len(keep) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, []byte(strings.Join(keep, "\n")+"\n"), 0o600)
}

// hostItem is the git host item a key's host is for (github, gitlab,
// bitbucket), or "".
func hostItem(a *answers.Answers, host string) string {
	switch host {
	case "github.com":
		return "github"
	case "bitbucket.org":
		return "bitbucket"
	}
	if gl := a.GitLab.Host; host == "gitlab.com" || gl != "" && strings.HasPrefix(host, gl) {
		return "gitlab"
	}
	return ""
}

// keysToAdd are the keys on Connection: each a run made, but those whose
// git hosts all read working (dropped from the file then).
func keysToAdd(state string, a *answers.Answers, working func(id string) bool) []ui.AddKey {
	var out []ui.AddKey
	for _, k := range readKeys(state) {
		added := len(k.Where) > 0
		for _, w := range k.Where {
			host, _, _ := strings.Cut(w, ": ")
			if id := hostItem(a, host); id == "" || !working(id) {
				added = false
			}
		}
		if added {
			_ = dropKey(state, k.Pub)
			continue
		}
		out = append(out, k)
	}
	return out
}

// appData is what the app shows of d.
func (d *screen) appData() ui.AppData {
	return ui.AppData{Info: d.info, Sub: d.sub, Items: d.items, Settings: d.settings, Conn: d.conn, Checked: d.updates, Tabs: d.tabs, First: d.first}
}
