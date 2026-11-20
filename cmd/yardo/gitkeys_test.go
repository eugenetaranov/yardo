package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/creds"
	"github.com/eugenetaranov/yardo/internal/ui"
)

const pasted = "-----BEGIN OPENSSH PRIVATE KEY----- b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW QyNTUxOQ== -----END OPENSSH PRIVATE KEY-----"

// keyAsker answers the key pages with picks, in order (a title's first
// word picks a page), and pastes paste.
func keyAsker(t *testing.T, a *answers.Answers, picks []string, paste ...string) (*asker, *[]string) {
	t.Helper()
	var titles []string
	home := t.TempDir()
	k := &asker{a: a, out: io.Discard, home: home,
		choose: func(title string, opts []string, sel string) (string, error) {
			titles = append(titles, title)
			if len(picks) == 0 {
				t.Fatalf("asked %q, no pick left", title)
			}
			p := picks[0]
			picks = picks[1:]
			if !slices.Contains(opts, p) {
				t.Fatalf("%q: %q not in %q", title, p, opts)
			}
			return p, nil
		},
		password: func(string, string) (string, error) {
			if len(paste) == 0 {
				return "", nil
			}
			p := paste[0]
			paste = paste[1:]
			return p, nil
		}}
	return k, &titles
}

func putKey(t *testing.T, home, rel, body string) {
	t.Helper()
	p := filepath.Join(home, ".ssh", rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAskGitKey(t *testing.T) {
	norm, err := normalizeKey(pasted)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(norm), "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW\nQyNTUxOQ==\n-----END") {
		t.Errorf("normalized:\n%s", norm)
	}
	if _, err := normalizeKey("ssh-ed25519 AAAA me@x"); err == nil {
		t.Error("a public key passed")
	}

	t.Run("make", func(t *testing.T) {
		a := &answers.Answers{}
		k, _ := keyAsker(t, a, []string{keyMake})
		c := &choices{secrets: creds.New()}
		if err := k.askGitKey(c, "bitbucket"); err != nil {
			t.Fatal(err)
		}
		if a.Bitbucket.Key != "~/.ssh/identities/bitbucket" || len(c.secrets.GitKeys)+len(c.secrets.GitReplace) != 0 {
			t.Errorf("key %q secrets %+v", a.Bitbucket.Key, c.secrets)
		}
	})
	t.Run("use one here", func(t *testing.T) {
		a := &answers.Answers{}
		k, titles := keyAsker(t, a, []string{keyUse, "~/.ssh/work_ed25519"})
		putKey(t, k.home, "work_ed25519", "-----BEGIN OPENSSH PRIVATE KEY-----\nx\n")
		putKey(t, k.home, "work_ed25519.pub", "ssh-ed25519 AAAA")
		putKey(t, k.home, "config", "Host x\n")
		putKey(t, k.home, "identities/github.old-2026-01-01", "-----BEGIN OPENSSH PRIVATE KEY-----\nx\n")
		putKey(t, k.home, "identities/gitlab", "-----BEGIN RSA PRIVATE KEY-----\nx\n")
		if got := k.localKeys(); !slices.Equal(got, []string{"~/.ssh/identities/gitlab", "~/.ssh/work_ed25519"}) {
			t.Errorf("keys = %v", got)
		}
		if err := k.askGitKey(&choices{secrets: creds.New()}, "github"); err != nil {
			t.Fatal(err)
		}
		if a.GitHub.Key != "~/.ssh/work_ed25519" || !strings.Contains((*titles)[0], "github.com") {
			t.Errorf("key %q titles %q", a.GitHub.Key, *titles)
		}
	})
	t.Run("paste", func(t *testing.T) {
		a := &answers.Answers{GitLab: answers.GitHost{Host: "git.acme.io"}}
		k, _ := keyAsker(t, a, []string{keyPaste}, "ssh-ed25519 AAAA public", pasted)
		c := &choices{secrets: creds.New()}
		if err := k.askGitKey(c, "gitlab"); err != nil {
			t.Fatal(err)
		}
		if a.GitLab.Key != "~/.ssh/identities/gitlab" || string(c.secrets.GitKeys["gitlab"]) != string(norm) {
			t.Errorf("key %q secrets %q", a.GitLab.Key, c.secrets.GitKeys)
		}
	})
	t.Run("none", func(t *testing.T) {
		a := &answers.Answers{GitLab: answers.GitHost{Mode: "sent"}}
		k, _ := keyAsker(t, a, []string{keyNone})
		if err := k.askGitKey(&choices{secrets: creds.New()}, "gitlab"); err != nil {
			t.Fatal(err)
		}
		if a.GitLab.Key != answers.NoKey {
			t.Errorf("key %q", a.GitLab.Key)
		}
	})
	// A key there: Use it leaves it; Replace it sets the flag the step reads.
	for _, c := range []struct {
		pick    string
		replace bool
	}{{keyUseIt, false}, {keyReplace, true}} {
		t.Run(c.pick, func(t *testing.T) {
			a := &answers.Answers{}
			k, titles := keyAsker(t, a, []string{keyPaste, c.pick}, pasted)
			putKey(t, k.home, "identities/github", "-----BEGIN OPENSSH PRIVATE KEY-----\nold\n")
			ch := &choices{secrets: creds.New()}
			if err := k.askGitKey(ch, "github"); err != nil {
				t.Fatal(err)
			}
			if len(*titles) != 2 || !strings.Contains((*titles)[1], "~/.ssh/identities/github is there already") {
				t.Errorf("titles %q", *titles)
			}
			if ch.secrets.GitReplace["github"] != c.replace || (ch.secrets.GitKeys["github"] != nil) != c.replace || a.GitHub.Key != "~/.ssh/identities/github" {
				t.Errorf("secrets %+v key %q", ch.secrets, a.GitHub.Key)
			}
			if b, _ := os.ReadFile(filepath.Join(k.home, ".ssh/identities/github")); !strings.Contains(string(b), "old") {
				t.Error("the key there was changed")
			}
		})
	}
}

// New git hosts are asked for their key; a re-run with the same picks
// asks nothing.
func TestQuestionsGitKey(t *testing.T) {
	cat, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &answers.Answers{User: "ada", Picks: []string{"zsh"}, TZ: "UTC", Prompt: "p10k"}
	k, titles := keyAsker(t, a, []string{keyMake})
	k.cat, k.installed = cat, func(string) bool { return false }
	if _, err := k.fromBoard(context.Background(), ui.BoardResult{Kind: ui.BoardApply, Adds: []string{"bitbucket"}}, false); err != nil {
		t.Fatal(err)
	}
	if len(*titles) != 1 || !strings.HasPrefix((*titles)[0], "Bitbucket's SSH key") || a.Bitbucket.Key == "" {
		t.Errorf("titles %q key %q", *titles, a.Bitbucket.Key)
	}
	a.Pick([]string{"bitbucket"})
	// Settings › Accept new hosts' SSH keys: saved, and the run has the
	// step that writes it.
	if row := sshAcceptNew(a); row.Value != "off" {
		t.Errorf("row = %+v", row)
	}
	c, err := k.fromBoard(context.Background(), ui.BoardResult{Kind: ui.BoardApply, Settings: map[string]string{"sshnew": "on"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(*titles) != 1 || a.SSHAcceptNew != "on" || sshAcceptNew(a).Value != "on" {
		t.Errorf("re-run asked %q; accept %q", *titles, a.SSHAcceptNew)
	}
	if !slices.ContainsFunc(c.items, func(it *catalog.Item) bool { return it.ID == "gitkey" }) {
		t.Error("the run has no gitkey")
	}
}
