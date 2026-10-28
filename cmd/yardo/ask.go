package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/creds"
	"github.com/eugenetaranov/yardo/internal/mcp"
	"github.com/eugenetaranov/yardo/internal/plan"
	"github.com/eugenetaranov/yardo/internal/profile"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// choices is what one interactive run picked and typed.
type choices struct {
	picks, miseExtra, mcpServers []string
	// dropped are saved picks unticked this time: no longer re-applied.
	dropped []string
	items   []*catalog.Item
	secrets *creds.Secrets
}

// asker runs the prompts. Its hooks are seams for tests.
type asker struct {
	cat   *catalog.Catalog
	a     *answers.Answers
	share string
	out   io.Writer
	// installed reports whether a tool is on the user's PATH.
	installed func(tool string) bool
	// profile: the answers already hold the profiles' picks; review them
	// instead of opening the picker. dropped are the picks they dropped.
	profile bool
	dropped []string
	// port is the MCPJungle port the user would get.
	port func() int
	// listenSet: the board set the gateway's access (don't ask it again).
	listenSet bool
}

// ask is a run with profiles: their picks, reviewed, and what they can't
// hold asked. (Without profiles, the board picks: see fromBoard.)
func (k *asker) ask(ctx context.Context) (*choices, error) {
	c, err := k.fromProfile(ctx)
	if err != nil || c == nil {
		return c, err
	}
	if k.review(c, "Install", "Quit") != "Install" {
		return nil, ui.ErrQuit
	}
	return c, nil
}

// fromBoard turns what the board queued into this run's choices (picks
// added and removed, extra mise tools, access settings), then asks what
// the new items need.
func (k *asker) fromBoard(ctx context.Context, res ui.BoardResult, first bool) (*choices, error) {
	saved := slices.Clone(k.a.Picks)
	picks := slices.DeleteFunc(slices.Clone(saved), func(id string) bool { return slices.Contains(res.Removes, id) })
	extras := slices.DeleteFunc(slices.Clone(k.a.MiseExtra), func(t string) bool { return slices.Contains(res.Removes, "mise:"+t) })
	var dropped []string
	for _, id := range res.Removes {
		if !strings.HasPrefix(id, "mise:") {
			dropped = append(dropped, id)
		}
	}
	for _, id := range res.Adds {
		if t, ok := strings.CutPrefix(id, "mise:"); ok {
			extras = append(extras, t)
		} else {
			picks = append(picks, id)
		}
	}
	for key, v := range res.Settings {
		switch key {
		case "mcp":
			k.a.MCPListen = v
			k.listenSet = true
		case "ports":
			k.a.DockerPorts = strings.TrimPrefix(v, "all")
		case "api":
			k.a.DockerAPI = strings.TrimPrefix(v, "off")
		case "accent":
			k.a.Accent = v
		}
	}
	c := &choices{secrets: creds.New(), picks: picks, miseExtra: extras, dropped: dropped}
	return c, k.questions(ctx, c, first, saved)
}

// change asks the question for key (see changes) again, for an item
// already set up; the secrets are what it typed (an MCP server's keys).
func (k *asker) change(ctx context.Context, key string) (*creds.Secrets, error) {
	c := &choices{secrets: creds.New(), picks: k.a.Picks, miseExtra: k.a.MiseExtra}
	var err error
	switch key {
	case "git":
		err = k.askGit()
	case "prompt":
		err = k.askPrompt()
	case "dotfiles":
		err = k.askDotfiles()
	case "mcp":
		if c.items, err = plan.Resolve(k.cat, c.picks, len(c.miseExtra) > 0); err == nil {
			err = k.askMCP(ctx, c, planHas(c.items), false)
		}
	default:
		err = fmt.Errorf("nothing to change for %q", key)
	}
	return c.secrets, err
}

