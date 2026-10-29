package steps

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

func init() { RegisterGo("gitattrib", func(catalog.Step) Step { return gitAttribStep{} }) }

// gitAttribStep keeps agents' attribution out of commits: a global
// commit-msg hook that drops a coding agent's Co-authored-by and
// "Generated with" lines (a person's stay). It's config-based (Git 2.54
// and later), so a repo's own hooks, husky's and pre-commit's still run;
// a global core.hooksPath would turn them off. Older git on Ubuntu is
// upgraded from the Ubuntu Git Maintainers' PPA; elsewhere it gets no hook.
type gitAttribStep struct{}

// The git-core PPA (https://launchpad.net/~git-core/+archive/ubuntu/ppa):
// the current git release, built for each supported Ubuntu.
const (
	gitPPAKey = "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0xF911AB184317630C59970973E363C90F8F1B6217"
	gitPPAFpr = "F911AB184317630C59970973E363C90F8F1B6217"
	gitPPA    = `
Types: deb
URIs: https://ppa.launchpadcontent.net/git-core/ppa/ubuntu
Suites: {{codename}}
Components: main
Signed-By: {{keyring}}
`
)

func gitPPARepo() aptRepoStep {
	return aptRepoStep{catalog.Step{Type: "apt_repo", Name: "git-core", KeyURL: gitPPAKey, KeyFingerprint: gitPPAFpr, Source: gitPPA}}
}

const (
	attribHook = "yardo-no-agent-trailer"
	attribFile = "~/.config/yardo/git/no-agent-trailer"
	// attribSaid marks that older git was noted (once, not every run).
	attribSaid = "~/.config/yardo/git/no-hook-noted"
)

func (gitAttribStep) Plan(*Env) []Action {
	return []Action{
		{"file", attribFile + " (drops agents' Co-authored-by and Generated-with lines)"},
		{"git", "hook." + attribHook + ".event=commit-msg, .command=" + attribFile + " (Git 2.54+)"},
		{"apt", "git from the git-core PPA, when git is older than 2.54 (Ubuntu)"},
	}
}

// gitAtLeast reports whether `git --version` says major.minor or later.
func gitAtLeast(out []byte, major, minor int) bool {
	m := regexp.MustCompile(`git version (\d+)\.(\d+)`).FindSubmatch(out)
	if m == nil {
		return false
	}
	a, _ := strconv.Atoi(string(m[1]))
	b, _ := strconv.Atoi(string(m[2]))
	return a > major || (a == major && b >= minor)
}

func (gitAttribStep) hasHooks(ctx context.Context, e *Env) bool {
	out, err := e.Exec.Run(ctx, Cmd{Name: "git", Args: []string{"--version"}, AsUser: true})
	return err == nil && gitAtLeast(out, 2, 54)
}

func (gitAttribStep) script(e *Env) ([]byte, error) {
	return fs.ReadFile(e.Files, "git-no-agent-trailer")
}

func (g gitAttribStep) Check(ctx context.Context, e *Env) (bool, error) {
	if !g.hasHooks(ctx, e) {
		_, err := os.Stat(e.Path(attribSaid))
		return err == nil, nil // Apply says why, once
	}
	want, err := g.script(e)
	if err != nil {
		return false, err
	}
	if cur, err := os.ReadFile(e.Path(attribFile)); err != nil || !bytes.Equal(cur, want) {
		return false, nil
	}
	for k, v := range g.config(e) {
		out, err := e.Exec.Run(ctx, Cmd{Name: "git", Args: []string{"config", "--global", "--get", k}, AsUser: true})
		if err != nil || string(bytes.TrimSpace(out)) != v {
			return false, nil
		}
	}
	return true, nil
}

func (gitAttribStep) config(e *Env) map[string]string {
	return map[string]string{
		"hook." + attribHook + ".event":   "commit-msg",
		"hook." + attribHook + ".command": filepath.Join(e.Home, ".config/yardo/git/no-agent-trailer"),
	}
}

// newGit upgrades git from the git-core PPA (not to the lock's version:
// the lock has the distro's git, older than the hook needs).
func (gitAttribStep) newGit(ctx context.Context, e *Env) error {
	r := gitPPARepo()
	if done, _ := r.Check(ctx, e); !done {
		if err := r.Apply(ctx, e); err != nil {
			return err
		}
	}
	if err := AptUpdate(ctx, e); err != nil {
		return err
	}
	_, err := e.run(ctx, Cmd{Name: "apt-get", Args: append(slices.Clone(AptOpts), "install", "--only-upgrade", "git"), Env: AptEnv})
	return err
}

func (g gitAttribStep) Apply(ctx context.Context, e *Env) error {
	if !g.hasHooks(ctx, e) && distro(e) == "ubuntu" {
		if err := g.newGit(ctx, e); err != nil {
			return fmt.Errorf("git 2.54+ from the git-core PPA (for the commit-msg hook): %w", err)
		}
	}
	if !g.hasHooks(ctx, e) {
		e.note("Agent co-author lines: git here is older than 2.54, so yardo set no hook to drop them " +
			"(a global core.hooksPath would stop your repos' own hooks); Claude Code's own setting keeps them out.")
		if err := os.MkdirAll(filepath.Dir(e.Path(attribSaid)), 0o755); err != nil {
			return err
		}
		return writeFile(e.Path(attribSaid), []byte("git was older than 2.54: no commit-msg hook\n"), 0o644)
	}
	b, err := g.script(e)
	if err != nil {
		return err
	}
	dir := filepath.Dir(e.Path(attribFile))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeFile(e.Path(attribFile), b, 0o755); err != nil {
		return err
	}
	for _, p := range []string{filepath.Dir(dir), dir, e.Path(attribFile)} {
		if err := e.chown(p, e.UID, e.GID); err != nil {
			return err
		}
	}
	for k, v := range g.config(e) {
		if _, err := e.run(ctx, Cmd{Name: "git", Args: []string{"config", "--global", "--replace-all", k, v}, AsUser: true}); err != nil {
			return fmt.Errorf("git config %s: %w", k, err)
		}
	}
	return nil
}
