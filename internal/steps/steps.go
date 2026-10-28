// Package steps runs catalog steps: each one can say what it would do
// (Plan), whether it's already done (Check), and do it (Apply). Commands go
// through an Execer and files through Env.Root, so tests run against a
// temp directory and a fake executor.
package steps

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/creds"
)

// Cmd is one command to run.
type Cmd struct {
	Name  string
	Args  []string
	Env   []string // extra KEY=value
	Stdin []byte
	// AsUser runs it as the target user, from their home, with mise's
	// shims on PATH.
	AsUser bool
	// As runs it as this user (a service user), as it is.
	As string
}

func (c Cmd) String() string {
	s := c.Name
	for _, a := range c.Args {
		if a == "" || strings.ContainsAny(a, " \t'\"$\\") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		s += " " + a
	}
	if c.AsUser {
		s = "(as user) " + s
	}
	if c.As != "" {
		s = "(as " + c.As + ") " + s
	}
	return s
}

// Execer runs commands; it returns combined output.
type Execer interface {
	Run(ctx context.Context, c Cmd) ([]byte, error)
}

// Env is what steps run against.
type Env struct {
	User, Group string
	UID, GID    int
	Home        string // the user's home, without Root
	Arch        string // dpkg architecture (amd64, arm64)
	Codename    string // Ubuntu/Debian codename
	Distro      string // ubuntu or debian: whose apt repos to use
	Kernel      string // the running kernel's release (uname -r)
	Exec        Execer
	Files       fs.FS // the catalog's files (file/template src)
	// Root prefixes every absolute path (tests); "" in production.
	Root string
	// Log gets each command and its output.
	Log io.Writer
	// Chown is false in tests run as non-root.
	Chown bool
	// Legacy runs the bash installer for one item id (see LegacyStep).
	Legacy func(ctx context.Context, id string) error
	// Download fetches a URL (apt keys). Tests replace it.
	Download func(ctx context.Context, url string) ([]byte, error)

	// Share is the directory yardo's files are installed in;
	// State holds its answers and created.json.
	Share, State string
	// Answers are the run's saved choices; Secrets what was typed this run
	// (never saved).
	Answers *answers.Answers
	Secrets *creds.Secrets
	// Lock pins mise and apt versions (a profile's [lock]); nil: latest.
	Lock *answers.Lock

	// checks caches what the Checks read, while it isn't nil (a status
	// run, which changes nothing between them): see CacheChecks.
	checks *checkCache
	// Note shows text after the run (a step that has to leave something
	// for the user to do).
	Note func(text string)
	// Key shows a public key made this run, last, on a line of its own.
	Key func(NewKey)
}

// NewKey is a public key made on the machine, to add on a git host.
type NewKey struct {
	// Host is the git host; Where says where on it to add the key.
	Host, Where string
	// Path is the public key's file (~/.ssh/…pub); Pub its text.
	Path, Pub string
}

func (e *Env) note(text string) {
	if e.Note != nil {
		e.Note(text)
	}
}

func (e *Env) key(k NewKey) {
	if e.Key != nil {
		e.Key(k)
	}
}

// record notes something yardo created, for `yardo forget`.
func (e *Env) record(kind, value string) error {
	if e.State == "" {
		return nil
	}
	return answers.AddCreated(e.State, answers.Created{Kind: kind, Value: value})
}

// Path resolves p: "~/x" is under the user's home; everything under Root.
func (e *Env) Path(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		p = filepath.Join(e.Home, strings.TrimPrefix(p, "~"))
	}
	return filepath.Join(e.Root, p)
}

// RunLogged runs c with the command and its output in the log.
func (e *Env) RunLogged(ctx context.Context, c Cmd) ([]byte, error) { return e.run(ctx, c) }

func (e *Env) run(ctx context.Context, c Cmd) ([]byte, error) {
	if e.Log != nil {
		fmt.Fprintf(e.Log, "$ %s\n", c)
	}
	out, err := e.Exec.Run(ctx, c)
	if e.Log != nil && len(out) > 0 {
		_, _ = e.Log.Write(out)
		if out[len(out)-1] != '\n' {
			fmt.Fprintln(e.Log)
		}
	}
	if err != nil {
		return out, fmt.Errorf("%s: %w", c.Name, err)
	}
	return out, nil
}

// owner says who owns path to: the user for ~/ paths unless the step says
// root, else root.
func (e *Env) owner(s catalog.Step) (uid, gid int, user bool) {
	user = s.Owner == "user" || (s.Owner == "" && strings.HasPrefix(s.To, "~"))
	if user {
		return e.UID, e.GID, true
	}
	return 0, 0, false
}

func (e *Env) chown(p string, uid, gid int) error {
	if !e.Chown {
		return nil
	}
	return os.Lchown(p, uid, gid)
}

// Action is one thing a step would do, for --dry-run.
type Action struct {
	Kind   string // apt, mise, repo, file, line, link, clone, unit, sysctl, shell, run, legacy
	Target string
}

func (a Action) String() string { return a.Kind + " " + a.Target }

