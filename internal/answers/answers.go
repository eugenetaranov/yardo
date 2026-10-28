// Package answers reads and writes yardo's saved answers,
// /var/lib/yardo/answers.json: everything ever picked, and the plain
// settings that go with it. Never a secret. The file is data: it is only
// parsed, never run.
package answers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Version is the answers.json format this package reads and writes.
const Version = 1

// File and Legacy are the answer files' names in the state directory.
// Legacy is the shell-format file yardo wrote before v0.39; it is
// converted to File once and kept as Legacy+".old".
const (
	File   = "answers.json"
	Legacy = "answers"
)

// Answers is answers.json. The JSON shape is shared with the bash
// installer (picker.py), so both can read what the other wrote.
type Answers struct {
	Version    int      `json:"version"`
	User       string   `json:"user"`
	Picks      []string `json:"picks"`
	MiseExtra  []string `json:"mise_extra"`
	MCPListen  string   `json:"mcp_listen"`
	MCPServers []string `json:"mcp_servers"`
	// WireAgents: nil means never asked.
	WireAgents *bool    `json:"wire_agents"`
	Git        Git      `json:"git"`
	TZ         string   `json:"tz"`
	GitHub     GitHost  `json:"github"`
	GitLab     GitHost  `json:"gitlab,omitzero"`
	Bitbucket  GitHost  `json:"bitbucket,omitzero"`
	NPM        NPM      `json:"npm"`
	Registries []string `json:"registries"`
	// Prompt is the zsh prompt: "p10k" (also "") or "starship".
	Prompt string `json:"prompt,omitempty"`
	// Dotfiles is the user's dotfiles repo (cloned to ~/.dotfiles).
	Dotfiles string `json:"dotfiles,omitempty"`
	// GitKeyHosts are other git hosts the sent SSH key (yardo gitkey) is
	// used for, besides GitHub, GitLab and Bitbucket in the "sent" mode.
	GitKeyHosts []string `json:"gitkey_hosts,omitempty"`
	// Layers are the picks each named profile set, by its name. Applying
	// a profile with that name again replaces its layer (SetLayer).
	Layers map[string][]string `json:"layers,omitempty"`
	// Hand are picks made by hand, or by an unnamed profile, that a named
	// layer also lists: no layer drops them. A pick in no layer is a hand
	// pick too, so this stays short.
	Hand []string `json:"hand,omitempty"`
	// MCPPort is the user's MCPJungle port (kept in system.json).
	MCPPort int `json:"-"`
}

// Git is the git identity.
type Git struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// GitHost holds how the VM reaches a git host: agent (the SSH agent
// forwarded with ssh -A), key (~/.ssh/id_ed25519, made here), sent
// (~/.ssh/yardo_git_key, sent with yardo gitkey) or, GitHub only, token
// (HTTPS). "" is key.
type GitHost struct {
	Mode string `json:"mode"`
	// Host is a self-managed GitLab ("" = gitlab.com).
	Host string `json:"host,omitempty"`
}

// GitLabHost is the GitLab host name.
func (a *Answers) GitLabHost() string {
	if a.GitLab.Host != "" {
		return a.GitLab.Host
	}
	return "gitlab.com"
}

// SendKeyTo sets up the sent SSH key for hosts (none: github.com):
// GitHub, GitLab and Bitbucket switch to the "sent" mode, other hosts go
// in GitKeyHosts. It returns the items to pick: the host items, and the
// hidden gitkey item when other hosts need it.
func (a *Answers) SendKeyTo(hosts []string) (picks []string) {
	if len(hosts) == 0 {
		hosts = []string{"github.com"}
	}
	for _, h := range hosts {
		switch h {
		case "github.com":
			a.GitHub.Mode, picks = "sent", Merge(picks, []string{"github"})
		case "bitbucket.org":
			a.Bitbucket.Mode, picks = "sent", Merge(picks, []string{"bitbucket"})
		case a.GitLabHost():
			a.GitLab.Mode, picks = "sent", Merge(picks, []string{"gitlab"})
		default:
			a.GitKeyHosts, picks = Merge(a.GitKeyHosts, []string{h}), Merge(picks, []string{"gitkey"})
		}
	}
	return picks
}

