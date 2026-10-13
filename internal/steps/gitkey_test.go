package steps

import (
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/creds"
)

const testKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----\n"

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
	cfg := read(t, e.Path("~/.ssh/config"))
	want := gitBegin + "\nHost github.com\n  User git\n  IdentityFile ~/.ssh/yardo_git_key\n  IdentitiesOnly yes\n" +
		"Host git.acme.io\n  User git\n  IdentityFile ~/.ssh/yardo_git_key\n  IdentitiesOnly yes\n  Port 2222\n  StrictHostKeyChecking accept-new\n" +
		gitEnd + "\n\nHost *\n  ServerAliveInterval 30\n"
	if cfg != want {
		t.Errorf("config:\n%s\nwant:\n%s", cfg, want)
	}
	kh := read(t, e.Path("~/.ssh/known_hosts"))
	if !strings.HasPrefix(kh, "10.0.0.5 ssh-ed25519 AAAA\ngithub.com ssh-ed25519 ") {
		t.Errorf("known_hosts = %q", kh)
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
	// The hosts change: the block is replaced, not added twice.
	e.Answers.GitKeyHosts = nil
	apply(t, e, st)
	if cfg := read(t, e.Path("~/.ssh/config")); strings.Count(cfg, gitBegin) != 1 || strings.Contains(cfg, "acme") {
		t.Errorf("config after a change:\n%s", cfg)
	}
	// A different key sent later replaces it.
	e.Secrets.GitKey = []byte(strings.Replace(testKey, "AAAAA", "BBBBB", 1))
	if check(t, e, st) {
		t.Error("Check = true with a new key sent")
	}
}

func TestGitKeyGenerated(t *testing.T) {
	x := &fakeExec{}
	e := shellEnv(t)
	e.Exec = x
	var keys []NewKey
	e.Key = func(k NewKey) { keys = append(keys, k) }
	e.Answers = &answers.Answers{Picks: []string{"github"}, GitHub: answers.GitHost{Mode: "sent"}}
	apply(t, e, step(t, catalog.Step{Type: "go:gitkey"}))
	if got := x.commands(); len(got) != 1 || !strings.Contains(got[0], "ssh-keygen -q -t ed25519") {
		t.Errorf("ran %q", got)
	}
	if len(keys) != 1 || keys[0].Host != "github.com" || !strings.Contains(keys[0].Where, "Deploy keys") || keys[0].Path != GitKeyPath+".pub" {
		t.Errorf("keys = %+v", keys)
	}
	if !strings.Contains(read(t, e.Path("~/.ssh/config")), "Host github.com\n") {
		t.Error("no github.com block")
	}
}

// Each host as its item's mode says: GitHub's own key and agent need no
// config block, GitLab and Bitbucket are trusted on first use, and their
// "key" mode makes ~/.ssh/id_ed25519.
func TestGitKeyModes(t *testing.T) {
	x := &fakeExec{}
	e := shellEnv(t)
	e.Exec = x
	var keys []NewKey
	e.Key = func(k NewKey) { keys = append(keys, k) }
	e.Answers = &answers.Answers{Picks: []string{"github", "gitlab", "bitbucket"},
		GitHub: answers.GitHost{Mode: "agent"}, GitLab: answers.GitHost{Mode: "agent", Host: "gitlab.acme.io"}, Bitbucket: answers.GitHost{Mode: "key"}}
	put(t, e.Path("~/.ssh/config"), gitBegin+"\nHost github.com\n  IdentityFile ~/.ssh/yardo_git_key\n"+gitEnd+"\n\nHost *\n  ServerAliveInterval 30\n")
	st := step(t, catalog.Step{Type: "go:gitkey"})
	apply(t, e, st)
	if got := x.commands(); len(got) != 1 || !strings.Contains(got[0], "id_ed25519") {
		t.Errorf("ran %q", got)
	}
	if len(keys) != 1 || keys[0].Host != "bitbucket.org" || keys[0].Path != "~/.ssh/id_ed25519.pub" {
		t.Errorf("keys = %+v", keys)
	}
	cfg := read(t, e.Path("~/.ssh/config"))
	want := gitBegin + "\nHost gitlab.acme.io\n  User git\n  StrictHostKeyChecking accept-new\n" +
		"Host bitbucket.org\n  User git\n  StrictHostKeyChecking accept-new\n" + gitEnd + "\n\nHost *\n  ServerAliveInterval 30\n"
	if cfg != want {
		t.Errorf("config:\n%s\nwant:\n%s", cfg, want)
	}
	if !strings.Contains(read(t, e.Path("~/.ssh/known_hosts")), "github.com ssh-ed25519 ") {
		t.Error("no GitHub host keys")
	}
	// GitHub alone over its own key: the old block goes, nothing else.
	e.Answers = &answers.Answers{Picks: []string{"github"}}
	apply(t, e, st)
	if cfg := read(t, e.Path("~/.ssh/config")); cfg != "Host *\n  ServerAliveInterval 30\n" {
		t.Errorf("config = %q", cfg)
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
	if a := st.Plan(e); len(a) != 1 {
		t.Errorf("plan = %v", a)
	}
}
