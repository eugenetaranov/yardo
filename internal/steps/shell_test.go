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
	put(t, e.Path("~/.zshrc"), "mine\n")
	apply(t, e, st)
	if got := read(t, e.Path("~/.zshrc")); !strings.Contains(got, `ZSH_THEME="robbyrussell"`) {
		t.Errorf("no default theme in\n%s", got)
	}
	if got := read(t, e.Path("~/.zshrc.orig")); got != "mine\n" {
		t.Errorf(".zshrc.orig = %q", got)
	}
	if !check(t, e, st) {
		t.Error("Check after Apply = false")
	}
	// Once p10k is cloned, re-rendering keeps its theme; .orig stays.
	if err := os.MkdirAll(e.Path(p10kDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if check(t, e, st) {
		t.Error("Check = true with p10k cloned and the default theme")
	}
	apply(t, e, st)
	if got := read(t, e.Path("~/.zshrc")); !strings.Contains(got, `ZSH_THEME="powerlevel10k/powerlevel10k"`) {
		t.Errorf("no p10k theme in\n%s", got)
	}
	if got := read(t, e.Path("~/.zshrc.orig")); got != "mine\n" {
		t.Errorf(".zshrc.orig = %q after a second render", got)
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
	got := read(t, e.Path("~/.zshrc"))
	if !strings.Contains(got, "ZSH_THEME=\"\"\n") || !strings.Contains(got, `eval "$(starship init zsh)"`) {
		t.Errorf(".zshrc:\n%s", got)
	}
	if zsh, err := exec.LookPath("zsh"); err == nil {
		if out, err := exec.Command(zsh, "-n", e.Path("~/.zshrc")).CombinedOutput(); err != nil {
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

// The steps of zshcfg, shellcfg, p10k and agentkeys all touch .zshrc: once
// each has run, none has more to do (a re-run changes nothing).
func TestZshrcSettles(t *testing.T) {
	e := shellEnv(t)
	zshrc := step(t, catalog.Step{Type: "go:zshrc"})
	dropin := step(t, catalog.Step{Type: "line", To: "~/.zshrc", Line: Dropins[0].Line, IfExists: true})
	theme := step(t, catalog.Step{Type: "line", To: "~/.zshrc", Match: "^ZSH_THEME=", Line: `ZSH_THEME="` + P10kTheme + `"`, IfExists: true})
	apply(t, e, zshrc)
	put(t, e.Path(Dropins[0].File), "# shell\n")
	apply(t, e, dropin)
	if err := os.MkdirAll(e.Path(p10kDir), 0o755); err != nil {
		t.Fatal(err)
	}
	apply(t, e, theme)
	// agentkeys (still bash) appends its line the same way.
	put(t, e.Path(Dropins[1].File), "# agent\n")
	b, _ := os.ReadFile(e.Path("~/.zshrc"))
	put(t, e.Path("~/.zshrc"), string(WithLine(b, Dropins[1].Line, "")))
	for name, st := range map[string]Step{"zshrc": zshrc, "dropin": dropin, "theme": theme} {
		if !check(t, e, st) {
			t.Errorf("%s: Check = false on a settled .zshrc", name)
		}
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

// The local override files are sourced last: drop-ins and line steps go
// before the block that sources ~/.zshrc.local, and each config file
// yardo writes ends by sourcing its local file.
func TestLocalOverridesLast(t *testing.T) {
	e := shellEnv(t)
	put(t, e.Path("~/.config/yardo/shell.sh"), "")
	apply(t, e, step(t, catalog.Step{Type: "go:zshrc"}))
	apply(t, e, step(t, catalog.Step{Type: "line", To: "~/.zshrc", Line: ". ~/.config/yardo/agent-env.sh", IfExists: true}))
	got := read(t, e.Path("~/.zshrc"))
	last := "[[ -r ~/.zshrc.local ]] && source ~/.zshrc.local\n"
	if !strings.HasSuffix(got, last) {
		t.Errorf(".zshrc doesn't end by sourcing .zshrc.local:\n%s", got)
	}
	mark := strings.Index(got, LastMark)
	for _, l := range []string{Dropins[0].Line, ". ~/.config/yardo/agent-env.sh"} {
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
