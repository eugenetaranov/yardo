package ui

import (
	"encoding/base64"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// Connection is the screen of how this machine is connected: its
// networks, started, stopped, set up and removed from here; what to set
// up on a laptop to reach it; the MCP gateway's tokens; and the keys runs
// made, to add elsewhere. What needs no typing runs in the background,
// the screen kept; a login's key and a new profile leave it (ConnResult)
// and come back. It's one of the app's views (see App).

// Network is one of the machine's networks, as the screen lists it.
type Network struct {
	// ID is what an action names: tailscale, netbird, wg:<name>,
	// ovpn:<name>; local for the local network, if:<name> for a VPN
	// interface yardo didn't set up (shown, not acted on).
	ID   string
	Name string
	// Kind is what it is, said beside a profile (WireGuard, OpenVPN).
	Kind string
	// State: up, down, login (installed, not set up), failed.
	State     string
	Addr, DNS string
	// Boot: a profile starts at boot ("on", "off"; "" for no profile).
	Boot string
	// File is a profile's.
	File string
	// Via: the user's session came in through it.
	Via bool
	// Link is Tailscale's login link, while a login waits for it.
	Link string
	// Note is more about it: a key that expires, why it's down.
	Note string
}

func (n Network) profile() bool {
	return strings.HasPrefix(n.ID, "wg:") || strings.HasPrefix(n.ID, "ovpn:")
}

func (n Network) mesh() bool { return n.ID == "tailscale" || n.ID == "netbird" }

// unit is a profile's systemd unit ("" for no profile).
func (n Network) unit() string {
	if name, ok := strings.CutPrefix(n.ID, "wg:"); ok {
		return "wg-quick@" + name
	}
	if name, ok := strings.CutPrefix(n.ID, "ovpn:"); ok {
		return "openvpn-client@" + name
	}
	return ""
}

// Setup is a thing to set up on the laptop: the lines to run or paste
// (c copies them), what's said about them (where they go, how you sign
// in), and the file of the token that <token> in them stands for (read
// when copied, never drawn).
type Setup struct {
	ID, What, How string
	Lines         []string
	Say           []string
	Token         string
}

// Token is an MCP gateway client's token: its name and its file (never
// its value).
type Token struct{ Name, File string }

// ConnData is what the screen shows.
type ConnData struct {
	Networks []Network
	Setups   []Setup
	Tokens   []Token
	// Gateway: the gateway listens beyond the machine, so it takes tokens.
	Gateway bool
	// WayBack is how to get in again when the network this session came
	// by stops ("" for the console only); Unsure: an ssh session yardo
	// can't place, so every stop warns.
	WayBack string
	Unsure  bool
	// CopyHint is how to let the terminal copy, said after a copy.
	CopyHint string
}

// ConnHooks are what the screen does to the machine. Act (start, stop,
// boot, noboot, remove), Link, NewToken and RevokeToken run in the
// background; Read and Tokens read the machine again after each.
type ConnHooks struct {
	Act         func(id, act string) error
	Read        func() []Network
	Link        func() (string, error)
	NewToken    func(name string) error
	RevokeToken func(name string) error
	Tokens      func() []Token
	ReadToken   func(file string) (string, error)
	Copy        func(s string) error
	DropKey     func(pub string) error
}

// ConnResult is what leaving Connection to type asks for: Do "login"
// (ID's key, then its login) or "profile" (a profile to add).
type ConnResult struct{ Do, ID string }

// connRow is one of the screen's rows: kind net, add (a profile), setup,
// token, newtoken or key; n its index in its list.
type connRow struct {
	kind string
	n    int
}

// connMsg is a message for the Connection screen, whichever view is up.
type connMsg interface{ forConn() }

// connDone is a background action finished: on key (a network's id, or
// token:<name>), saying say, or err; the networks and tokens read after.
type connDone struct {
	key, say string
	err      error
	nets     []Network
	tokens   []Token
}

// connTick polls while Tailscale waits for its login; connRead is a poll's
// read; connLink is the login link (or why there's none).
type (
	connTick struct{}
	connRead struct{ nets []Network }
	connLink struct {
		link string
		err  error
	}
)

func (connDone) forConn() {}
func (connTick) forConn() {}
func (connRead) forConn() {}
func (connLink) forConn() {}

// linkPoll is how often the screen looks whether a login by link is done.
var linkPoll = 2 * time.Second

// Connection is the screen; see NewConnection.
type Connection struct {
	d    ConnData
	keys []AddKey
	h    ConnHooks
	sel  int
	// busy is what's running, by key ("stopping…"); failed why it failed.
	busy, failed map[string]string
	dialog       *Dialog
	toast        string
	hits         []Hit
	width        int
	height       int
	// linking: Tailscale waits for its login by link (link, as `up` said
	// it); startLink: start that when the program starts.
	linking, startLink bool
	link               string
	// copied: something was copied, so the hint shows.
	copied bool
	// naming: a token's name is being typed.
	naming bool
	name   []rune
	result ConnResult
	// ended: left to type (result); left: q, home.
	ended, left bool
}

// NewConnection builds the screen on d, with the keys to add.
func NewConnection(d ConnData, keys []AddKey) *Connection {
	return &Connection{d: d, keys: keys, busy: map[string]string{}, failed: map[string]string{}, width: 100, height: 30}
}

// Hooks sets what the screen does to the machine.
func (c *Connection) Hooks(h ConnHooks) { c.h = h }

// LinkLogin has the screen start Tailscale's login by link, and wait
// for it, when it next starts.
func (c *Connection) LinkLogin() { c.startLink = true }

// Result is what leaving to type asked for.
func (c *Connection) Result() ConnResult { return c.result }

func (c *Connection) rows() []connRow {
	var out []connRow
	for n := range c.d.Networks {
		out = append(out, connRow{"net", n})
	}
	out = append(out, connRow{"add", 0})
	for n := range c.d.Setups {
		out = append(out, connRow{"setup", n})
	}
	if c.d.Gateway {
		for n := range c.d.Tokens {
			out = append(out, connRow{"token", n})
		}
		out = append(out, connRow{"newtoken", 0})
	}
	for n := range c.keys {
		out = append(out, connRow{"key", n})
	}
	return out
}

func (c *Connection) row() connRow {
	rs := c.rows()
	c.sel = clamp(c.sel, 0, len(rs)-1)
	return rs[c.sel]
}

// net is the network with id, if it's listed.
func (c *Connection) net(id string) *Network {
	for n := range c.d.Networks {
		if c.d.Networks[n].ID == id {
			return &c.d.Networks[n]
		}
	}
	return nil
}

func (c *Connection) Init() tea.Cmd {
	if c.startLink {
		c.startLink = false
		return c.beginLink()
	}
	return nil
}

func (c *Connection) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		c.width, c.height = v.Width, v.Height
	case MoveMsg:
		c.sel = clamp(c.sel+v.N, 0, len(c.rows())-1)
	case tea.KeyMsg:
		return c, c.key(v)
	case connDone:
		delete(c.busy, v.key)
		if v.nets != nil {
			c.d.Networks = v.nets
		}
		if v.tokens != nil || c.h.Tokens != nil {
			c.d.Tokens = v.tokens
		}
		if v.err != nil {
			c.failed[v.key] = lastOf(v.err.Error())
			c.toast = "✗ " + lastOf(v.err.Error())
		} else {
			delete(c.failed, v.key)
			c.toast = v.say
		}
		c.sel = clamp(c.sel, 0, len(c.rows())-1)
	case connLink:
		if v.link != "" {
			c.link = v.link
		}
		if v.err != nil && c.link == "" && c.linking {
			if n := c.net("tailscale"); n == nil || n.State != "up" {
				c.linking = false
				c.failed["tailscale"] = lastOf(v.err.Error())
				c.toast = "✗ No login link from Tailscale: in another terminal, sudo tailscale up."
			}
		}
	case connTick:
		if !c.linking || c.h.Read == nil {
			return c, nil
		}
		read := c.h.Read
		return c, func() tea.Msg { return connRead{read()} }
	case connRead:
		if !c.linking {
			return c, nil
		}
		c.d.Networks = v.nets
		if n := c.net("tailscale"); n != nil && n.State == "up" {
			c.linking, c.link = false, ""
			c.toast = "✓ Tailscale is connected."
			return c, nil
		}
		return c, tea.Tick(linkPoll, func(time.Time) tea.Msg { return connTick{} })
	}
	return c, nil
}

