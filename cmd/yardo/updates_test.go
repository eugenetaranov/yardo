package main

import (
	"testing"
	"time"
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
