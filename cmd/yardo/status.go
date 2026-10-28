package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/plan"
	"github.com/eugenetaranov/yardo/internal/run"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// statusSchema versions `yardo status --json`; pmox and scripts read it.
const statusSchema = 1

// statusReport is `yardo status --json`: every user's items.
type statusReport struct {
	Schema    int          `json:"schema"`
	Host      string       `json:"host"`
	CheckedAt string       `json:"checked_at"`
	Users     []userStatus `json:"users"`
}

// userStatus is one user's items, problems first.
type userStatus struct {
	User   string         `json:"user"`
	Counts map[string]int `json:"counts"`
	Items  []itemStatus   `json:"items"`
}

type itemStatus struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Tab    string `json:"tab"`
	Scope  string `json:"scope"` // system or user
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
	// Inputs are the values left empty ("file: KEY").
	Inputs []string `json:"inputs,omitempty"`
}

// stateOrder sorts problems first: what to fix, then what to fill in.
var stateOrder = map[string]int{"down": 0, "partial": 1, "missing": 2, "secret": 3, "unchecked": 4, "ok": 5}

// userStatusOf checks user's items (and extra mise tools) without changing
// anything.
func userStatusOf(ctx context.Context, env *steps.Env, user string, items []*catalog.Item, extra []string) userStatus {
	us := userStatus{User: user, Counts: map[string]int{}}
	for _, s := range run.Verify(ctx, env, items) {
		if s.Item.Hidden() {
			continue
		}
		scope := "user"
		if s.Item.System() {
			scope = "system"
		}
		us.Items = append(us.Items, itemStatus{ID: s.Item.ID, Label: s.Item.Label, Tab: s.Item.Tab, Scope: scope,
			State: s.State, Detail: s.Detail, Done: s.Done, Total: s.Total, Inputs: s.Inputs})
	}
	for _, t := range extra {
		st := itemStatus{ID: "mise:" + steps.MiseName(t), Label: t + " (extra mise tool)", Tab: "extra", Scope: "user", State: "ok", Total: 1, Done: 1}
		if ok, _ := steps.MiseHas(ctx, env, []string{t}); !ok {
			st.State, st.Done, st.Detail = "missing", 0, "mise use -g "+t
		}
		us.Items = append(us.Items, st)
	}
	sort.SliceStable(us.Items, func(i, j int) bool { return stateOrder[us.Items[i].State] < stateOrder[us.Items[j].State] })
	for _, it := range us.Items {
		us.Counts[it.State]++
	}
	return us
}

// printStatus writes one user's status for people: a summary line, then
// each item, problems first.
func printStatus(w io.Writer, us userStatus) {
	var sum []string
	for _, s := range []struct{ state, say string }{
		{"ok", "ok"}, {"down", "down"}, {"partial", "partial"}, {"missing", "missing"},
		{"secret", "with values to fill in"}, {"unchecked", "not checked"},
	} {
		if n := us.Counts[s.state]; n > 0 {
			sum = append(sum, fmt.Sprintf("%d %s", n, s.say))
		}
	}
	fmt.Fprintf(w, "%s: %s\n", us.User, strings.Join(sum, " · "))
	for _, it := range us.Items {
		line := fmt.Sprintf("  %-9s %s", it.State, it.Label)
		switch {
		case it.State == "partial":
			line += fmt.Sprintf(" (%d of %d steps): %s", it.Done, it.Total, it.Detail)
		case it.State == "secret":
			line += ": " + strings.Join(it.Inputs, ", ")
		case it.Detail != "" && it.State != "ok":
			line += ": " + it.Detail
		}
		fmt.Fprintln(w, line)
	}
}

// userStatusCache is where the user's own copy of their status is, for
// `yardo status` without sudo.
const userStatusCache = ".cache/yardo/status.json"

