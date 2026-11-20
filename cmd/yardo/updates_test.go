package main

import (
	"strings"
	"testing"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/steps"
)

// A check holds for updatesTTL: read back while younger, nil after (or
// with none saved, or one from the future).
func TestLoadUpdates(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	if loadUpdates(dir, "ada", now) != nil {
		t.Fatal("a check with none saved")
	}
	u := (&updateCheck{At: now.Add(-2 * time.Hour), ByID: map[string][]string{"go": {"go 1.25.1 → 1.25.2"}}}).finished()
	if err := u.save(dir, "ada"); err != nil {
		t.Fatal(err)
	}
	got := loadUpdates(dir, "ada", now)
	if !got.ready() || got.ByID["go"][0] != "go 1.25.1 → 1.25.2" {
		t.Fatalf("read back: %+v", got)
	}
	if loadUpdates(dir, "ada", now.Add(updatesTTL)) != nil {
		t.Error("a check older than the TTL")
	}
	if loadUpdates(dir, "ada", now.Add(-3*time.Hour)) != nil {
		t.Error("a check from the future")
	}
	if msg := got.msg(); msg.At == "" || len(msg.ByID) != 1 {
		t.Errorf("msg %+v", msg)
	}
	var none *updateCheck
	if none.ready() {
		t.Error("no check is ready")
	}
	none.wait(nil) // no check: nothing to wait for
}

func TestClock(t *testing.T) {
	now := time.Date(2026, 11, 6, 18, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, ""},
		{time.Date(2026, 11, 6, 9, 5, 0, 0, time.UTC), "09:05"},
		{time.Date(2026, 11, 5, 9, 5, 0, 0, time.UTC), "Nov 5 09:05"},
	} {
		if got := clock(c.t, now); got != c.want {
			t.Errorf("clock(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

// The IDs queued split into their mise tools and apt packages (mise:
// IDs as tools); what a lock pins stays.
func TestOutdatedFor(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	env := &steps.Env{User: "ada", Home: "/home/ada"}
	mise := map[string]string{"terraform": "1.9 → 1.10", "tflint": "0.5 → 0.6", "uv": "0.4 → 0.5", "glow": "2.0 → 2.1"}
	apt := map[string]string{"docker-ce": "27.1 → 27.2", "tmux": "3.4-1 → 3.4-2"}
	tools, pkgs := outdatedFor(cat, env, []string{"terraform", "docker", "mise:uv"}, mise, apt)
	if strings.Join(tools, " ") != "terraform tflint uv" || strings.Join(pkgs, " ") != "docker-ce" {
		t.Errorf("tools %v pkgs %v", tools, pkgs)
	}
	env.Lock = &answers.Lock{Mise: map[string]string{"terraform": "1.9"}, Apt: map[string]string{"docker-ce": "27.1"}}
	tools, pkgs = outdatedFor(cat, env, []string{"terraform", "docker"}, mise, apt)
	if strings.Join(tools, " ") != "tflint uv" || len(pkgs) != 0 {
		t.Errorf("pinned: tools %v pkgs %v", tools, pkgs)
	}
	if !pinned(env.Lock, []string{"terraform 1.9 → 1.10"}) || pinned(env.Lock, []string{"terraform 1.9 → 1.10", "tflint 0.5 → 0.6"}) || pinned(nil, []string{"x 1 → 2"}) {
		t.Error("pinned")
	}
}

// After an update, only what was updated leaves the saved check.
func TestUpdatesDrop(t *testing.T) {
	state := t.TempDir()
	u := (&updateCheck{At: time.Now(), ByID: map[string][]string{"node": {"node 22 → 24"}, "gh": {"gh 2.60 → 2.62"}, "uv": {"uv 0.4 → 0.5"}}}).finished()
	if err := u.save(state, "ada"); err != nil {
		t.Fatal(err)
	}
	if err := u.drop([]string{"gh"}, state, "ada"); err != nil {
		t.Fatal(err)
	}
	got := loadUpdates(state, "ada", time.Now())
	if got == nil || len(got.ByID) != 2 || got.ByID["gh"] != nil || got.ByID["node"] == nil {
		t.Errorf("left %v", got)
	}
}
