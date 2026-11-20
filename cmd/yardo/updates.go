package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/plan"
	"github.com/eugenetaranov/yardo/internal/run"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// updatesTTL is how long a check for newer versions holds: once it's
// older, the menu checks again, in the background.
const updatesTTL = 24 * time.Hour

// updateCheck is a check for newer versions (apt and mise) of a user's
// items: run in the background while the menu is up, or read back from
// the last one (status/<user>.updates.json).
type updateCheck struct {
	done chan struct{}
	// At is when it was done; ByID each board item's newer versions
	// ("terraform 1.9.0 → 1.10.1"). Read them once done is closed.
	At   time.Time           `json:"checked_at"`
	ByID map[string][]string `json:"updates"`
}

func updatesFile(state, user string) string {
	return filepath.Join(state, "status", user+".updates.json")
}

// loadUpdates is the last check for user, when it's younger than
// updatesTTL at now; nil otherwise.
func loadUpdates(state, user string, now time.Time) *updateCheck {
	b, err := os.ReadFile(updatesFile(state, user))
	if err != nil {
		return nil
	}
	u := &updateCheck{}
	if json.Unmarshal(b, u) != nil || now.Sub(u.At) > updatesTTL || u.At.After(now) {
		return nil
	}
	return u.finished()
}

// finished closes done: a check read back, or one with nothing to check.
func (u *updateCheck) finished() *updateCheck {
	u.done = make(chan struct{})
	close(u.done)
	return u
}

func (u *updateCheck) save(state, user string) error {
	b, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(updatesFile(state, user)), 0o700); err != nil {
		return err
	}
	return os.WriteFile(updatesFile(state, user), append(b, '\n'), 0o600)
}

// ready reports whether the check is done (false for none).
func (u *updateCheck) ready() bool {
	if u == nil {
		return false
	}
	select {
	case <-u.done:
		return true
	default:
		return false
	}
}

// wait waits for a check still running, saying so on w: what runs next
// may use apt too.
func (u *updateCheck) wait(w io.Writer) {
	if u != nil && !u.ready() {
		fmt.Fprintln(w, "Finishing the check for updates (apt and mise)…")
		<-u.done
	}
}

// msg waits for the check, then is what the menu is told.
func (u *updateCheck) msg() ui.UpdatesMsg {
	<-u.done
	return ui.UpdatesMsg{ByID: u.ByID, At: clock(u.At, time.Now())}
}

// clock is t for the menu: its time today, else its day too.
func clock(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return t.Format("15:04")
	}
	return t.Format("Jan 2 15:04")
}

// checkUpdates starts a check for newer versions of a's items, in the
// background; it saves what it finds for the next run.
func (o *options) checkUpdates(ctx context.Context, cat *catalog.Catalog, a *answers.Answers) *updateCheck {
	u := &updateCheck{done: make(chan struct{})}
	items, err := plan.Resolve(cat, a.Picks, len(a.MiseExtra) > 0)
	// Its own copy of the answers: the menu may change them meanwhile.
	ac := *a
	env, err2 := o.env(&ac, nil, nil)
	if err != nil || err2 != nil {
		u.At = time.Now()
		close(u.done)
		return u
	}
	extra, user := slices.Clone(a.MiseExtra), a.User
	go func() {
		defer close(u.done)
		mise, apt := steps.Outdated(ctx, env)
		by := map[string][]string{}
		for _, it := range items {
			if it.Hidden() {
				continue
			}
			if up := steps.ItemUpdates(env, it.Steps, mise, apt); len(up) > 0 {
				by[it.ID] = up
			}
		}
		for _, t := range extra {
			if v := mise[steps.MiseName(t)]; v != "" {
				by["mise:"+t] = []string{steps.MiseName(t) + " " + v}
			}
		}
		u.ByID, u.At = by, time.Now()
		_ = u.save(o.state, user)
	}()
	return u
}

// drop takes ids, updated, off the check, and saves it: the rest stay
// listed until the next check.
func (u *updateCheck) drop(ids []string, state, user string) error {
	if u == nil || u.ByID == nil {
		return nil
	}
	for _, id := range ids {
		delete(u.ByID, id)
	}
	return u.save(state, user)
}

// outdatedFor are the mise tools and apt packages of ids (items, and
// mise:<tool> for extra tools) that mise and apt have newer versions of;
// what the env's lock pins stays.
func outdatedFor(cat *catalog.Catalog, env *steps.Env, ids []string, mise, apt map[string]string) (tools, pkgs []string) {
	for _, id := range ids {
		if t, ok := strings.CutPrefix(id, "mise:"); ok {
			if n := steps.MiseName(t); mise[n] != "" {
				tools = append(tools, n)
			}
			continue
		}
		if it := cat.Item(id); it != nil {
			t, p := steps.ItemOutdated(env, it.Steps, mise, apt)
			tools, pkgs = append(tools, t...), append(pkgs, p...)
		}
	}
	if l := env.Lock; l != nil {
		tools = slices.DeleteFunc(tools, func(t string) bool { return l.Mise[t] != "" })
		pkgs = slices.DeleteFunc(pkgs, func(p string) bool { return l.Apt[p] != "" })
	}
	return slices.Compact(slices.Sorted(slices.Values(tools))), slices.Compact(slices.Sorted(slices.Values(pkgs)))
}

// updateStep updates ids to their newest versions, as one counted step
// of r's run (looking again for what's newer first).
func (o *options) updateStep(ctx context.Context, r *run.Runner, env *steps.Env, cat *catalog.Catalog, ids []string) error {
	title := "Updating " + strings.Join(updateNames(cat, ids), ", ")
	r.Step(title)
	mise, apt := steps.Outdated(ctx, env)
	tools, pkgs := outdatedFor(cat, env, ids, mise, apt)
	if len(tools)+len(pkgs) == 0 {
		r.Progress.Done("ok", "Nothing to update: already current")
		return nil
	}
	if err := steps.Upgrade(ctx, env, tools, pkgs); err != nil {
		r.Progress.Done("fail", title)
		return err
	}
	r.Progress.Done("ok", "Updated "+strings.Join(append(tools, pkgs...), ", "))
	return nil
}

// updateNames are ids as the screens name them.
func updateNames(cat *catalog.Catalog, ids []string) []string {
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

// pinned reports whether every tool in an item's update lines ("node
// 22.12 → 24.11") is pinned by lock.
func pinned(lock *answers.Lock, updates []string) bool {
	if lock.Empty() || len(updates) == 0 {
		return false
	}
	for _, u := range updates {
		n, _, _ := strings.Cut(u, " ")
		if lock.Mise[n] == "" && lock.Apt[n] == "" {
			return false
		}
	}
	return true
}
