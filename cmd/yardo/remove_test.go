package main

import (
	"bytes"
	"context"
	"errors"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/steps"
)

// removeExec answers removal's commands: every package installed, mise
// has aws-cli; fail makes a command fail.
type removeExec struct {
	ran  []string
	fail func(c steps.Cmd) bool
}

func (x *removeExec) Run(_ context.Context, c steps.Cmd) ([]byte, error) {
	x.ran = append(x.ran, c.String())
	if x.fail != nil && x.fail(c) {
		return nil, errors.New("failed")
	}
	if c.Name == "mise" && len(c.Args) > 0 && c.Args[0] == "ls" {
		return []byte(`{"aws-cli":[{"version":"2"}]}`), nil
	}
	return nil, nil
}

func (x *removeExec) changed() []string {
	var out []string
	for _, c := range x.ran {
		if strings.Contains(c, "purge") || strings.HasPrefix(c, "bash -c") || strings.Contains(c, "unuse") {
			out = append(out, c)
		}
	}
	return out
}

func removeSetup(t *testing.T, picks ...string) (*options, *catalog.Catalog, *answers.Store, *removeExec) {
	t.Helper()
	me, _ := user.Current()
	state := t.TempDir()
	if err := answers.Write(state, &answers.Answers{User: me.Username, Picks: picks}); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	x := &removeExec{}
	o := &options{state: state, log: filepath.Join(state, "log"), user: me.Username, exec: x, root: t.TempDir()}
	return o, cat, o.store(cat), x
}

// yardo remove --dry-run says what each item takes off and changes
// nothing; a login's name means its client.
func TestRemoveDryRun(t *testing.T) {
	o, cat, st, x := removeSetup(t, "aws", "netbird-install", "netbird")
	var out, errb bytes.Buffer
	if err := o.removeCmd(cat, st, []string{"aws", "NetBird", "--dry-run"}, &out, &errb); err != nil {
		t.Fatalf("%v\n%s", err, errb.String())
	}
	for _, want := range []string{"✗ Uninstall AWS tools", "your ~/.aws (profiles, keys) stays", "Takes off: ", "aws-cli",
		"✗ Uninstall NetBird", "the peer stays in your NetBird dashboard"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("lacks %q:\n%s", want, out.String())
		}
	}
	if c := x.changed(); len(c) > 0 {
		t.Errorf("a dry run ran %q", c)
	}
}

// One yardo won't remove (or isn't installed): exit 2, saying why, and
// nothing removed, not even the others.
func TestRemoveRefuses(t *testing.T) {
	o, cat, st, x := removeSetup(t, "aws", "gitid")
	var out, errb bytes.Buffer
	err := o.removeCmd(cat, st, []string{"gitid", "aws", "terraform", "--yes"}, &out, &errb)
	var e *exitErr
	if !errors.As(err, &e) || e.code != 2 || !strings.Contains(err.Error(), "~/.gitconfig") || !strings.Contains(err.Error(), "terraform isn't installed") {
		t.Fatalf("err %v", err)
	}
	if c := x.changed(); len(c) > 0 {
		t.Errorf("ran %q", c)
	}
}

// One fails, the other still goes: the failed one stays picked, exit 1.
func TestRemoveOneFails(t *testing.T) {
	o, cat, st, x := removeSetup(t, "aws", "netbird-install", "netbird")
	x.fail = func(c steps.Cmd) bool {
		return c.Name == "bash" && strings.Contains(strings.Join(c.Args, " "), "netbird down")
	}
	var out, errb bytes.Buffer
	err := o.removeCmd(cat, st, []string{"netbird", "aws", "--yes"}, &out, &errb)
	var e *exitErr
	if !errors.As(err, &e) || e.code != 1 {
		t.Fatalf("err %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "✗ Uninstalling NetBird failed") || !strings.Contains(out.String(), "✓ Uninstalled AWS tools.") {
		t.Errorf("said:\n%s", out.String())
	}
	a, _, err := st.Load(o.user)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.Picks, []string{"netbird-install", "netbird"}) {
		t.Errorf("picks %q", a.Picks)
	}
	if !slices.ContainsFunc(x.ran, func(c string) bool { return strings.Contains(c, "mise unuse -g aws-cli") }) {
		t.Errorf("aws-cli not taken off:\n%s", strings.Join(x.ran, "\n"))
	}
}

// Uninstalling an item stops managing what needs it; the board's plan
// says so, and that an add queued with it installs it again.
func TestPlanLines(t *testing.T) {
	o, cat, _, _ := removeSetup(t, "netbird-install", "netbird")
	a := &answers.Answers{User: o.user, Picks: []string{"netbird-install", "netbird"}}
	ls := strings.Join(o.planLines(context.Background(), cat, a, []string{"netbird-install"}, []string{"netbird"})["netbird-install"], "\n")
	for _, want := range []string{"It disconnects from NetBird", "NetBird login need it and will stop working", "↺ NetBird login, queued to install, needs it"} {
		if !strings.Contains(ls, want) {
			t.Errorf("lacks %q:\n%s", want, ls)
		}
	}
}
