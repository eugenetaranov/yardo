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
// nothing; netbird is NetBird, whose login goes with it.
func TestRemoveDryRun(t *testing.T) {
	o, cat, st, x := removeSetup(t, "aws", "netbird-install", "netbird")
	var out, errb bytes.Buffer
	if err := o.removeCmd(cat, st, []string{"aws", "NetBird", "--dry-run"}, &out, &errb); err != nil {
		t.Fatalf("%v\n%s", err, errb.String())
	}
	for _, want := range []string{"Uninstall AWS tools", "your ~/.aws (profiles, keys) stays", "Goes: ", "aws-cli",
		"Uninstall NetBird", "the peer stays in your NetBird dashboard", "Also uninstalls NetBird login: they need it."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("lacks %q:\n%s", want, out.String())
		}
	}
	if c := x.changed(); len(c) > 0 {
		t.Errorf("a dry run ran %q", c)
	}
}

// One yardo can't uninstall (or isn't installed): exit 2, saying why,
// and nothing removed, not even the others.
func TestRemoveRefuses(t *testing.T) {
	o, cat, st, x := removeSetup(t, "aws", "github")
	var out, errb bytes.Buffer
	err := o.removeCmd(cat, st, []string{"github", "aws", "terraform", "--yes"}, &out, &errb)
	var e *exitErr
	if !errors.As(err, &e) || e.code != 2 || !strings.Contains(err.Error(), "gh auth logout") || !strings.Contains(err.Error(), "terraform isn't installed") {
		t.Fatalf("err %v", err)
	}
	if c := x.changed(); len(c) > 0 {
		t.Errorf("ran %q", c)
	}
}

// One fails, the others still go: the failed one stays picked, exit 1.
// The login goes first: it needs NetBird.
func TestRemoveOneFails(t *testing.T) {
	o, cat, st, x := removeSetup(t, "aws", "netbird-install", "netbird")
	x.fail = func(c steps.Cmd) bool {
		return c.Name == "bash" && strings.Contains(strings.Join(c.Args, " "), "netbird service uninstall")
	}
	var out, errb bytes.Buffer
	err := o.removeCmd(cat, st, []string{"netbird", "aws", "--yes"}, &out, &errb)
	var e *exitErr
	if !errors.As(err, &e) || e.code != 1 {
		t.Fatalf("err %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "✗ NetBird: the uninstall stopped") || !strings.Contains(out.String(), "✓ AWS tools uninstalled.") ||
		!strings.Contains(out.String(), "✓ NetBird login uninstalled.") {
		t.Errorf("said:\n%s", out.String())
	}
	a, _, err := st.Load(o.user)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.Picks, []string{"netbird-install"}) {
		t.Errorf("picks %q", a.Picks)
	}
	if !slices.ContainsFunc(x.ran, func(c string) bool { return strings.Contains(c, "mise unuse -g aws-cli") }) {
		t.Errorf("aws-cli not taken off:\n%s", strings.Join(x.ran, "\n"))
	}
}

// What needs an item goes first; one yardo can't uninstall is forgotten.
func TestPlanRemoval(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := planRemoval(cat, []string{"netbird-install", "netbird", "mcpjungle-install", "mcpjungle"}, []string{"netbird-install", "mcpjungle-install"})
	if !slices.Equal(r.Order, []string{"netbird", "netbird-install", "mcpjungle-install"}) {
		t.Errorf("order %q", r.Order)
	}
	if !slices.Equal(r.Also["netbird-install"], []string{"netbird"}) || !slices.Equal(r.Forget["mcpjungle-install"], []string{"mcpjungle"}) {
		t.Errorf("also %v forget %v", r.Also, r.Forget)
	}
}

// The plan says what goes with it, and that an add queued with it
// brings it back.
func TestPlans(t *testing.T) {
	o, cat, _, _ := removeSetup(t, "netbird-install", "netbird")
	a := &answers.Answers{User: o.user, Picks: []string{"netbird-install", "netbird"}}
	p := o.plans(context.Background(), cat, a, []string{"netbird-install"}, []string{"netbird"})["netbird-install"]
	if !strings.Contains(p.Say, "disconnects from NetBird") || !slices.Equal(p.Also, []string{"NetBird login"}) || !slices.Equal(p.Back, []string{"NetBird login"}) {
		t.Errorf("plan %+v", p)
	}
	if shortTool("pipx:git+https://github.com/eugenetaranov/rds-mysql.git") != "rds-mysql" || shortTool("github:eugenetaranov/ssmsearch") != "ssmsearch" || shortTool("~/.zshrc") != "~/.zshrc" {
		t.Error("shortTool")
	}
}