// questions asks every text prompt in one place, for c's picks: what's
// new since saved (all of it on a first run), plus what every plan needs.
func (k *asker) questions(ctx context.Context, c *choices, first bool, saved []string) error {
	var err error
	if c.items, err = plan.Resolve(k.cat, c.picks, len(c.miseExtra) > 0); err != nil {
		return err
	}
	// Questions are for what's new: picks re-applied keep their answers.
	has := planHas(c.items)
	isNew := has
	if !first {
		var added []string
		for _, id := range c.picks {
			if !slices.Contains(saved, id) {
				added = append(added, id)
			}
		}
		newItems, err := plan.Resolve(k.cat, added, false)
		if err != nil {
			return err
		}
		isNew = planHas(newItems)
	}
	if isNew("gitid") {
		if err := k.askGit(); err != nil {
			return err
		}
	}
	if isNew("zsh") {
		if err := k.askPrompt(); err != nil {
			return err
		}
	}
	// Every plan has tzlocale: asked once.
	if has("tzlocale") && k.a.TZ == "" {
		if err := k.askTZ(); err != nil {
			return err
		}
	}
	if isNew("mcpjungle-install") && !k.listenSet {
		if err := k.askMCPListen(); err != nil {
			return err
		}
	}
	if has("mcpjungle-install") && k.a.MCPListen == "public" && !steps.ValidDomain(k.a.MCPDomain) {
		if err := k.askMCPDomain(); err != nil {
			return err
		}
	}
	if isNew("docker-ports") || (has("docker-ports") && k.a.DockerPorts == "") {
		if err := k.askDockerPorts(); err != nil {
			return err
		}
	}
	if isNew("docker-api") || (has("docker-api") && k.a.DockerAPI == "") {
		if err := k.askDockerAPI(); err != nil {
			return err
		}
	}
	// The server list shows on every run (the saved ones ticked); only
	// new servers are asked about.
	if has("mcpjungle") {
		if err := k.askMCP(ctx, c, has, isNew("mcpjungle")); err != nil {
			return err
		}
	}
	if isNew("dotfiles") {
		if err := k.askDotfiles(); err != nil {
			return err
		}
	}
	return k.askSecrets(c, isNew)
}

// fromProfile plans the profiles' picks (already in the answers) and asks
// only what a profile can't hold: secrets, and settings it left unset.
func (k *asker) fromProfile(ctx context.Context) (*choices, error) {
	a := k.a
	c := &choices{secrets: creds.New(), miseExtra: a.MiseExtra, mcpServers: a.MCPServers}
	var err error
	if c.items, err = plan.Resolve(k.cat, a.Picks, len(a.MiseExtra) > 0); err != nil {
		return nil, err
	}
	if len(c.items) == 0 {
		fmt.Fprintln(k.out, "The profiles pick nothing.")
		return nil, nil
	}
	has := planHas(c.items)
	if has("gitid") && a.Git.Name+a.Git.Email == "" {
		if err := k.askGit(); err != nil {
			return nil, err
		}
	}
	if has("tzlocale") && a.TZ == "" {
		if err := k.askTZ(); err != nil {
			return nil, err
		}
	}
	return c, k.askSecrets(c, has)
}

// uiTabs are the picker's tabs: a group's tabs become one tab, named
// after the group, with them as its sections.
func uiTabs(tabs []catalog.Tab) []ui.Tab {
	var out []ui.Tab
	for _, t := range tabs {
		if t.Group == "" {
			out = append(out, ui.Tab{ID: t.ID, Title: t.Title})
			continue
		}
		i := slices.IndexFunc(out, func(u ui.Tab) bool { return u.ID == "group:"+t.Group })
		if i < 0 {
			out = append(out, ui.Tab{ID: "group:" + t.Group, Title: t.Group})
			i = len(out) - 1
		}
		out[i].Sections = append(out[i].Sections, ui.Tab{ID: t.ID, Title: t.Title})
	}
	return out
}

// planHas reports whether an id is in the plan.
func planHas(items []*catalog.Item) func(string) bool {
	picks := " " + strings.Join(plan.IDs(items), " ") + " "
	return func(id string) bool { return strings.Contains(picks, " "+id+" ") }
}

