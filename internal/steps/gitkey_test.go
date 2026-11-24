package steps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/creds"
)

const testKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----\n"

// The includes yardo adds around a config of the user's.
func included(user string) string {
	return fmt.Sprintf("Include config.d/yardo-git\n%sMatch all\nInclude config.d/yardo-defaults\n", user)
}

func TestGitKeySent(t *testing.T) {
	e := shellEnv(t)
	e.Answers = &answers.Answers{Picks: []string{"github"}, GitHub: answers.GitHost{Mode: "sent"}, GitKeyHosts: []string{"git.acme.io:2222"}}
	e.Secrets = creds.New()
	e.Secrets.GitKey = []byte(testKey)
	put(t, e.Path("~/.ssh/config"), "Host *\n  ServerAliveInterval 30\n")
	put(t, e.Path("~/.ssh/known_hosts"), "10.0.0.5 ssh-ed25519 AAAA")
	st := step(t, catalog.Step{Type: "go:gitkey"})
	if check(t, e, st) {
		t.Fatal("Check = true before Apply")
	}
	apply(t, e, st)
	if got := read(t, e.Path(GitKeyPath)); got != testKey {
		t.Errorf("key = %q", got)
	}
	if cfg := read(t, e.Path("~/.ssh/config")); cfg != included("Host *\n  ServerAliveInterval 30\n") {
		t.Errorf("config:\n%s", cfg)
	}
	git := read(t, e.Path(gitConfig))
	want := "Host github.com\n  User git\n  IdentityFile ~/.ssh/yardo_git_key\n  IdentitiesOnly yes\n" +
		"Host git.acme.io\n  User git\n  IdentityFile ~/.ssh/yardo_git_key\n  IdentitiesOnly yes\n  Port 2222\n  StrictHostKeyChecking accept-new\n"
	if !strings.HasSuffix(git, want) || !strings.HasPrefix(git, "# yardo's") {
		t.Errorf("%s:\n%s\nwant:\n%s", gitConfig, git, want)
	}
	if fi, err := os.Stat(e.Path(gitConfig)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("%s: %v %v", gitConfig, fi.Mode(), err)
	}
	if fi, err := os.Stat(filepath.Dir(e.Path(gitConfig))); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("config.d: %v %v", fi.Mode(), err)
	}
	kh := read(t, e.Path("~/.ssh/known_hosts"))
	if !strings.HasPrefix(kh, "10.0.0.5 ssh-ed25519 AAAA\ngithub.com ssh-ed25519 ") {
		t.Errorf("known_hosts = %q", kh)
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
	// The hosts change: the file is rewritten, ~/.ssh/config untouched.
	e.Answers.GitKeyHosts = nil
	apply(t, e, st)
	if strings.Contains(read(t, e.Path(gitConfig)), "acme") {
		t.Error("acme still set")
	}
	if cfg := read(t, e.Path("~/.ssh/config")); strings.Count(cfg, "Include") != 2 {
		t.Errorf("config after a change:\n%s", cfg)
	}
	// A different key sent later replaces it.
	e.Secrets.GitKey = []byte(strings.Replace(testKey, "AAAAA", "BBBBB", 1))
	if check(t, e, st) {
		t.Error("Check = true with a new key sent")
	}
}

