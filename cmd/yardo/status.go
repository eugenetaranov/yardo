package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/addon"
	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/mcp"
	"github.com/eugenetaranov/yardo/internal/plan"
	"github.com/eugenetaranov/yardo/internal/run"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/tui"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// statusSchema versions `yardo status --json`; pmox and scripts read it.
const statusSchema = 1

// statusReport is `yardo status --json`: every user's items.
type statusReport struct {
	Schema    int          `json:"schema"`
	Host      string       `json:"host"`
	CheckedAt string       `json:"checked_at"`
	Users     []userStatus `json:"users"`
}

// userStatus is one user's items, problems first.
type userStatus struct {
	User   string         `json:"user"`
	Counts map[string]int `json:"counts"`
	Items  []itemStatus   `json:"items"`
}

type itemStatus struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Tab    string `json:"tab"`
	Scope  string `json:"scope"` // system or user
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
	// Inputs are the values left empty ("file: KEY").
	Inputs []string `json:"inputs,omitempty"`
	// Updates are newer versions of its tools ("terraform 1.9.0 → 1.10.1"),
	// when asked for (--updates).
	Updates []string `json:"updates,omitempty"`
}

// stateOrder sorts problems first: what to fix, then what to fill in.
var stateOrder = map[string]int{"down": 0, "drifted": 1, "outdated": 2, "partial": 3, "missing": 4, "secret": 5, "unchecked": 6, "ok": 7}

// userStatusOf checks user's items (and extra mise tools) without changing
// anything. An item that yardo applied before (applied) and that is now
// missing or partial has drifted.
// With updates, it also asks mise and apt what's outdated (slow).
func userStatusOf(ctx context.Context, env *steps.Env, user string, items []*catalog.Item, extra []string, applied map[string]string, updates bool) userStatus {
	us := userStatus{User: user, Counts: map[string]int{}}
	var mise, apt map[string]string
	if updates {
		mise, apt = steps.Outdated(ctx, env)
	}
	for _, s := range run.Verify(ctx, env, items) {
		if s.Item.Hidden() {
			continue
		}
		scope := "user"
		if s.Item.System() {
			scope = "system"
		}
		state := s.State
		if (state == "missing" || state == "partial") && applied[s.Item.ID] != "" {
			state = "drifted"
			// Applied by another yardo: its files may just be older than
			// this one's, not changed by hand.
			if _, by, ok := strings.Cut(applied[s.Item.ID], " "); ok && by != version {
				state = "outdated"
			}
		}
		us.Items = append(us.Items, itemStatus{ID: s.Item.ID, Label: s.Item.Label, Tab: s.Item.Tab, Scope: scope,
			State: state, Detail: s.Detail, Done: s.Done, Total: s.Total, Inputs: s.Inputs,
			Updates: steps.ItemUpdates(env, s.Item.Steps, mise, apt)})
	}
	for _, t := range extra {
		st := itemStatus{ID: "mise:" + steps.MiseName(t), Label: t + " (extra mise tool)", Tab: "extra", Scope: "user", State: "ok", Total: 1, Done: 1}
		if ok, _ := steps.MiseHas(ctx, env, []string{t}); !ok {
			st.State, st.Done, st.Detail = "missing", 0, "mise use -g "+t
		}
		if v := mise[steps.MiseName(t)]; v != "" {
			st.Updates = []string{steps.MiseName(t) + " " + v}
		}
		us.Items = append(us.Items, st)
	}
	sort.SliceStable(us.Items, func(i, j int) bool { return stateOrder[us.Items[i].State] < stateOrder[us.Items[j].State] })
	for _, it := range us.Items {
		us.Counts[it.State]++
		if len(it.Updates) > 0 {
			us.Counts["updates"]++
		}
	}
	return us
}