// saveStatus keeps the user's latest status in the state directory
// (status/<user>.json), for a dashboard to show at once, and a copy in the
// user's ~/.cache/yardo, written as them (so a symlink there leads
// nowhere they couldn't write themselves).
func saveStatus(ctx context.Context, env *steps.Env, state string, us userStatus, now time.Time) error {
	b, err := json.MarshalIndent(struct {
		Schema    int    `json:"schema"`
		CheckedAt string `json:"checked_at"`
		userStatus
	}{statusSchema, now.UTC().Format(time.RFC3339), us}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if env != nil {
		_, _ = env.Exec.Run(ctx, steps.Cmd{Name: "sh", Args: []string{"-c",
			`umask 077 && mkdir -p "$HOME/.cache/yardo" && cat > "$HOME/` + userStatusCache + `.tmp" && mv -f "$HOME/` + userStatusCache + `.tmp" "$HOME/` + userStatusCache + `"`},
			Stdin: b, AsUser: true})
	}
	return saveStatusFile(state, us.User, b)
}

func saveStatusFile(state, user string, b []byte) error {
	dir := filepath.Join(state, "status")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p := filepath.Join(dir, user+".json")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// writeStatusJSON writes the report (`yardo status --json`).
func writeStatusJSON(w io.Writer, users []userStatus, now time.Time) error {
	host, _ := os.Hostname()
	if users == nil {
		users = []userStatus{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(statusReport{Schema: statusSchema, Host: host, CheckedAt: now.UTC().Format(time.RFC3339), Users: users})
}

// home checks the user's items and shows the home screen.
func (o *options) home(ctx context.Context, cat *catalog.Catalog, a *answers.Answers, stdout io.Writer) (ui.HomeAction, error) {
	fmt.Fprintf(stdout, "Checking what's set up for %s…\n", a.User)
	items, err := plan.Resolve(cat, a.Picks, len(a.MiseExtra) > 0)
	if err != nil {
		return ui.HomeQuit, err
	}
	env, err := o.env(a, nil, nil)
	if err != nil {
		return ui.HomeQuit, err
	}
	now := time.Now()
	us := userStatusOf(ctx, env, a.User, items, a.MiseExtra)
	_ = saveStatus(ctx, env, o.state, us, now)
	// Where a fine item is listed: its tab's group (Dev, DevOps) or tab,
	// and every configure tab under Configure.
	group := map[string]string{}
	var groups []string
	add := func(g string) {
		if !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	for _, t := range cat.Tabs {
		g := t.Title
		switch {
		case t.PhaseOf() == catalog.PhaseConfigure:
			g = "Configure"
		case t.Group != "":
			g = t.Group
		}
		group[t.ID] = g
		add(g)
	}
	add("Extra mise tools")
	group["extra"] = "Extra mise tools"
	var hi []*ui.HomeItem
	for _, it := range us.Items {
		src := ""
		if c := cat.Item(it.ID); c != nil {
			src = c.Describe()
		}
		hi = append(hi, &ui.HomeItem{ID: it.ID, Label: it.Label, Group: group[it.Tab], State: it.State, Detail: it.Detail, Inputs: it.Inputs, Source: src})
	}
	avail := 0
	for _, it := range cat.Items {
		if !it.Hidden() && !it.Always && !slices.Contains(a.Picks, it.ID) {
			avail++
		}
	}
	host, _ := os.Hostname()
	return ui.NewHome("yardo · "+host+" · "+a.User, "checked "+now.Format("15:04"), hi, groups, avail).Run()
}

// cachedStatus is `yardo status` without sudo: the copy the last check
// left in ~/.cache/yardo, and how old it is.
func cachedStatus(args []string, w io.Writer, now time.Time) error {
	asJSON := slices.Contains(args, "--json") || slices.Contains(args, "-json")
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(home, userStatusCache))
	if err != nil {
		return errors.New("no status checked yet: run 'sudo yardo status'")
	}
	if asJSON {
		_, err := w.Write(b)
		return err
	}
	var s struct {
		CheckedAt string `json:"checked_at"`
		userStatus
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("%s: %w", userStatusCache, err)
	}
	printStatus(w, s.userStatus)
	if t, err := time.Parse(time.RFC3339, s.CheckedAt); err == nil {
		fmt.Fprintf(w, "\nChecked %s; 'sudo yardo status' checks again.\n", ago(now.Sub(t)))
	}
	return nil
}

// ago says how long ago, roughly: "just now", "12m ago", "2h ago", "3d ago".
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