// lastOf is s's last line.
func lastOf(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// beginLink starts Tailscale's login by link, and the poll that sees it
// done.
func (c *Connection) beginLink() tea.Cmd {
	if c.h.Link == nil {
		return nil
	}
	c.linking, c.link = true, ""
	delete(c.failed, "tailscale")
	c.toast = "Asking Tailscale for a login link…"
	link := c.h.Link
	return tea.Batch(func() tea.Msg {
		l, err := link()
		return connLink{l, err}
	}, tea.Tick(linkPoll, func(time.Time) tea.Msg { return connTick{} }))
}

// run does do in the background, key busy as busy, then reads the
// machine again; say is what it says when it worked.
func (c *Connection) run(key, busy, say string, do func() error) tea.Cmd {
	c.busy[key] = busy
	delete(c.failed, key)
	c.toast = ""
	read, tokens := c.h.Read, c.h.Tokens
	return func() tea.Msg {
		m := connDone{key: key, say: say, err: do()}
		if read != nil {
			m.nets = read()
		}
		if tokens != nil {
			m.tokens = tokens()
		}
		return m
	}
}

// act does act to network n, in the background.
func (c *Connection) act(n Network, act string) tea.Cmd {
	busy := map[string]string{"start": "starting…", "stop": "stopping…", "boot": "turning boot on…", "noboot": "turning boot off…", "remove": "removing…"}[act]
	say := map[string]string{
		"start":  "✓ " + n.Name + " started.",
		"stop":   "✓ " + n.Name + " stopped" + map[bool]string{true: " (still logged in: space starts it again).", false: "."}[n.mesh()],
		"boot":   "✓ " + n.Name + " starts at boot.",
		"noboot": "✓ " + n.Name + " no longer starts at boot.",
		"remove": "✓ " + n.Name + " removed.",
	}[act]
	do := c.h.Act
	return c.run(n.ID, busy, say, func() error {
		if do == nil {
			return nil
		}
		return do(n.ID, act)
	})
}

// cuts reports whether stopping or removing n could end this session.
func (c *Connection) cuts(n Network) bool { return n.Via || c.d.Unsure }

// warning is what stopping or removing n says first when it could end
// this session.
func (c *Connection) warning(n Network) []string {
	back := "the machine's console"
	if c.d.WayBack != "" {
		back = c.d.WayBack + ", or the machine's console"
	}
	first := "You came in through " + n.Name
	if n.Addr != "" {
		first += " (" + n.Addr + ")"
	}
	first += ": this ends this session, and yardo with it."
	if !n.Via {
		first = "yardo can't tell which network this ssh session came in on: if it's " + n.Name + ", this ends it."
	}
	return []string{first, "To get back in: " + back + "."}
}

// removeSay is what removing n takes, and what stays.
func removeSay(n Network) []string {
	switch n.ID {
	case "tailscale":
		return []string{"Logs this machine out of your tailnet (tailscale logout).",
			"Stays: Tailscale itself, and this machine in the tailnet's admin console until you delete it there (Machines › … › Remove)."}
	case "netbird":
		return []string{"Disconnects this machine and logs it out of NetBird (netbird down, netbird logout).",
			"Stays: NetBird itself, and the peer in your NetBird dashboard until you delete it there."}
	}
	return []string{"Stops " + n.Name + ", stops it starting at boot, and deletes its file, " + n.File + ".",
		"Stays: " + n.Kind + " itself, and your other profiles."}
}

func (c *Connection) key(v tea.KeyMsg) tea.Cmd {
	k := v.String()
	if c.dialog != nil {
		return dialogKey(&c.dialog, k)
	}
	if c.naming {
		return c.nameKey(v)
	}
	c.toast = ""
	r := c.row()
	switch k {
	case "q", "esc":
		c.linking, c.left = false, true
		return nil
	case "up", "k":
		c.sel = clamp(c.sel-1, 0, len(c.rows())-1)
		return nil
	case "down", "j":
		c.sel = clamp(c.sel+1, 0, len(c.rows())-1)
		return nil
	case "tab", "shift+tab":
		c.nextSection(k == "tab")
		return nil
	}
	switch r.kind {
	case "net", "add":
		return c.netKey(k, r)
	case "setup":
		s := c.d.Setups[r.n]
		switch {
		case k == "c" || k == "enter":
			c.copyLines(s.Lines, s.Token)
		case k == "n" && strings.HasPrefix(s.ID, "mcp") && c.d.Gateway:
			c.naming, c.name = true, nil
		}
	case "token", "newtoken":
		switch {
		case k == "n" || k == "enter" && r.kind == "newtoken":
			c.naming, c.name = true, nil
		case k == "d" && r.kind == "token":
			t := c.d.Tokens[r.n]
			c.dialog = &Dialog{Title: "Revoke " + t.Name + "'s token?", YN: true, Q: "no",
				Lines: []string{"The gateway refuses it from now on, and " + t.File + " is deleted.", "A client using it loses the MCP servers until it gets a new one."},
				Opts:  [][2]string{{"Keep it (n)", "no"}, {"Revoke it (y)", "yes"}},
				On: func(a string) tea.Cmd {
					if a != "yes" {
						return nil
					}
					rm := c.h.RevokeToken
					return c.run("token:"+t.Name, "revoking…", "✓ "+t.Name+"'s token is revoked: the gateway refuses it.", func() error {
						if rm == nil {
							return nil
						}
						return rm(t.Name)
					})
				}}
		}
	case "key":
		key := c.keys[r.n]
		switch k {
		case "c", "enter":
			c.copyLines([]string{key.Pub}, "")
		case "d":
			if c.h.DropKey != nil {
				if err := c.h.DropKey(key.Pub); err != nil {
					c.toast = "✗ " + err.Error()
					return nil
				}
			}
			c.keys = append(append([]AddKey{}, c.keys[:r.n]...), c.keys[r.n+1:]...)
			c.toast = "Dismissed the " + key.For() + " key."
		}
	}
	return nil
}

// netKey is k on a network's row (or Add a profile's).
func (c *Connection) netKey(k string, r connRow) tea.Cmd {
	if k == "n" || r.kind == "add" && k == "enter" {
		c.result, c.ended = ConnResult{Do: "profile"}, true
		return nil
	}
	if r.kind == "add" {
		return nil
	}
	n := c.d.Networks[r.n]
	acts := n.mesh() || n.profile()
	if b := c.busy[n.ID]; b != "" && (k == " " || k == "b" || k == "d" || k == "enter") {
		c.toast = n.Name + ": " + b
		return nil
	}
	switch {
	case k == "c" && c.linkOf(n) != "":
		c.copyLines([]string{c.linkOf(n)}, "")
	case !acts && (k == " " || k == "b" || k == "d"):
		if n.ID == "local" {
			c.toast = "The local network isn't yardo's to stop."
		} else {
			c.toast = n.Name + " wasn't set up from here: yardo leaves it be."
		}
	case k == "enter" && n.State == "login":
		c.result, c.ended = ConnResult{Do: "login", ID: n.ID}, true
	case k == " " && n.State == "login":
		c.toast = n.Name + " isn't set up: enter sets it up."
	case k == " ":
		act := "start"
		if n.State == "up" {
			act = "stop"
		}
		if act == "stop" && c.cuts(n) {
			c.dialog = &Dialog{Title: "Stop " + n.Name + "?", YN: true, Q: "no", Lines: c.warning(n),
				Opts: [][2]string{{"Keep it running (n)", "no"}, {"Stop " + n.Name + " (y)", "yes"}},
				On: func(a string) tea.Cmd {
					if a == "yes" {
						return c.act(n, "stop")
					}
					return nil
				}}
			return nil
		}
		return c.act(n, act)
	case k == "b" && n.profile():
		if n.Boot == "on" {
			return c.act(n, "noboot")
		}
		return c.act(n, "boot")
	case k == "b":
		c.toast = n.Name + " starts at boot as it was left: running or stopped."
	case k == "d" && n.State == "login" && n.mesh():
		c.toast = n.Name + " isn't set up: nothing to remove (Tools uninstalls it)."
	case k == "d":
		lines := removeSay(n)
		if c.cuts(n) {
			lines = append(append(c.warning(n), ""), lines...)
		}
		c.dialog = &Dialog{Title: "Remove " + n.Name + "?", YN: true, Q: "no", Lines: lines,
			Opts: [][2]string{{"Keep it (n)", "no"}, {"Remove " + n.Name + " (y)", "yes"}},
			On: func(a string) tea.Cmd {
				if a == "yes" {
					return c.act(n, "remove")
				}
				return nil
			}}
	}
	return nil
}

// linkOf is n's login link, while it waits for one.
func (c *Connection) linkOf(n Network) string {
	if n.ID != "tailscale" || n.State == "up" {
		return ""
	}
	if n.Link != "" {
		return n.Link
	}
	if c.linking {
		return c.link
	}
	return ""
}

// nameKey is a key while a token's name is typed.
func (c *Connection) nameKey(v tea.KeyMsg) tea.Cmd {
	switch v.Type {
	case tea.KeyEsc:
		c.naming = false
	case tea.KeyBackspace:
		if len(c.name) > 0 {
			c.name = c.name[:len(c.name)-1]
		}
	case tea.KeyEnter:
		name := string(c.name)
		if name == "" {
			c.naming = false
			return nil
		}
		for _, t := range c.d.Tokens {
			if t.Name == name {
				c.toast = "There's a " + name + " already: d revokes it first."
				return nil
			}
		}
		c.naming = false
		mk := c.h.NewToken
		return c.run("token:"+name, "making…", "✓ "+name+"'s token is made; c on an MCP row copies it with the line.", func() error {
			if mk == nil {
				return nil
			}
			return mk(name)
		})
	case tea.KeyRunes:
		for _, r := range v.Runes {
			if len(c.name) < 32 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' && len(c.name) > 0) {
				c.name = append(c.name, r)
			}
		}
	}
	return nil
}

