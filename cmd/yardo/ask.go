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
	"github.com/eugenetaranov/yardo/internal/assets"
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
	items                        []*catalog.Item
	secrets                      *creds.Secrets
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
}

// ask runs every prompt, then the review; it loops on "Start over".
func (k *asker) ask(ctx context.Context) (*choices, error) {
	if k.profile {
		c, err := k.fromProfile(ctx)
		if err != nil || c == nil {
			return c, err
		}
		if k.review(c, "Install", "Quit") != "Install" {
			return nil, ui.ErrQuit
		}
		return c, nil
	}
	for {
		c, err := k.once(ctx)
		if err != nil || c == nil {
			return c, err
		}
		switch k.review(c, "Install", "Start over", "Quit") {
		case "Install":
			return c, nil
		case "Start over":
			continue
		default:
			return nil, ui.ErrQuit
		}
	}
}

func (k *asker) once(ctx context.Context) (*choices, error) {
	fmt.Fprintf(k.out, "yardo — setting up %s (another user: --user <name>). Nothing is preselected; run it again any time to add more.\n", k.a.User)

	// 1. Pick, on two screens: what to install (every install category,
	// plus mise's registry), then what to configure (logins, keys,
	// settings) for it. Esc on the second goes back to the first.
	inst := ui.NewPicker("yardo [1/2] Install", uiTabs(k.cat.PhaseTabs(catalog.PhaseInstall)),
		ui.Dedupe(append(ui.CatalogEntries(k.cat, catalog.PhaseInstall), ui.LoadMise(miseRegistry(ctx, k.share))...)),
		ui.Options{Next: "configure"})
	conf := ui.CatalogEntries(k.cat, catalog.PhaseConfigure)
	var installs, configures []string
	for {
		var out ui.Outcome
		var err error
		if installs, out, err = inst.Run(); err != nil {
			return nil, err
		}
		if out != ui.Done {
			return nil, ui.ErrQuit
		}
		shown := k.configurable(conf, installs)
		note := "Installing: nothing"
		if len(installs) > 0 {
			note = "Installing: " + strings.Join(k.shortLabels(installs), " · ")
		}
		cfg := ui.NewPicker("yardo [2/2] Configure", uiTabs(k.cat.PhaseTabs(catalog.PhaseConfigure)), shown,
			ui.Options{Review: true, Back: true, Note: note + " (esc to change)"})
		if configures, out, err = cfg.Run(); err != nil {
			return nil, err
		}
		if out == ui.Done {
			break
		}
		if out == ui.Quit {
			return nil, ui.ErrQuit
		}
	}
	c := &choices{secrets: creds.New()}
	for _, id := range append(installs, configures...) {
		if t, ok := strings.CutPrefix(id, "mise:"); ok {
			c.miseExtra = append(c.miseExtra, t)
		} else {
			c.picks = append(c.picks, id)
		}
	}

	// 2. Type: every text prompt in one place.
	var err error
	if c.items, err = plan.Resolve(k.cat, c.picks, len(c.miseExtra) > 0); err != nil {
		return nil, err
	}
	if len(c.items) == 0 {
		fmt.Fprintln(k.out, "Nothing picked.")
		return nil, nil
	}
	has := planHas(c.items)
	if has("gitid") {
		if err := k.askGit(); err != nil {
			return nil, err
		}
	}
	if has("zsh") {
		if err := k.askPrompt(); err != nil {
			return nil, err
		}
	}
	// Every plan has tzlocale: asked once.
	if has("tzlocale") && k.a.TZ == "" {
		if err := k.askTZ(); err != nil {
			return nil, err
		}
	}
	if has("proxy") {
		if err := k.askProxy(); err != nil {
			return nil, err
		}
	}
	if has("corpca") {
		if err := k.askCA(); err != nil {
			return nil, err
		}
	}
	if has("mcpjungle-install") {
		if err := k.askMCPListen(); err != nil {
			return nil, err
		}
	}
	if has("mcpjungle") {
		if err := k.askMCP(ctx, c, has); err != nil {
			return nil, err
		}
	}
	if has("dotfiles") {
		if err := k.askDotfiles(); err != nil {
			return nil, err
		}
	}
	return c, k.askSecrets(c, has)
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

func uiTabs(tabs []catalog.Tab) []ui.Tab {
	out := make([]ui.Tab, len(tabs))
	for i, t := range tabs {
		out[i] = ui.Tab{ID: t.ID, Title: t.Title}
	}
	return out
}

// shortLabels are the items' labels without their parenthesized details
// (mise tools by name).
func (k *asker) shortLabels(ids []string) []string {
	var out []string
	for _, id := range ids {
		if it := k.cat.Item(id); it != nil {
			short, _, _ := strings.Cut(it.Label, " (")
			out = append(out, short)
		} else {
			out = append(out, strings.TrimPrefix(id, "mise:"))
		}
	}
	return out
}

// configurable are the configure entries that make sense with what's
// installed: those whose install items are picked now, were picked on an
// earlier run, or whose tool is already on PATH (registry logins only
// with Docker, the npm registry only with Node). Entries left out are
// unticked, so going back and dropping Docker drops its logins too.
func (k *asker) configurable(conf []*ui.Entry, installs []string) []*ui.Entry {
	avail := map[string]bool{}
	if items, err := plan.Resolve(k.cat, answers.Merge(k.a.Picks, installs), false); err == nil {
		for _, it := range items {
			avail[it.ID] = true
		}
	}
	ok := func(id string) bool {
		it := k.cat.Item(id)
		if it == nil || it.Hidden() || it.Always || it.Configure() || avail[id] {
			return true
		}
		return k.installed != nil && k.installed(strings.TrimSuffix(id, "-install"))
	}
	var out []*ui.Entry
	for _, e := range conf {
		it := k.cat.Item(e.ID)
		if it == nil || !slices.ContainsFunc(it.Needs, func(n string) bool { return !ok(n) }) {
			out = append(out, e)
		} else {
			e.On = false
		}
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
	if has("registries") {
		if err := k.askRegistries(c); err != nil {
			return err
		}
	}
	if has("sops") {
		if err := k.askAgeKey(c); err != nil {
			return err
		}
	}
	if has("sudopw") && !hasPassword(k.a.User) {
		fmt.Fprintf(k.out, "\n%s has no password; sudo will ask for one, so set it now.\n", k.a.User)
		for {
			pw, err := ui.Password("New password for "+k.a.User+" (empty: set it later with 'sudo passwd "+k.a.User+"')", "")
			if err != nil || pw == "" {
				return err
			}
			again, err := ui.Password("The same password again", "")
			if err != nil {
				return err
			}
			if pw == again {
				c.secrets.SudoPW = pw
				break
			}
			fmt.Fprintln(k.out, "They don't match.")
		}
	}
	if has("tailscale") && exec.Command("tailscale", "status").Run() != nil {
		if c.secrets.TSAuthKey, err = ui.Password("Tailscale auth key (empty: run 'sudo tailscale up' later)", ""); err != nil {
			return err
		}
	}
	return nil
}

// miseRegistry is the live registry when mise is installed, else yardo's
// snapshot in the share directory.
func miseRegistry(ctx context.Context, share string) string {
	if out, err := exec.CommandContext(ctx, "mise", "registry", "-J", "--hide-aliased").Output(); err == nil {
		if tsv := ui.RegistryTSV(out); tsv != "" {
			return tsv
		}
	}
	return string(assets.MiseRegistry())
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

// askProxy asks for the proxy and the hosts that bypass it.
func (k *asker) askProxy() error {
	cur := k.a.Proxy
	if cur.URL == "" {
		cur.URL = os.Getenv("https_proxy")
		if cur.URL == "" {
			cur.URL = os.Getenv("HTTPS_PROXY")
		}
	}
	if cur.NoProxy == "" {
		cur.NoProxy = "localhost,127.0.0.1,::1"
	}
	for {
		u, err := ui.Input("Proxy URL (e.g. http://proxy.example.com:3128)", "No credentials in it: yardo never saves secrets.", cur.URL)
		if err != nil {
			return err
		}
		if err := answers.CheckProxy(u); err != nil {
			fmt.Fprintf(k.out, "%s: %v\n", u, err)
			cur.URL = u
			continue
		}
		cur.URL = u
		break
	}
	for {
		n, err := ui.Input("Hosts that bypass the proxy, comma-separated", "", cur.NoProxy)
		if err != nil {
			return err
		}
		if err := answers.CheckNoProxy(n); err != nil {
			fmt.Fprintln(k.out, err)
			cur.NoProxy = n
			continue
		}
		cur.NoProxy = n
		break
	}
	k.a.Proxy = cur
	return nil
}

// askCA asks where the internal CA bundle comes from.
func (k *asker) askCA() error {
	cur := k.a.CA
	for {
		src, err := ui.Input("CA bundle: a PEM file on this VM, or an https URL", "Behind a TLS-inspecting proxy, copy it to the VM first (the URL can't be trusted yet).", cur.Source)
		if err != nil {
			return err
		}
		cur.Source = src
		if strings.HasPrefix(src, "https://") {
			if cur.SHA256, err = ui.Input("Its SHA-256 (sha256sum bundle.pem)", "", cur.SHA256); err != nil {
				return err
			}
		} else {
			cur.SHA256 = ""
		}
		if err := answers.CheckCA(cur); err != nil {
			fmt.Fprintln(k.out, err)
			continue
		}
		k.a.CA = cur
		return nil
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
	one, ts, all := "this machine only", "the Tailscale address only (your tailnet)", "all interfaces (no auth — trusted networks only)"
	opts := []string{one}
	if out, err := exec.Command("tailscale", "ip", "-4").Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		opts = append(opts, ts)
	}
	opts = append(opts, all)
	sel := map[string]string{"0.0.0.0": all, "tailscale": ts}[k.a.MCPListen]
	if !slices.Contains(opts, sel) {
		sel = one
	}
	l, err := ui.Choose("MCPJungle gateway listens on", opts, sel)
	if err != nil {
		return err
	}
	k.a.MCPListen = map[string]string{one: "127.0.0.1", ts: "tailscale", all: "0.0.0.0"}[l]
	return nil
}

// askMCP asks for the MCP servers, their keys, and whether to add the
// gateway to the agents.
func (k *asker) askMCP(ctx context.Context, c *choices, has func(string) bool) error {
	dirs := mcp.Dirs(k.share)
	var entries []*ui.Entry
	specs := map[string]*mcp.Spec{}
	for _, n := range mcp.Names(dirs) {
		s, err := mcp.Load(ctx, dirs, n)
		if err != nil {
			continue
		}
		specs[n] = s
		entries = append(entries, &ui.Entry{Tab: "mcp", ID: n, Label: n, Source: s.Description + " · " + s.Image + s.URL})
	}
	servers, out, err := ui.NewPicker("MCPJungle — which MCP servers?", []ui.Tab{{ID: "mcp", Title: "MCP servers"}}, entries, ui.Options{}).Run()
	if err != nil {
		return err
	}
	if out != ui.Done {
		return ui.ErrQuit
	}
	c.mcpServers = servers
	// Servers set up before stay enabled (drop one by editing ~/.mcp/servers).
	k.a.MCPServers = answers.Merge(k.a.MCPServers, servers)

	agents := []string{"claude", "codex", "gemini", "opencode", "goose"}
	if slices.ContainsFunc(agents, func(a string) bool { return has(a) || k.installed(a) }) {
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
		if s == nil || len(s.Keys)+len(s.Optional) == 0 {
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
		"agent": wayAgent, "key": "A key made here (~/.ssh/id_ed25519; the closing notes say where to add it on " + name + ")", "sent": waySent})
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
		"agent": wayAgent, "key": "A key made here (~/.ssh/id_ed25519, uploaded to GitHub)", "sent": waySent,
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
		if _, err := os.Stat(filepath.Join(home, ".ssh", "id_ed25519")); err != nil {
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
				if key.Secret() {
					v, err := ui.Password(key.Prompt, "")
					if err != nil {
						return err
					}
					c.secrets.Cred[it.ID+" "+key.Name] = v
					continue
				}
				p := answerField(k.a, key.Answer)
				cur := *p
				if cur == "" {
					cur = key.Default
				}
				v, err := ui.Input(key.Prompt, "", cur)
				if err != nil {
					return err
				}
				*p = v
			}
		}
	}
	if s := k.a.NPM.Scope; s != "" && !strings.HasPrefix(s, "@") {
		k.a.NPM.Scope = "@" + s
	}
	return nil
}

// answerField is the saved answer a cred key's setting goes in
// (catalog.CredAnswers).
func answerField(a *answers.Answers, name string) *string {
	switch name {
	case "npm.scope":
		return &a.NPM.Scope
	case "npm.registry":
		return &a.NPM.Registry
	}
	var discard string
	return &discard
}

func (k *asker) askRegistries(c *choices) error {
	def := strings.Join(k.a.Registries, " ")
	if def == "" {
		def = "ghcr.io"
	}
	v, err := ui.Input("Registries, space-separated (e.g. ghcr.io registry.gitlab.com)",
		"Each is logged in with 'docker login'. Empty password: later (ghcr.io uses the gh token when it has read:packages).", def)
	if err != nil {
		return err
	}
	k.a.Registries = strings.Fields(v)
	for _, r := range k.a.Registries {
		if c.secrets.RegUser[r], err = ui.Input(r+": username", "", ""); err != nil {
			return err
		}
		if c.secrets.RegPass[r], err = ui.Password(r+": password or token", ""); err != nil {
			return err
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
// (the proxy, the CA bundle), wherever its phase puts it.
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

// hasPassword reports whether the user has a usable password (passwd -S).
func hasPassword(name string) bool {
	out, err := exec.Command("passwd", "-S", name).Output()
	f := strings.Fields(string(out))
	return err == nil && len(f) > 1 && f[1] == "P"
}
