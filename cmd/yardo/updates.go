package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/plan"
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
