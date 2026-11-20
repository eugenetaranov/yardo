package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// The git host key page's options.
const (
	keyMake    = "Make a new key"
	keyUse     = "Use a key on this machine"
	keyPaste   = "Paste a key"
	keyNone    = "No key here (a forwarded agent, or HTTPS)"
	keyUseIt   = "Use it"
	keyReplace = "Replace it (the old one is moved aside)"
)

// gitHostNames are the git host items' names.
var gitHostNames = map[string]string{"github": "GitHub", "gitlab": "GitLab", "bitbucket": "Bitbucket"}

func (k *asker) chooseOne(title string, opts []string, sel string) (string, error) {
	if k.choose != nil {
		return k.choose(title, opts, sel)
	}
	return ui.Choose(title, opts, sel)
}

func (k *asker) secret(title, placeholder string) (string, error) {
	if k.password != nil {
		return k.password(title, placeholder)
	}
	return ui.Password(title, placeholder)
}

func (k *asker) homeDir() string {
	if k.home != "" {
		return k.home
	}
	return homeOf(k.a.User)
}

// expand is a ~/ path in the user's home.
func (k *asker) expand(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(k.homeDir(), rest)
	}
	return p
}

// askGitKey asks for the git host item id's SSH key: one made (or kept)
// at ~/.ssh/identities/<id>, one of the user's, one pasted, or none.
func (k *asker) askGitKey(c *choices, id string) error {
	g := k.a.GitHostOf(id)
	host := map[string]string{"github": "github.com", "gitlab": k.a.GitLabHost(), "bitbucket": "bitbucket.org"}[id]
	sel := keyMake
	switch {
	case g.Key == answers.NoKey:
		sel = keyNone
	case g.Key != "" && g.Key != answers.IdentityKey(id):
		sel = keyUse
	}
	for {
		ch, err := k.chooseOne(gitHostNames[id]+"'s SSH key, for git to "+host, []string{keyMake, keyUse, keyPaste, keyNone}, sel)
		if err != nil {
			return err
		}
		switch ch {
		case keyMake:
			return k.useIdentity(c, id, nil)
		case keyUse:
			keys := k.localKeys()
			if len(keys) == 0 {
				fmt.Fprintln(k.out, "No private key under ~/.ssh.")
				sel = keyMake
				continue
			}
			cur := g.Key
			if !slices.Contains(keys, cur) {
				cur = keys[0]
			}
			if g.Key, err = k.chooseOne("Which key, for "+host+"?", keys, cur); err != nil {
				return err
			}
			return nil
		case keyPaste:
			key, err := k.pasteKey(host)
			if err != nil {
				return err
			}
			if key == nil {
				sel = keyPaste
				continue // empty: back to the choice
			}
			return k.useIdentity(c, id, key)
		default:
			g.Key = answers.NoKey
			return nil
		}
	}
}

// pasteKey asks for a private key until one is pasted (nil: none).
func (k *asker) pasteKey(host string) ([]byte, error) {
	for {
		v, err := k.secret("Paste the private key for "+host+" (empty: back)", "-----BEGIN OPENSSH PRIVATE KEY----- …")
		if err != nil || strings.TrimSpace(v) == "" {
			return nil, err
		}
		if key, err := normalizeKey(v); err == nil {
			return key, nil
		}
		fmt.Fprintln(k.out, "That isn't an SSH private key (-----BEGIN … PRIVATE KEY-----): paste the private one, not its .pub.")
	}
}

// normalizeKey is a pasted private key as a key file: a terminal's input
// turns its line breaks into spaces, so its body is wrapped again.
func normalizeKey(v string) ([]byte, error) {
	v = strings.TrimSpace(v)
	b, rest, ok := strings.Cut(v, "-----BEGIN ")
	if !ok || strings.TrimSpace(b) != "" {
		return nil, errors.New("not a private key")
	}
	kind, rest, ok := strings.Cut(rest, "-----")
	if !ok || !strings.HasSuffix(kind, "PRIVATE KEY") {
		return nil, errors.New("not a private key")
	}
	body, tail, ok := strings.Cut(rest, "-----END "+kind+"-----")
	if !ok || strings.TrimSpace(tail) != "" || strings.Contains(body, ":") {
		return nil, errors.New("not a private key")
	}
	body = strings.Join(strings.Fields(body), "")
	var out bytes.Buffer
	out.WriteString("-----BEGIN " + kind + "-----\n")
	for len(body) > 70 {
		out.WriteString(body[:70] + "\n")
		body = body[70:]
	}
	if body != "" {
		out.WriteString(body + "\n")
	}
	out.WriteString("-----END " + kind + "-----\n")
	return out.Bytes(), nil
}

// useIdentity sets the host's key to ~/.ssh/identities/<id>: the key
// pasted (nil: one made). A key there already is shown and kept unless
// the user picks Replace it.
func (k *asker) useIdentity(c *choices, id string, key []byte) error {
	p := answers.IdentityKey(id)
	k.a.GitHostOf(id).Key = p
	delete(c.secrets.GitKeys, id)
	delete(c.secrets.GitReplace, id)
	cur, err := os.ReadFile(k.expand(p))
	if err != nil || key != nil && bytes.Equal(cur, key) {
		if key != nil {
			c.secrets.GitKeys[id] = key
		}
		return nil
	}
	ch, err := k.chooseOne(p+" is there already: "+keyInfo(k.expand(p)), []string{keyUseIt, keyReplace}, keyUseIt)
	if err != nil {
		return err
	}
	if ch == keyReplace {
		c.secrets.GitReplace[id] = true
		if key != nil {
			c.secrets.GitKeys[id] = key
		}
	}
	return nil
}

// keyInfo is ssh-keygen -lf's line for a key: its size, fingerprint,
// comment and type.
func keyInfo(p string) string {
	out, err := exec.Command("ssh-keygen", "-lf", p).Output()
	if err != nil {
		return "a key yardo can't read"
	}
	return strings.TrimSpace(string(out))
}

// localKeys are the private keys under ~/.ssh (~/ paths), but those
// moved aside in ~/.ssh/identities.
func (k *asker) localKeys() []string {
	dir := filepath.Join(k.homeDir(), ".ssh")
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() || strings.HasSuffix(p, ".pub") {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if strings.HasPrefix(rel, "identities/") && strings.Contains(rel, ".old-") {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		head := make([]byte, 64)
		n, _ := f.Read(head)
		if line, _, _ := bytes.Cut(head[:n], []byte("\n")); bytes.HasPrefix(line, []byte("-----BEGIN ")) && bytes.HasSuffix(bytes.TrimSpace(line), []byte("PRIVATE KEY-----")) {
			out = append(out, "~/.ssh/"+rel)
		}
		return nil
	})
	return out
}
