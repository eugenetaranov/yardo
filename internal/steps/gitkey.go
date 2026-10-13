package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
)

func init() { RegisterGo("gitkey", func(catalog.Step) Step { return gitKeyStep{} }) }

// gitKeyStep sets up SSH for the git hosts the GitHub, GitLab and
// Bitbucket items reach (and any others yardo gitkey was given), as each
// one's mode says. "sent": ~/.ssh/yardo_git_key (sent by yardo gitkey on
// stdin, kept, or generated) and a block in ~/.ssh/config that makes every
// connection to the host use it, so it works for any repo it is allowed on
// (one repo for a deploy key, all of them for an account key). "key"
// (GitLab, Bitbucket): ~/.ssh/id_ed25519, made when missing (GitHub's
// login makes and uploads its own). Every host but github.com, whose
// published host keys go in known_hosts, is trusted on first use.
type gitKeyStep struct{}

const (
	// GitKeyPath is where the sent key goes.
	GitKeyPath = "~/.ssh/yardo_git_key"
	ownKey     = "~/.ssh/id_ed25519"
	sshConfig  = "~/.ssh/config"
	knownHosts = "~/.ssh/known_hosts"
	gitBegin   = "# >>> yardo gitkey (managed)"
	gitEnd     = "# <<< yardo gitkey"
)

// gitHost is a git host ("host" or "host:port") and how it is reached.
type gitHost struct {
	host, mode string
	// keys is where to add a key made here.
	keys string
}

// gitHosts are the picked host items' hosts (GitHub over a token needs no
// SSH), then the other hosts the sent key is for.
func gitHosts(e *Env) []gitHost {
	a := e.Answers
	if a == nil {
		a = &answers.Answers{}
	}
	var out []gitHost
	add := func(id, host string, m answers.GitHost, keys string) {
		mode := m.Mode
		if mode == "" {
			mode = "key"
		}
		if slices.Contains(a.Picks, id) && mode != "token" {
			out = append(out, gitHost{host, mode, keys})
		}
	}
	add("github", "github.com", a.GitHub, "a deploy key (repo → Settings → Deploy keys; one repo) or on your account (https://github.com/settings/keys; all your repos)")
	add("gitlab", a.GitLabHost(), a.GitLab, "a deploy key (project → Settings → Repository → Deploy keys) or on your account (https://"+a.GitLabHost()+"/-/user_settings/ssh_keys)")
	add("bitbucket", "bitbucket.org", a.Bitbucket, "an access key (repository settings → Access keys) or on your account (https://bitbucket.org/account/settings/ssh-keys/)")
	for _, h := range a.GitKeyHosts {
		if !slices.ContainsFunc(out, func(g gitHost) bool { return g.host == h }) {
			out = append(out, gitHost{h, "sent", "that host"})
		}
	}
	// A key sent before any host was set up is for GitHub.
	if sentKey(e) != nil && !slices.ContainsFunc(out, func(g gitHost) bool { return g.mode == "sent" }) {
		out = append(out, gitHost{"github.com", "sent", ""})
	}
	return out
}

func withMode(hosts []gitHost, mode string) []gitHost {
	return slices.DeleteFunc(slices.Clone(hosts), func(g gitHost) bool { return g.mode != mode || (mode == "key" && g.host == "github.com") })
}

func names(hosts []gitHost) string {
	var n []string
	for _, g := range hosts {
		n = append(n, g.host)
	}
	return strings.Join(n, ", ")
}

func sentKey(e *Env) []byte {
	if e.Secrets == nil {
		return nil
	}
	return e.Secrets.GitKey
}