// printStatus writes one user's status for people: a summary line, then
// each item, problems first.
func printStatus(w io.Writer, us userStatus) {
	var sum []string
	for _, s := range []struct{ state, say string }{
		{"ok", "ok"}, {"down", "down"}, {"drifted", "drifted"}, {"outdated", "behind this yardo"}, {"partial", "partial"}, {"missing", "missing"},
		{"secret", "with values to fill in"}, {"unchecked", "not checked"},
	} {
		if n := us.Counts[s.state]; n > 0 {
			sum = append(sum, fmt.Sprintf("%d %s", n, s.say))
		}
	}
	if n := us.Counts["updates"]; n > 0 {
		sum = append(sum, fmt.Sprintf("%d with updates", n))
	}
	fmt.Fprintf(w, "%s: %s\n", us.User, strings.Join(sum, " · "))
	var upd []string
	for _, it := range us.Items {
		line := fmt.Sprintf("  %-9s %s", it.State, it.Label)
		if len(it.Updates) > 0 {
			upd = append(upd, fmt.Sprintf("  update    %s: %s", it.Label, strings.Join(it.Updates, ", ")))
		}
		switch {
		case it.State == "partial":
			line += fmt.Sprintf(" (%d of %d steps): %s", it.Done, it.Total, it.Detail)
		case it.State == "secret":
			line += ": " + strings.Join(it.Inputs, ", ")
		case it.Detail != "" && it.State != "ok":
			line += ": " + it.Detail
		}
		fmt.Fprintln(w, line)
	}
	for _, l := range upd {
		fmt.Fprintln(w, l)
	}
}

// appliedFile is when, and by which yardo version, each of user's items
// was last applied as it was meant to be (status/<user>.applied.json,
// "<time> <version>"; older files have the time only): what "drifted" and
// "outdated" go by.
func appliedFile(state, user string) string {
	return filepath.Join(state, "status", user+".applied.json")
}

func loadApplied(state, user string) map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(appliedFile(state, user)); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

// markApplied records that ids were applied, as meant, at now, by this
// yardo.
func markApplied(state, user string, ids []string, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	m := loadApplied(state, user)
	for _, id := range ids {
		m[id] = now.UTC().Format(time.RFC3339) + " " + version
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(appliedFile(state, user)), 0o700); err != nil {
		return err
	}
	return os.WriteFile(appliedFile(state, user), append(b, '\n'), 0o600)
}

// userStatusCache is where the user's own copy of their status is, for
// `yardo status` without sudo.
const userStatusCache = ".cache/yardo/status.json"

