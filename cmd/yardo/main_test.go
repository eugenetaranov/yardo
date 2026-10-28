package main

import (
	"bytes"
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/profile"
	"github.com/eugenetaranov/yardo/internal/steps"
)

func TestMain(m *testing.M) {
	// Homes (and the users' answers files) under a temp dir, not the real one.
	dir, err := os.MkdirTemp("", "yardo-homes")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("YARDO_HOMES", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestDryRunPlan(t *testing.T) {
	state := t.TempDir()
	me, _ := user.Current()
	if err := answers.Write(state, &answers.Answers{User: me.Username, Picks: []string{"zsh", "terraform"}, MiseExtra: []string{"starship"}}); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if err := cli([]string{"--yes", "--dry-run", "--state", state}, &out, &errb); err != nil {
		t.Fatalf("run: %v\n%s", err, errb.String())
	}
	got := out.String()
	for _, want := range []string{"For user: " + me.Username, "zsh (login shell)\n  apt    zsh git\n  shell  ", "  clone  https://github.com/romkatv/powerlevel10k.git", "mise   terraform@latest terragrunt@latest", "Extra mise tools\n  mise   starship", "== Install\n", "== Configure\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestYesWithoutAnswers(t *testing.T) {
	err := cli([]string{"--yes", "--dry-run", "--state", t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no saved choices yet") {
		t.Errorf("err = %v", err)
	}
}

func TestLegacyCmd(t *testing.T) {
	c := legacyCmd("/usr/local/share/yardo/yardo.sh", "/var/lib/yardo", "--only", "docker")
	if c.String() != "bash /usr/local/share/yardo/yardo.sh --only docker" {
		t.Errorf("cmd = %s", c)
	}
	env := strings.Join(c.Env, " ")
	if !strings.Contains(env, "YARDO_STATE=/var/lib/yardo") || !strings.Contains(env, "YARDO_SHARE=/usr/local/share/yardo") {
		t.Errorf("env = %v", c.Env)
	}
	if legacyRunner(nil, "/nonexistent/yardo.sh", "/tmp") != nil {
		t.Error("a missing bash installer should give no legacy runner")
	}
}

func TestBashHandles(t *testing.T) {
	for args, want := range map[string]bool{
		"": false, "--dry-run": false, "forget": true, "forget --yes": true, "--help": true,
		"--yes": false, "-y --dry-run": false, "--verify": false, "--version": false,
	} {
		if got := bashHandles(strings.Fields(args)); got != want {
			t.Errorf("bashHandles(%q) = %v, want %v", args, got, want)
		}
	}
}

func TestExportAndProfile(t *testing.T) {
	me, _ := user.Current()
	src := t.TempDir()
	if err := answers.Write(src, &answers.Answers{User: me.Username, Picks: []string{"dbclients", "terraform"}, TZ: "UTC"}); err != nil {
		t.Fatal(err)
	}
	f := t.TempDir() + "/team.toml"
	if err := cli([]string{"--state", src, "export", f}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := cli([]string{"--state", src, "export", "a.toml", "b.toml"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Error("export with two files: want an error")
	}
	// What's locked depends on what this machine has; it must parse back.
	var locked bytes.Buffer
	if err := cli([]string{"--state", src, "export", "--lock"}, &locked, &bytes.Buffer{}); err != nil || !strings.Contains(locked.String(), "terraform") {
		t.Errorf("export --lock: %v\n%s", err, locked.String())
	}
	if _, err := profile.Parse("locked", locked.Bytes()); err != nil {
		t.Errorf("export --lock doesn't parse: %v\n%s", err, locked.String())
	}
	me2 := t.TempDir() + "/me.toml"
	if err := os.WriteFile(me2, []byte("[system]\ntz = \"Europe/Berlin\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	t.Setenv("SUDO_USER", me.Username)
	var out bytes.Buffer
	if err := cli([]string{"--state", dst, "--profile", f, "--profile", me2, "--yes", "--dry-run"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Terraform") || !strings.Contains(out.String(), "DB clients") {
		t.Errorf("dry run of the profile:\n%s", out.String())
	}
	if _, err := os.Stat(dst + "/answers.json"); err == nil {
		t.Error("--dry-run saved the profile's answers")
	}
	if err := cli([]string{"--state", dst, "--profile", "http://example.com/p.toml", "--yes", "--dry-run"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("plain http profile: err = %v", err)
	}
}

// A named profile applied again lists what it dropped; --user names a
// user to create, which a dry run only reports.
func TestProfileLayersAndUser(t *testing.T) {
	me, _ := user.Current()
	state := t.TempDir()
	a := &answers.Answers{User: me.Username, Picks: []string{"dbclients", "neovim", "terraform"}, Layers: map[string][]string{"team": {"neovim", "terraform"}}}
	if err := answers.Write(state, a); err != nil {
		t.Fatal(err)
	}
	f := t.TempDir() + "/team.toml"
	if err := os.WriteFile(f, []byte("name = \"team\"\npicks = [\"neovim\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	t.Setenv("SUDO_USER", me.Username)
	if err := cli([]string{"--state", state, "--profile", f, "--yes", "--dry-run"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// Layers are per user: a new user has nothing for the profile to drop.
	if err := cli([]string{"--state", state, "--profile", f, "--user", "yardonewuser", "--yes", "--dry-run"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Count(got, "Dropped by the profile") != 1 {
		t.Errorf("want one drop list (for %s):\n%s", me.Username, got)
	}
	for _, want := range []string{"Dropped by the profile (still installed, no longer re-applied):\n  - Terraform + terragrunt, tflint, terraform-docs, checkov (terraform)\n", "Would create user yardonewuser", "For user: yardonewuser\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Terraform\n") {
		t.Errorf("dropped item still planned:\n%s", got)
	}
	if b, _, _ := answers.Read(state); b.User != me.Username {
		t.Error("--dry-run saved the answers")
	}
	if err := cli([]string{"--state", state, "--user", "Bad User", "--yes", "--dry-run"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Error("want an error for --user 'Bad User'")
	}
}

type fakeExec struct{ cmds []string }

func (f *fakeExec) Run(_ context.Context, c steps.Cmd) ([]byte, error) {
	f.cmds = append(f.cmds, c.String())
	if c.Name == "id" {
		return []byte("ubuntu adm sudo docker\n"), nil
	}
	return nil, nil
}

func TestCreateUser(t *testing.T) {
	sudoersDir = t.TempDir()
	x := &fakeExec{}
	if err := createUser(context.Background(), x, "ada", "ubuntu"); err != nil {
		t.Fatal(err)
	}
	if want := "useradd -m -U -s /bin/bash -G adm,sudo,docker ada"; len(x.cmds) != 2 || x.cmds[1] != want {
		t.Errorf("commands = %q, want %q last", x.cmds, want)
	}
	b, err := os.ReadFile(sudoersDir + "/90-yardo-ada")
	if err != nil || string(b) != "ada ALL=(ALL) NOPASSWD:ALL\n" {
		t.Errorf("sudo rule = %q, %v", b, err)
	}
}

// A profile's [lock] shows in the plan: exact mise versions, apt pkg=ver.
func TestProfileLockPlan(t *testing.T) {
	me, _ := user.Current()
	t.Setenv("SUDO_USER", me.Username)
	f := t.TempDir() + "/locked.toml"
	src := "picks = [\"zsh\", \"terraform\"]\nmise_extra = [\"starship\"]\n[lock.mise]\nterraform = \"1.9.8\"\nstarship = \"1.21.1\"\n[lock.apt]\nzsh = \"5.9-6ubuntu2\"\n"
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := cli([]string{"--state", t.TempDir(), "--profile", f, "--yes", "--dry-run"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"apt    zsh=5.9-6ubuntu2 git\n", "mise   terraform@1.9.8 ", "Extra mise tools\n  mise   starship@1.21.1\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestTargets(t *testing.T) {
	root := t.TempDir()
	st := &answers.Store{State: root + "/state", IsSystem: func(string) bool { return false },
		Account: func(n string) (answers.Account, error) {
			return answers.Account{Home: root + "/" + n, UID: os.Getuid(), GID: os.Getgid()}, os.MkdirAll(root+"/"+n, 0o755)
		}}
	for _, u := range []string{"bob", "ada"} {
		if err := st.Save(&answers.Answers{User: u, Picks: []string{"zsh"}}); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SUDO_USER", "ubuntu")
	o := &options{}
	if got, err := o.targets(st, nil, false, &bytes.Buffer{}); err != nil || strings.Join(got, " ") != "ada bob" {
		t.Errorf("--yes: %v, %v", got, err)
	}
	if got, _ := o.targets(st, []*profile.Profile{{User: "cy"}, {}}, false, &bytes.Buffer{}); strings.Join(got, " ") != "cy" {
		t.Errorf("profile user: %v", got)
	}
	// Interactive: the user who ran sudo, without asking.
	if got, err := o.targets(st, nil, true, &bytes.Buffer{}); err != nil || strings.Join(got, " ") != "ubuntu" {
		t.Errorf("interactive: %v, %v", got, err)
	}
	if got, _ := o.targets(st, []*profile.Profile{{}}, false, &bytes.Buffer{}); strings.Join(got, " ") != "ubuntu" {
		t.Errorf("profile without a user: %v", got)
	}
	o.user = "dee"
	if got, _ := o.targets(st, []*profile.Profile{{User: "cy"}}, false, &bytes.Buffer{}); strings.Join(got, " ") != "dee" {
		t.Errorf("--user: %v", got)
	}
}

// yardo gitkey: the key on stdin, set up alone for the sudo user.
func TestGitKeySubcommand(t *testing.T) {
	me, _ := user.Current()
	t.Setenv("SUDO_USER", me.Username)
	state := t.TempDir()
	if err := answers.Write(state, &answers.Answers{User: me.Username, Picks: []string{"zsh", "dbclients"}}); err != nil {
		t.Fatal(err)
	}
	keyf := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyf, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nx\n-----END OPENSSH PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(keyf)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	old := os.Stdin
	os.Stdin = in
	defer func() { os.Stdin = old }()
	var out bytes.Buffer
	if err := cli([]string{"--state", state, "--dry-run", "gitkey", "--host", "github.com", "--host", "git.acme.io"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "SSH for the git hosts") || !strings.Contains(got, "write the key sent on stdin") || !strings.Contains(got, "github.com, git.acme.io") {
		t.Errorf("plan:\n%s", got)
	}
	if strings.Contains(got, "zsh (login shell)") {
		t.Errorf("gitkey ran more than its item:\n%s", got)
	}
	if err := cli([]string{"--state", state, "--dry-run", "gitkey", "--host", "bad host"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Error("want an error for a bad --host")
	}
}

// Git defaults keeps the global ignores as a block, not a one-off file.
func TestDryRunGitIgnores(t *testing.T) {
	me, _ := user.Current()
	state := t.TempDir()
	if err := answers.Write(state, &answers.Answers{User: me.Username, Picks: []string{"gitcfg"}}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := cli([]string{"--yes", "--dry-run", "--state", state}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "block  ~/.config/git/ignore (yardo's block from gitignore_global)") {
		t.Errorf("plan:\n%s", out.String())
	}
}

// New keys come last: where to add each, then the key alone on its line.
func TestPrintKeys(t *testing.T) {
	state := t.TempDir()
	rows := "github.com\ton GitHub\t~/.ssh/yardo_git_key.pub\tssh-ed25519 AAAA1 ada@vm (git)\n" +
		"git.acme.io\tthere\t~/.ssh/yardo_git_key.pub\tssh-ed25519 AAAA1 ada@vm (git)\n" +
		"bitbucket.org\ton Bitbucket\t~/.ssh/id_ed25519.pub\tssh-ed25519 BBBB2 ada@vm\n"
	if err := os.WriteFile(filepath.Join(state, keysFile), []byte(rows), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	printKeys(&out, state, "/home/ada")
	want := "\nSSH key to add\n  github.com: on GitHub\n  git.acme.io: there\n\nssh-ed25519 AAAA1 ada@vm (git)\n\nShow it again: cat /home/ada/.ssh/yardo_git_key.pub\n" +
		"\nSSH key to add\n  bitbucket.org: on Bitbucket\n\nssh-ed25519 BBBB2 ada@vm\n\nShow it again: cat /home/ada/.ssh/id_ed25519.pub\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
	out.Reset()
	printKeys(&out, t.TempDir(), "/home/ada")
	if out.Len() != 0 {
		t.Errorf("no keys: %q", out.String())
	}
}

func TestNewShellHint(t *testing.T) {
	for changed, want := range map[string]string{
		"":                  "",
		"dbclients tmux":    "",
		"mise neovim":       "exec bash -l",
		"shellcfg":          "exec bash -l",
		"zshbin mise zshrc": "exec zsh -l",
	} {
		if got := newShellHint(strings.Fields(changed)); !strings.Contains(got, want) || (want == "") != (got == "") {
			t.Errorf("%q: %q, want %q", changed, got, want)
		}
	}
}