// SentKeyHosts are the hosts the sent SSH key is for: the picked host
// items in the "sent" mode, then GitKeyHosts.
func (a *Answers) SentKeyHosts() []string {
	var out []string
	for _, h := range []struct {
		id, host string
		m        GitHost
	}{{"github", "github.com", a.GitHub}, {"gitlab", a.GitLabHost(), a.GitLab}, {"bitbucket", "bitbucket.org", a.Bitbucket}} {
		if h.m.Mode == "sent" && slices.Contains(a.Picks, h.id) {
			out = append(out, h.host)
		}
	}
	return Merge(out, a.GitKeyHosts)
}

// migrateGitKey turns a pick of the old gitkey item (one SSH key for the
// hosts in GitKeyHosts, github.com when none) into the host items.
func (a *Answers) migrateGitKey() (notes []string) {
	if !slices.Contains(a.Picks, "gitkey") && !slices.Contains(a.Hand, "gitkey") {
		return nil
	}
	hosts := a.GitKeyHosts
	a.GitKeyHosts = nil
	picks := a.SendKeyTo(hosts)
	if slices.Equal(picks, []string{"gitkey"}) {
		return nil // other hosts only: still the gitkey item
	}
	swap := func(l []string) []string {
		if i := slices.Index(l, "gitkey"); i >= 0 {
			l = Merge(slices.Delete(slices.Clone(l), i, i+1), picks)
		}
		return l
	}
	a.Picks, a.Hand = swap(a.Picks), swap(a.Hand)
	for n, l := range a.Layers {
		a.Layers[n] = swap(l)
	}
	return []string{"The git SSH key is now part of the GitHub, GitLab and Bitbucket items (each asks how to reach it): picked " +
		strings.Join(slices.DeleteFunc(slices.Clone(picks), func(p string) bool { return p == "gitkey" }), ", ") + " with the key you send."}
}

// NPM is the private npm registry's settings (its token is never saved).
type NPM struct {
	Scope    string `json:"scope"`
	Registry string `json:"registry"`
}

// PickMap maps catalog ids renamed, merged or removed since v0.29 to what
// replaced them.
var PickMap = map[string][]string{
	"kubectl": {"k8s"}, "kubectx": {"k8s"}, "helm": {"k8s"}, "k9s": {"k8s"},
	"shell":      {"zsh"}, // split into three in v0.38, one again in v0.2
	"typescript": {"node"},
	"kotlin":     {"java"},
	"editors":    {"neovim"}, // Vim gave way to Neovim, the one editor
	"chrony":     {},         // no longer an item: clock sync is the launcher's job
	"glab":       {"gitlab"}, // the GitLab item: glab and the SSH key
	"sshkey":     {},         // the GitHub, GitLab and Bitbucket items make the key
	// v0.2: one zsh item with its parts, tools folded into their bundles,
	// hardening always on.
	"p10k": {"zsh"}, "zshcfg": {"zsh"}, "fzftab": {"zsh"}, "shellcfg": {"zsh"}, "cli": {"zsh"}, "docs": {"zsh"},
	"tmuxcfg": {"tmux"}, "vimcfg": {"neovim"},
	"k8sextra": {"k8s"}, "krew": {"k8stools"}, "helmfile": {"k8stools"},
	"runners": {"devcli"}, "task": {"devcli"}, "yq": {"devcli"}, "http": {"devcli"},
	"pgcli": {"dbclients"}, "iaclint": {"terraform"}, "agekey": {"sops"},
	"needrestart": {}, // with security updates, which every machine gets
	"ufw":         {}, // no longer an item
	"act":         {}, // comes with the GitHub item
	"ctools":      {"docker"},
	"lint":        {"devcli"},
	"gitcfg":      {"gitid"},
	"lazygit":     {"gitid"}, // comes with the Git item
	"gitlab-cli":  {},        // comes with the GitLab item
	"agentsafe":   {"agentcfg"},
	"k3d":         {"k8s"},
	"kind":        {"k8s", "k8stools"},
	"gitops":      {"k8stools"},
	"devloop":     {"k8stools"},
	"talos":       {"k8stools"},
	"corpca":      {}, // no longer an item
	"registries":  {}, // no longer an item
	"iacscan":     {}, // checkov comes with Terraform; trivy is gone
	// no longer items: the less common languages, profiling
	"bun": {}, "deno": {}, "ruby": {}, "php": {}, "cpp": {}, "zig": {}, "dotnet": {}, "profiling": {},
	"npmrc":      {}, // no longer an item
	"monitoring": {}, // no longer an item
	"sudopw":     {}, // no longer an item
	// always applied now
	"autoupdate-tools": {},
}