// copyLines copies lines to the laptop's clipboard, through the terminal;
// <token> in them is the token in file, read now.
func (c *Connection) copyLines(lines []string, file string) {
	s := strings.Join(lines, "\n")
	if file != "" && strings.Contains(s, "<token>") && c.h.ReadToken != nil {
		tok, err := c.h.ReadToken(file)
		if err != nil {
			c.toast = "✗ The token isn't there to read (" + lastOf(err.Error()) + "): n on an MCP row makes one."
			return
		}
		s = strings.ReplaceAll(s, "<token>", strings.TrimSpace(tok))
	}
	if c.h.Copy != nil {
		if err := c.h.Copy(s); err != nil {
			c.toast = "✗ " + err.Error()
			return
		}
	}
	c.copied = true
	what := "it"
	if len(lines) > 1 {
		what = plural(len(lines), "line", "lines")
	}
	c.toast = "Copied " + what + " to your laptop's clipboard, if your terminal lets it (see below)."
}

// nextSection moves to the next section's first row (or the last's).
func (c *Connection) nextSection(fwd bool) {
	rs := c.rows()
	sec := func(k string) string {
		switch k {
		case "net", "add":
			return "net"
		case "token", "newtoken":
			return "token"
		}
		return k
	}
	cur := sec(rs[c.sel].kind)
	if fwd {
		for n := c.sel + 1; n < len(rs); n++ {
			if sec(rs[n].kind) != cur {
				c.sel = n
				return
			}
		}
		c.sel = 0
		return
	}
	// Back: the start of this section, else the previous one's.
	start := c.sel
	for start > 0 && sec(rs[start-1].kind) == cur {
		start--
	}
	if start != c.sel {
		c.sel = start
		return
	}
	if start == 0 {
		start = len(rs)
	}
	prev := sec(rs[start-1].kind)
	for start > 0 && sec(rs[start-1].kind) == prev {
		start--
	}
	c.sel = start
}