// saveStatus keeps the user's latest status in the state directory
// (status/<user>.json), for a dashboard to show at once, and a copy in the
// user's ~/.cache/yardo, written as them (so a symlink there leads
// nowhere they couldn't write themselves).
func saveStatus(ctx context.Context, env *steps.Env, state string, us userStatus, now time.Time) error {
	b, err := json.MarshalIndent(struct {
		Schema    int    `json:"schema"`
		CheckedAt string `json:"checked_at"`
		userStatus
	}{statusSchema, now.UTC().Format(time.RFC3339), us}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if env != nil {
		_, _ = env.Exec.Run(ctx, steps.Cmd{Name: "sh", Args: []string{"-c",
			`umask 077 && mkdir -p "$HOME/.cache/yardo" && cat > "$HOME/` + userStatusCache + `.tmp" && mv -f "$HOME/` + userStatusCache + `.tmp" "$HOME/` + userStatusCache + `"`},
			Stdin: b, AsUser: true})
	}
	return saveStatusFile(state, us.User, b)
}

func saveStatusFile(state, user string, b []byte) error {
	dir := filepath.Join(state, "status")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p := filepath.Join(dir, user+".json")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// writeStatusJSON writes the report (`yardo status --json`).
func writeStatusJSON(w io.Writer, users []userStatus, now time.Time) error {
	host, _ := os.Hostname()
	if users == nil {
		users = []userStatus{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(statusReport{Schema: statusSchema, Host: host, CheckedAt: now.UTC().Format(time.RFC3339), Users: users})
}

// changes are the picked items whose answers enter can change (asked
// again, then the item applied again): label and asker.change's key.
var changes = map[string][2]string{
	"gitid":     {"git name and email", "git"},
	"zsh":       {"the zsh prompt", "prompt"},
	"dotfiles":  {"the dotfiles repo", "dotfiles"},
	"mcpjungle": {"the MCP servers", "mcp"},
	"github":    {"its SSH key", "key:github"},
	"gitlab":    {"its SSH key", "key:gitlab"},
	"bitbucket": {"its SSH key", "key:bitbucket"},
}

// sshAcceptNew is the Settings row for accepting new hosts' SSH keys.
func sshAcceptNew(a *answers.Answers) *ui.BoardSetting {
	return &ui.BoardSetting{Key: "sshnew", Label: "Accept new hosts' SSH keys", Value: cmp.Or(a.SSHAcceptNew, "off"), Choices: [][2]string{
		{"off", "ask me (ssh's way)"}, {"on", "accept new, refuse changed"}}}
}

// screen is what the menu and the board show: every item, ticked when
// picked, with its health; the settings (who can reach what, the look);
// how to connect from a laptop.
type screen struct {
	info     ui.HomeInfo
	sub      string
	first    bool
	updates  bool
	tabs     []ui.Tab
	items    []*ui.BoardItem
	settings []*ui.BoardSetting
	conn     ui.ConnData
}

// configureInto is the board tab each configure tab is listed under (a
// group's tab), when not a tab of its own.
var configureInto = map[string]string{"agents": "group:AI", "logins": "group:System"}

// screenData checks the user's items (not on a first run: nothing's there
// yet) and gathers what the screens show. Newer versions come from the
// check for updates (withUpdates).
func (o *options) screenData(ctx context.Context, cat *catalog.Catalog, a *answers.Answers, first bool) (*screen, error) {
	status := map[string]itemStatus{}
	now := time.Now()
	if !first {
		fmt.Printf("Checking what's set up for %s…\n", a.User)
		items, err := plan.Resolve(cat, a.Picks, len(a.MiseExtra) > 0)
		if err != nil {
			return nil, err
		}
		env, err := o.env(a, nil, nil)
		if err != nil {
			return nil, err
		}
		us := userStatusOf(ctx, env, a.User, items, a.MiseExtra, loadApplied(o.state, a.User), false)
		_ = saveStatus(ctx, env, o.state, us, now)
		for _, it := range us.Items {
			status[it.ID] = it
		}
	}
	// The install tabs, with the configure tabs joined to them: Agent
	// setup under AI, Git & SSH a tab of its own (VPN logins are in their
	// tools' rows).
	// Who can reach what (sysconf's visible items) is the menu's.
	tabs := uiTabs(cat.PhaseTabs(catalog.PhaseInstall))
	for _, t := range cat.PhaseTabs(catalog.PhaseConfigure) {
		if t.ID == "sysconf" {
			continue
		}
		if n := slices.IndexFunc(tabs, func(u ui.Tab) bool { return u.ID == configureInto[t.ID] }); n >= 0 {
			tabs[n].Sections = append(tabs[n].Sections, ui.Tab{ID: t.ID, Title: t.Title})
		} else {
			tabs = append(tabs, ui.Tab{ID: t.ID, Title: t.Title})
		}
	}

	picked := func(id string) bool { return slices.Contains(a.Picks, id) }
	// inPlan: picked or needed by a pick, for what's served (Pages).
	inPlan := planned(cat, a)
	var items []*ui.BoardItem
	for _, it := range cat.Items {
		// Access settings are Settings' rows, not items.
		if it.Hidden() || it.Always || it.ID == "docker-ports" {
			continue
		}
		bi := &ui.BoardItem{ID: it.ID, Label: it.Label, Tab: it.Tab, Picked: picked(it.ID), Source: it.Describe(),
			PartOf: it.PartOf, Removable: it.Removable(), RemoveSay: it.RemoveSay, KeepSay: it.KeepSay, Protected: it.Protected}
		if first && it.Default {
			bi.Pending = "add"
		}
		if c, ok := changes[it.ID]; ok && bi.Picked {
			bi.Changes = [][2]string{c}
		}
		if bi.Picked && !slices.Contains(a.Hand, it.ID) {
			// The team profile that picked it, by name: the first, when
			// more than one did.
			for _, n := range slices.Sorted(maps.Keys(a.Layers)) {
				if slices.Contains(a.Layers[n], it.ID) {
					bi.Team = n
					break
				}
			}
		}
		if bi.Picked {
			for _, n := range neededBy(cat, a.Picks, it.ID) {
				bi.NeededBy = append(bi.NeededBy, ui.Short(cat.Item(n).Label))
			}
		}
		if st, ok := status[it.ID]; ok && bi.Picked {
			bi.State, bi.Detail, bi.Inputs = st.State, st.Detail, st.Inputs
		}
		items = append(items, bi)
	}
	foldParts(items)
	for _, t := range a.MiseExtra {
		bi := &ui.BoardItem{ID: "mise:" + t, Label: t + " (extra mise tool)", Tab: ui.ExtraTab, Picked: true, Source: "mise use -g " + t,
			Removable: true, RemoveSay: "uninstalls it (mise unuse -g " + t + ")"}
		if st, ok := status["mise:"+steps.MiseName(t)]; ok {
			bi.State, bi.Detail = st.State, st.Detail
		}
		items = append(items, bi)
	}
	// Who can reach what: for what's picked (or, on a first run, queued).
	has := func(id string) bool {
		return inPlan(id) || (first && cat.Item(id) != nil && cat.Item(id).Default)
	}
	var settings []*ui.BoardSetting
	// Who reaches the gateway reaches ~/pages too (at the gateway's domain
	// when public: offered only with the gateway).
	if gw := has("mcpjungle-install"); gw || has("pages") {
		v := a.MCPListen
		switch v {
		case "tailscale":
			v = "vpn"
		case "vpn", "public", "0.0.0.0":
		default:
			v = "127.0.0.1"
		}
		label := "MCP gateway"
		switch {
		case gw && has("pages"):
			label = "MCP gateway and web pages"
		case !gw:
			label = "Web pages (~/pages)"
		}
		choices := [][2]string{{"127.0.0.1", "this machine only"}, {"vpn", "this machine and your VPNs (with a token)"},
			{"public", "the internet, at a domain name (HTTPS, with a token)"}, {"0.0.0.0", "everywhere, plain HTTP (with a token)"}}
		if !gw {
			choices = [][2]string{{"127.0.0.1", "this machine only (an SSH tunnel)"}, {"vpn", "this machine and your VPNs"}, {"0.0.0.0", "everywhere, plain HTTP"}}
			if v == "public" {
				v = "127.0.0.1"
			}
		}
		settings = append(settings, &ui.BoardSetting{Key: "mcp", Label: label, Value: v, Choices: choices})
	}
	if has("docker") {
		ports := a.DockerPorts
		if ports == "" {
			ports = "all"
		}
		settings = append(settings, &ui.BoardSetting{Key: "ports", Label: "Container ports", Value: ports, Choices: [][2]string{
			{"all", "everywhere (Docker's way)"}, {"vpn", "this machine and your VPNs"}, {"local", "this machine only"}}})
	}
	if has("gitid") || has("github") || has("gitlab") || has("bitbucket") {
		settings = append(settings, sshAcceptNew(a))
	}
	// Session on login: the session file's, which the login's question
	// writes too.
	if has("zsh") || has("zshcfg") {
		settings = append(settings, &ui.BoardSetting{Key: "session", Label: "Session on login", Section: ui.ShellSection,
			Value: steps.Session(&steps.Env{Home: homeOf(a.User)}), Choices: [][2]string{
				{"tmux", "tmux"}, {"zellij", "zellij"}, {"ask", "ask at the next ssh login"}, {"off", "off: a plain shell"}}})
	}
	// The editor theme: Vim's colours and Neovim's (the file's, which
	// Settings writes).
	if has("neovim") || has("zsh") || has("shellcfg") {
		et := &ui.BoardSetting{Key: "editortheme", Label: "Editor theme", Section: ui.LookSection,
			Value: steps.EditorTheme(&steps.Env{Home: homeOf(a.User)})}
		for _, t := range steps.EditorThemes {
			et.Choices = append(et.Choices, [2]string{t.Key, t.Name})
		}
		settings = append(settings, et)
	}
	// The theme: yardo's screens.
	theme := &ui.BoardSetting{Key: "theme", Label: "Theme", Section: ui.LookSection, Value: cmp.Or(a.Theme, "classic"), Swatches: map[string]string{}}
	for _, l := range tui.Looks {
		theme.Choices = append(theme.Choices, [2]string{l.Key, l.Name})
		theme.Swatches[l.Key] = cmp.Or(string(l.Accent), steps.AccentHex(a.Accent))
	}
	settings = append(settings, theme)
	// The accent colour: yardo's, the prompt's, tmux's.
	look := &ui.BoardSetting{Key: "accent", Label: "Accent colour", Section: ui.LookSection, Value: a.Accent, Swatches: map[string]string{}}
	if look.Value == "" {
		look.Value = steps.Accents[0].Key
	}
	for _, c := range steps.Accents {
		look.Choices = append(look.Choices, [2]string{c.Key, c.Name})
		look.Swatches[c.Key] = c.Hex
	}
	settings = append(settings, look)
	addrs, _ := withNames(ctx, steps.OSExec{}, addresses())
	tokens := laptopTokens(a.User)
	host, _ := os.Hostname()
	sub := osName()
	if len(addrs) > 0 {
		sub += "  ·  " + addrs[0].ip
	}
	info := ui.HomeInfo{Host: host, User: a.User, Connect: append(connectRows(a, addrs, tokens, picked("mcpjungle")), pagesRows(a, addrs, inPlan("pages"))...)}
	if !first {
		info.Dash = dashboard(ctx)
	}
	if picked("mcpjungle") && !first {
		specs := o.mcpSpecs(ctx)
		// Watchful: servers changed (or never registered) register now.
		sync, down := o.mcpStatus(ctx, a)
		if pending := mcpPending(sync); len(pending) > 0 && !down {
			fmt.Printf("Registering MCP servers: %s…\n", strings.Join(pending, ", "))
			_ = o.mcpSyncNow(ctx, a, io.Discard)
			sync, down = o.mcpStatus(ctx, a)
		}
		info.MCP = mcpRows(a.MCPServers, findMCP(ctx, a.User, specs), status["mcpjungle"].Inputs, specs, len(status) > 0, sync)
		if info.MCP == nil {
			info.MCP = []ui.MCPServer{}
		}
		var names []string
		for _, r := range info.MCP {
			names = append(names, r.Name)
		}
		if len(names) > 0 {
			src := o.mcpSources(ctx, a, names)
			for n := range info.MCP {
				if f, ok := src[info.MCP[n].Name]; ok {
					info.MCP[n].From, info.MCP[n].Cached = f.from, f.cached()
				}
			}
		}
		for _, n := range slices.Sorted(maps.Keys(specs)) {
			if retiredMCP[n] != "" {
				continue
			}
			info.MCPSpecs = append(info.MCPSpecs, ui.MCPSpec{Name: n, About: specs[n].Description})
		}
		// Servers from elsewhere: approved ones say so, with their pin.
		if as, err := addon.Approvals(a.User); err == nil {
			for _, ap := range as {
				for n := range info.MCP {
					if ap.Kind == "mcp" && info.MCP[n].Name == ap.Name {
						info.MCP[n].About = strings.TrimSpace(info.MCP[n].About + " · checked " + ap.At.Format("2006-01-02") + ", " + shortPin(ap.Pin))
					}
				}
			}
		}
		// Servers from elsewhere held for the user's decision.
		if hs, err := addon.List(a.User); err == nil {
			for _, h := range hs {
				if h.Kind == "mcp" {
					info.MCP = append(info.MCP, ui.MCPServer{Entry: h.Name, Name: h.Name, State: "held",
						About: "From " + h.Source + ": " + verdictSays(h.Report.Verdict()) + "."})
				}
			}
		}
		st := status["mcpjungle-install"].State
		info.MCPDown = down || st == "down" || st == "missing"
		info.MCPAllOff = sync["*"].state == "off"
		info.MCPOrphans = orphanNames(mcpOrphans(ctx, a.User))
	}
	if env, err := newEnv(a.User, o.share(), io.Discard, false); err == nil && !first {
		info.Skills, info.SkillsAllOff, _ = skillRows(env)
	}
	info.LastRun = loadLastRun(o.state, a.User, o.log, now)
	info.Keys = keysToAdd(o.state, a, func(id string) bool { return status[id].State == "ok" })
	return &screen{info: info, sub: sub, first: first,
		tabs: tabs, items: items, settings: settings, conn: o.connData(ctx, a, addrs, inPlan)}, nil
}

// withUpdates gives the items the newer versions u found, once it's
// done; what lock pins says so.
func (d *screen) withUpdates(u *updateCheck, lock *answers.Lock) {
	if !u.ready() {
		return
	}
	for _, i := range d.items {
		if i.Picked {
			i.Updates = u.ByID[i.ID]
			i.Pinned = pinned(lock, i.Updates)
		}
	}
	d.updates, d.info.Checked = true, clock(u.At, time.Now())
}

// osName is the OS's name and version (/etc/os-release).
func osName() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return "Linux"
}

// fillIn opens the file holding the first of an item's empty values
// (inputs, "file: KEY") in an editor: as the user for their own files, as
// the gateway's user (mcp-sync edit) for an MCP server's. It returns the
// items to re-apply after, or nil; an MCP server's registers right after,
// and toast says how it went.
func (o *options) fillIn(ctx context.Context, a *answers.Answers, inputs []string) ([]string, string, error) {
	if len(inputs) == 0 {
		return nil, "", nil
	}
	path, _, _ := strings.Cut(inputs[0], ": ")
	home := homeOf(a.User)
	if rest, ok := strings.CutPrefix(path, "~/.mcp/"); ok {
		name := strings.SplitN(rest, "/", 2)[0]
		if err := editMCP(ctx, a.User, name); err != nil {
			return nil, "", err
		}
		fmt.Printf("Registering %s…\n", name)
		_ = o.mcpSyncNow(ctx, a, io.Discard)
		s, _ := o.mcpStatus(ctx, a)
		v, ok := s[name]
		return nil, mcpSays(name, mcpSpecOf(a.MCPServers, name), v, ok), nil
	}
	args := append([]string{"-u", a.User, "--", "sh", "-c", steps.UserShell, "_"}, editorFor(ctx, a.User, home)...)
	c := exec.CommandContext(ctx, "runuser", append(args, strings.Replace(path, "~", home, 1))...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return nil, "", fmt.Errorf("editing %s: %w", path, err)
	}
	return nil, "", nil
}

// mcpSpecs are the catalog's MCP servers, loaded once (the menu's data
// is gathered again after each action).
func (o *options) mcpSpecs(ctx context.Context) map[string]*mcp.Spec {
	if o.specs == nil {
		dirs := mcp.Dirs(o.share())
		o.specs = map[string]*mcp.Spec{}
		for _, n := range mcp.Names(dirs) {
			if s, err := mcp.Load(ctx, dirs, n); err == nil {
				o.specs[n] = s
			}
		}
	}
	return o.specs
}

// editMCP opens MCP server name's ~/.mcp/<name>/env in the editor, as
// ~/.mcp's owner (mcp-sync edit): the gateway's user, or the user.
func editMCP(ctx context.Context, user, name string) error {
	home := homeOf(user)
	// The gateway's user runs no binary from the user's home (their
	// mise's nvim): the system's vim then.
	ed := editorFor(ctx, user, home)[0]
	if strings.HasPrefix(ed, home+"/") {
		ed = systemEditor()
	}
	c := exec.CommandContext(ctx, "runuser", "-u", mcpOwner(user), "--",
		"env", "MCP_DIR="+filepath.Join(home, ".mcp"), "EDITOR="+ed, "VISUAL="+ed, "/usr/local/bin/mcp-sync", "edit", name)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("editing ~/.mcp/%s/env: %w", name, err)
	}
	return nil
}

// editorMark marks the line the user's shell prints its editor on.
const editorMark = "__yardo_editor__"

// editorFor is the user's editor, as a command and its arguments:
// $VISUAL or $EDITOR when yardo has it (its re-run through sudo keeps
// them), else what the user's own shell sets (yardo's shell config sets
// nvim or vim), else vim, else vi.
func editorFor(ctx context.Context, user, home string) []string {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if f := strings.Fields(ed); len(f) > 0 {
		if p, err := exec.LookPath(f[0]); err == nil {
			return append([]string{p}, f[1:]...)
		}
	}
	// Their interactive shell, which reads their rc files: where EDITOR
	// is set. It prints the editor's full path on a marked line (rc files
	// may print other things).
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	q := `e=${VISUAL:-$EDITOR}; [ -n "$e" ] && printf '\n` + editorMark + `\t%s\t%s\n' "$(command -v -- "${e%% *}")" "$e"`
	c := exec.CommandContext(ctx, "runuser", "-u", user, "--", "sh", "-c", `cd && exec "$SHELL" -ic "$1"`, "_", q)
	c.Env = append(os.Environ(), "TERM=dumb")
	out, _ := c.Output()
	if cmd := parseEditor(string(out), home); cmd != nil {
		return cmd
	}
	return []string{systemEditor()}
}

// parseEditor reads the editor off the shell's marked line: its full path,
// then the editor as set (its arguments after the first word).
func parseEditor(out, home string) []string {
	for _, l := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(l), editorMark+"\t")
		if !ok {
			continue
		}
		path, set, _ := strings.Cut(rest, "\t")
		if !strings.HasPrefix(path, "/") {
			return nil // not found, or a shell function or alias
		}
		f := strings.Fields(set)
		if len(f) == 0 {
			return nil
		}
		return append([]string{path}, f[1:]...)
	}
	return nil
}