func (gitKeyStep) Plan(e *Env) []Action {
	hosts := gitHosts(e)
	var out []Action
	if sent := withMode(hosts, "sent"); len(sent) > 0 {
		key := "keep " + GitKeyPath + " (or generate one)"
		if sentKey(e) != nil {
			key = "write the key sent on stdin to " + GitKeyPath
		}
		out = append(out, Action{"file", key + " for " + names(sent)})
	}
	if own := withMode(hosts, "key"); len(own) > 0 {
		out = append(out, Action{"file", "keep " + ownKey + " (or generate one) for " + names(own)})
	}
	if len(hosts) > 0 {
		out = append(out, Action{"file", sshConfig + ": " + names(hosts)})
	}
	return out
}

// configBlock is the ~/.ssh/config block for the hosts ("" when none
// needs one: github.com reached with its own key or an agent).
func configBlock(hosts []gitHost) string {
	var b strings.Builder
	for _, g := range hosts {
		host, port, _ := strings.Cut(g.host, ":")
		if g.mode != "sent" && host == "github.com" && port == "" {
			continue
		}
		fmt.Fprintf(&b, "Host %s\n  User git\n", host)
		if g.mode == "sent" {
			fmt.Fprintf(&b, "  IdentityFile %s\n  IdentitiesOnly yes\n", GitKeyPath)
		}
		if port != "" {
			fmt.Fprintf(&b, "  Port %s\n", port)
		}
		if host != "github.com" {
			b.WriteString("  StrictHostKeyChecking accept-new\n")
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return gitBegin + "\n" + b.String() + gitEnd + "\n"
}

// withConfigBlock puts the block first in ~/.ssh/config (ssh takes the
// first value it finds), replacing an earlier one; an empty block removes it.
func withConfigBlock(cur []byte, block string) []byte {
	out := WithBlock(cur, gitBegin, gitEnd, block)
	if block == "" {
		out = bytes.TrimPrefix(out, []byte("\n"))
	}
	return out
}

// sshRead and sshWrite reach the user's ~/.ssh as the user when running
// as root (a symlink there leads nowhere the user couldn't write), and
// directly otherwise (tests).
func sshRead(ctx context.Context, e *Env, p string) ([]byte, error) {
	if !e.Chown {
		b, err := os.ReadFile(e.Path(p))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return b, err
	}
	out, err := e.Exec.Run(ctx, Cmd{Name: "sh", Args: []string{"-c", `[ -e "$1" ] || exit 0; printf x; cat -- "$1"`, "_", e.Path(p)}, AsUser: true})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out[1:], nil
}

func sshWrite(ctx context.Context, e *Env, p string, b []byte) error {
	if !e.Chown {
		if err := os.MkdirAll(filepath.Dir(e.Path(p)), 0o700); err != nil {
			return err
		}
		return writeFile(e.Path(p), b, 0o600)
	}
	_, err := e.run(ctx, Cmd{Name: "sh", Args: []string{"-c",
		`umask 077 && mkdir -p -- "$(dirname -- "$1")" && chmod 700 -- "$(dirname -- "$1")" && cat > "$1.tmp" && chmod 600 "$1.tmp" && mv -f -- "$1.tmp" "$1"`, "_", e.Path(p)},
		Stdin: b, AsUser: true})
	return err
}

func (s gitKeyStep) Check(ctx context.Context, e *Env) (bool, error) {
	hosts := gitHosts(e)
	if len(withMode(hosts, "sent")) > 0 {
		key, err := sshRead(ctx, e, GitKeyPath)
		if err != nil || key == nil {
			return false, err
		}
		if sent := sentKey(e); sent != nil && !bytes.Equal(key, sent) {
			return false, nil
		}
	}
	if len(withMode(hosts, "key")) > 0 {
		if key, err := sshRead(ctx, e, ownKey); err != nil || key == nil {
			return false, err
		}
	}
	cfg, err := sshRead(ctx, e, sshConfig)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(withConfigBlock(cfg, configBlock(hosts)), cfg) {
		return false, nil
	}
	return s.knownDone(ctx, e, hosts)
}

// knownDone reports whether GitHub's host keys are in known_hosts (when
// github.com is one of the hosts).
func (gitKeyStep) knownDone(ctx context.Context, e *Env, hosts []gitHost) (bool, error) {
	if !slices.ContainsFunc(hosts, func(g gitHost) bool { return g.host == "github.com" }) {
		return true, nil
	}
	kh, err := sshRead(ctx, e, knownHosts)
	return err == nil && bytes.Contains(kh, []byte("github.com ssh-ed25519 ")), err
}

// generate makes an ed25519 key at path and notes where to add it.
func generate(ctx context.Context, e *Env, path, comment string, hosts []gitHost) error {
	if _, err := e.run(ctx, Cmd{Name: "sh", Args: []string{"-c",
		`umask 077 && mkdir -p ~/.ssh && ssh-keygen -q -t ed25519 -N "" -C "$1" -f "$2"`, "_", comment, e.Path(path)}, AsUser: true}); err != nil {
		return err
	}
	pub, _ := sshRead(ctx, e, path+".pub")
	for _, g := range hosts {
		if g.keys != "" {
			e.note(fmt.Sprintf("A new SSH key for %s: add it as %s: %s", g.host, g.keys, strings.TrimSpace(string(pub))))
		}
	}
	return nil
}

func (s gitKeyStep) Apply(ctx context.Context, e *Env) error {
	hosts := gitHosts(e)
	if sent := withMode(hosts, "sent"); len(sent) > 0 {
		key, err := sshRead(ctx, e, GitKeyPath)
		if err != nil {
			return err
		}
		switch k := sentKey(e); {
		case k != nil:
			if !bytes.Contains(k, []byte("PRIVATE KEY-----")) {
				return errors.New("what was sent isn't an SSH private key")
			}
			if err := sshWrite(ctx, e, GitKeyPath, k); err != nil {
				return err
			}
			if err := s.publicKey(ctx, e); err != nil {
				e.note("git SSH key: " + err.Error())
			}
		case key == nil:
			if err := generate(ctx, e, GitKeyPath, e.User+"@"+hostname()+" (git)", sent); err != nil {
				return err
			}
		}
	}
	if own := withMode(hosts, "key"); len(own) > 0 {
		key, err := sshRead(ctx, e, ownKey)
		if err != nil {
			return err
		}
		if key == nil {
			if err := generate(ctx, e, ownKey, e.User+"@"+hostname(), own); err != nil {
				return err
			}
		}
	}
	cfg, err := sshRead(ctx, e, sshConfig)
	if err != nil {
		return err
	}
	if want := withConfigBlock(cfg, configBlock(hosts)); !bytes.Equal(want, cfg) {
		if err := sshWrite(ctx, e, sshConfig, want); err != nil {
			return err
		}
	}
	if done, _ := s.knownDone(ctx, e, hosts); !done {
		gh, err := fs.ReadFile(e.Files, "github_known_hosts")
		if err != nil {
			return err
		}
		kh, err := sshRead(ctx, e, knownHosts)
		if err != nil {
			return err
		}
		if len(kh) > 0 && !bytes.HasSuffix(kh, []byte("\n")) {
			kh = append(kh, '\n')
		}
		if err := sshWrite(ctx, e, knownHosts, append(kh, gh...)); err != nil {
			return err
		}
	}
	return nil
}

// publicKey writes the sent key's .pub next to it (ssh-keygen -y); a key
// with a passphrase can't be read unattended, which is reported.
func (gitKeyStep) publicKey(ctx context.Context, e *Env) error {
	if !e.Chown {
		return nil
	}
	out, err := e.Exec.Run(ctx, Cmd{Name: "ssh-keygen", Args: []string{"-y", "-P", "", "-f", e.Path(GitKeyPath)}, AsUser: true})
	if err != nil {
		return errors.New("it has a passphrase, so git asks for it on each use; a key without one (or a forwarded agent) works unattended")
	}
	return sshWrite(ctx, e, GitKeyPath+".pub", out)
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}
