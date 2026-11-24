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
//	sudo yardo export [--name N] [--lock] [file]
//	                              write the saved choices as a profile
//	                              (--lock: with the installed versions)
package main

import (
	"bytes"
	"cmp"
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

	"github.com/eugenetaranov/yardo/internal/addon"
	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/assets"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/creds"
	"github.com/eugenetaranov/yardo/internal/mcp"
	"github.com/eugenetaranov/yardo/internal/plan"
	"github.com/eugenetaranov/yardo/internal/profile"
	"github.com/eugenetaranov/yardo/internal/run"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/tui"
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
	if wantsHelp(os.Args[1:]) {
		printHelp(os.Stdout, os.Args[1:])
		return
	}
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
		if e := (*exitErr)(nil); errors.As(err, &e) {
			os.Exit(e.code)
		}
		os.Exit(1)
	}
}

type options struct {
	yes, dryRun, verify, showVersion, list bool
	// specs are the catalog's MCP servers, once loaded (mcpSpecs).
	specs map[string]*mcp.Spec
	// startMCP: `yardo mcp` opens the menu on its MCP servers.
	startMCP bool
	// status: `yardo status` (verify, reported per item; --json for
	// scripts), collected in statuses.
	status, json, updates bool
	// ui is the screen sudo yardo opens on: menu, or board.
	ui string
	// theme is the look for this run (tui.Looks), over the saved one.
	theme                    string
	statuses                 []userStatus
	state, log, legacy, user string
	profiles                 list
	// lock is the profiles' [lock] tables, merged.
	lock *answers.Lock
	// only limits a --yes run to one item (gitkey), with gitKey (from
	// stdin) for gitHosts and the git host items gitProviders (replacing
	// the key there when gitReplace).
	only         string
	gitKey       []byte
	gitHosts     []string
	gitProviders []string
	gitReplace   bool
	// in answers yardo remove's question (os.Stdin when nil); exec runs
	// the commands instead of the system, and root prefixes every path
	// (tests).
	in   io.Reader
	exec steps.Execer
	root string
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
	fs.StringVar(&o.ui, "ui", "menu", "the screen sudo yardo opens on: menu (the home) or board (Tools, the tabbed checklist)")
	fs.StringVar(&o.theme, "theme", "", "the look of yardo's screens for this run: classic, bridge, covert or tactical (Settings › Look keeps one)")
	o.legacy = envOr("YARDO_LEGACY", defaultLegacy)
	fs.Usage = func() { fmt.Fprintln(stderr, "Run 'yardo --help' for the commands and flags.") }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printHelp(stdout, args)
			return nil
		}
		return err
	}
	if o.ui != "menu" && o.ui != "board" {
		return fmt.Errorf("--ui %q: want menu or board", o.ui)
	}
	if _, ok := tui.LookOf(o.theme); o.theme != "" && !ok {
		return fmt.Errorf("--theme %q: classic, bridge, covert or tactical", o.theme)
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
			script := false
			for _, x := range fs.Args()[1:] {
				if x != "--script" {
					return fmt.Errorf("connect: unknown %q (connect [--script])", x)
				}
				script = true
			}
			return o.connect(stdout, cat, st, script)
		case "remove", "uninstall":
			return o.removeCmd(cat, st, fs.Args()[1:], stdout, stderr)
		case "mcp":
			if fs.NArg() > 1 {
				ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
				defer stop()
				if fs.Arg(1) == "add" {
					return o.mcpAdd(ctx, cat, st, stdout, fs.Args()[2:])
				}
				return o.mcpCmd(ctx, stdout, st, fs.Args()[1:])
			}
			o.startMCP = true // the menu, on its MCP servers
		case "skills":
			args := fs.Args()[1:]
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			switch {
			case len(args) > 0 && args[0] == "add":
				return o.skillsAdd(ctx, cat, st, stdout, args[1:])
			case len(args) == 2 && (args[0] == "check" || args[0] == "delete" || args[0] == "restore"):
				r, _, err := o.addonRun(ctx, cat, st, stdout, false)
				if err != nil {
					return err
				}
				switch args[0] {
				case "check":
					r.ensureVeil(ctx)
					return r.checkSkill(ctx, args[1])
				case "restore":
					return r.restoreSkill(args[1])
				}
				if err := steps.DeleteSkill(r.env, args[1]); err != nil {
					return err
				}
				_ = addon.Forget(r.env.User, "skill", args[1])
				fmt.Fprintf(stdout, "✓ %s deleted.\n", args[1])
				return nil
			}
			return o.skillsCmd(stdout, st, args)
		case "addons":
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return o.addonsCmd(ctx, cat, st, stdout, fs.Args()[1:])
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
			return fmt.Errorf("unknown command %q (commands: status, remove, mcp, skills, addons, export, connect, gitkey, version, forget; see yardo --help)", fs.Arg(0))
		}
	}
	if !o.dryRun && !o.verify {
		if err := readOSRelease("/etc/os-release").supported(); err != nil {
			return err
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
	// What was set up before this run (the GitHub login is asked for
	// only when it's new).
	before := func(string) bool { return false }
	if had, err := plan.Resolve(cat, a.Picks, false); err == nil {
		before = planHas(had)
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
	first := len(a.Picks) == 0
	// based: what's new in the base for this machine, installed by this
	// run before anything else.
	var based []string
	if first {
		a.Base = catalog.BaseRev // the first run offers the base itself
	} else if a.Base < catalog.BaseRev {
		based = baseChange(cat, a, stderr)
		changed = true
	}
	ui.SetLook(cmp.Or(o.theme, a.Theme), steps.AccentHex(a.Accent))
	// stay: the menu (or the board) comes back after each run, until q;
	// lastErr is the last run's, returned then.
	stay := interactive && len(profs) == 0
	var lastErr error
	// upd: the check for newer versions, the last one's while it holds,
	// else one run in the background while the menu is up.
	var upd *updateCheck
	if stay && !first {
		if upd = loadUpdates(o.state, a.User, time.Now()); upd == nil {
			upd = o.checkUpdates(ctx, cat, a)
		}
	}
	// One program for every screen: app, showing d (read again after a
	// run, not as the screens change). open is the view to show when it
	// comes back (a menu screen, or the board), toast what it says.
	var d, shown *screen
	var app *ui.App
	var open, toast string
	// back is the view a run comes back to instead of the home
	// (Connection, after a login); link: Connection starts Tailscale's
	// login by link; prof: the profile to add once its client is
	// installed.
	var back string
	var link bool
	var prof *newProfile
	// done: how the last uninstall went, shown on the board until
	// dismissed.
	var done *uninstalled
	// chose: the first run's choices were asked; ready: its review said
	// Install (nothing more to ask); customized: it said Customize…,
	// what it chose queued on the board; firstAsk: its asker, with what
	// was asked.
	chose := false
	var ready *choices
	var customized []string
	var firstAsk *asker
	switch {
	case stay && o.startMCP && !first:
		open = "mcp"
	case stay && o.ui == "board" && !first:
		open = "board"
	}
	prep := func(x *ui.App) {
		x.Menu.Switching(o.mcpSwitcher(ctx, a))
		x.Menu.SwitchingSkills(o.skillSwitcher(a))
		x.Menu.Checker(func() func() ui.UpdatesMsg {
			upd = o.checkUpdates(ctx, cat, a)
			return upd.msg
		})
		x.Conn.Hooks(o.connHooks(ctx, a))
		if upd != nil && !upd.ready() {
			x.Menu.Checking(upd.msg)
		}
		x.Board.SetPlanner(func(ids, adds []string) map[string]ui.UninstallPlan { return o.plans(ctx, cat, a, ids, adds) })
	}
	for {
		var items []*catalog.Item
		extra := a.MiseExtra
		sec := creds.New()
		// fix: the items the board re-applies now (a fix, or after filling
		// in a value); board: what it queued, when that's applied.
		var fix []string
		var board *ui.BoardResult
		// mcpEdit: the MCP servers picked new, to fill in once the run has
		// made their env files; updates: what the board queued to update,
		// done last in the run.
		var mcpEdit, updates []string
		if based != nil && interactive {
			fix, based = based, nil // installed now, not left to fix
		}
		if stay && fix == nil {
		loop:
			for {
				if first && !chose {
					// A first run asks everything, then shows it all; Install
					// runs it, Customize… opens the board with it queued.
					firstAsk = &asker{cat: cat, a: a, share: o.share(), out: stdout, installed: func(t string) bool { return userHas(a.User, t) },
						port: func() int { p, _ := st.Port(user); return p }}
					c, adds, pick, err := firstAsk.firstRun(ctx)
					if errors.Is(err, ui.ErrQuit) {
						return lastErr // nothing saved
					} else if err != nil {
						return err
					}
					chose = true
					switch pick {
					case "Install":
						ready = c
						break loop
					case "Customize…":
						customized = adds
						firstAsk.asked, firstAsk.secrets = c.picks, c.secrets
					default:
						return lastErr
					}
				}
				if d == nil {
					if d, err = o.screenData(ctx, cat, a, first); err != nil {
						return err
					}
				}
				d.withUpdates(upd, o.lock)
				if app == nil || shown != d {
					if app == nil {
						app = ui.NewApp(d.appData(), prep)
					} else {
						app.Reload(d.appData())
					}
					shown = d
				}
				if customized != nil {
					for _, it := range app.Board.Items() {
						if !it.Picked && slices.Contains(customized, it.ID) {
							it.Pending = "add"
						}
					}
					customized, open = nil, "board"
				}
				if done != nil {
					app.Board.ShowResult(done.Title(), done.Lines, done.Failed)
					done, open = nil, "board"
				}
				if open != "" {
					app.Open(open)
					open = ""
				}
				if link {
					app.Conn.LinkLogin()
					link = false
				}
				app.Toast(toast)
				toast = ""
				out, err := app.Run()
				if err != nil {
					return err
				}
				switch {
				case out.Quit:
					return lastErr
				case out.Menu != nil:
					res := *out.Menu
					switch res.Kind {
					case ui.MenuChange:
						k := &asker{cat: cat, a: a, share: o.share(), out: stdout, installed: func(t string) bool { return userHas(a.User, t) }}
						before := *a
						if sec, err = k.change(ctx, res.Change); errors.Is(err, ui.ErrQuit) {
							*a, sec = before, creds.New()
							continue // back to Settings, nothing changed
						} else if err != nil {
							return err
						}
						fix, changed = []string{res.ID}, true
						break loop
					case ui.MenuUpdates:
						upd = o.checkUpdates(ctx, cat, a)
						app.Menu.Checking(upd.msg)
						toast = "Checking for updates in the background: what it finds shows here."
					case ui.MenuApply:
						// Session on login is a file of the user's: written, and
						// shown, without a run.
						if v, ok := res.Settings["session"]; ok {
							delete(res.Settings, "session")
							a.Session = strings.TrimPrefix(v, "ask")
							if !o.dryRun {
								env, err := newEnv(a.User, o.share(), io.Discard, false)
								if err != nil {
									return err
								}
								if err := steps.SetSession(env, v); err != nil {
									return err
								}
								if err := st.Save(a); err != nil {
									return err
								}
							}
							if len(res.Settings) == 0 {
								d, open, toast = nil, "settings", "Session on login: "+v+"."
								break
							}
						}
						// The editor theme is a file of the user's too: written
						// without a run; Vim and Neovim read it as they start.
						if v, ok := res.Settings["editortheme"]; ok {
							delete(res.Settings, "editortheme")
							a.EditorTheme = v
							if !o.dryRun {
								env, err := newEnv(a.User, o.share(), io.Discard, false)
								if err != nil {
									return err
								}
								if err := steps.SetEditorTheme(env, v); err != nil {
									return err
								}
								if err := st.Save(a); err != nil {
									return err
								}
							}
							if len(res.Settings) == 0 {
								d, open, toast = nil, "settings", "Editor theme: "+steps.EditorThemeName(v)+". Vim and Neovim use it from their next start."
								break
							}
						}
						// The theme is yardo's own: kept, and shown, without a run.
						if v, ok := res.Settings["theme"]; ok {
							delete(res.Settings, "theme")
							a.Theme, o.theme = v, ""
							ui.SetLook(a.Theme, steps.AccentHex(a.Accent))
							if !o.dryRun {
								if err := st.Save(a); err != nil {
									return err
								}
							}
							if len(res.Settings) == 0 {
								l, _ := tui.LookOf(v)
								d, open, toast = nil, "settings", "Theme: "+l.Name+"."
								break
							}
						}
						board = &ui.BoardResult{Kind: ui.BoardApply, Settings: res.Settings}
						break loop
					case ui.MenuSkills:
						r, err := (&asker{cat: cat, a: a, share: o.share(), out: stdout}).addons(ctx)
						if err == nil {
							toast, err = r.menuDo(ctx, "skill", res.MCP)
						}
						if err != nil {
							toast = "✗ " + err.Error()
						}
						d, open = nil, "skills"
						continue
					case ui.MenuMCP:
						k := &asker{cat: cat, a: a, share: o.share(), out: io.Discard, installed: func(t string) bool { return userHas(a.User, t) },
							port: func() int { p, _ := st.Port(user); return p }}
						out, err := k.mcpDo(ctx, res.MCP, o.mcpSpecs(ctx))
						if err != nil {
							toast, d = "✗ "+err.Error(), nil
							continue
						}
						if out.sync != "" {
							if err := st.Save(a); err != nil {
								return err
							}
							fmt.Fprintf(stdout, "Registering %s…\n", out.sync)
							_ = o.mcpSyncNow(ctx, a, io.Discard)
							s, _ := o.mcpStatus(ctx, a)
							v, ok := s[out.sync]
							toast, d = mcpSays(out.sync, mcpSpecOf(a.MCPServers, out.sync), v, ok), nil
							continue
						}
						if out.apply {
							// The gateway's install too: this version's catalog
							// and mcp-sync, and the gateway started.
							fix, changed, mcpEdit = []string{"mcpjungle-install", "mcpjungle"}, true, out.edit
							break loop
						}
						if err := st.Save(a); err != nil {
							return err
						}
						toast, d = out.toast, nil
					}
				case out.Conn != nil:
					c, err := o.connDo(ctx, a, *out.Conn, stdout)
					if err != nil {
						return err
					}
					open, toast, link = "connection", c.toast, c.link
					if c.fix != nil {
						if c.pick != "" && !slices.Contains(a.Picks, c.pick) {
							a.Pick([]string{c.pick})
						}
						fix, sec, changed, back, prof = c.fix, c.sec, true, "connection", c.prof
						break loop
					}
					d = nil
				case out.Board != nil:
					res := *out.Board
					switch res.Kind {
					case ui.BoardFix:
						// A tool's parts (its login) are fixed with it.
						fix = append([]string{res.ID}, partsOf(cat, a.Picks, res.ID)...)
						break loop
					case ui.BoardEdit:
						if fix, _, err = o.fillIn(ctx, a, res.Inputs); err != nil {
							return err
						}
						if fix != nil {
							break loop
						}
						d = nil
					case ui.BoardChange:
						k := &asker{cat: cat, a: a, share: o.share(), out: stdout, installed: func(t string) bool { return userHas(a.User, t) }}
						before := *a
						if sec, err = k.change(ctx, res.Change); errors.Is(err, ui.ErrQuit) {
							*a, sec = before, creds.New()
							continue // back to the board, nothing changed
						} else if err != nil {
							return err
						}
						fix, changed = append([]string{res.ID}, partsOf(cat, a.Picks, res.ID)...), true
						break loop
					case ui.BoardApply:
						// Uninstalls first; then the rest of the queue, if any.
						if len(res.Uninstalls) > 0 {
							if upd != nil && !upd.ready() {
								fmt.Fprintln(stdout, "Waiting for the check for updates to finish (it holds apt)…")
							}
							upd.wait(io.Discard)
							r, err := o.uninstall(ctx, cat, st, a, res.Uninstalls, stdout)
							if err != nil {
								return err
							}
							if len(res.Adds) == 0 && len(res.Forgets) == 0 && len(res.Settings) == 0 && len(res.Updates) == 0 && len(res.Fixes) == 0 {
								done, d = &r, nil // back to the board, checked again
								continue
							}
							for _, l := range r.Lines {
								fmt.Fprintln(stdout, l)
							}
						}
						updates = res.Updates
						if len(res.Adds)+len(res.Forgets)+len(res.Settings) == 0 && len(updates)+len(res.Fixes) > 0 {
							// Only fixes and updates: those items (a tool's parts with
							// it), applied again, then the updates. With more queued,
							// the full run applies the fixes with everything else.
							fix = slices.Clone(updates)
							for _, id := range res.Fixes {
								for _, f := range append([]string{id}, partsOf(cat, a.Picks, id)...) {
									if !slices.Contains(fix, f) {
										fix = append(fix, f)
									}
								}
							}
							break loop
						}
						board = &res
						break loop
					}
				}
			}
		}
		upd.wait(stdout)
		err := func() error {
			if interactive && fix == nil {
				k := &asker{cat: cat, a: a, share: o.share(), out: stdout, installed: func(t string) bool { return userHas(a.User, t) },
					profile: len(profs) > 0, dropped: dropped, port: func() int { p, _ := st.Port(user); return p }, promptSet: chose}
				var c *choices
				var err error
				switch {
				case ready != nil:
					// The first run's review said Install: asked already.
					c, ready = ready, nil
				case board != nil && firstAsk != nil:
					// Customize…: what the board added is asked, then the
					// review again.
					k.asked, k.secrets = firstAsk.asked, firstAsk.secrets
					if c, err = k.fromBoard(ctx, *board, first); err == nil && k.review(c, "Install", "Quit") != "Install" {
						return errQuitFirst
					}
					if errors.Is(err, ui.ErrQuit) {
						return errQuitFirst
					}
				case board != nil:
					c, err = k.fromBoard(ctx, *board, first)
				default:
					c, err = k.ask(ctx)
				}
				if errors.Is(err, ui.ErrQuit) {
					fmt.Fprintln(stdout, "Cancelled.")
					return errCancelled
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
				ui.SetLook(cmp.Or(o.theme, a.Theme), steps.AccentHex(a.Accent)) // a new one shows from now on
				items, extra, sec = c.items, c.miseExtra, c.secrets
				changed = true
			} else {
				if o.only != "" {
					a.Pick(o.giveKey(a, sec))
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
					// What the board re-applies, and the helpers (mise) it
					// needs; extra mise tools only when one of them is listed.
					items = slices.DeleteFunc(items, func(it *catalog.Item) bool { return !it.Hidden() && !slices.Contains(fix, it.ID) })
					if !slices.ContainsFunc(fix, func(id string) bool { return strings.HasPrefix(id, "mise:") }) {
						extra = nil
					}
				}
				listDropped(stdout, cat, dropped)
			}
			// ~/pages' port is the gateway's plus 1000: allocated with it.
			if a.MCPPort == 0 && slices.ContainsFunc(items, func(it *catalog.Item) bool { return it.ID == "mcpjungle-install" || it.ID == "pages" }) {
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
			// Fixed or applied again on purpose: git-sourced tools are fetched again.
			env.Reinstall = fix != nil
			if created {
				env.Note(fmt.Sprintf("Created user %s (passwordless sudo, no password or SSH key): add a key to %s/.ssh/authorized_keys, or use 'sudo -iu %s'.", a.User, env.Home, a.User))
			}
			rec := &run.Record{P: progressFor(stdout)}
			var prog run.Progress = rec
			bash := legacyRunner(env, o.legacy, o.state)
			if bash != nil {
				stdin := sec.Encode()
				env.Legacy = func(ctx context.Context, id string) error {
					// The GitHub steps may open the browser login: they get the
					// terminal when one of them is new or being fixed, not on
					// every apply (else gh waits for a login nobody asked for).
					if interactive && (id == "github" || id == "gitsign") && ghLogin(items, before, fix) {
						rec.Pause()
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
			}
			// The keys file is kept across runs (Connection lists what's
			// in it); keysBefore: the keys there before this run.
			keysBefore := readKeys(o.state)
			r := &run.Runner{Env: env, Progress: prog, TailLines: 25}
			if zshPrime(env) {
				r.More = 1
			}
			if len(updates) > 0 {
				r.More++
			}
			res := r.Run(ctx, items, extra)
			if len(updates) > 0 {
				if err := o.updateStep(ctx, r, env, cat, updates); err != nil {
					res.Failed = append(res.Failed, "updates")
					res.Errors["updates"] = err.Error()
				} else {
					_ = upd.drop(updates, o.state, a.User)
				}
			}
			if err := markApplied(o.state, a.User, slices.Concat(res.Changed, res.Unchanged), time.Now()); err != nil {
				fmt.Fprintf(stderr, "yardo: %v\n", err)
			}
			if err := primeZshCompletions(ctx, env, r); err != nil {
				res.Failed = append(res.Failed, "zshcomp")
				res.Errors["zshcomp"] = err.Error()
			}
			// While bash runs steps, its notes (with "Secrets to fill in") are the
			// fuller ~/yardo.txt.
			// last: what the run did, for the home's Last run.
			last := &lastRun{When: time.Now(), OK: res.OK(), Failed: res.Failed}
			if bash != nil {
				if out, err := bashOut(ctx, env, o.legacy, o.state, "--finish"); err != nil {
					fmt.Fprintf(stderr, "yardo: notes: %v\n", err)
				} else {
					_, _ = stdout.Write(out)
					if t := strings.TrimSpace(string(out)); t != "" {
						last.Notes = append(last.Notes, strings.Split(t, "\n")...)
					}
					if err := appendUnverified(env, items); err != nil {
						fmt.Fprintf(stderr, "yardo: %s/yardo.txt: %v\n", env.Home, err)
					}
				}
			} else {
				if b, err := os.ReadFile(notesFile); err == nil && len(b) > 0 {
					for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
						fmt.Fprintf(stdout, "! %s\n", l)
						last.Notes = append(last.Notes, l)
					}
				}
				if o.only != "" {
					// One item: the summary of everything stays as it is.
				} else if err := writeSummary(env, items, extra); err != nil {
					fmt.Fprintf(stderr, "yardo: %s/yardo.txt: %v\n", env.Home, err)
				}
			}
			if len(mcpEdit) > 0 && !slices.Contains(res.Failed, "mcpjungle") {
				if err := o.fillMCP(ctx, r, a, items, mcpEdit, stdout); err != nil {
					fmt.Fprintf(stderr, "yardo: %v\n", err)
				}
			}
			fmt.Fprintf(stdout, "How each tool was installed and how to update it: %s/yardo.txt\nLog: %s\n", env.Home, o.log)
			printKeys(stdout, newKeys(keysBefore, readKeys(o.state)), env.Home)
			hint := func(l string) {
				fmt.Fprintln(stdout, l)
				last.Hints = append(last.Hints, l)
			}
			if planHas(items)("mcpjungle") && !before("mcpjungle") && len(a.MCPServers) == 0 {
				hint("MCP servers: add GitHub, Jira, Grafana and more from 'sudo yardo' → MCP servers.")
			}
			// Newly set up Neovim or tmux: how their copies reach the laptop.
			if has := planHas(items); (has("neovim") && !before("neovim")) || (has("tmux") && !before("tmux")) {
				for _, l := range clipboardHint(os.Getenv("LC_TERMINAL")) {
					hint(ui.Unmark(l))
				}
			}
			if h := newShellHint(res.Changed); h != "" {
				hint(h)
			}
			last.Lines = failTails(cat, rec.Lines, res)
			if err := last.save(o.state, a.User); err != nil {
				fmt.Fprintf(stderr, "yardo: %v\n", err)
			}
			if !res.OK() {
				for _, id := range res.Failed {
					fmt.Fprintf(stderr, "✗ %s: %s\n", id, res.Errors[id])
				}
				return &runFailed{ids: res.Failed}
			}
			return nil
		}()
		if !stay || o.dryRun || o.verify || errors.Is(err, errQuitFirst) {
			if errors.Is(err, errCancelled) || errors.Is(err, errQuitFirst) {
				return nil
			}
			return err
		}
		// Back to the menu (or the board), saying how it went; what's set up
		// now is checked again.
		toast, lastErr = ranSays(cat, fix, updates, err), err
		if errors.Is(err, errCancelled) {
			lastErr = nil
		}
		if had, err := plan.Resolve(cat, a.Picks, false); err == nil {
			before = planHas(had)
		}
		changed, dropped, d, firstAsk = false, nil, nil, nil
		first = len(a.Picks) == 0
		chose = chose && !first
		open = "home"
		if o.ui == "board" {
			open = "board"
		}
		if back != "" {
			open, back = back, ""
		}
		if prof != nil {
			// Its client is installed now (or the run said why not).
			if err == nil {
				toast = o.addProfile(ctx, a, *prof, stdout)
			}
			prof = nil
		}
	}
}

// errCancelled is a run left at its questions: nothing changed.
var errCancelled = errors.New("cancelled")

// errQuitFirst is a first run quit at its questions or its review: yardo
// ends, nothing saved.
var errQuitFirst = errors.New("quit")

// runFailed is a run in which ids failed.
type runFailed struct{ ids []string }

func (e *runFailed) Error() string {
	return "failed: " + strings.Join(e.ids, " ") + " — fix and re-run yardo"
}

// ranSays is the home's line after a run of fix (or of what was queued),
// with the tools updated after it, which ended in err: what it did, or
// what failed.
func ranSays(cat *catalog.Catalog, fix, updated []string, err error) string {
	names := func(ids []string) string {
		var out []string
		for _, id := range ids {
			if it := cat.Item(id); it == nil {
				out = append(out, strings.TrimPrefix(id, "mise:"))
			} else if !it.Hidden() {
				out = append(out, ui.Short(it.Label))
			}
		}
		return strings.Join(out, ", ")
	}
	var failed *runFailed
	switch {
	case errors.Is(err, errCancelled):
		return "Cancelled: nothing changed."
	case errors.As(err, &failed):
		return "✗ " + names(failed.ids) + " failed · Last run shows what happened"
	case err != nil:
		msg := err.Error()
		return "✗ " + strings.ToUpper(msg[:1]) + msg[1:]
	}
	if n := names(updated); n != "" {
		return "✓ Updated " + n
	}
	if n := names(fix); n != "" {
		return "✓ Done: " + n
	}
	return "✓ Done"
}

// gitKeyArgs reads `yardo gitkey [github|gitlab|bitbucket] [--host h]...
// [--replace]`: the private key on stdin (ssh host 'sudo yardo gitkey' <
// key) for the sudo user (or --user), set up alone, as with --yes.
func (o *options) gitKeyArgs(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("yardo gitkey", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var hosts list
	fs.Var(&hosts, "host", "a git host to use the key for: github.com, bitbucket.org, your GitLab or another (repeatable; default github.com)")
	replace := fs.Bool("replace", false, "replace the key in ~/.ssh/identities (the old one is moved aside)")
	var providers []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if args = fs.Args(); len(args) == 0 {
			break
		}
		if !slices.Contains(answers.GitProviders, args[0]) {
			return fmt.Errorf("gitkey: %q: want github, gitlab or bitbucket (or --host HOST)", args[0])
		}
		providers, args = answers.Merge(providers, args[:1]), args[1:]
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
	o.yes, o.only, o.gitKey, o.gitHosts, o.gitProviders, o.gitReplace = true, "gitkey", key, hosts, providers, *replace
	return nil
}

// giveKey sets up the key sent to yardo gitkey: for the git host items
// named or reached at its hosts (GitHub when none), in
// ~/.ssh/identities; for other hosts, ~/.ssh/yardo_git_key. It returns
// the items to pick.
func (o *options) giveKey(a *answers.Answers, sec *creds.Secrets) []string {
	var picks []string
	if len(o.gitHosts) > 0 || len(o.gitProviders) == 0 {
		picks = a.GiveKeyTo(o.gitHosts)
	}
	for _, id := range o.gitProviders {
		a.GitHostOf(id).Key = answers.IdentityKey(id)
	}
	for _, id := range answers.Merge(picks, o.gitProviders) {
		if a.GitHostOf(id) == nil {
			sec.GitKey = o.gitKey
			continue
		}
		sec.GitKeys[id], sec.GitReplace[id] = o.gitKey, o.gitReplace
	}
	return answers.Merge(picks, o.gitProviders)
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

// zshPrime reports whether primeZshCompletions has work: the completions
// file and zsh are there.
func zshPrime(env *steps.Env) bool {
	if _, err := os.Stat(env.Path("~/.config/yardo/completions.zsh")); err != nil {
		return false
	}
	_, err := exec.LookPath("zsh")
	return err == nil
}

// primeZshCompletions fills the zsh completion cache once every tool of
// the run is installed, so the first login doesn't pay for it: one of
// r's steps.
func primeZshCompletions(ctx context.Context, env *steps.Env, r *run.Runner) error {
	if !zshPrime(env) {
		return nil
	}
	const title = "zsh completions for installed tools"
	r.Step(title)
	_, err := env.RunLogged(ctx, steps.Cmd{Name: "zsh", Args: []string{"-c", "source ~/.config/yardo/completions.zsh && yardo_comp_refresh"}, AsUser: true})
	if err != nil {
		r.Progress.Done("fail", title)
		return err
	}
	r.Progress.Done("ok", title)
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
	if o.exec != nil {
		env.Exec, env.Root = o.exec, o.root
	}
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

// keysFile holds the public keys runs made (host, where to add it, file,
// key per line): printed after the run that made them, and listed on
// Connection until added or dismissed.
const keysFile = ".keys"

// newKeys are the keys in now that weren't in before: this run's.
func newKeys(before, now []ui.AddKey) []ui.AddKey {
	var out []ui.AddKey
	for _, k := range now {
		if !slices.ContainsFunc(before, func(b ui.AddKey) bool { return b.Pub == k.Pub && slices.Equal(b.Where, k.Where) }) {
			out = append(out, k)
		}
	}
	return out
}

// printKeys shows the run's new public keys after everything else: where
// to add each, then the key alone on a line between blank lines, so a
// triple-click copies exactly it. (Connection lists them too, until
// they're added.)
func printKeys(w io.Writer, keys []ui.AddKey, home string) {
	for _, k := range keys {
		path := k.Path
		if rest, ok := strings.CutPrefix(path, "~/"); ok {
			path = filepath.Join(home, rest)
		}
		fmt.Fprintf(w, "\nSSH key to add\n")
		for _, wh := range k.Where {
			fmt.Fprintf(w, "  %s\n", wh)
		}
		fmt.Fprintf(w, "\n%s\n\nShow it again: cat %s\n", k.Pub, path)
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
	name := fs.String("name", "", "name the profile, so applying a changed one drops the picks it no longer lists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name != "" && !profile.ValidName(*name) {
		return fmt.Errorf("export: --name %q: want letters, digits, '.', '_' or '-'", *name)
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
	b := profile.Export(a, l, *name)
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
// forget isn't ported yet.
func bashHandles(args []string) bool {
	return slices.Contains(args, "forget")
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
	// Keep a forwarded agent, the user's editor, and a proxy their shell has.
	var keep []string
	// SSH_CONNECTION and SSH_TTY tell Connection the network this session
	// came by; TMUX, how to copy through to the laptop.
	for _, k := range []string{"SSH_AUTH_SOCK", "SSH_CONNECTION", "SSH_TTY", "TMUX", "LC_TERMINAL", "VISUAL", "EDITOR", "http_proxy", "https_proxy", "no_proxy", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		if os.Getenv(k) != "" {
			keep = append(keep, k)
		}
	}
	if len(keep) > 0 {
		argv = append(argv, "--preserve-env="+strings.Join(keep, ","))
	}
	return syscall.Exec(sudo, append(append(argv, self), args...), os.Environ())
}

// fillMCP opens each new MCP server's env file (its spec's skeleton: the
// settings, the optional ones commented out) in the editor, then applies
// the MCP servers again, which registers those filled in.
func (o *options) fillMCP(ctx context.Context, r *run.Runner, a *answers.Answers, items []*catalog.Item, names []string, stdout io.Writer) error {
	k := &asker{a: a, out: stdout}
	for _, n := range names {
		env, says, ok, err := k.askPick(ctx, mcpSpecOf(a.MCPServers, n), n)
		if err != nil {
			return err
		}
		if ok {
			if err := k.setEnv(ctx, n, env); err != nil {
				return err
			}
			fmt.Fprintln(stdout, says)
			continue
		}
		fmt.Fprintf(stdout, "\n%s: its settings open in the editor. Fill in the values, uncomment the options you want, save and quit. (Later: sudo yardo mcp edit %s)\n", n, n)
		if err := editMCP(ctx, a.User, n); err != nil {
			return err
		}
	}
	mcpItems := slices.DeleteFunc(slices.Clone(items), func(it *catalog.Item) bool { return it.ID != "mcpjungle" })
	if len(mcpItems) > 0 {
		_ = r.Run(ctx, mcpItems, nil) // how each went is said below
	}
	s, _ := o.mcpStatus(ctx, a)
	for _, n := range names {
		v, ok := s[n]
		fmt.Fprintln(stdout, mcpSays(n, mcpSpecOf(a.MCPServers, n), v, ok))
	}
	return nil
}

// ghLogin reports whether this run logs gh in: GitHub or commit signing
// is new in the plan (not set up before), or being fixed.
func ghLogin(items []*catalog.Item, before func(string) bool, fix []string) bool {
	has := planHas(items)
	for _, id := range []string{"github", "gitsign"} {
		if (has(id) && !before(id)) || slices.Contains(fix, id) {
			return true
		}
	}
	return false
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