// View is the screen drawn alone, as the app draws it.
func (c *Connection) View() string { return standalone(c, HomeInfo{}, c.width, c.height) }

func (c *Connection) Path() string  { return "Home › Connection" }
func (c *Connection) Toast() string { return c.toast }
func (c *Connection) Hits() []Hit   { return c.hits }
func (c *Connection) Typing() bool  { return c.naming && c.dialog == nil }

func (c *Connection) Back() string {
	switch {
	case c.dialog != nil:
		return "cancel"
	case c.naming:
		return ""
	}
	return "back"
}

// Keys are the footer's: the row's own.
func (c *Connection) Keys() []Key {
	if c.dialog != nil {
		return dialogKeys(c.dialog)
	}
	if c.naming {
		return []Key{{Key: "enter", What: "make it"}, {Key: "esc", What: "cancel"}}
	}
	r := c.row()
	switch r.kind {
	case "net":
		n := c.d.Networks[r.n]
		var ks []Key
		switch {
		case n.State == "login":
			ks = append(ks, Key{Key: "enter", What: "set it up"})
		case n.State == "up" && (n.mesh() || n.profile()):
			ks = append(ks, Key{Key: "space", What: "stop"})
		case n.mesh() || n.profile():
			ks = append(ks, Key{Key: "space", What: "start"})
		}
		if n.profile() {
			ks = append(ks, Key{Key: "b", What: map[bool]string{true: "not at boot", false: "at boot"}[n.Boot == "on"]})
		}
		if c.linkOf(n) != "" {
			ks = append(ks, Key{Key: "c", What: "copy the link"})
		}
		if (n.mesh() && n.State != "login") || n.profile() {
			ks = append(ks, Key{Key: "d", What: "remove"})
		}
		return append(ks, Key{Key: "n", What: "add a profile"}, Key{Key: "tab", What: "next section"})
	case "add":
		return []Key{{Key: "enter", What: "add a profile"}, {Key: "tab", What: "next section"}}
	case "setup":
		ks := []Key{{Key: "c", What: "copy"}}
		if strings.HasPrefix(c.d.Setups[r.n].ID, "mcp") && c.d.Gateway {
			ks = append(ks, Key{Key: "n", What: "new token"})
		}
		return append(ks, Key{Key: "tab", What: "next section"})
	case "token":
		return []Key{{Key: "d", What: "revoke"}, {Key: "n", What: "new token"}, {Key: "tab", What: "next section"}}
	case "newtoken":
		return []Key{{Key: "enter", What: "new token"}, {Key: "tab", What: "next section"}}
	}
	return []Key{{Key: "c", What: "copy"}, {Key: "d", What: "dismiss"}, {Key: "tab", What: "next section"}}
}