// Step is a runnable catalog step.
type Step interface {
	Plan(e *Env) []Action
	// Check reports whether the step has nothing to do.
	Check(ctx context.Context, e *Env) (bool, error)
	Apply(ctx context.Context, e *Env) error
}

// Follower is a step that only runs when an earlier step of the same item
// changed something (a restart, a daemon-reload).
type Follower interface{ Follows() bool }

// BeforeApt is a step the runner applies with the apt repositories, before
// the batch installs packages (a config a package reads when it first
// starts).
type BeforeApt interface{ BeforeApt() bool }

// Always is a step that runs on every run, after its item's other steps,
// and is idempotent itself (registering MCP servers): it doesn't count as
// a change, and a failure is a note, not a failed item.
type Always interface{ Always() bool }

// Inputs is a step that can be waiting for values the user left empty
// (keys to fill in): Missing lists them, as "file: KEY".
type Inputs interface {
	Missing(ctx context.Context, e *Env) []string
}

// Service is a step that runs a service: Down reports whether it is set up
// (enabled) but not running, and its name.
type Service interface {
	Down(ctx context.Context, e *Env) (bool, string)
}

// checkCache is what Checks read once per status run: the user's mise
// tools, and the installed packages.
type checkCache struct {
	mise    map[string][]miseVersion
	miseOK  bool
	miseSet bool
	debs    map[string]bool
}

// CacheChecks makes the Checks share what they read (mise's tools, dpkg's
// packages) until the returned func is called. Only for runs that change
// nothing between Checks (status, verify).
func (e *Env) CacheChecks() (done func()) {
	e.checks = &checkCache{}
	return func() { e.checks = nil }
}

// checkAll reports whether every step but the followers has nothing to
// do: a Go step made of other steps.
func checkAll(ctx context.Context, e *Env, sts []Step) (bool, error) {
	for _, st := range sts {
		if f, ok := st.(Follower); ok && f.Follows() {
			continue
		}
		if done, err := st.Check(ctx, e); err != nil || !done {
			return false, err
		}
	}
	return true, nil
}

// applyAll applies the steps as the runner does an item's: each one that
// has something to do, a follower only after a change (changed: one
// already happened).
func applyAll(ctx context.Context, e *Env, sts []Step, changed bool) error {
	for _, st := range sts {
		if f, ok := st.(Follower); ok && f.Follows() {
			if !changed {
				continue
			}
		} else if done, err := st.Check(ctx, e); err != nil {
			return err
		} else if done {
			continue
		}
		if err := st.Apply(ctx, e); err != nil {
			return err
		}
		changed = true
	}
	return nil
}

// mustSteps makes the steps of a fixed list (a Go step's parts).
func mustSteps(id string, ss ...catalog.Step) []Step {
	out := make([]Step, len(ss))
	for i, s := range ss {
		st, err := New(id, s)
		if err != nil {
			panic(err)
		}
		out[i] = st
	}
	return out
}

// GoStep makes a registered Go step for an item.
type GoStep func(s catalog.Step) Step

var goSteps = map[string]GoStep{}

// RegisterGo registers a `go:<name>` step.
func RegisterGo(name string, f GoStep) {
	goSteps[name] = f
	catalog.GoSteps[name] = true
}

// New makes the runnable step for s, for item id.
func New(id string, s catalog.Step) (Step, error) {
	switch s.Type {
	case "apt":
		return aptStep{s}, nil
	case "mise":
		return miseStep{s}, nil
	case "apt_repo":
		return aptRepoStep{s}, nil
	case "file":
		return fileStep{s, false}, nil
	case "template":
		return fileStep{s, true}, nil
	case "link":
		return linkStep{s}, nil
	case "line":
		return lineStep{s}, nil
	case "block":
		return blockStep{s}, nil
	case "git_clone":
		return gitCloneStep{s}, nil
	case "systemctl":
		return systemctlStep{s}, nil
	case "sysctl":
		return sysctlStep{s}, nil
	case "git_config":
		return gitConfigStep{s}, nil
	case "user_shell":
		return userShellStep{s}, nil
	case "run":
		return runStep{s}, nil
	case "note":
		return noteStep{s}, nil
	case "cred":
		return credStep{id, s}, nil
	case "legacy":
		return LegacyStep{ID: id}, nil
	}
	if name, ok := strings.CutPrefix(s.Type, "go:"); ok {
		if f := goSteps[name]; f != nil {
			return f(s), nil
		}
		return nil, fmt.Errorf("no Go step %q", name)
	}
	return nil, fmt.Errorf("unknown step type %q", s.Type)
}

// LegacyStep runs the bundled bash installer for one item, until the item
// is ported to Go steps.
type LegacyStep struct{ ID string }

func (l LegacyStep) Plan(*Env) []Action {
	return []Action{{"legacy", "yardo.sh --only " + l.ID}}
}

// Check is false: the bash functions are idempotent and cheap to re-run.
func (LegacyStep) Check(context.Context, *Env) (bool, error) { return false, nil }

func (l LegacyStep) Apply(ctx context.Context, e *Env) error {
	if e.Legacy == nil {
		return fmt.Errorf("%s: the bash installer isn't available", l.ID)
	}
	return e.Legacy(ctx, l.ID)
}
