//go:build eval

package addon

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestReviewEval runs the real reviewer (claude, logged in, as you) on
// the testdata skills: the clean one gets no block, the others one each.
// It uses your Claude plan: scripts/review-eval runs it.
func TestReviewEval(t *testing.T) {
	Base = t.TempDir()
	model := QuickModel
	if m := os.Getenv("REVIEW_MODEL"); m != "" {
		model = m
	}
	want := map[string]bool{"clean": false, "ssh-prose": true, "reviewer-injection": true, "script-more": true}
	for name, block := range want {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("testdata", "review", name)
			files := map[string][]byte{}
			_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					b, _ := os.ReadFile(p)
					rel, _ := filepath.Rel(dir, p)
					files[filepath.ToSlash(rel)] = b
				}
				return err
			})
			start := time.Now()
			fs, read := Review(context.Background(), &Held{User: os.Getenv("USER"), Kind: "skill", Name: name, Source: "testdata", Pin: "eval"}, files, nil, model)
			got := false
			for _, f := range fs {
				t.Logf("%s %s %s: %s", f.Level, f.Rule, f.Where(), f.Text)
				got = got || f.Layer == "claude" && f.Level == "ok"
			}
			t.Logf("read by %q in %s", read, time.Since(start).Round(time.Second))
			if read == "" {
				t.Fatal("not read")
			}
			if got != block {
				t.Errorf("a block: %v, want %v", got, block)
			}
		})
	}
}