// Help is every key the screen takes.
func (c *Connection) Help() []Key {
	return []Key{
		{Key: "space", What: "stop a network, or start it (never logs out or deletes)"},
		{Key: "b", What: "a profile: start it at boot, or not"},
		{Key: "enter", What: "set up a VPN that isn't; on the rest, as c"},
		{Key: "n", What: "add a WireGuard or OpenVPN profile; on the gateway's rows, a client's token"},
		{Key: "d", What: "remove a network (asked first), revoke a token, dismiss a key"},
		{Key: "c", What: "copy the row's lines (or the login link) to your laptop's clipboard"},
		{Key: "tab", What: "the next section: Networks, From your laptop, Gateway tokens, Keys to add"},
	}
}

// netState is how n reads, in a few words, in its colour.
func (c *Connection) netState(n Network) string {
	if b := c.busy[n.ID]; b != "" {
		return accent.Render(b)
	}
	if _, ok := c.failed[n.ID]; ok {
		return yellow.Render("▲ failed")
	}
	switch n.State {
	case "up":
		return green.Render("● connected")
	case "down":
		return muted.Render("○ stopped")
	case "login":
		if c.linkOf(n) != "" || n.ID == "tailscale" && c.linking {
			return accent.Render("○ waiting for login")
		}
		return accent.Render(tui.Cur.Mark.Need + " not set up")
	case "failed":
		return yellow.Render("▲ failed")
	}
	return muted.Render("○ " + n.State)
}