// pickNotes explain a migration to the user.
var pickNotes = map[string]string{
	"chrony":     "chrony is no longer an item (clock sync is set up when the machine is launched); this machine keeps the chrony it has.",
	"sshkey":     "The SSH key item is gone: ~/.ssh/id_ed25519 stays, and the GitHub, GitLab and Bitbucket items make it when they use it.",
	"ufw":        "The firewall (ufw) is no longer an item: yardo leaves this machine's ufw as it is (turn it off with 'sudo ufw disable').",
	"act":        "act is no longer an item of its own: it comes with the GitHub item; the one installed stays.",
	"sudopw":     "\"sudo asks for your password\" is no longer an item: yardo leaves this machine's sudo rules as they are.",
	"gitlab-cli": "glab is no longer an item of its own: it comes with the GitLab item; the one installed stays.",
	"corpca":     "The internal CA bundle is no longer an item: certificates it installed stay trusted (remove them from /usr/local/share/ca-certificates and run 'sudo update-ca-certificates').",
	"monitoring": "node_exporter + glances is no longer an item: what it installed stays (remove with 'sudo apt remove prometheus-node-exporter').",
	"editors":    "Vim is no longer an item: Neovim, set up the same way, replaces it. Vim and ~/.vimrc stay as they are.",
	"registries": "Container registry logins are no longer an item: logins already made stay (sudo yardo forget still logs out of them); log in with 'docker login <registry>'.",
	"bun":        "Bun is no longer an item: the one installed stays, and the weekly tool updates still update it.",
	"deno":       "Deno is no longer an item: the one installed stays, and the weekly tool updates still update it.",
	"ruby":       "Ruby is no longer an item: the one installed stays, and the weekly tool updates still update it.",
	"php":        "PHP is no longer an item: what it installed stays (remove with 'sudo apt remove php-cli composer').",
	"cpp":        "C/C++ is no longer an item: gcc and make still come with the essentials; clang, cmake and gdb stay installed.",
	"zig":        "Zig is no longer an item: the one installed stays, and the weekly tool updates still update it.",
	"dotnet":     ".NET is no longer an item: the one installed stays, and the weekly tool updates still update it.",
	"profiling":  "Profiling (perf, valgrind, bpftrace) is no longer an item: what it installed stays.",
	"iacscan":    "The IaC scanners item is gone: checkov comes with the Terraform item, and trivy is no longer installed (the ones installed stay).",
	"npmrc":      "The private npm registry is no longer an item: yardo's block in ~/.npmrc stays (sudo yardo forget still removes it); edit ~/.npmrc yourself.",
}

// MigratePicks applies PickMap, keeping order and dropping duplicates. It
// returns the notes for picks that went away.
func MigratePicks(picks []string) (out, notes []string) {
	seen := map[string]bool{}
	noted := map[string]bool{}
	for _, p := range picks {
		repl, ok := PickMap[p]
		if !ok {
			repl = []string{p}
		}
		for _, q := range repl {
			if !seen[q] {
				seen[q] = true
				out = append(out, q)
			}
		}
		if n, ok := pickNotes[p]; ok && !noted[n] {
			noted[n] = true
			notes = append(notes, n)
		}
	}
	return out, notes
}

// Read loads the saved answers from state, migrated. A missing file is
// empty answers. A legacy shell-format file is decoded (never run),
// converted to answers.json and kept as answers.old when state is
// writable. Warnings (skipped lines, migration notes) come back in notes.
func Read(state string) (a *Answers, notes []string, err error) {
	path := filepath.Join(state, File)
	legacy := filepath.Join(state, Legacy)
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		a = &Answers{}
		if err := json.Unmarshal(b, a); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		if a.Version != Version {
			return nil, nil, fmt.Errorf("%s: version %d; this yardo reads %d", path, a.Version, Version)
		}
	case errors.Is(err, fs.ErrNotExist):
		lb, lerr := os.ReadFile(legacy)
		if errors.Is(lerr, fs.ErrNotExist) {
			return &Answers{Version: Version}, nil, nil
		}
		if lerr != nil {
			return nil, nil, lerr
		}
		a, notes = decodeLegacy(legacy, string(lb))
		if writable(state) {
			if err := Write(state, a); err != nil {
				return nil, nil, err
			}
			if err := os.Rename(legacy, legacy+".old"); err != nil {
				return nil, nil, err
			}
		}
	default:
		return nil, nil, err
	}
	a.clean()
	more := a.migrateGitKey()
	var mp []string
	a.Picks, mp = MigratePicks(a.Picks)
	more = append(more, mp...)
	a.Hand, _ = MigratePicks(a.Hand)
	for n, l := range a.Layers {
		a.Layers[n], _ = MigratePicks(l)
	}
	return a, append(notes, more...), nil
}