// Each host's key: the one chosen (a path, none), else what its mode
// said before, giving the same IdentityFile as an earlier yardo did.
func TestGitKeyResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		m    answers.GitHost
		want string // the entry's IdentityFile, "" for none, "-" for no entry
	}{
		{"new (profile)", answers.GitHost{Key: "~/.ssh/identities/gitlab"}, "~/.ssh/identities/gitlab"},
		{"picked", answers.GitHost{Key: "~/.ssh/work_ed25519"}, "~/.ssh/work_ed25519"},
		{"none", answers.GitHost{Key: "none"}, "-"},
		{"none wins over a mode", answers.GitHost{Key: "none", Mode: "sent"}, "-"},
		{"old own", answers.GitHost{}, ""},
		{"old agent", answers.GitHost{Mode: "agent"}, ""},
		{"old key", answers.GitHost{Mode: "key"}, "~/.ssh/gitlab"},
		{"old sent", answers.GitHost{Mode: "sent"}, GitKeyPath},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := shellEnv(t)
			e.Answers = &answers.Answers{Picks: []string{"gitlab"}, GitLab: c.m}
			hosts := gitHosts(e)
			if c.want == "-" {
				if len(hosts) != 0 {
					t.Errorf("hosts = %+v", hosts)
				}
				return
			}
			put(t, e.Path("~/.ssh/gitlab"), testKey)
			if err := ownKeys(t.Context(), e, hosts); err != nil {
				t.Fatal(err)
			}
			got, _ := entries(hosts, nil)
			if !strings.Contains(got, "Host gitlab.com\n") {
				t.Errorf("entries:\n%s", got)
			}
			if c.want == "" && strings.Contains(got, "IdentityFile") || c.want != "" && !strings.Contains(got, fmt.Sprintf("IdentityFile %s\n", c.want)) {
				t.Errorf("entries:\n%s\nwant IdentityFile %q", got, c.want)
			}
		})
	}
	// A new host under --yes or a profile gets ~/.ssh/identities/<item>;
	// one set up before, or chosen, keeps its own.
	a := &answers.Answers{Picks: []string{"github"}, Bitbucket: answers.GitHost{Key: "~/.ssh/mine"}}
	a.NewGitHosts([]string{"github", "gitlab", "bitbucket", "zsh"})
	if a.GitHub.Key != "" || a.GitLab.Key != "~/.ssh/identities/gitlab" || a.Bitbucket.Key != "~/.ssh/mine" {
		t.Errorf("keys: %+v %+v %+v", a.GitHub, a.GitLab, a.Bitbucket)
	}
}

