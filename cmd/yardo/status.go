package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/plan"
	"github.com/eugenetaranov/yardo/internal/run"
	"github.com/eugenetaranov/yardo/internal/steps"
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
var stateOrder = map[string]int{"down": 0, "drifted": 1, "partial": 2, "missing": 3, "secret": 4, "unchecked": 5, "ok": 6}

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
		{"ok", "ok"}, {"down", "down"}, {"drifted", "drifted"}, {"partial", "partial"}, {"missing", "missing"},
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

// appliedFile is when yardo last applied each of user's items, as it was
// meant to be (status/<user>.applied.json): what "drifted" goes by.
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

// markApplied records that ids were applied, as meant, at now.
func markApplied(state, user string, ids []string, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	m := loadApplied(state, user)
	for _, id := range ids {
		m[id] = now.UTC().Format(time.RFC3339)
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

// board checks the user's items (not on a first run: nothing's there
// yet) and shows the board: every item, ticked when picked, with its
// health; This VM's access settings and actions. With updates, it also
// checks for newer versions (slow).
// changes are the picked items whose answers enter can change (asked
// again, then the item applied again): label and asker.change's key.
var changes = map[string][2]string{
	"gitid":     {"git name and email", "git"},
	"zsh":       {"the zsh prompt", "prompt"},
	"dotfiles":  {"the dotfiles repo", "dotfiles"},
	"mcpjungle": {"the MCP servers", "mcp"},
}

func (o *options) board(ctx context.Context, cat *catalog.Catalog, a *answers.Answers, first, updates bool) (ui.BoardResult, error) {
	status := map[string]itemStatus{}
	now := time.Now()
	if !first {
		fmt.Printf("Checking what's set up for %s…\n", a.User)
		if updates {
			fmt.Println("And for newer versions (apt and mise): this takes a while…")
		}
		items, err := plan.Resolve(cat, a.Picks, len(a.MiseExtra) > 0)
		if err != nil {
			return ui.BoardResult{}, err
		}
		env, err := o.env(a, nil, nil)
		if err != nil {
			return ui.BoardResult{}, err
		}
		us := userStatusOf(ctx, env, a.User, items, a.MiseExtra, loadApplied(o.state, a.User), updates)
		_ = saveStatus(ctx, env, o.state, us, now)
		for _, it := range us.Items {
			status[it.ID] = it
		}
	}
	// The install tabs, then This VM: the configure tabs, who can reach
	// what, and the machine's actions.
	tabs := uiTabs(cat.PhaseTabs(catalog.PhaseInstall))
	vm := ui.Tab{ID: "__vm__", Title: "This VM"}
	for _, t := range cat.PhaseTabs(catalog.PhaseConfigure) {
		vm.Sections = append(vm.Sections, ui.Tab{ID: t.ID, Title: t.Title})
	}
	vm.Sections = append(vm.Sections, ui.Tab{ID: "__access__", Title: "Who can reach it"}, ui.Tab{ID: "__look__", Title: "Look"},
		ui.Tab{ID: "__machine__", Title: "This machine"})
	tabs = append(tabs, vm)

	picked := func(id string) bool { return slices.Contains(a.Picks, id) }
	var items []*ui.BoardItem
	for _, it := range cat.Items {
		// Access settings are This VM's rows, not items.
		if it.Hidden() || it.Always || it.ID == "docker-ports" || it.ID == "docker-api" {
			continue
		}
		bi := &ui.BoardItem{ID: it.ID, Label: it.Label, Tab: it.Tab, Picked: picked(it.ID), Source: it.Describe()}
		if first && it.Default {
			bi.Pending = "add"
		}
		if c, ok := changes[it.ID]; ok && bi.Picked {
			bi.Changes = [][2]string{c}
		}
		if st, ok := status[it.ID]; ok && bi.Picked {
			bi.State, bi.Detail, bi.Inputs, bi.Updates = st.State, st.Detail, st.Inputs, st.Updates
		}
		items = append(items, bi)
	}
	for _, t := range a.MiseExtra {
		bi := &ui.BoardItem{ID: "mise:" + t, Label: t + " (extra mise tool)", Tab: ui.ExtraTab, Picked: true, Source: "mise use -g " + t}
		if st, ok := status["mise:"+steps.MiseName(t)]; ok {
			bi.State, bi.Detail, bi.Updates = st.State, st.Detail, st.Updates
		}
		items = append(items, bi)
	}
	// Who can reach what: for what's picked (or, on a first run, queued).
	has := func(id string) bool {
		return picked(id) || (first && cat.Item(id) != nil && cat.Item(id).Default)
	}
	var settings []*ui.BoardSetting
	if has("mcpjungle-install") {
		v := a.MCPListen
		switch v {
		case "tailscale":
			v = "vpn"
		case "vpn", "public", "0.0.0.0":
		default:
			v = "127.0.0.1"
		}
		settings = append(settings, &ui.BoardSetting{Key: "mcp", Label: "MCP gateway", Value: v, Choices: [][2]string{
			{"127.0.0.1", "this machine only"}, {"vpn", "this machine and your VPNs (with a token)"},
			{"public", "the internet, at a domain name (HTTPS, with a token)"}, {"0.0.0.0", "everywhere, plain HTTP (with a token)"}}})
	}
	if has("docker") {
		ports := a.DockerPorts
		if ports == "" {
			ports = "all"
		}
		settings = append(settings, &ui.BoardSetting{Key: "ports", Label: "Container ports", Value: ports, Choices: [][2]string{
			{"all", "everywhere (Docker's way)"}, {"vpn", "this machine and your VPNs"}, {"local", "this machine only"}}})
		api := a.DockerAPI
		if api == "" {
			api = "off"
		}
		settings = append(settings, &ui.BoardSetting{Key: "api", Label: "Docker API", Value: api, Choices: [][2]string{
			{"off", "off: Docker over SSH only"}, {"lan", "over TLS, from the local network and your VPNs"}, {"vpn", "over TLS, from your VPNs only"}}})
	}
	// The accent colour: yardo's, the prompt's, tmux's.
	look := &ui.BoardSetting{Key: "accent", Label: "Accent colour", Section: "__look__", Value: a.Accent, Swatches: map[string]string{}}
	if look.Value == "" {
		look.Value = steps.Accents[0].Key
	}
	for _, c := range steps.Accents {
		look.Choices = append(look.Choices, [2]string{c.Key, c.Name})
		look.Swatches[c.Key] = c.Hex
	}
	settings = append(settings, look)
	actions := []ui.BoardAction{
		{ID: "connect", Label: "Connect from a laptop", Note: "Docker over SSH, the MCP gateway"},
		{ID: "updates", Label: "Check for updates", Note: "apt and mise; takes a while"},
		{ID: "refresh", Label: "Check again", Note: "checked " + now.Format("15:04")},
	}
	var buf strings.Builder
	printConnect(&buf, a, addresses())
	host, _ := os.Hostname()
	b := ui.NewBoard(ui.HomeInfo{Host: host, User: a.User, Checked: now.Format("15:04")}, first, tabs, items, settings, actions,
		strings.Split(strings.TrimRight(buf.String(), "\n"), "\n"))
	return b.Run()
}

// fillIn opens the file holding the first of an item's empty values
// (inputs, "file: KEY") in an editor: as the user for their own files, as
// the gateway's user (mcp-sync edit) for an MCP server's. It returns the
// items to re-apply after (an MCP server's registers then), or nil.
func (o *options) fillIn(ctx context.Context, a *answers.Answers, inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	path, _, _ := strings.Cut(inputs[0], ": ")
	home := homeOf(a.User)
	editor := `exec "$(command -v sensible-editor || echo vi)" "$@"`
	var c *exec.Cmd
	var fix []string
	if rest, ok := strings.CutPrefix(path, "~/.mcp/"); ok {
		name := strings.SplitN(rest, "/", 2)[0]
		c = exec.CommandContext(ctx, "runuser", "-u", steps.MCPUser(&steps.Env{User: a.User}), "--",
			"env", "MCP_DIR="+filepath.Join(home, ".mcp"), "EDITOR=sensible-editor", "/usr/local/bin/mcp-sync", "edit", name)
		fix = []string{"mcpjungle"}
	} else {
		c = exec.CommandContext(ctx, "runuser", "-u", a.User, "--", "sh", "-c", editor, "_", strings.Replace(path, "~", home, 1))
	}
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("editing %s: %w", path, err)
	}
	return fix, nil
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