// clean drops list entries that aren't single words (the bash side splits
// them on spaces).
func (a *Answers) clean() {
	for _, l := range []*[]string{&a.Picks, &a.MiseExtra, &a.MCPServers, &a.Registries} {
		out := (*l)[:0]
		for _, s := range *l {
			if s != "" && !strings.ContainsAny(s, " \t\n") {
				out = append(out, s)
			}
		}
		*l = out
	}
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".w")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// Write saves a to state/answers.json (0600, atomically), creating state
// (0700) if needed.
func Write(state string, a *Answers) error {
	if err := os.MkdirAll(state, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(state, 0o700); err != nil {
		return err
	}
	a.Version = Version
	for _, l := range []*[]string{&a.Picks, &a.MiseExtra, &a.MCPServers, &a.Registries} {
		if *l == nil {
			*l = []string{}
		}
	}
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(state, File), append(b, '\n'), 0o600)
}

// writeAtomic writes b to path through a temp file, with mode.
func writeAtomic(path string, b []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Merge returns a's items followed by b's new ones (choices accumulate
// across runs).
func Merge(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, l := range [][]string{a, b} {
		for _, s := range l {
			if s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// Pick adds picks made by hand (or by an unnamed profile): no named
// profile drops them later.
func (a *Answers) Pick(ids []string) {
	for _, id := range ids {
		if a.inLayer(id, "") {
			a.Hand = Merge(a.Hand, []string{id})
		}
	}
	a.Picks = Merge(a.Picks, ids)
}

// Unpick drops ids from the picks, so they stop being re-applied; what
// they installed stays. A named profile that lists one adds it back when
// applied again.
func (a *Answers) Unpick(ids []string) {
	drop := func(id string) bool { return slices.Contains(ids, id) }
	a.Picks = slices.DeleteFunc(a.Picks, drop)
	a.Hand = slices.DeleteFunc(a.Hand, drop)
}

// SetLayer makes ids the named profile's picks, replacing what it set
// before. It returns the picks the profile no longer lists that nothing
// else keeps (hand picks and other layers' picks stay); they leave Picks,
// so they stop being re-applied, but stay installed.
func (a *Answers) SetLayer(name string, ids []string) (dropped []string) {
	for _, id := range ids {
		// Picked before any layer claimed it: it's the user's own.
		if slices.Contains(a.Picks, id) && !a.inLayer(id, "") {
			a.Hand = Merge(a.Hand, []string{id})
		}
	}
	old := a.Layers[name]
	if a.Layers == nil {
		a.Layers = map[string][]string{}
	}
	a.Layers[name] = Merge(nil, ids)
	for _, id := range old {
		if !slices.Contains(ids, id) && !slices.Contains(a.Hand, id) && !a.inLayer(id, name) {
			dropped = append(dropped, id)
		}
	}
	a.Picks = slices.DeleteFunc(Merge(a.Picks, ids), func(id string) bool { return slices.Contains(dropped, id) })
	return dropped
}

// inLayer reports whether a named layer other than except lists id.
func (a *Answers) inLayer(id, except string) bool {
	for n, l := range a.Layers {
		if n != except && slices.Contains(l, id) {
			return true
		}
	}
	return false
}

var repoRe = regexp.MustCompile(`^(https://[^\s'"\\$` + "`" + `;]+|ssh://[^\s'"\\$` + "`" + `;]+|[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[A-Za-z0-9._/~-]+)$`)

// CheckRepo checks a git repo URL: https://, ssh:// or user@host:path.
func CheckRepo(u string) error {
	if !repoRe.MatchString(u) || strings.HasPrefix(u, "-") {
		return errors.New("want an https://, ssh:// or git@host:path repo URL")
	}
	return nil
}

var hostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:[0-9]{1,5})?$`)

// ValidHost reports whether h is a host name (with an optional :port) for
// ~/.ssh/config.
func ValidHost(h string) bool { return hostRe.MatchString(h) }
