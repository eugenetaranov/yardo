package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/run"
	"github.com/eugenetaranov/yardo/internal/steps"
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

// saveStatus keeps the user's latest status in the state directory
// (status/<user>.json), for a dashboard to show at once.
func saveStatus(state string, us userStatus, now time.Time) error {
	dir := filepath.Join(state, "status")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(struct {
		Schema    int    `json:"schema"`
		CheckedAt string `json:"checked_at"`
		userStatus
	}{statusSchema, now.UTC().Format(time.RFC3339), us}, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(dir, us.User+".json")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
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
