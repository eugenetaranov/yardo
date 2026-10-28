// Command yardo picks what to install on this machine, then installs
// it: one binary for linux, its files embedded (internal/assets).
//
//	sudo yardo             choose tools, languages, AI and settings
//	sudo yardo --yes       re-apply everything chosen so far, no questions
//	yardo --dry-run        ask, then show what would happen
//	sudo yardo --profile team.toml --profile me.toml [--yes]
//	                              apply profiles (review first without --yes)
//	yardo version [up [--check] [--yes] [--version vX.Y.Z]]
//	                              the version; upgrade to the latest release
//	sudo yardo export [--lock] [file]
//	                              write the saved choices as a profile
//	                              (--lock: with the installed versions)
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/assets"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/creds"
	"github.com/eugenetaranov/yardo/internal/plan"
	"github.com/eugenetaranov/yardo/internal/profile"
	"github.com/eugenetaranov/yardo/internal/run"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
	"golang.org/x/term"
)

// version is set by the build (-X main.version=...).
var version = "dev"

const (
	defaultState = "/var/lib/yardo"
	defaultLog   = "/var/log/yardo.log"
	// defaultLegacy is the bash installer the legacy steps run (until
	// every item is ported), in the share directory yardo extracts.
	defaultLegacy = assets.ShareDir + "/" + assets.LegacyScript
)

