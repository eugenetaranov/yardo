package steps

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
)

func put(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLineAppendsOnce(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	p := e.Path("~/.bashrc")
	put(t, p, "export A=1\n")
	st := step(t, catalog.Step{Type: "line", To: "~/.bashrc", Line: ". ~/x"})
	if check(t, e, st) {
		t.Fatal("Check before Apply = true")
	}
	apply(t, e, st)
	apply(t, e, st)
	// As bash's add_dropin: after a blank line.
	if got := read(t, p); got != "export A=1\n\n. ~/x\n" {
		t.Errorf(".bashrc = %q", got)
	}
	if !check(t, e, st) {
		t.Error("Check after Apply = false")
	}
}

func TestLineMissingFile(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	st := step(t, catalog.Step{Type: "line", To: "~/.zshrc", Line: ". ~/x", IfExists: true})
	if !check(t, e, st) {
		t.Error("if_exists: a missing file has nothing to do")
	}
	apply(t, e, st)
	if _, err := os.Stat(e.Path("~/.zshrc")); err == nil {
		t.Error("if_exists created the file")
	}
	st = step(t, catalog.Step{Type: "line", To: "~/.bashrc", Line: ". ~/x"})
	apply(t, e, st)
	if got := read(t, e.Path("~/.bashrc")); got != ". ~/x\n" {
		t.Errorf(".bashrc = %q", got)
	}
}

func TestLineMatchReplaces(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	p := e.Path("~/.zshrc")
	put(t, p, "a\nZSH_THEME=\"robbyrussell\"\nb\n")
	st := step(t, catalog.Step{Type: "line", To: "~/.zshrc", Match: "^ZSH_THEME=", Line: `ZSH_THEME="p"`})
	apply(t, e, st)
	if got := read(t, p); got != "a\nZSH_THEME=\"p\"\nb\n" {
		t.Errorf(".zshrc = %q", got)
	}
	if !check(t, e, st) {
		t.Error("Check after Apply = false")
	}
}

func shellEnv(t *testing.T) *Env {
	e := testEnv(t, &fakeExec{})
	e.Files = os.DirFS("../assets/conf")
	return e
}

func TestZshrcTheme(t *testing.T) {
	e := shellEnv(t)
	st := step(t, catalog.Step{Type: "go:zshrc"})
	apply(t, e, st)
	if got := read(t, e.Path(yardoZshrc)); !strings.Contains(got, `ZSH_THEME="robbyrussell"`) || !strings.HasPrefix(got, "# yardo's zsh setup") {
		t.Errorf("no default theme or header in\n%s", got)
	}
	if !check(t, e, st) {
		t.Error("Check after Apply = false")
	}
	// Once p10k is cloned, re-rendering keeps its theme.
	if err := os.MkdirAll(e.Path(p10kDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if check(t, e, st) {
		t.Error("Check = true with p10k cloned and the default theme")
	}
	apply(t, e, st)
	if got := read(t, e.Path(yardoZshrc)); !strings.Contains(got, `ZSH_THEME="powerlevel10k/powerlevel10k"`) {
		t.Errorf("no p10k theme in\n%s", got)
	}
}

// ~/.zshrc is the user's: yardo keeps its block first and leaves the rest.
func TestZshrcIsTheUsers(t *testing.T) {
	e := shellEnv(t)
	st := step(t, catalog.Step{Type: "go:zshrc"})
	p := e.Path(zshrc)
	// No .zshrc: created, the block alone.
	apply(t, e, st)
	if got := read(t, p); got != zshrcBlock {
		t.Errorf("new .zshrc = %q", got)
	}
	// An alias added below survives a re-run, and isn't drift.
	put(t, p, zshrcBlock+"\nalias k=kubectl\n")
	if !check(t, e, st) {
		t.Error("Check = false with the user's alias")
	}
	apply(t, e, st)
	if got := read(t, p); got != zshrcBlock+"\nalias k=kubectl\n" {
		t.Errorf("re-run changed .zshrc: %q", got)
	}
	// The block deleted: back on top, the rest unchanged.
	put(t, p, "alias k=kubectl\nexport EDITOR=hx\n")
	if check(t, e, st) {
		t.Error("Check = true without the block")
	}
	apply(t, e, st)
	if got := read(t, p); got != zshrcBlock+"\nalias k=kubectl\nexport EDITOR=hx\n" {
		t.Errorf(".zshrc = %q", got)
	}
	for _, f := range []string{"~/.zshrc.orig", zshrcSaved} {
		if exists(e.Path(f)) {
			t.Errorf("%s made", f)
		}
	}
}

// A .zshrc of the user's own on a new install: kept under the block, no
// .orig; one that loads oh-my-zsh itself gets a note, once.
func TestZshrcOwnKept(t *testing.T) {
	e := shellEnv(t)
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	st := step(t, catalog.Step{Type: "go:zshrc"})
	var own strings.Builder
	for i := range 40 {
		fmt.Fprintf(&own, "export V%d=%d\n", i, i)
	}
	put(t, e.Path(zshrc), own.String())
	apply(t, e, st)
	if got := read(t, e.Path(zshrc)); got != zshrcBlock+"\n"+own.String() {
		t.Errorf(".zshrc = %q", got)
	}
	if exists(e.Path("~/.zshrc.orig")) || len(notes) != 0 {
		t.Errorf(".orig made, or notes %q", notes)
	}
	put(t, e.Path(zshrc), "export ZSH=~/.oh-my-zsh\nsource $ZSH/oh-my-zsh.sh\n")
	apply(t, e, st)
	if len(notes) != 1 || !strings.Contains(notes[0], "loads twice") {
		t.Errorf("notes %q", notes)
	}
	apply(t, e, st)
	if len(notes) != 1 {
		t.Errorf("noted again: %q", notes)
	}
}

// A .zshrc an older yardo wrote (the template's marker, no block) is
// saved once and replaced by the block, with a note; a second run saves
// and notes nothing. Drop-in lines an older yardo appended to the user's
// own file go.
func TestZshrcMovedOver(t *testing.T) {
	e := shellEnv(t)
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	st := step(t, catalog.Step{Type: "go:zshrc"})
	old := "export ZSH=\"$HOME/.oh-my-zsh\"\nalias mine=1\n\n" + Dropins[0].Line + "\n\n" + LastMark + "\n[[ -r ~/.zshrc.local ]] && source ~/.zshrc.local\n"
	put(t, e.Path(zshrc), old)
	apply(t, e, st)
	if got := read(t, e.Path(zshrc)); got != zshrcBlock {
		t.Errorf(".zshrc = %q", got)
	}
	if got := read(t, e.Path(zshrcSaved)); got != old {
		t.Errorf("saved = %q", got)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "~/.zshrc.before-yardo-split") {
		t.Errorf("notes %q", notes)
	}
	apply(t, e, st)
	if len(notes) != 1 || read(t, e.Path(zshrc)) != zshrcBlock || !check(t, e, st) {
		t.Errorf("second run: notes %q, .zshrc %q", notes, read(t, e.Path(zshrc)))
	}
	// The user's own file, with lines an older yardo appended.
	put(t, e.Path(zshrc), "alias k=kubectl\n\n"+Dropins[0].Line+"\n\n"+Dropins[3].Line+"\n")
	apply(t, e, st)
	if got := read(t, e.Path(zshrc)); got != zshrcBlock+"\nalias k=kubectl\n" {
		t.Errorf(".zshrc = %q", got)
	}
}

// A .zshrc linked into a dotfiles repo isn't written through: the run
// notes the lines to add.
func TestZshrcLinked(t *testing.T) {
	e := shellEnv(t)
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	st := step(t, catalog.Step{Type: "go:zshrc"})
	repo := e.Path("~/.dotfiles/zsh/.zshrc")
	put(t, repo, "alias k=kubectl\n")
	if err := os.Symlink(repo, e.Path(zshrc)); err != nil {
		t.Fatal(err)
	}
	apply(t, e, st)
	if got := read(t, repo); got != "alias k=kubectl\n" {
		t.Errorf("wrote through the link: %q", got)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], repo) || !strings.Contains(notes[0], zshSourceLine) {
		t.Errorf("notes %q", notes)
	}
	if !exists(e.Path(yardoZshrc)) {
		t.Error("yardo's zsh file not written")
	}
	put(t, repo, zshSourceLine+"\nalias k=kubectl\n")
	if !check(t, e, st) {
		t.Error("Check = false with the line in the linked file")
	}
}

// Starship: no oh-my-zsh theme, starship's init instead of ~/.p10k.zsh,
// even with p10k cloned; the prompt step installs starship, not p10k.
func TestZshrcStarship(t *testing.T) {
	x := &fakeExec{}
	e := shellEnv(t)
	e.Exec = x
	e.Answers = &answers.Answers{Prompt: "starship"}
	if err := os.MkdirAll(e.Path(p10kDir), 0o755); err != nil {
		t.Fatal(err)
	}
	apply(t, e, step(t, catalog.Step{Type: "go:zshrc"}))
	got := read(t, e.Path(yardoZshrc))
	if !strings.Contains(got, "ZSH_THEME=\"\"\n") || !strings.Contains(got, `eval "$(starship init zsh)"`) {
		t.Errorf("yardo's zshrc:\n%s", got)
	}
	if zsh, err := exec.LookPath("zsh"); err == nil {
		if out, err := exec.Command(zsh, "-n", e.Path(yardoZshrc)).CombinedOutput(); err != nil {
			t.Errorf("zsh -n: %v\n%s", err, out)
		}
	}
	pr := step(t, catalog.Step{Type: "go:prompt"})
	if plan := fmt.Sprint(pr.Plan(e)); !strings.Contains(plan, "starship") || strings.Contains(plan, "powerlevel10k") {
		t.Errorf("starship plan = %s", plan)
	}
	apply(t, e, pr)
	if !strings.Contains(read(t, e.Path("~/.config/starship.toml")), "format") {
		t.Error("no starship.toml")
	}
	e.Answers.Prompt = ""
	if plan := fmt.Sprint(pr.Plan(e)); !strings.Contains(plan, "powerlevel10k") || strings.Contains(plan, "starship") {
		t.Errorf("p10k plan = %s", plan)
	}
}

// With the zsh config, yardo's zsh file sources the drop-ins and .zshrc
// has only the block; without it (zsh alone), the block holds them. No
// .zshrc (bash): nothing. Once each step has run, none has more to do.
func TestZshDropins(t *testing.T) {
	e := shellEnv(t)
	dropin := step(t, catalog.Step{Type: "go:zshdropin"})
	put(t, e.Path(Dropins[0].File), "# shell\n")
	apply(t, e, dropin)
	if exists(e.Path(zshrc)) || !check(t, e, dropin) {
		t.Error("no .zshrc: made one, or not done")
	}
	// zsh alone.
	put(t, e.Path(zshrc), "")
	if check(t, e, dropin) {
		t.Error("Check = true without the block")
	}
	apply(t, e, dropin)
	if got, want := read(t, e.Path(zshrc)), zshBlock([]string{Dropins[0].Line}); got != want {
		t.Errorf(".zshrc = %q, want %q", got, want)
	}
	put(t, e.Path(Dropins[3].File), "# helpers\n")
	apply(t, e, dropin)
	if got, want := read(t, e.Path(zshrc)), zshBlock([]string{Dropins[0].Line, Dropins[3].Line}); got != want {
		t.Errorf(".zshrc = %q, want %q", got, want)
	}
	// The zsh config: the drop-ins move into yardo's file.
	zshrcSt := step(t, catalog.Step{Type: "go:zshrc"})
	apply(t, e, zshrcSt)
	if got := read(t, e.Path(zshrc)); got != zshrcBlock {
		t.Errorf(".zshrc = %q", got)
	}
	put(t, e.Path(Dropins[1].File), "# agent\n")
	if check(t, e, dropin) {
		t.Error("Check = true with a new drop-in yardo's file lacks")
	}
	apply(t, e, dropin)
	y := read(t, e.Path(yardoZshrc))
	for _, i := range []int{0, 1, 3} {
		if !strings.Contains(y, Dropins[i].Line) {
			t.Errorf("yardo's file lacks %q", Dropins[i].Line)
		}
	}
	for name, st := range map[string]Step{"zshrc": zshrcSt, "dropin": dropin} {
		if !check(t, e, st) {
			t.Errorf("%s: Check = false once settled", name)
		}
	}
	if !catalog.GoUndone["zshdropin"] || !catalog.GoUndone["zshrc"] {
		t.Error("zsh's steps don't undo themselves")
	}
}

// A new zsh, in a temp HOME with a stand-in oh-my-zsh: the user's lines
// below the block win, and ~/.zshrc.local still loads.
func TestZshrcOrder(t *testing.T) {
	z, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	e := shellEnv(t)
	put(t, e.Path("~/.oh-my-zsh/oh-my-zsh.sh"), "alias k=omz\n")
	put(t, e.Path(Dropins[0].File), "alias k=dropin\nexport FROM_DROPIN=1\n")
	put(t, e.Path("~/.zshrc.local"), "export FROM_LOCAL=1\n")
	put(t, e.Path(zshrc), "alias k=kubectl\nexport EDITOR=hx\n")
	apply(t, e, step(t, catalog.Step{Type: "go:zshrc"}))
	home := e.Path("~")
	cmd := exec.Command(z, "-ic", `print -r -- "$EDITOR|$FROM_DROPIN|$FROM_LOCAL|$aliases[k]"`)
	cmd.Env = []string{"HOME=" + home, "ZDOTDIR=" + home, "PATH=/usr/bin:/bin", "TERM=dumb"}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("zsh: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); !strings.HasSuffix(got, "hx|1|1|kubectl") {
		t.Errorf("got %q, want hx|1|1|kubectl", got)
	}
}

func TestLineTop(t *testing.T) {
	e := testEnv(t, &fakeExec{})
	p := e.Path("~/.bashrc")
	put(t, p, "case $- in *i*) ;; *) return;; esac\n")
	st := step(t, catalog.Step{Type: "line", To: "~/.bashrc", Line: "export PATH=x", Top: true})
	apply(t, e, st)
	apply(t, e, st)
	if got := read(t, p); got != "export PATH=x\ncase $- in *i*) ;; *) return;; esac\n" {
		t.Errorf(".bashrc = %q", got)
	}
}

func TestAptKernel(t *testing.T) {
	x := &fakeExec{}
	e := testEnv(t, x)
	e.Kernel = "6.8.0-45-generic"
	st := step(t, catalog.Step{Type: "apt", Packages: []string{"linux-tools-{{kernel}}"}})
	apply(t, e, st)
	if got := x.commands(); len(got) != 1 || !strings.HasSuffix(got[0], " install linux-tools-6.8.0-45-generic") {
		t.Errorf("ran %q", got)
	}
}

// The local override files are sourced last: drop-ins go before the
// block that sources ~/.zshrc.local in yardo's zsh file, and each config
// file yardo writes ends by sourcing its local file.
func TestLocalOverridesLast(t *testing.T) {
	e := shellEnv(t)
	put(t, e.Path("~/.config/yardo/shell.sh"), "")
	put(t, e.Path("~/.config/yardo/agent-env.sh"), "")
	apply(t, e, step(t, catalog.Step{Type: "go:zshrc"}))
	got := read(t, e.Path(yardoZshrc))
	last := "[[ -r ~/.zshrc.local ]] && source ~/.zshrc.local\n"
	if !strings.HasSuffix(got, last) {
		t.Errorf("yardo's zshrc doesn't end by sourcing .zshrc.local:\n%s", got)
	}
	mark := strings.Index(got, LastMark)
	for _, l := range []string{Dropins[0].Line, Dropins[1].Line} {
		if i := strings.Index(got, l); i < 0 || i > mark {
			t.Errorf("%q isn't before the local overrides:\n%s", l, got)
		}
	}
	for f, want := range map[string]string{
		"shell.sh":  "[ -r ~/.config/yardo/local.sh ] && . ~/.config/yardo/local.sh\n",
		"tmux.conf": "source-file -q ~/.tmux.local.conf\n",
		"vimrc":     "if filereadable(expand('~/.vimrc.local'))\n  source ~/.vimrc.local\nendif\n",
	} {
		b, err := fs.ReadFile(e.Files, f)
		if err != nil || !strings.HasSuffix(string(b), want) {
			t.Errorf("%s doesn't end with %q (%v)", f, want, err)
		}
	}
}

func TestWithLineBeforeMark(t *testing.T) {
	b := []byte("a\n\n" + LastMark + "\nsource local\n")
	if got, want := string(WithLine(b, "x", "")), "a\n\nx\n\n"+LastMark+"\nsource local\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := WithLine(b, "source local", ""); string(got) != string(b) {
		t.Errorf("a line already there moved: %q", got)
	}
}

func TestDotfiles(t *testing.T) {
	x := &fakeExec{}
	e := testEnv(t, x)
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	st := step(t, catalog.Step{Type: "go:dotfiles"})
	e.Answers = &answers.Answers{}
	apply(t, e, st)
	if len(notes) != 1 || len(x.ran) != 0 {
		t.Fatalf("no repo: notes %q, ran %q", notes, x.commands())
	}
	e.Answers.Dotfiles = "https://github.com/ada/dotfiles"
	// The clone (the fake doesn't make one): then stow, no install script.
	x.reply = func(c Cmd) ([]byte, error) {
		if c.Name == "git" && c.Args[0] == "clone" {
			put(t, e.Path("/home/ada/.dotfiles/.git/HEAD"), "ref")
			put(t, e.Path("/home/ada/.dotfiles/zsh/.zshrc.local"), "")
			put(t, e.Path("/home/ada/.dotfiles/git/.gitconfig"), "")
			put(t, e.Path("/home/ada/.dotfiles/README.md"), "")
		}
		return nil, nil
	}
	apply(t, e, st)
	want := []string{"(as user) git clone -- https://github.com/ada/dotfiles /home/ada/.dotfiles", "(as user) stow -d /home/ada/.dotfiles -t /home/ada --restow git zsh"}
	if got := x.commands(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("ran\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Next run: pull, and the install script wins over stow.
	x.ran = nil
	put(t, e.Path("/home/ada/.dotfiles/bootstrap.sh"), "#!/bin/sh\n")
	apply(t, e, st)
	got := x.commands()
	if len(got) != 2 || got[0] != "(as user) git -C /home/ada/.dotfiles pull --ff-only" || !strings.HasSuffix(got[1], " _ /home/ada/.dotfiles bootstrap.sh") {
		t.Errorf("ran %q", got)
	}
}

// The accent colour reaches the prompt (yardo's own config, rewritten when
// the accent changes; the user's own left alone) and tmux.
func TestAccent(t *testing.T) {
	e := shellEnv(t)
	e.Answers = &answers.Answers{Accent: "lagoon"}
	pr := step(t, catalog.Step{Type: "go:prompt"})
	apply(t, e, pr)
	p10k := read(t, e.Path("~/.p10k.zsh"))
	if !strings.Contains(p10k, "POWERLEVEL9K_DIR_FOREGROUND='#2BB3C0'") || !strings.HasPrefix(p10k, "# Powerlevel10k config written by yardo") {
		t.Errorf(".p10k.zsh:\n%s", p10k)
	}
	if zsh, err := exec.LookPath("zsh"); err == nil {
		if out, err := exec.Command(zsh, "-n", e.Path("~/.p10k.zsh")).CombinedOutput(); err != nil {
			t.Errorf("zsh -n: %v\n%s", err, out)
		}
	}
	// A new accent: yardo's file is rewritten.
	e.Answers.Accent = "pistachio"
	if check(t, e, pr) {
		t.Error("Check = true with the accent changed")
	}
	apply(t, e, pr)
	if !strings.Contains(read(t, e.Path("~/.p10k.zsh")), "'#93C572'") {
		t.Error("the accent didn't change")
	}
	// The user's own (p10k configure's) is theirs.
	put(t, e.Path("~/.p10k.zsh"), "# Generated by Powerlevel10k configuration wizard\n")
	e.Answers.Accent = "rose"
	own := ownedStep{src: "p10k-yardo.zsh", to: "~/.p10k.zsh", marker: "# Powerlevel10k config written by yardo"}
	if !check(t, e, own) {
		t.Error("the user's own .p10k.zsh: Check = false")
	}
	apply(t, e, pr)
	if got := read(t, e.Path("~/.p10k.zsh")); strings.Contains(got, "#E36A8C") {
		t.Error("the user's own .p10k.zsh was rewritten")
	}
	// Starship, and an older yardo starship.toml (its first line changed since).
	e.Answers.Prompt = "starship"
	put(t, e.Path("~/.config/starship.toml"), "# yardo's Starship prompt: one line, the essentials. Yours to change\nformat = \"\"\n")
	apply(t, e, pr)
	if got := read(t, e.Path("~/.config/starship.toml")); !strings.Contains(got, `success_symbol = "[❯](bold #E36A8C)"`) {
		t.Errorf("starship.toml:\n%s", got)
	}
	tm := step(t, catalog.Step{Type: "template", Src: "tmux.conf", To: "~/.tmux.conf"})
	apply(t, e, tm)
	if got := read(t, e.Path("~/.tmux.conf")); !strings.Contains(got, `pane-active-border-style "fg=#E36A8C"`) || !strings.Contains(got, "#{pane_current_path}") {
		t.Errorf("tmux.conf:\n%s", got)
	}
	sh := step(t, catalog.Step{Type: "template", Src: "shell.sh", To: "~/.config/yardo/shell.sh"})
	apply(t, e, sh)
	if got := read(t, e.Path("~/.config/yardo/shell.sh")); !strings.Contains(got, "export YARDO_ACCENT='#E36A8C'") {
		t.Error("shell.sh lacks YARDO_ACCENT")
	}
}

// Session on login: what Settings or a profile set is written to the
// session file, the user's; with nothing set, the login's answer stays.
func TestSessionSetting(t *testing.T) {
	e := shellEnv(t)
	e.Answers = &answers.Answers{}
	st := step(t, catalog.Step{Type: "go:session"})
	put(t, e.Path(SessionFile), "zellij\n")
	if !check(t, e, st) {
		t.Error("nothing set: Check = false")
	}
	apply(t, e, st)
	if Session(e) != "zellij" {
		t.Errorf("the login's answer changed: %q", Session(e))
	}
	e.Answers.Session = "off"
	if check(t, e, st) {
		t.Error("Check = true with the setting off and the file zellij")
	}
	apply(t, e, st)
	if got := read(t, e.Path(SessionFile)); got != "off\n" || !check(t, e, st) {
		t.Errorf("session file %q", got)
	}
	if err := SetSession(e, "ask"); err != nil || exists(e.Path(SessionFile)) || Session(e) != "ask" {
		t.Errorf("ask: %v, file there %v", err, exists(e.Path(SessionFile)))
	}
}

func TestEditorThemeSetting(t *testing.T) {
	e := shellEnv(t)
	e.Answers = &answers.Answers{}
	st := step(t, catalog.Step{Type: "go:editortheme"})
	if !check(t, e, st) || EditorTheme(e) != "tokyonight" {
		t.Errorf("nothing set: Check false or theme %q", EditorTheme(e))
	}
	e.Answers.EditorTheme = "kanagawa"
	if check(t, e, st) {
		t.Error("Check = true with no file")
	}
	apply(t, e, st)
	got := read(t, e.Path(EditorThemeFile))
	if !strings.Contains(got, "\ntheme kanagawa\nnvim kanagawa-wave\nvim habamax\n") || !check(t, e, st) {
		t.Errorf("editor theme file %q", got)
	}
	if err := SetEditorTheme(e, "nope"); err != nil || EditorTheme(e) != "tokyonight" {
		t.Errorf("unknown key: %v, %q", err, EditorTheme(e))
	}
}