// Old answers keep their key; GitHub with the user's own key, which ssh
// finds by itself, needs no entry; agent hosts are trusted on first use.
func TestGitKeyModes(t *testing.T) {
	x := &fakeExec{}
	e := shellEnv(t)
	e.Exec = x
	var keys []NewKey
	e.Key = func(k NewKey) { keys = append(keys, k) }
	e.Answers = &answers.Answers{Picks: []string{"github", "gitlab", "bitbucket"},
		GitHub: answers.GitHost{Mode: "agent"}, GitLab: answers.GitHost{Mode: "agent", Host: "gitlab.acme.io"}, Bitbucket: answers.GitHost{Mode: "key"}}
	st := step(t, catalog.Step{Type: "go:gitkey"})
	apply(t, e, st)
	if got := x.commands(); len(got) != 1 || !strings.Contains(got[0], "/.ssh/bitbucket") || !strings.Contains(got[0], "(bitbucket)") {
		t.Errorf("ran %q", got)
	}
	if len(keys) != 1 || keys[0].Host != "bitbucket.org" || keys[0].Path != "~/.ssh/bitbucket.pub" {
		t.Errorf("keys = %+v", keys)
	}
	git := read(t, e.Path(gitConfig))
	want := "Host gitlab.acme.io\n  User git\n  StrictHostKeyChecking accept-new\n" +
		"Host bitbucket.org ssh.bitbucket.org\n  HostName ssh.bitbucket.org\n  User git\n  IdentityFile ~/.ssh/bitbucket\n  IdentitiesOnly yes\n"
	if !strings.HasSuffix(git, want) {
		t.Errorf("%s:\n%s\nwant:\n%s", gitConfig, git, want)
	}
	kh := read(t, e.Path("~/.ssh/known_hosts"))
	for _, h := range []string{"github.com ssh-ed25519 ", "\nbitbucket.org ssh-ed25519 ", "\nssh.bitbucket.org ssh-ed25519 "} {
		if !strings.Contains(kh, h) {
			t.Errorf("known_hosts has no %q", h)
		}
	}
	// GitHub alone, as set up before: the user's own key; no entry.
	e.Answers = &answers.Answers{Picks: []string{"github"}}
	put(t, e.Path("~/.ssh/id_ed25519"), testKey)
	apply(t, e, st)
	if git := read(t, e.Path(gitConfig)); git != "" {
		t.Errorf("%s = %q", gitConfig, git)
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
}

// No key of the user's own and no other way: ~/.ssh/id_ed25519 is made.
func TestGitKeyOwnMade(t *testing.T) {
	x := &fakeExec{}
	e := shellEnv(t)
	e.Exec = x
	e.Answers = &answers.Answers{Picks: []string{"gitlab"}}
	st := step(t, catalog.Step{Type: "go:gitkey"})
	apply(t, e, st)
	if got := strings.Join(x.commands(), "\n"); !strings.Contains(got, "ssh-keygen") || !strings.HasSuffix(got, "/.ssh/id_ed25519") {
		t.Errorf("ran %q", got)
	}
	if git := read(t, e.Path(gitConfig)); strings.Contains(git, "IdentityFile") || !strings.Contains(git, "Host gitlab.com") {
		t.Errorf("%s:\n%s", gitConfig, git)
	}
}

// A machine whose "key" mode made ~/.ssh/id_ed25519 (an earlier yardo)
// keeps it: it's on the accounts already, and ssh finds it by itself.
func TestGitKeyKeepsIDEd25519(t *testing.T) {
	x := &fakeExec{}
	e := shellEnv(t)
	e.Exec = x
	e.Answers = &answers.Answers{Picks: []string{"github", "bitbucket"}, Bitbucket: answers.GitHost{Mode: "key"}}
	put(t, e.Path("~/.ssh/id_ed25519"), testKey)
	st := step(t, catalog.Step{Type: "go:gitkey"})
	apply(t, e, st)
	if got := x.commands(); len(got) != 0 {
		t.Errorf("made a key: %q", got)
	}
	git := read(t, e.Path(gitConfig))
	if strings.Contains(git, "IdentityFile") || strings.Contains(git, "Host github.com") {
		t.Errorf("%s:\n%s", gitConfig, git)
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
	if OwnKey("github", func(p string) bool { return p == "~/.ssh/github" || p == ownKey }) != "~/.ssh/github" {
		t.Error("~/.ssh/github should win over id_ed25519")
	}
}

// A key in ~/.ssh/identities: made when missing; one given is written;
// one there is kept, and replaced only when asked, moved aside first.
func TestGitKeyIdentity(t *testing.T) {
	x := &fakeExec{}
	e := shellEnv(t)
	e.Exec = x
	var keys []NewKey
	e.Key = func(k NewKey) { keys = append(keys, k) }
	e.Secrets = creds.New()
	e.Answers = &answers.Answers{Picks: []string{"bitbucket"}, Bitbucket: answers.GitHost{Key: answers.IdentityKey("bitbucket")}}
	st := step(t, catalog.Step{Type: "go:gitkey"})
	key := "~/.ssh/identities/bitbucket"
	apply(t, e, st)
	if got := strings.Join(x.commands(), "\n"); !strings.Contains(got, "ssh-keygen -q -t ed25519") || !strings.HasSuffix(got, "/.ssh/identities/bitbucket") || !strings.Contains(got, "chmod 644") {
		t.Errorf("ran %q", got)
	}
	if len(keys) != 1 || keys[0].Path != key+".pub" || !strings.Contains(keys[0].Where, "bitbucket.org/account/settings/ssh-keys") {
		t.Errorf("keys = %+v", keys)
	}
	if git := read(t, e.Path(gitConfig)); !strings.Contains(git, fmt.Sprintf("IdentityFile %s\n", key)) {
		t.Errorf("%s:\n%s", gitConfig, git)
	}

	// Given (pasted or sent) with none there: written.
	x.ran, keys = nil, nil
	e.Secrets.GitKeys["bitbucket"] = []byte(testKey)
	apply(t, e, st)
	if got := read(t, e.Path(key)); got != testKey {
		t.Errorf("key = %q", got)
	}
	if fi, _ := os.Stat(filepath.Dir(e.Path(key))); fi.Mode().Perm() != 0o700 {
		t.Errorf("identities: %v", fi.Mode())
	}
	if len(keys) != 1 {
		t.Errorf("not shown: %+v", keys)
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}

	// Another given, not replacing: refused, the key kept.
	other := strings.Replace(testKey, "AAAAA", "BBBBB", 1)
	e.Secrets.GitKeys["bitbucket"] = []byte(other)
	if err := st.Apply(t.Context(), e); err == nil || !strings.Contains(err.Error(), "--replace") {
		t.Errorf("Apply = %v, want a refusal", err)
	}
	if got := read(t, e.Path(key)); got != testKey {
		t.Error("the key was replaced unasked")
	}

	// Replacing: the old one (and its .pub) moved aside, then written.
	put(t, e.Path(key+".pub"), "ssh-ed25519 AAAA old\n")
	day := time.Now().Format("2006-01-02")
	put(t, e.Path(fmt.Sprintf("%s.old-%s", key, day)), "taken")
	e.Secrets.GitReplace["bitbucket"] = true
	if check(t, e, st) {
		t.Error("Check = true with a replace asked")
	}
	apply(t, e, st)
	if got := read(t, e.Path(key)); got != other {
		t.Errorf("key = %q", got)
	}
	if got := read(t, e.Path(fmt.Sprintf("%s.old-%s-2", key, day))); got != testKey {
		t.Errorf("old key = %q", got)
	}
	if got := read(t, e.Path(fmt.Sprintf("%s.old-%s-2.pub", key, day))); got != "ssh-ed25519 AAAA old\n" {
		t.Errorf("old .pub = %q", got)
	}
	if !check(t, e, st) {
		t.Error("Check = false after replacing")
	}

	// A public key given: refused.
	e.Secrets.GitKeys["bitbucket"], e.Secrets.GitReplace["bitbucket"] = []byte("ssh-ed25519 AAAA me"), true
	if err := st.Apply(t.Context(), e); err == nil || !strings.Contains(err.Error(), "isn't an SSH private key") {
		t.Errorf("Apply = %v", err)
	}
}

// A key of the user's picked: used where it is, never written.
func TestGitKeyPicked(t *testing.T) {
	x := &fakeExec{}
	e := shellEnv(t)
	e.Exec = x
	e.Answers = &answers.Answers{Picks: []string{"github"}, GitHub: answers.GitHost{Key: "~/.ssh/work_ed25519"}}
	st := step(t, catalog.Step{Type: "go:gitkey"})
	if err := st.Apply(t.Context(), e); err == nil || !strings.Contains(err.Error(), "isn't there") {
		t.Errorf("Apply with no key = %v", err)
	}
	put(t, e.Path("~/.ssh/work_ed25519"), testKey)
	apply(t, e, st)
	if len(x.ran) != 0 {
		t.Errorf("ran %q", x.commands())
	}
	if _, err := os.Stat(e.Path("~/.ssh/identities")); err == nil {
		t.Error("~/.ssh/identities written")
	}
	if git := read(t, e.Path(gitConfig)); !strings.Contains(git, "Host github.com\n  User git\n  IdentityFile ~/.ssh/work_ed25519\n") {
		t.Errorf("%s:\n%s", gitConfig, git)
	}
}

// ~/.ssh/config: the user's lines kept, an earlier yardo's block moved
// out, the Include lines added once and kept where the user moved them.
func TestGitKeyConfig(t *testing.T) {
	for _, c := range []struct{ name, cur, want string }{
		{"none", "", included("")},
		{"user's", "# mine\nHost db\n  User me\n", "# mine\n" + included("Host db\n  User me\n")},
		{"no Host", "ForwardAgent no", "ForwardAgent no\n" + included("")},
		{"old block", gitBegin + "\nHost github.com\n  IdentityFile ~/.ssh/github\n" + gitEnd + "\n\nHost db\n  User me\n", included("Host db\n  User me\n")},
		{"moved by the user", "Host db\n  User me\nInclude ~/.ssh/config.d/yardo-git\nMatch all\nInclude config.d/yardo-defaults\nHost x\n",
			"Host db\n  User me\nInclude ~/.ssh/config.d/yardo-git\nMatch all\nInclude config.d/yardo-defaults\nHost x\n"},
	} {
		if got := string(withIncludes([]byte(c.cur))); got != c.want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}
	// Migrated: the old block's host now in yardo's file, same key.
	e := shellEnv(t)
	e.Answers = &answers.Answers{Picks: []string{"github"}, GitHub: answers.GitHost{Mode: "key"}}
	put(t, e.Path("~/.ssh/github"), testKey)
	put(t, e.Path("~/.ssh/config"), gitBegin+"\nHost github.com\n  User git\n  IdentityFile ~/.ssh/github\n  IdentitiesOnly yes\n"+gitEnd+"\n\nHost db\n  User me\n")
	apply(t, e, step(t, catalog.Step{Type: "go:gitkey"}))
	if cfg := read(t, e.Path("~/.ssh/config")); cfg != included("Host db\n  User me\n") {
		t.Errorf("config:\n%s", cfg)
	}
	if git := read(t, e.Path(gitConfig)); !strings.Contains(git, "Host github.com\n  User git\n  IdentityFile ~/.ssh/github\n") {
		t.Errorf("%s:\n%s", gitConfig, git)
	}
	sshG(t, e, "github.com", "identityfile ~/.ssh/github")
}

// A host the user's config sets is left to them, and said; Host * isn't
// one.
func TestGitKeyUserHosts(t *testing.T) {
	hosts := []gitHost{{host: "github.com", id: "github", mode: "path", key: "~/.ssh/identities/github"},
		{host: "bitbucket.org", id: "bitbucket", mode: "path", key: "~/.ssh/identities/bitbucket"}}
	for _, c := range []struct {
		name, cfg string
		own       map[string]int
	}{
		{"github", "Host *\n  User me\n\nHost github.com\n  IdentityFile ~/.ssh/mine\n", map[string]int{"github.com": 4}},
		{"wildcard", "Host *.bitbucket.org !x.bitbucket.org\n  User me\n", map[string]int{"ssh.bitbucket.org": 1}},
		{"star", "Host *\n  User me\nHost * ?\n", map[string]int{}},
	} {
		own := userHosts([]byte(c.cfg), hosts)
		if len(own) != len(c.own) {
			t.Errorf("%s: %v, want %v", c.name, own, c.own)
		}
		for h, n := range c.own {
			if own[h] != n {
				t.Errorf("%s: %v, want %v", c.name, own, c.own)
			}
		}
	}
	e := shellEnv(t)
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	e.Answers = &answers.Answers{Picks: []string{"github"}, GitHub: answers.GitHost{Key: "~/.ssh/work"}}
	put(t, e.Path("~/.ssh/work"), testKey)
	put(t, e.Path("~/.ssh/config"), "Host a\n  User me\n\n\n\n\n\n\n\n\n\nHost github.com\n  IdentityFile ~/.ssh/mine\n")
	apply(t, e, step(t, catalog.Step{Type: "go:gitkey"}))
	if git := read(t, e.Path(gitConfig)); git != "" {
		t.Errorf("%s:\n%s", gitConfig, git)
	}
	// Line 12: the Include line went in above.
	if want := "~/.ssh/config sets github.com (line 13): yardo leaves it to you; its key is ~/.ssh/work"; len(notes) != 1 || notes[0] != want {
		t.Errorf("notes = %q", notes)
	}
}

// Accepting new hosts' keys: in yardo's defaults, after everything of the
// user's, so a StrictHostKeyChecking of theirs wins, for every host even
// when their config ends inside a Host block.
func TestGitKeyAcceptNew(t *testing.T) {
	e := shellEnv(t)
	e.Answers = &answers.Answers{SSHAcceptNew: "on"}
	put(t, e.Path("~/.ssh/id_ed25519"), testKey)
	put(t, e.Path("~/.ssh/config"), "Host prod-*\n  StrictHostKeyChecking yes\n")
	st := step(t, catalog.Step{Type: "go:gitkey"})
	apply(t, e, st)
	if got := read(t, e.Path(sshDefaults)); !strings.Contains(got, "\nStrictHostKeyChecking accept-new\n") {
		t.Errorf("%s = %q", sshDefaults, got)
	}
	sshG(t, e, "new-box", "stricthostkeychecking accept-new")
	sshG(t, e, "prod-1", "stricthostkeychecking true")
	e.Answers.SSHAcceptNew = ""
	if check(t, e, st) {
		t.Error("Check = true with the setting off")
	}
	apply(t, e, st)
	if got := read(t, e.Path(sshDefaults)); got != "" {
		t.Errorf("%s = %q", sshDefaults, got)
	}
}

// sshG checks that ssh -G host gives want, with the config's Includes
// made absolute (ssh reads relative ones under the real ~/.ssh).
func sshG(t *testing.T, e *Env, host, want string) {
	t.Helper()
	if _, err := exec.LookPath("ssh"); err != nil {
		return
	}
	cfg := strings.ReplaceAll(read(t, e.Path("~/.ssh/config")), "Include config.d/", fmt.Sprintf("Include %s/", e.Path("~/.ssh/config.d")))
	f := filepath.Join(t.TempDir(), "config")
	put(t, f, cfg)
	out, err := exec.Command("ssh", "-F", f, "-G", host).Output()
	if err != nil {
		t.Fatalf("ssh -G %s: %v", host, err)
	}
	if !strings.Contains(string(out), fmt.Sprintf("\n%s\n", want)) {
		t.Errorf("ssh -G %s has no %q:\n%s", host, want, out)
	}
}