// systemEditor is vim when it's installed, else vi.
func systemEditor() string {
	if p, err := exec.LookPath("vim"); err == nil {
		return p
	}
	return "vi"
}

// cachedStatus is `yardo status` without sudo: the copy the last check
// left in ~/.cache/yardo, and how old it is.
func cachedStatus(args []string, w io.Writer, now time.Time) error {
	asJSON := slices.Contains(args, "--json") || slices.Contains(args, "-json")
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(home, userStatusCache))
	if err != nil {
		return errors.New("no status checked yet: run 'sudo yardo status'")
	}
	if asJSON {
		_, err := w.Write(b)
		return err
	}
	var s struct {
		CheckedAt string `json:"checked_at"`
		userStatus
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("%s: %w", userStatusCache, err)
	}
	printStatus(w, s.userStatus)
	if t, err := time.Parse(time.RFC3339, s.CheckedAt); err == nil {
		fmt.Fprintf(w, "\nChecked %s; 'sudo yardo status' checks again.\n", ago(now.Sub(t)))
	}
	return nil
}

// ago says how long ago, roughly: "just now", "12m ago", "2h ago", "3d ago".
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// stateRank orders states by how much they need the user: a row shows
// the worst of its tool's and its parts'.
var stateRank = map[string]int{"": 0, "ok": 0, "unchecked": 1, "outdated": 2, "drifted": 3, "partial": 4, "secret": 5, "missing": 6, "down": 7}

// foldParts shows each picked part (a VPN login) in its tool's row: its
// state when it's worse, the values it waits for, its answers to change.
func foldParts(items []*ui.BoardItem) {
	for _, p := range items {
		if p.PartOf == "" || !p.Picked {
			continue
		}
		n := slices.IndexFunc(items, func(i *ui.BoardItem) bool { return i.ID == p.PartOf })
		if n < 0 || !items[n].Picked {
			continue
		}
		t := items[n]
		if stateRank[p.State] > stateRank[t.State] {
			t.State, t.Detail = p.State, ui.Short(p.Label)+": "+p.Detail
		}
		t.Inputs = append(t.Inputs, p.Inputs...)
		t.Changes = append(t.Changes, p.Changes...)
	}
}