func main() {
	// yardo version [up]: no share dir, no root except to replace a plain
	// binary.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		if len(os.Args) > 2 && os.Geteuid() != 0 && needsRootUpgrade(os.Args[2:]) {
			err := asRoot(os.Args[1:])
			fmt.Fprintf(os.Stderr, "yardo: %v\n", err)
			os.Exit(1)
		}
		if err := versionCmd(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "yardo: %v\n", err)
			os.Exit(1)
		}
		return
	}
	// The bash part, mcp-sync and the MCP catalog are read from disk:
	// put this version's there (as root; once per version).
	if os.Getenv("YARDO_LEGACY") == "" && os.Geteuid() == 0 {
		if err := assets.Extract(assets.ShareDir, version); err != nil {
			fmt.Fprintf(os.Stderr, "yardo: %v\n", err)
			os.Exit(1)
		}
	}
	if bashHandles(os.Args[1:]) {
		if os.Geteuid() != 0 && os.Getenv("YARDO_LEGACY") == "" && slices.Contains(os.Args[1:], "forget") {
			err := asRoot(os.Args[1:])
			fmt.Fprintf(os.Stderr, "yardo: %v\n", err)
			os.Exit(1)
		}
		script := envOr("YARDO_LEGACY", defaultLegacy)
		err := handoff(script, os.Args[1:])
		fmt.Fprintf(os.Stderr, "yardo: %v\n", err)
		os.Exit(1)
	}
	// yardo status without sudo: the last check's copy in ~/.cache/yardo.
	if os.Geteuid() != 0 && len(os.Args) > 1 && os.Args[1] == "status" {
		if err := cachedStatus(os.Args[2:], os.Stdout, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "yardo: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if os.Geteuid() != 0 && needsRoot(os.Args[1:]) {
		err := asRoot(os.Args[1:])
		fmt.Fprintf(os.Stderr, "yardo: %v\n", err)
		os.Exit(1)
	}
	if err := cli(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "yardo: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	yes, dryRun, verify, showVersion, list bool
	// status: `yardo status` (verify, reported per item; --json for
	// scripts), collected in statuses.
	status, json, updates    bool
	statuses                 []userStatus
	state, log, legacy, user string
	profiles                 list
	// lock is the profiles' [lock] tables, merged.
	lock *answers.Lock
	// only limits a --yes run to one item (gitkey), with gitKey (from
	// stdin) and gitHosts.
	only     string
	gitKey   []byte
	gitHosts []string
}

// list is a repeatable string flag.
type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func cli(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("yardo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.BoolVar(&o.yes, "yes", false, "re-apply everything chosen so far, no questions")
	fs.BoolVar(&o.yes, "y", false, "short for --yes")
	fs.BoolVar(&o.dryRun, "dry-run", false, "show what would happen; change nothing")
	fs.BoolVar(&o.dryRun, "n", false, "short for --dry-run")
	fs.BoolVar(&o.verify, "verify", false, "check that the saved items are installed; change nothing")
	fs.BoolVar(&o.showVersion, "version", false, "print the version")
	fs.BoolVar(&o.list, "list", false, "list the catalog's items (id, tab, label)")
	fs.StringVar(&o.state, "state", envOr("YARDO_STATE", defaultState), "state directory")
	fs.StringVar(&o.log, "log", envOr("YARDO_LOG", defaultLog), "log file (every command and its output)")
	fs.Var(&o.profiles, "profile", "apply a profile (file or https URL; repeatable, later ones win)")
	fs.StringVar(&o.user, "user", "", "the account to set up, created if missing (wins over profiles)")
	o.legacy = envOr("YARDO_LEGACY", defaultLegacy)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if o.showVersion {
		fmt.Fprintln(stdout, version)
		return nil
	}
	cat, err := catalog.Load(localCatalog(), assets.Conf())
	if err != nil {
		return err
	}
	if o.list {
		for _, it := range cat.Items {
			if !it.Hidden() {
				fmt.Fprintf(stdout, "%s\t%s\t%s\n", it.ID, it.Tab, it.Label)
			}
		}
		return nil
	}
	st := o.store(cat)
	if fs.NArg() > 0 {
		switch fs.Arg(0) {
		case "export":
			return o.export(stdout, stderr, cat, st, fs.Args()[1:])
		case "gitkey":
			if err := o.gitKeyArgs(fs.Args()[1:], stderr); err != nil {
				return err
			}
		case "connect":
			return o.connect(stdout, st)
		case "status":
			sfs := flag.NewFlagSet("yardo status", flag.ContinueOnError)
			sfs.SetOutput(stderr)
			sfs.BoolVar(&o.json, "json", false, "print JSON (schema 1) for scripts")
			sfs.BoolVar(&o.updates, "updates", false, "also check for newer versions (apt and mise; slow)")
			if err := sfs.Parse(fs.Args()[1:]); err != nil {
				return err
			}
			o.status, o.verify = true, true
		default:
			return fmt.Errorf("unknown argument %q", fs.Arg(0))
		}
	}
	interactive := !o.yes && !o.verify
	if o.user != "" && !profile.ValidUser(o.user) {
		return fmt.Errorf("--user %q is not a valid user name", o.user)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var profs []*profile.Profile
	for _, src := range o.profiles {
		p, b, err := loadProfile(ctx, src)
		if err != nil {
			return err
		}
		if interactive && strings.HasPrefix(src, "https://") {
			fmt.Fprintf(stdout, "Profile %s:\n%s\n", src, b)
		}
		if p.Lock != nil {
			if o.lock == nil {
				o.lock = &answers.Lock{}
			}
			o.lock.Add(p.Lock)
		}
		profs = append(profs, p)
	}
	if interactive && (!term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd()))) {
		return errors.New("needs a terminal (or --yes to re-apply saved choices)")
	}
	users, err := o.targets(st, profs, interactive, stderr)
	if err != nil {
		return err
	}
	var failed []string
	if o.status {
		defer func() {
			if o.json {
				_ = writeStatusJSON(stdout, o.statuses, time.Now())
			}
		}()
	}
	for _, u := range users {
		if len(users) > 1 && !o.json {
			fmt.Fprintf(stdout, "\n== %s\n", u)
		}
		err := o.runUser(ctx, cat, st, u, profs, interactive, stdout, stderr)
		if len(users) == 1 {
			return err
		}
		if err != nil {
			fmt.Fprintf(stderr, "yardo: %s: %v\n", u, err)
			failed = append(failed, u)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed for %s", strings.Join(failed, " "))
	}
	return nil
}

// store is the saved answers: system.json in the state directory and
// each user's file in their home.
func (o *options) store(cat *catalog.Catalog) *answers.Store {
	st := &answers.Store{State: o.state, Chown: os.Geteuid() == 0,
		IsSystem: func(id string) bool { it := cat.Item(id); return it != nil && it.System() }}
	// Tests keep homes under a directory of their own.
	if dir := os.Getenv("YARDO_HOMES"); dir != "" {
		st.Account = func(name string) (answers.Account, error) {
			if _, err := answers.LookupAccount(name); err != nil {
				return answers.Account{}, err
			}
			home := filepath.Join(dir, name)
			return answers.Account{Home: home, UID: os.Getuid(), GID: os.Getgid()}, os.MkdirAll(home, 0o755)
		}
	}
	return st
}

// targets are the users this run sets up: --user, else the profiles'
// user, else (interactive or profiles) the user who ran sudo, else
// (--yes, --verify) every user set up. Only an interactive run with no
// sudo user to go by (a root login) and not exactly one user set up
// asks.
func (o *options) targets(st *answers.Store, profs []*profile.Profile, interactive bool, stderr io.Writer) ([]string, error) {
	if o.user != "" {
		return []string{o.user}, nil
	}
	for i := len(profs) - 1; i >= 0; i-- {
		if profs[i].User != "" {
			return []string{profs[i].User}, nil
		}
	}
	saved, notes, err := st.Users()
	for _, n := range notes {
		fmt.Fprintln(stderr, "yardo: "+n)
	}
	if err != nil {
		return nil, err
	}
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "root" {
		sudoUser = "" // sudo from a root shell: no login user to go by
	}
	switch {
	case (interactive || len(profs) > 0) && sudoUser != "":
		return []string{sudoUser}, nil
	case interactive && len(saved) == 1:
		return saved, nil
	case interactive:
		def := ""
		if len(saved) > 0 {
			def = saved[0]
		}
		u, err := askUser(def)
		if errors.Is(err, ui.ErrQuit) {
			fmt.Fprintln(stderr, "Cancelled.")
			return nil, nil
		}
		return []string{u}, err
	case len(profs) > 0:
		return nil, errors.New("no user to set up: give --user, or run yardo with sudo from that user's login")
	case len(saved) == 0:
		return nil, errors.New("no saved choices yet; run yardo without --yes first")
	}
	return saved, nil
}

// runUser applies the profiles to user's answers, asks (interactive) or
// re-applies them, saves them, and installs (or plans, or verifies).
func (o *options) runUser(ctx context.Context, cat *catalog.Catalog, st *answers.Store, user string, profs []*profile.Profile, interactive bool, stdout, stderr io.Writer) error {
	a, notes, err := st.Load(user)
	if err != nil {
		return err
	}
	for _, n := range notes {
		fmt.Fprintln(stderr, "yardo: "+n)
	}
	var dropped []string
	for _, p := range profs {
		d, warn := p.Apply(a, cat)
		for _, w := range warn {
			fmt.Fprintln(stderr, "yardo: "+w)
		}
		dropped = answers.Merge(dropped, d)
	}
	a.User = user // a profile's user lost to --user
	// A later profile (or the same one, unnamed) may have kept it.
	dropped = slices.DeleteFunc(dropped, func(id string) bool { return slices.Contains(a.Picks, id) })
	changed := len(profs) > 0
	var items []*catalog.Item
	extra := a.MiseExtra
	sec := creds.New()
	// fix: the items the home screen re-applies (f, or after e); startTab
	// the picker tab it opens on (a).
	var fix []string
	startTab := ""
	if interactive && len(profs) == 0 && len(a.Picks) > 0 {
		// Set up already: the home screen first; the picker from there.
		updates := false
	home:
		for {
			h, act, err := o.home(ctx, cat, a, stdout, updates)
			if err != nil {
				return err
			}
			updates = false
			switch act {
			case ui.HomeUpdates:
				updates = true
			case ui.HomeQuit:
				return nil
			case ui.HomeChange:
				startTab = h.StartTab()
				break home
			case ui.HomeFix:
				fix = h.Fix()
				break home
			case ui.HomeEdit:
				if fix, err = o.fillIn(ctx, a, h.Selected()); err != nil {
					return err
				}
				if fix != nil {
					break home
				}
			case ui.HomeConnect:
				printConnect(stdout, a, addresses())
				fmt.Fprint(stdout, "\nPress Enter to go back.")
				_, _ = fmt.Fscanln(os.Stdin)
			}
		}
	}
	if interactive && fix == nil {
		k := &asker{cat: cat, a: a, share: o.share(), out: stdout, startTab: startTab, installed: func(t string) bool { return userHas(a.User, t) },
			profile: len(profs) > 0, dropped: dropped, port: func() int { p, _ := st.Port(user); return p }}
		c, err := k.ask(ctx)
		if errors.Is(err, ui.ErrQuit) {
			fmt.Fprintln(stdout, "Cancelled.")
			return nil
		}
		if err != nil || c == nil {
			return err
		}
		a.Unpick(c.dropped)
		if slices.Contains(c.dropped, "docker-ports") {
			a.DockerPorts = "" // unticked: published ports reachable everywhere again
		}
		a.Pick(c.picks)
		a.MiseExtra = c.miseExtra
		items, extra, sec = c.items, c.miseExtra, c.secrets
		changed = true
	} else {
		if o.only != "" {
			a.Pick(a.SendKeyTo(o.gitHosts))
			sec.GitKey = o.gitKey
			changed = true
		}
		if len(a.Picks) == 0 {
			return errors.New("no saved choices yet; run yardo without --yes first")
		}
		if items, err = plan.Resolve(cat, a.Picks, len(a.MiseExtra) > 0); err != nil {
			return err
		}
		if o.only != "" {
			items = slices.DeleteFunc(items, func(it *catalog.Item) bool { return it.ID != o.only })
		}
		if fix != nil {
			// What the home screen re-applies, and the helpers (mise) it
			// needs; extra mise tools only when one of them is listed.
			items = slices.DeleteFunc(items, func(it *catalog.Item) bool { return !it.Hidden() && !slices.Contains(fix, it.ID) })
			if !slices.ContainsFunc(fix, func(id string) bool { return strings.HasPrefix(id, "mise:") }) {
				extra = nil
			}
		}
		listDropped(stdout, cat, dropped)
	}
	if a.MCPPort == 0 && slices.ContainsFunc(items, func(it *catalog.Item) bool { return it.ID == "mcpjungle-install" }) {
		if a.MCPPort, err = st.Port(a.User); err != nil {
			return err
		}
		changed = true
	}
	created, err := o.ensureUser(ctx, a.User, stdout)
	if err != nil {
		return err
	}
	if !o.dryRun && !o.verify {
		save := st.Save
		if !changed {
			// Only answers.json, which the bash steps read for this user.
			save = func(a *answers.Answers) error { return answers.Write(o.state, a) }
		}
		if err := save(a); err != nil {
			return err
		}
	}
	if o.dryRun || o.verify {
		env, err := o.env(a, sec, nil)
		if err != nil {
			return err
		}
		if o.dryRun {
			return run.DryRun(stdout, env, items, extra)
		}
		if o.status {
			us := userStatusOf(ctx, env, a.User, items, extra, loadApplied(o.state, a.User), o.updates)
			if err := saveStatus(ctx, env, o.state, us, time.Now()); err != nil {
				fmt.Fprintf(stderr, "yardo: saving the status: %v\n", err)
			}
			o.statuses = append(o.statuses, us)
			if !o.json {
				printStatus(stdout, us)
			}
			return nil
		}
		return verify(ctx, stdout, env, items)
	}
	logf, err := os.OpenFile(o.log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	fmt.Fprintf(logf, "=== yardo %s, %s, user %s\n", version, time.Now().Format(time.RFC3339), a.User)
	env, err := o.env(a, sec, logf)
	if err != nil {
		return err
	}
	if created {
		env.Note(fmt.Sprintf("Created user %s (passwordless sudo, no password or SSH key): add a key to %s/.ssh/authorized_keys, or use 'sudo -iu %s'.", a.User, env.Home, a.User))
	}
	prog := progressFor(stdout)
	bash := legacyRunner(env, o.legacy, o.state)
	if bash != nil {
		stdin := sec.Encode()
		env.Legacy = func(ctx context.Context, id string) error {
			// The GitHub steps may open the browser login: in an
			// interactive run they get the terminal.
			if interactive && (id == "github" || id == "gitsign") {
				if l, ok := prog.(*run.Live); ok {
					l.Pause()
				}
				c := exec.CommandContext(ctx, "bash", o.legacy, "--only", id, "--secrets")
				c.Env = append(os.Environ(), legacyCmd(o.legacy, o.state).Env...)
				c.Stdin, c.Stdout, c.Stderr = bytes.NewReader(stdin), os.Stdout, os.Stderr
				return c.Run()
			}
			cmd := legacyCmd(o.legacy, o.state, "--only", id, "--secrets")
			cmd.Stdin = stdin
			_, err := env.RunLogged(ctx, cmd)
			return err
		}
		prog.Start("Getting ready")
		if err := bash(ctx, "--begin"); err != nil {
			prog.Done("fail", "Getting ready")
			return err
		}
		prog.Done("ok", "Ready to install")
	}
	notesFile := filepath.Join(o.state, ".notes")
	if bash == nil {
		_ = os.Remove(notesFile) // bash's --begin does this when it runs
		_ = os.Remove(filepath.Join(o.state, keysFile))
	}
	r := &run.Runner{Env: env, Progress: prog, TailLines: 25}
	res := r.Run(ctx, items, extra)
	if err := markApplied(o.state, a.User, slices.Concat(res.Changed, res.Unchanged), time.Now()); err != nil {
		fmt.Fprintf(stderr, "yardo: %v\n", err)
	}
	if err := primeZshCompletions(ctx, env, r.Progress); err != nil {
		res.Failed = append(res.Failed, "zshcomp")
		res.Errors["zshcomp"] = err.Error()
	}
	// While bash runs steps, its notes (with "Secrets to fill in") are the
	// fuller ~/yardo.txt.
	if bash != nil {
		if out, err := bashOut(ctx, env, o.legacy, o.state, "--finish"); err != nil {
			fmt.Fprintf(stderr, "yardo: notes: %v\n", err)
		} else {
			_, _ = stdout.Write(out)
			if err := appendUnverified(env, items); err != nil {
				fmt.Fprintf(stderr, "yardo: %s/yardo.txt: %v\n", env.Home, err)
			}
		}
	} else {
		if b, err := os.ReadFile(notesFile); err == nil && len(b) > 0 {
			for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				fmt.Fprintf(stdout, "! %s\n", l)
			}
		}
		if o.only != "" {
			// One item: the summary of everything stays as it is.
		} else if err := writeSummary(env, items, extra); err != nil {
			fmt.Fprintf(stderr, "yardo: %s/yardo.txt: %v\n", env.Home, err)
		}
	}
	fmt.Fprintf(stdout, "How each tool was installed and how to update it: %s/yardo.txt\nLog: %s\n", env.Home, o.log)
	printKeys(stdout, o.state, env.Home)
	if h := newShellHint(res.Changed); h != "" {
		fmt.Fprintln(stdout, h)
	}
	if !res.OK() {
		for _, id := range res.Failed {
			fmt.Fprintf(stderr, "✗ %s: %s\n", id, res.Errors[id])
		}
		return fmt.Errorf("failed: %s — fix and re-run yardo", strings.Join(res.Failed, " "))
	}
	return nil
}

// gitKeyArgs reads `yardo gitkey [--host h]...`: the private key
// on stdin (ssh host 'sudo yardo gitkey' < key) for the sudo user (or --user),
// set up alone, as with --yes.
func (o *options) gitKeyArgs(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("yardo gitkey", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var hosts list
	fs.Var(&hosts, "host", "a git host to use the key for: github.com, bitbucket.org, your GitLab or another (repeatable; default github.com)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	for _, h := range hosts {
		if !answers.ValidHost(h) {
			return fmt.Errorf("--host %q is not a host name", h)
		}
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("gitkey reads the private key on stdin: ssh <this machine> 'sudo yardo gitkey' < <key file>")
	}
	key, err := io.ReadAll(io.LimitReader(os.Stdin, 64<<10))
	if err != nil {
		return err
	}
	if !bytes.Contains(key, []byte("PRIVATE KEY-----")) {
		return errors.New("stdin isn't an SSH private key")
	}
	if o.user == "" {
		o.user = os.Getenv("SUDO_USER")
	}
	if o.user == "" {
		return errors.New("no user to set the key up for: give --user")
	}
	o.yes, o.only, o.gitKey, o.gitHosts = true, "gitkey", key, hosts
	return nil
}

// progressFor shows each step with a spinner and its elapsed time on a
// terminal, and plain result lines otherwise (logs, runs streamed over SSH).
func progressFor(w io.Writer) run.Progress {
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		width, _, _ := term.GetSize(int(f.Fd()))
		return &run.Live{W: w, Width: width}
	}
	return run.Plain{W: w}
}

// primeZshCompletions fills the zsh completion cache once every tool of
// the run is installed, so the first login doesn't pay for it.
func primeZshCompletions(ctx context.Context, env *steps.Env, p run.Progress) error {
	if _, err := os.Stat(env.Path("~/.config/yardo/completions.zsh")); err != nil {
		return nil
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		return nil
	}
	const title = "zsh completions for installed tools"
	p.Start(title)
	_, err := env.RunLogged(ctx, steps.Cmd{Name: "zsh", Args: []string{"-c", "source ~/.config/yardo/completions.zsh && yardo_comp_refresh"}, AsUser: true})
	if err != nil {
		p.Done("fail", title)
		return err
	}
	p.Done("ok", title)
	return nil
}

func verify(ctx context.Context, w io.Writer, env *steps.Env, items []*catalog.Item) error {
	missing := 0
	for _, s := range run.Verify(ctx, env, items) {
		if s.Item.Hidden() {
			continue
		}
		switch {
		case s.State == "ok":
			fmt.Fprintf(w, "ok         %s\n", s.Item.Label)
		case s.State == "unchecked":
			fmt.Fprintf(w, "unchecked  %s (not ported yet)\n", s.Item.Label)
		case s.State == "secret":
			fmt.Fprintf(w, "secret     %s: fill in %s\n", s.Item.Label, strings.Join(s.Inputs, ", "))
		case s.Problem():
			missing++
			fmt.Fprintf(w, "%-10s %s: %s\n", s.State, s.Item.Label, s.Detail)
		}
	}
	if missing > 0 {
		return fmt.Errorf("%d item(s) missing", missing)
	}
	return nil
}

// env is the steps' Env for the run: the user's, with the saved answers
// and this run's secrets. Notes go to the state's .notes (shown at the
// end of the run) and the log.
func (o *options) env(a *answers.Answers, sec *creds.Secrets, log io.Writer) (*steps.Env, error) {
	env, err := newEnv(a.User, o.share(), log, o.dryRun)
	if err != nil {
		return nil, err
	}
	env.Share, env.State, env.Answers, env.Secrets, env.Lock = o.share(), o.state, a, sec, o.lock
	env.Note = func(text string) {
		if log != nil {
			fmt.Fprintf(log, "NOTE: %s\n", text)
		}
		f, err := os.OpenFile(filepath.Join(o.state, ".notes"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintln(f, text)
	}
	env.Key = func(k steps.NewKey) {
		if log != nil {
			fmt.Fprintf(log, "NEW KEY for %s: %s\n", k.Host, k.Pub)
		}
		f, err := os.OpenFile(filepath.Join(o.state, keysFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, "%s\t%s\t%s\t%s\n", k.Host, k.Where, k.Path, k.Pub)
	}
	return env, nil
}

// keysFile holds the public keys made this run (host, where to add it,
// file, key per line), shown last.
const keysFile = ".keys"

// printKeys shows the run's new public keys after everything else: where
// to add each, then the key alone on a line between blank lines, so a
// triple-click copies exactly it.
func printKeys(w io.Writer, state, home string) {
	b, err := os.ReadFile(filepath.Join(state, keysFile))
	if err != nil {
		return
	}
	type key struct {
		path, pub string
		where     []string
	}
	var keys []*key
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.SplitN(l, "\t", 4)
		if len(f) != 4 || f[3] == "" {
			continue
		}
		i := slices.IndexFunc(keys, func(k *key) bool { return k.pub == f[3] })
		if i < 0 {
			keys = append(keys, &key{path: f[2], pub: f[3]})
			i = len(keys) - 1
		}
		keys[i].where = append(keys[i].where, f[0]+": "+f[1])
	}
	for _, k := range keys {
		path := k.path
		if rest, ok := strings.CutPrefix(path, "~/"); ok {
			path = filepath.Join(home, rest)
		}
		fmt.Fprintf(w, "\nSSH key to add\n")
		for _, wh := range k.where {
			fmt.Fprintf(w, "  %s\n", wh)
		}
		fmt.Fprintf(w, "\n%s\n\nShow it again: cat %s\n", k.pub, path)
	}
}

// share is the directory yardo's files are installed in (next to
// the bash script).
func (o *options) share() string { return filepath.Dir(o.legacy) }

// localCatalog is /etc/yardo/catalog.d, if there is one.
func localCatalog() fs.FS {
	if fi, err := os.Stat(catalog.LocalDir); err == nil && fi.IsDir() {
		return os.DirFS(catalog.LocalDir)
	}
	return nil
}

// loadProfile reads a profile from a file or an https URL; b is its text.
func loadProfile(ctx context.Context, src string) (p *profile.Profile, b []byte, err error) {
	switch {
	case strings.HasPrefix(src, "https://"):
		b, err = download(ctx, src)
	case strings.Contains(src, "://"):
		return nil, nil, fmt.Errorf("%s: profiles load from a file or an https URL", src)
	default:
		b, err = os.ReadFile(src)
	}
	if err != nil {
		return nil, nil, err
	}
	p, err = profile.Parse(src, b)
	return p, b, err
}

// listDropped prints the picks named profiles dropped.
func listDropped(w io.Writer, cat *catalog.Catalog, ids []string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintln(w, "Dropped by the profile (still installed, no longer re-applied):")
	for _, id := range ids {
		label := id
		if it := cat.Item(id); it != nil {
			label = fmt.Sprintf("%s (%s)", it.Label, id)
		}
		fmt.Fprintf(w, "  - %s\n", label)
	}
}

// ensureUser creates the target user when it doesn't exist (a dry run
// says it would; verify fails).
func (o *options) ensureUser(ctx context.Context, name string, w io.Writer) (created bool, err error) {
	if _, err := user.Lookup(name); err == nil {
		return false, nil
	}
	switch {
	case o.verify:
		return false, fmt.Errorf("user %q does not exist", name)
	case o.dryRun:
		fmt.Fprintf(w, "Would create user %s (passwordless sudo, the groups of the user running sudo)\n", name)
		return false, nil
	}
	if err := createUser(ctx, steps.OSExec{}, name, os.Getenv("SUDO_USER")); err != nil {
		return false, err
	}
	fmt.Fprintf(w, "Created user %s.\n", name)
	return true, nil
}

// export writes the saved answers as a profile, to file or stdout. With
// --lock it adds the installed versions of the plan's mise tools and apt
// packages.
func (o *options) export(stdout, stderr io.Writer, cat *catalog.Catalog, st *answers.Store, args []string) error {
	fs := flag.NewFlagSet("yardo export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lock := fs.Bool("lock", false, "add a [lock] table with the installed mise and apt versions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("export: unexpected argument %q", fs.Arg(1))
	}
	file := fs.Arg(0)
	user := o.user
	if user == "" {
		saved, _, err := st.Users()
		if err != nil {
			return err
		}
		switch {
		case slices.Contains(saved, os.Getenv("SUDO_USER")):
			user = os.Getenv("SUDO_USER")
		case len(saved) == 1:
			user = saved[0]
		case len(saved) > 1:
			return fmt.Errorf("export: several users are set up (%s); pick one with --user", strings.Join(saved, " "))
		}
	}
	a, notes, err := st.Load(user)
	if err != nil {
		return err
	}
	for _, n := range notes {
		fmt.Fprintln(stderr, "yardo: "+n)
	}
	if len(a.Picks) == 0 {
		return errors.New("no saved choices to export yet")
	}
	var l *answers.Lock
	if *lock {
		var err error
		if l, err = o.versions(context.Background(), stderr, cat, a); err != nil {
			return err
		}
	}
	b := profile.Export(a, l)
	if file == "" {
		_, err := stdout.Write(b)
		return err
	}
	return os.WriteFile(file, b, 0o644)
}

// versions reads the installed versions of the saved plan's mise tools
// and apt packages.
func (o *options) versions(ctx context.Context, stderr io.Writer, cat *catalog.Catalog, a *answers.Answers) (*answers.Lock, error) {
	items, err := plan.Resolve(cat, a.Picks, len(a.MiseExtra) > 0)
	if err != nil {
		return nil, err
	}
	env, err := o.env(a, creds.New(), nil)
	if err != nil {
		return nil, err
	}
	var tools, pkgs []string
	for _, it := range items {
		for _, s := range it.Steps {
			switch s.Type {
			case "mise":
				tools = append(tools, s.Tools...)
			case "apt":
				for _, p := range s.Packages {
					if !strings.Contains(p, "{{") { // per-kernel names don't carry over
						pkgs = append(pkgs, p)
					}
				}
			}
		}
	}
	l, missing := steps.Versions(ctx, env, answers.Merge(tools, a.MiseExtra), answers.Merge(nil, pkgs))
	if len(missing) > 0 {
		fmt.Fprintf(stderr, "yardo: not installed, so not locked: %s\n", strings.Join(missing, " "))
	}
	return l, nil
}

// legacyRunner runs the bash installer with args (logged), or is nil when
// it isn't installed.
func legacyRunner(env *steps.Env, script, state string) func(ctx context.Context, args ...string) error {
	if _, err := os.Stat(script); err != nil {
		return nil
	}
	return func(ctx context.Context, args ...string) error {
		_, err := env.RunLogged(ctx, legacyCmd(script, state, args...))
		return err
	}
}

func bashOut(ctx context.Context, env *steps.Env, script, state string, args ...string) ([]byte, error) {
	return env.Exec.Run(ctx, legacyCmd(script, state, args...))
}

func legacyCmd(script, state string, args ...string) steps.Cmd {
	return steps.Cmd{Name: "bash", Args: append([]string{script}, args...),
		Env: []string{"YARDO_STATE=" + state, "YARDO_SHARE=" + filepath.Dir(script)}}
}

// appendUnverified adds the "Installed without a pinned checksum" section
// to the ~/yardo.txt the bash installer just wrote, which lacks it.
func appendUnverified(env *steps.Env, items []*catalog.Item) error {
	u := run.Unverified(items)
	if len(u) == 0 {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(env.Home, "yardo.txt"), os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	fmt.Fprintln(f, "\nInstalled without a pinned checksum (what was fetched wasn't checked against a known hash):")
	for _, x := range u {
		fmt.Fprintf(f, "  %-40s %s\n", x.Item.Label, x.Reason)
	}
	return f.Close()
}

func writeSummary(env *steps.Env, items []*catalog.Item, extra []string) error {
	p := filepath.Join(env.Home, "yardo.txt")
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	run.Summary(f, items, extra, time.Now())
	fmt.Fprintln(f, "Run 'sudo yardo' again to add more.")
	if err := f.Close(); err != nil {
		return err
	}
	if env.Chown {
		return os.Chown(p, env.UID, env.GID)
	}
	return nil
}

// bashHandles says whether the bash yardo runs this command line:
// forget isn't ported yet, and its help describes both.
func bashHandles(args []string) bool {
	for _, a := range args {
		switch a {
		case "forget", "-h", "--help", "-help":
			return true
		}
	}
	return false
}

// userHas reports whether tool is on the user's PATH (mise shims included).
func userHas(user, tool string) bool {
	return exec.Command("runuser", "-u", user, "--", "sh", "-c", steps.UserShell, "_", "sh", "-c", "command -v "+tool).Run() == nil
}

// needsRoot: everything but --version and --list reads the root-only state.
func needsRoot(args []string) bool {
	for _, a := range args {
		switch a {
		case "--version", "-version", "--list", "-list":
			return false
		}
	}
	return true
}

// asRoot re-runs this command through sudo, keeping a forwarded SSH agent
// (as the bash yardo does).
func asRoot(args []string) error {
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return errors.New("needs root: run it with sudo")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	argv := []string{"sudo"}
	// Keep a forwarded agent, and a proxy the user's shell has.
	var keep []string
	for _, k := range []string{"SSH_AUTH_SOCK", "http_proxy", "https_proxy", "no_proxy", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		if os.Getenv(k) != "" {
			keep = append(keep, k)
		}
	}
	if len(keep) > 0 {
		argv = append(argv, "--preserve-env="+strings.Join(keep, ","))
	}
	return syscall.Exec(sudo, append(append(argv, self), args...), os.Environ())
}

// handoff replaces this process with the bash yardo.
func handoff(script string, args []string) error {
	if _, err := os.Stat(script); err != nil {
		// Not unpacked yet (run without root, e.g. --help): a private copy.
		dir := filepath.Join(os.TempDir(), "yardo-"+strconv.Itoa(os.Getuid())+"-"+version)
		if err := assets.Extract(dir, version); err != nil {
			return err
		}
		script = filepath.Join(dir, assets.LegacyScript)
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		return err
	}
	return syscall.Exec(bash, append([]string{"bash", script}, args...), os.Environ())
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// newShellHint is what to run so the shell yardo was started from sees
// this run's changes: mise's shims on PATH (the mise item, run once) or
// shell setup only a new shell reads. Empty when nothing needs it.
func newShellHint(changed []string) string {
	switch {
	case slices.Contains(changed, "zshbin"):
		return "Your login shell is now zsh: log in again, or run: exec zsh -l"
	case slices.Contains(changed, "mise"), slices.Contains(changed, "shellcfg"):
		return "New tools are on PATH in new shells: log in again, or run: exec bash -l"
	}
	return ""
}