// pasteLines are lines to paste, cut to w cells a piece (never wrapped at
// spaces: c copies them whole).
func pasteLines(l string, w int) []string {
	var out []string
	for runewidth.StringWidth(l) > w && w > 4 {
		cut := runewidth.Truncate(l, w, "")
		out = append(out, cut)
		l = l[len(cut):]
	}
	return append(out, l)
}

// Body is the screen under the header, w by h.
func (c *Connection) Body(w, h int) string {
	c.hits = nil
	rs := c.rows()
	c.sel = clamp(c.sel, 0, len(rs)-1)
	var lines []string
	var rowOf []int
	selLine := 0
	add := func(row int, ls ...string) {
		for _, l := range ls {
			lines = append(lines, ansi.Truncate(l, w, "…"))
			rowOf = append(rowOf, row)
		}
	}
	head := func(t, note string) {
		add(-1, "", "  "+bold.Render(t)+muted.Render(note))
	}
	mark := func(n int, s string, col int) string {
		if n == c.sel {
			selLine = len(lines)
			return "  " + rowOn.Render(padTo(" › "+s, col+3))
		}
		return "     " + padTo(s, col)
	}
	col := 16
	for _, n := range c.d.Networks {
		col = max(col, min(24, runewidth.StringWidth(n.Name)))
	}
	add(-1, "")
	at := 0
	head("Networks", "  ·  space starts or stops one  ·  d removes it  ·  n adds a profile")
	for _, n := range c.d.Networks {
		var bits []string
		if n.Kind != "" {
			bits = append(bits, n.Kind)
		}
		if n.Addr != "" {
			bits = append(bits, n.Addr)
		}
		if n.DNS != "" {
			bits = append(bits, n.DNS)
		}
		switch n.Boot {
		case "on":
			bits = append(bits, "starts at boot")
		case "off":
			bits = append(bits, "not at boot")
		}
		l := mark(at, runewidth.Truncate(n.Name, col, "…"), col) + "  " + padTo(c.netState(n), 20) + "  "
		if n.Via {
			l += accent.Render("← this session") + "  "
		}
		add(at, l+muted.Render(strings.Join(bits, " · ")))
		switch {
		case c.failed[n.ID] != "":
			why := "✗ " + c.failed[n.ID]
			if u := n.unit(); u != "" {
				why += "  ·  the rest: sudo journalctl -u " + u
			}
			add(at, "        "+yellow.Render(why))
		case c.linkOf(n) != "":
			add(at, "        Log in at "+keyOn.Render(c.linkOf(n))+muted.Render("  ·  c copies it; this turns connected once you have"))
		case n.ID == "tailscale" && c.linking:
			add(at, "        "+muted.Render("Asking Tailscale for a login link…"))
		case n.State == "login":
			add(at, "        "+muted.Render("Enter sets it up: its key, or for Tailscale a login link."))
		case n.Note != "":
			add(at, "        "+muted.Render(n.Note))
		}
		at++
	}
	if len(c.d.Networks) == 0 {
		add(-1, "     "+muted.Render("None found."))
	}
	add(at, mark(at, "+ Add a WireGuard or OpenVPN profile", col+24))
	at++

	head("From your laptop", "  ·  c copies the row's lines; they use this machine's VPN address first")
	wcol := 12
	for _, s := range c.d.Setups {
		wcol = max(wcol, runewidth.StringWidth(s.What))
	}
	for n, s := range c.d.Setups {
		add(at, mark(at, s.What, wcol)+"  "+muted.Render(s.How))
		if at == c.sel {
			for _, l := range s.Lines {
				for _, p := range pasteLines(l, w-10) {
					add(at, "        "+keyOn.Render(p))
				}
			}
			for _, l := range s.Say {
				for _, wl := range wrap(l, w-10) {
					add(at, "        "+muted.Render(wl))
				}
			}
		}
		_ = n
		at++
	}

	if c.d.Gateway {
		head("Gateway tokens", "  ·  a client's own, by name: the token stays in its file")
		for _, t := range c.d.Tokens {
			state := muted.Render("sudo cat " + t.File)
			if b := c.busy["token:"+t.Name]; b != "" {
				state = accent.Render(b)
			} else if f := c.failed["token:"+t.Name]; f != "" {
				state = yellow.Render("✗ " + f)
			}
			add(at, mark(at, t.Name, wcol)+"  "+state)
			at++
		}
		// One being made, not listed yet.
		for k, b := range c.busy {
			if name, ok := strings.CutPrefix(k, "token:"); ok && b == "making…" {
				add(-1, "     "+padTo(name, wcol)+"  "+accent.Render(b))
			}
		}
		newRow := "+ Make a token for a client"
		if c.naming {
			newRow = "Name: " + string(c.name) + "▏"
		}
		add(at, mark(at, newRow, wcol+24))
		if c.naming {
			add(at, "        "+muted.Render("Letters, digits, - _ . : the client's name (ci, laptop). Enter makes it; esc doesn't."))
		}
		at++
	}

	if len(c.keys) > 0 {
		head("Keys to add", "  ·  where each says; d dismisses one once it's added")
		for _, k := range c.keys {
			for i, wh := range k.Where {
				if i == 0 {
					add(at, mark(at, wh, max(wcol, w-12)))
				} else {
					add(at, "     "+wh)
				}
			}
			if at == c.sel {
				for _, p := range pasteLines(k.Pub, w-10) {
					add(at, "        "+keyOn.Render(p))
				}
				if k.Path != "" {
					add(at, "        "+muted.Render("Show it again: cat "+k.Path))
				}
			}
			at++
		}
	}
	if c.copied && c.d.CopyHint != "" {
		add(-1, "")
		for _, l := range wrap("Nothing copied? "+c.d.CopyHint, w-6) {
			add(-1, "  "+muted.Render(l))
		}
	}

	room := h
	var dl []string
	if c.dialog != nil {
		dl = dialogLines(c.dialog, w, 0, nil)
		room = max(1, h-len(dl))
	}
	start := windowStart(len(lines), selLine, room)
	shown := window(lines, selLine, room)
	for i := range shown {
		if r := rowOf[start+i]; r >= 0 {
			r := r
			c.hits = append(c.hits, Hit{Y0: i, Y1: i, Do: func() { c.sel = r }})
		}
	}
	if c.dialog != nil {
		var dh []Hit
		dl = dialogLines(c.dialog, w, len(shown), &dh)
		c.hits = dh
		shown = append(shown, dl...)
	}
	return strings.Join(shown, "\n")
}

// OSC52 is the escape sequence that puts s on the terminal's clipboard
// (the laptop's, over ssh); under tmux, wrapped for it to pass on.
func OSC52(s string, tmux bool) string {
	seq := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(s)) + "\a"
	if tmux {
		return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	}
	return seq
}