// askUser asks which user to set up, offering to create a missing one.
func askUser(def string) (string, error) {
	u, err := ui.Input("Set up for which user?", "", def)
	if err != nil {
		return "", err
	}
	if _, err := user.Lookup(u); err != nil {
		if !profile.ValidUser(u) {
			return "", fmt.Errorf("user %q does not exist", u)
		}
		yes, err := ui.Confirm("User "+u+" does not exist. Create it?", "Like the VM's first user: passwordless sudo and the same groups.", false)
		if err != nil {
			return "", err
		}
		if !yes {
			return "", fmt.Errorf("user %q does not exist", u)
		}
	}
	return u, nil
}

// askSecrets asks for the credentials and secrets of the plan's items.
func (k *asker) askSecrets(c *choices, has func(string) bool) error {
	var err error
	if has("github") {
		if err := k.askGitHub(c); err != nil {
			return err
		}
	}
	if has("gitlab") {
		if err := k.askGitLab(); err != nil {
			return err
		}
	}
	if has("bitbucket") {
		if err := k.askGitHost("Bitbucket", "bitbucket.org", &k.a.Bitbucket); err != nil {
			return err
		}
	}
	if err := k.askCreds(c, has); err != nil {
		return err
	}
	if has("sops") {
		if err := k.askAgeKey(c); err != nil {
			return err
		}
	}
	if has("tailscale") && exec.Command("tailscale", "status").Run() != nil {
		if c.secrets.TSAuthKey, err = ui.Password("Tailscale auth key (empty: run 'sudo tailscale up' later)", ""); err != nil {
			return err
		}
	}
	if has("netbird") && !netbirdConnected() {
		if c.secrets.NBSetupKey, err = ui.Password("NetBird setup key (empty: run 'sudo netbird up' later)", ""); err != nil {
			return err
		}
		if c.secrets.NBSetupKey != "" {
			if c.secrets.NBManagementURL, err = ui.Input("NetBird management URL", "Empty for NetBird's cloud; your own server's URL (https://netbird.example.com) if you self-host.", ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// netbirdConnected reports whether NetBird is logged in to its management
// server already.
func netbirdConnected() bool {
	out, err := exec.Command("netbird", "status").Output()
	return err == nil && strings.Contains(string(out), "Management: Connected")
}

func (k *asker) gitConfig(key string) string {
	out, _ := exec.Command("runuser", "-u", k.a.User, "--", "git", "config", "--global", key).Output()
	return strings.TrimSpace(string(out))
}

func (k *asker) askGit() error {
	name, email := k.gitConfig("user.name"), k.gitConfig("user.email")
	if name == "" {
		name = k.a.Git.Name
	}
	if email == "" {
		email = k.a.Git.Email
	}
	var err error
	desc := "Leave empty to take it from your GitHub account after the gh login."
	if k.a.Git.Name, err = ui.Input("Git user.name", desc, name); err != nil {
		return err
	}
	k.a.Git.Email, err = ui.Input("Git user.email", desc, email)
	return err
}

func (k *asker) askTZ() error {
	cur := k.a.TZ
	if cur == "" {
		out, _ := exec.Command("timedatectl", "show", "-p", "Timezone", "--value").Output()
		cur = strings.TrimSpace(string(out))
	}
	if cur == "" {
		cur = "Etc/UTC"
	}
	zones, _ := exec.Command("timedatectl", "list-timezones").Output()
	for {
		tz, err := ui.Input("Timezone (e.g. Europe/Berlin, America/New_York)", "", cur)
		if err != nil {
			return err
		}
		if len(zones) == 0 || strings.Contains("\n"+string(zones), "\n"+tz+"\n") {
			k.a.TZ = tz
			return nil
		}
		fmt.Fprintf(k.out, "Unknown timezone %q.\n", tz)
		cur = tz
	}
}

// askPrompt asks which zsh prompt to set up.
func (k *asker) askPrompt() error {
	p10k := "Powerlevel10k (lean, one line, icons; p10k configure restyles it)"
	starship := "Starship (one line, fast; ~/.config/starship.toml)"
	cur := p10k
	if k.a.Prompt == "starship" {
		cur = starship
	}
	ch, err := ui.Choose("zsh prompt", []string{p10k, starship}, cur)
	if err != nil {
		return err
	}
	k.a.Prompt = "p10k"
	if ch == starship {
		k.a.Prompt = "starship"
	}
	return nil
}

// askDotfiles asks for the dotfiles repo.
func (k *asker) askDotfiles() error {
	cur := k.a.Dotfiles
	for {
		v, err := ui.Input("Dotfiles repo (e.g. https://github.com/you/dotfiles or git@github.com:you/dotfiles)", "A private repo over SSH needs a key the VM can use (or ssh -A).", cur)
		if err != nil {
			return err
		}
		if err := answers.CheckRepo(v); err != nil {
			fmt.Fprintln(k.out, err)
			cur = v
			continue
		}
		k.a.Dotfiles = v
		return nil
	}
}

// askMCPListen asks where the gateway listens (its unit binds it).
func (k *asker) askMCPListen() error {
	one := "this machine only (no tokens)"
	vpn := "this machine and your VPNs: Tailscale, NetBird, WireGuard, OpenVPN (clients need a token)"
	pub := "the internet, at a domain name: HTTPS by Caddy, fail2ban (clients need a token)"
	all := "everywhere, plain HTTP (clients need a token; trusted networks only)"
	sel := map[string]string{"vpn": vpn, "tailscale": vpn, "public": pub, "0.0.0.0": all}[k.a.MCPListen]
	if sel == "" {
		sel = one
	}
	l, err := ui.Choose("Who can reach the MCPJungle gateway?", []string{one, vpn, pub, all}, sel)
	if err != nil {
		return err
	}
	k.a.MCPListen = map[string]string{one: "127.0.0.1", vpn: "vpn", pub: "public", all: "0.0.0.0"}[l]
	if k.a.MCPListen == "public" {
		return k.askMCPDomain()
	}
	return nil
}

// askMCPDomain asks the gateway's domain name, for "public".
func (k *asker) askMCPDomain() error {
	for {
		d, err := ui.Input("The MCP gateway's domain name (e.g. mcp.example.com)", "Its DNS must point at this machine, with TCP 80 and 443 open, for the certificate.", k.a.MCPDomain)
		if err != nil {
			return err
		}
		if d = strings.ToLower(strings.TrimSpace(d)); steps.ValidDomain(d) {
			k.a.MCPDomain = d
			return nil
		}
		fmt.Fprintf(k.out, "Not a domain name: %q\n", d)
	}
}

// askDockerAPI asks who can reach the Docker API over TLS.
func (k *asker) askDockerAPI() error {
	lan := "the local network (10/8, 172.16/12, 192.168/16, Tailscale's 100.64/10) and your VPNs"
	vpn := "your VPNs only (Tailscale, NetBird, WireGuard, OpenVPN)"
	sel := lan
	if k.a.DockerAPI == "vpn" {
		sel = vpn
	}
	ch, err := ui.Choose("The Docker API over TLS (port 2376, client certificates) can be reached from", []string{lan, vpn}, sel)
	if err != nil {
		return err
	}
	k.a.DockerAPI = map[string]string{lan: "lan", vpn: "vpn"}[ch]
	return nil
}

// askDockerPorts asks where containers' published ports can be reached
// from, besides this machine.
func (k *asker) askDockerPorts() error {
	vpn := "this machine and your VPNs (Tailscale, NetBird, WireGuard, OpenVPN)"
	local := "this machine only"
	sel := vpn
	if k.a.DockerPorts == "local" {
		sel = local
	}
	ch, err := ui.Choose("Containers' published ports (docker run -p) can be reached from", []string{vpn, local}, sel)
	if err != nil {
		return err
	}
	k.a.DockerPorts = map[string]string{vpn: "vpn", local: "local"}[ch]
	return nil
}

// askMCP asks for the MCP servers, their keys, and whether to add the
// gateway to the agents.
func (k *asker) askMCP(ctx context.Context, c *choices, has func(string) bool, fresh bool) error {
	dirs := mcp.Dirs(k.share)
	var entries []*ui.Entry
	specs := map[string]*mcp.Spec{}
	for _, n := range mcp.Names(dirs) {
		s, err := mcp.Load(ctx, dirs, n)
		if err != nil {
			continue
		}
		specs[n] = s
		entries = append(entries, &ui.Entry{Tab: "mcp", ID: n, Label: n, Source: s.Description + " · " + s.Image + s.URL, On: slices.Contains(k.a.MCPServers, n)})
	}
	servers, out, err := ui.NewPicker("MCPJungle — which MCP servers?", []ui.Tab{{ID: "mcp", Title: "MCP servers"}}, entries, ui.Options{}).Run()
	if err != nil {
		return err
	}
	if out != ui.Done {
		return ui.ErrQuit
	}
	c.mcpServers = servers
	// An unticked server stops being kept up; it stays enabled (drop one
	// by editing ~/.mcp/servers).
	prev := k.a.MCPServers
	k.a.MCPServers = servers

	agents := []string{"claude", "codex", "gemini", "opencode", "goose"}
	if (fresh || k.a.WireAgents == nil) && slices.ContainsFunc(agents, func(a string) bool { return has(a) || k.installed(a) }) {
		def := k.a.WireAgents == nil || *k.a.WireAgents
		yes, err := ui.Confirm("Add the MCPJungle gateway to "+k.a.User+"'s coding agents?",
			"Claude Code, Codex, Gemini CLI, OpenCode and Goose (those installed) can then use every MCP server registered in MCPJungle.", def)
		if err != nil {
			return err
		}
		k.a.WireAgents = &yes
	}

	home := homeOf(k.a.User)
	for _, n := range servers {
		s := specs[n]
		if s == nil || len(s.Keys)+len(s.Optional) == 0 || slices.Contains(prev, n) {
			continue
		}
		envf := filepath.Join(home, ".mcp", n, "env")
		envb := readAsOwner(k.a.User, envf)
		keys := append([]string{}, s.Keys...)
		if len(s.Optional) > 0 {
			var opt []*ui.Entry
			for _, o := range s.Optional {
				opt = append(opt, &ui.Entry{Tab: "opt", ID: o, Label: o, Source: "optional · leave unticked to skip"})
			}
			chosen, out, err := ui.NewPicker(n+" — optional settings (enter to skip)", []ui.Tab{{ID: "opt", Title: "Optional settings"}}, opt, ui.Options{}).Run()
			if err != nil {
				return err
			}
			if out == ui.Done {
				keys = append(keys, chosen...)
			}
		}
		fmt.Fprintf(k.out, "\n%s — %s\nLeave a value empty to fill it in later: mcp-sync edit %s\n", n, s.Description, n)
		for _, key := range keys {
			cur := steps.EnvGet(envb, key)
			if cur == "" {
				cur = s.Defaults[key]
			}
			var v string
			if mcp.IsSecret(key) {
				ph := ""
				if cur != "" {
					ph = "(unchanged)"
				}
				if v, err = ui.Password(n+": "+key, ph); err != nil {
					return err
				}
				if v == "" {
					v = cur
				}
			} else if v, err = ui.Input(n+": "+key, "", cur); err != nil {
				return err
			}
			c.secrets.MCP[n+" "+key] = v
		}
	}
	return nil
}

// The ways to reach a git host over SSH, as the host prompts offer them.
const (
	wayAgent = "Forwarded SSH agent (no key on this machine; connect with ssh -A)"
	waySent  = "A key you send from your machine (ssh <this machine> 'sudo yardo gitkey' < <key file>; a deploy or account key)"
)

// chooseWay asks how to reach a git host: ways maps each mode to its
// label, in the order given; the saved mode, else the agent when one is
// forwarded, else a key made here, is the default.
func chooseWay(title, cur string, modes []string, ways map[string]string) (string, error) {
	if cur == "" {
		cur = "key"
		if os.Getenv("SSH_AUTH_SOCK") != "" && exec.Command("ssh-add", "-l").Run() == nil {
			cur = "agent"
		}
	}
	var opts []string
	for _, m := range modes {
		opts = append(opts, ways[m])
	}
	ch, err := ui.Choose(title, opts, ways[cur])
	if err != nil {
		return "", err
	}
	for _, m := range modes {
		if ways[m] == ch {
			return m, nil
		}
	}
	return cur, nil
}

// sentHint says how to send the key for host.
func (k *asker) sentHint(host string) {
	fmt.Fprintf(k.out, "Send the key from your machine:  ssh <this machine> 'sudo yardo gitkey --host %s' < <key file>\n(until then yardo makes one here; the closing notes say where to add it).\n", host)
}

// askGitHost asks how to reach a git host without a CLI login (Bitbucket).
func (k *asker) askGitHost(name, host string, m *answers.GitHost) error {
	mode, err := chooseWay("How should this VM reach "+name+"?", m.Mode, []string{"agent", "key", "sent"}, map[string]string{
		"agent": wayAgent, "key": "A key made here (~/.ssh/" + strings.ToLower(name) + "; the closing notes say where to add it on " + name + ")", "sent": waySent})
	if err != nil {
		return err
	}
	m.Mode = mode
	if mode == "sent" {
		k.sentHint(host)
	}
	return nil
}

// askGitLab asks for the GitLab host (gitlab.com or your own) and how to
// reach it.
func (k *asker) askGitLab() error {
	cur := k.a.GitLabHost()
	for {
		v, err := ui.Input("GitLab host (gitlab.com, or your own GitLab's host name)", "", cur)
		if err != nil {
			return err
		}
		v = strings.TrimSpace(v)
		if answers.ValidHost(v) {
			k.a.GitLab.Host = v
			if v == "gitlab.com" {
				k.a.GitLab.Host = ""
			}
			break
		}
		fmt.Fprintf(k.out, "Not a host name: %q\n", v)
		cur = v
	}
	return k.askGitHost("GitLab", k.a.GitLabHost(), &k.a.GitLab)
}

func (k *asker) askGitHub(c *choices) error {
	mode, err := chooseWay("How should this VM reach GitHub?", k.a.GitHub.Mode, []string{"agent", "key", "sent", "token"}, map[string]string{
		"agent": wayAgent, "key": "A key made here (~/.ssh/github, uploaded to GitHub)", "sent": waySent,
		"token": "A fine-grained token (git over HTTPS; no key)"})
	if err != nil {
		return err
	}
	k.a.GitHub.Mode = mode
	if mode == "sent" {
		k.sentHint("github.com")
	}
	home := homeOf(k.a.User)
	if k.a.GitHub.Mode == "key" {
		key := steps.OwnKey("github", func(p string) bool {
			_, err := os.Stat(filepath.Join(home, strings.TrimPrefix(p, "~/")))
			return err == nil
		})
		if _, err := os.Stat(filepath.Join(home, strings.TrimPrefix(key, "~/"))); err != nil {
			yes, err := ui.Confirm("Protect the new key with a passphrase?", "ssh-keygen asks for it after the installs.", false)
			if err != nil {
				return err
			}
			if yes {
				c.secrets.GHPassphrase = "yes"
			}
		}
	}
	if k.a.GitHub.Mode == "token" && exec.Command("runuser", "-u", k.a.User, "--", "gh", "auth", "status", "-h", "github.com").Run() != nil {
		fmt.Fprintln(k.out, "Create one at https://github.com/settings/personal-access-tokens (Contents: read/write on your repos).")
		if c.secrets.GHToken, err = ui.Password("GitHub fine-grained token (empty: later)", ""); err != nil {
			return err
		}
	}
	return nil
}

func (k *asker) askCreds(c *choices, has func(string) bool) error {
	for _, it := range k.cat.Items {
		if !has(it.ID) {
			continue
		}
		shown := false
		for _, s := range it.Steps {
			if s.Type != "cred" {
				continue
			}
			for _, key := range s.Keys {
				if key.When != "" && !has(key.When) && !k.installed(key.When) {
					continue
				}
				if !shown {
					fmt.Fprintf(k.out, "\n%s\nLeave a value empty to fill it in later: ~/yardo.txt lists what is missing.\n", it.Label)
					shown = true
				}
				v, err := ui.Password(key.Prompt, "")
				if err != nil {
					return err
				}
				c.secrets.Cred[it.ID+" "+key.Name] = v
			}
		}
	}
	return nil
}

func (k *asker) askAgeKey(c *choices) error {
	f := filepath.Join(homeOf(k.a.User), ".config", "sops", "age", "keys.txt")
	// A key line, not the skeleton's comment that mentions one.
	if b, err := os.ReadFile(f); err == nil && steps.HasAgeKey(b) {
		c.secrets.AgeChoice = "keep"
		return nil
	}
	ch, err := ui.Choose("age key for "+k.a.User, []string{"Generate a new key", "Paste an existing key", "Later"}, "Generate a new key")
	if err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(ch, "Gen"):
		c.secrets.AgeChoice = "generate"
	case strings.HasPrefix(ch, "Paste"):
		c.secrets.AgeChoice = "paste"
		if c.secrets.AgeKey, err = ui.Password("age secret key (AGE-SECRET-KEY-1…)", ""); err != nil {
			return err
		}
		if c.secrets.AgeKey == "" {
			c.secrets.AgeChoice = "later"
		}
	default:
		c.secrets.AgeChoice = "later"
	}
	return nil
}

// review shows the plan and what the profiles dropped, and asks which of
// options (Install first).
func (k *asker) review(c *choices, options ...string) string {
	if _, err := user.Lookup(k.a.User); err != nil {
		fmt.Fprintf(k.out, "\nFor user: %s (new account)\n", k.a.User)
	} else {
		fmt.Fprintf(k.out, "\nFor user: %s\n", k.a.User)
	}
	// Installing first, then configuring: the order the run takes.
	for _, conf := range []bool{false, true} {
		var picked, always []string
		for _, it := range c.items {
			switch {
			case it.Hidden() || (it.Configure() || it.Last) != conf:
			case it.Always:
				short, _, _ := strings.Cut(it.Label, " (")
				always = append(always, short)
			case conf && beforeApt(it):
				picked = append(picked, it.Label+" — applied first")
			default:
				picked = append(picked, it.Label)
			}
		}
		if !conf && len(c.miseExtra) > 0 {
			picked = append(picked, "Extra mise tools: "+strings.Join(c.miseExtra, " "))
		}
		fmt.Fprintln(k.out, map[bool]string{false: "Install:", true: "Configure:"}[conf])
		for _, l := range picked {
			fmt.Fprintf(k.out, "  ✓ %s\n", l)
		}
		if len(always) > 0 {
			fmt.Fprintf(k.out, "  Always: %s\n", strings.Join(always, " · "))
		}
		if len(picked)+len(always) == 0 {
			fmt.Fprintln(k.out, "  (nothing)")
		}
	}
	if len(c.mcpServers) > 0 {
		port := k.a.MCPPort
		if port == 0 && k.port != nil {
			port = k.port()
		}
		host := k.a.MCPListen
		if host == "tailscale" {
			host = "<tailscale address>"
		}
		fmt.Fprintf(k.out, "  MCPJungle: http://%s:%d/mcp · servers: %s\n", host, port, strings.Join(k.a.MCPServers, " "))
	}
	if k.a.Git.Name+k.a.Git.Email != "" {
		fmt.Fprintf(k.out, "  Git: %s <%s>\n", k.a.Git.Name, k.a.Git.Email)
	}
	listDropped(k.out, k.cat, k.dropped)
	ch, err := ui.Choose("Ready?", options, options[0])
	if err != nil {
		return "Quit"
	}
	return ch
}

// beforeApt reports whether the item is applied before anything installs
// (the CA bundle), wherever its phase puts it.
func beforeApt(it *catalog.Item) bool {
	for _, s := range it.Steps {
		if st, err := steps.New(it.ID, s); err == nil {
			if b, ok := st.(steps.BeforeApt); ok && b.BeforeApt() {
				return true
			}
		}
	}
	return false
}

func homeOf(name string) string {
	if u, err := user.Lookup(name); err == nil {
		return u.HomeDir
	}
	return "/home/" + name
}

// readAsOwner reads a file under login's ~/.mcp as its owner: the
// gateway's service user, or login before there is one. A symlink planted
// there then can't show root's files.
func readAsOwner(login, path string) []byte {
	owner := "mcp-" + login
	if _, err := user.Lookup(owner); err != nil {
		owner = login
	}
	out, err := exec.Command("runuser", "-u", owner, "--", "cat", "--", path).Output()
	if err != nil {
		return nil
	}
	return out
}
