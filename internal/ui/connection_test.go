package ui

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const secretTok = "f00dfeedf00dfeed"

// testConn is the screen with Tailscale up (the session's way in), a
// WireGuard profile stopped, NetBird not set up, the local network; the
// laptop's rows; a token; a key to add. acts records what it did.
func testConn(acts *[]string) *Connection {
	nets := []Network{
		{ID: "tailscale", Name: "Tailscale", State: "up", Addr: "100.64.1.7", DNS: "vm.tail1234.ts.net", Via: true},
		{ID: "netbird", Name: "NetBird", State: "login"},
		{ID: "wg:office", Name: "office", Kind: "WireGuard", State: "down", Boot: "on", File: "/etc/wireguard/office.conf"},
		{ID: "local", Name: "Local network", Kind: "eth0", State: "up", Addr: "192.168.0.5"},
	}
	c := NewConnection(ConnData{
		Networks: nets,
		Setups: []Setup{
			{ID: "ssh", What: "SSH", How: "ssh vm", Lines: []string{"Host vm", "  HostName 100.64.1.7"}, Say: []string{"Add it to ~/.ssh/config."}},
			{ID: "mcp-claude", What: "Claude Code", How: "claude mcp add vm", Token: "~/.mcp/clients/laptop.token",
				Lines: []string{`claude mcp add --transport http vm http://100.64.1.7:8081/mcp --header "Authorization: Bearer <token>"`}},
		},
		Tokens:   []Token{{Name: "laptop", File: "~/.mcp/clients/laptop.token"}},
		Gateway:  true,
		WayBack:  "ssh ada@192.168.0.5 (local network)",
		CopyHint: "iTerm2 needs it allowed.",
	}, []AddKey{{Pub: "ssh-ed25519 AAAA ada@vm", Where: []string{"github.com: add it at github.com/settings/keys"}}})
	c.Hooks(ConnHooks{
		Act: func(id, act string) error {
			*acts = append(*acts, act+" "+id)
			if id == "wg:office" && act == "start" {
				return errors.New("systemctl start wg-quick@office: Job failed.\nRTNETLINK answers: File exists")
			}
			return nil
		},
		Read:        func() []Network { return nets },
		NewToken:    func(name string) error { *acts = append(*acts, "token "+name); return nil },
		RevokeToken: func(name string) error { *acts = append(*acts, "revoke "+name); return nil },
		Tokens:      func() []Token { return []Token{{Name: "laptop", File: "~/.mcp/clients/laptop.token"}} },
		ReadToken:   func(string) (string, error) { return secretTok + "\n", nil },
		Copy:        func(s string) error { *acts = append(*acts, "copy "+s); return nil },
	})
	c.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return c
}

// ck presses keys on c, running what they start (in the background) to
// its end, as the program would.
func ck(c *Connection, keys ...string) {
	for _, k := range keys {
		msg := keyMsg(k)
		if k == "space" {
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		}
		_, cmd := c.Update(msg)
		drain(c, cmd)
	}
}

// drain runs cmd and gives c what it says (not ticks: they'd wait).
func drain(c *Connection, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch m := cmd().(type) {
	case tea.BatchMsg:
		for _, x := range m {
			drain(c, x)
		}
	case connMsg:
		_, next := c.Update(m)
		if _, tick := m.(connRead); !tick {
			drain(c, next)
		}
	}
}

func onConnRow(c *Connection, kind string, n int) {
	for i, r := range c.rows() {
		if r.kind == kind && r.n == n {
			c.sel = i
		}
	}
}

// The screen lists every network with its state, address and name, and
// the session's own; the laptop's rows open to their lines; tokens by
// name and file; never a token. At 80 columns too.
func TestConnectionView(t *testing.T) {
	var acts []string
	c := testConn(&acts)
	for _, w := range []int{120, 80} {
		c.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		c.sel = 0
		v := ansi.Strip(c.View())
		for _, want := range []string{"Home › Connection", "Networks", "Tailscale", "● connected", "100.64.1.7", "← this session",
			"NetBird", "not set up", "Enter sets it up", "office", "○ stopped", "WireGuard", "starts at boot", "Local network",
			"+ Add a WireGuard or OpenVPN profile", "From your laptop", "Claude Code", "Gateway tokens", "laptop", "Keys to add", "space stop"} {
			if !strings.Contains(v, want) {
				t.Errorf("%d columns: lacks %q:\n%s", w, want, v)
			}
		}
		for _, l := range strings.Split(v, "\n") {
			if ansi.StringWidth(l) > w {
				t.Errorf("%d columns: a line is %d wide: %q", w, ansi.StringWidth(l), l)
			}
		}
	}
	onConnRow(c, "setup", 1)
	v := ansi.Strip(c.View())
	if !strings.Contains(v, "Bearer <token>") || strings.Contains(v, secretTok) {
		t.Errorf("claude row:\n%s", v)
	}
	ck(c, "c")
	if len(acts) != 1 || !strings.Contains(acts[0], "Bearer "+secretTok) || strings.Contains(ansi.Strip(c.View()), secretTok) {
		t.Errorf("copy: %v\n%s", acts, c.View())
	}
	if v := ansi.Strip(c.View()); !strings.Contains(v, "Copied it") || !strings.Contains(v, "Nothing copied? iTerm2 needs it allowed.") {
		t.Errorf("copied:\n%s", v)
	}
}

// Space stops the network the session came by only after a warning that
// names the way back, no by default; another stops at once. A start that
// fails says why on its row.
func TestConnectionStartStop(t *testing.T) {
	var acts []string
	c := testConn(&acts)
	onConnRow(c, "net", 0)
	ck(c, "space")
	if c.dialog == nil || len(acts) != 0 {
		t.Fatalf("no warning: %v", acts)
	}
	v := ansi.Strip(c.View())
	for _, want := range []string{"Stop Tailscale?", "this ends this session", "ssh ada@192.168.0.5 (local network)", "› Keep it running"} {
		if !strings.Contains(v, want) {
			t.Errorf("warning lacks %q:\n%s", want, v)
		}
	}
	ck(c, "enter")
	if len(acts) != 0 {
		t.Errorf("no by default: %v", acts)
	}
	ck(c, "space", "y")
	if len(acts) != 1 || acts[0] != "stop tailscale" || !strings.Contains(c.Toast(), "Tailscale stopped (still logged in") {
		t.Errorf("stop: %v %q", acts, c.Toast())
	}
	acts = nil
	c.d.Networks[0].Via = false
	ck(c, "space")
	if c.dialog != nil || len(acts) != 1 {
		t.Errorf("not the way in: asked (%v)", acts)
	}
	acts = nil
	onConnRow(c, "net", 2)
	ck(c, "space")
	if len(acts) != 1 || acts[0] != "start wg:office" {
		t.Fatalf("start: %v", acts)
	}
	if v := ansi.Strip(c.View()); !strings.Contains(v, "▲ failed") || !strings.Contains(v, "✗ RTNETLINK answers: File exists") || !strings.Contains(v, "journalctl -u wg-quick@office") {
		t.Errorf("failed:\n%s", v)
	}
	ck(c, "b")
	if acts[len(acts)-1] != "noboot wg:office" {
		t.Errorf("b: %v", acts)
	}
	// Unsure where the session came from: every stop warns.
	c.d.Unsure = true
	onConnRow(c, "net", 0)
	ck(c, "space")
	if c.dialog == nil || !strings.Contains(ansi.Strip(c.View()), "can't tell which network") {
		t.Errorf("unsure:\n%s", c.View())
	}
}

// d asks first, saying what goes and what stays: Tailscale's machine in
// the admin console; a profile's file.
func TestConnectionRemove(t *testing.T) {
	var acts []string
	c := testConn(&acts)
	c.d.Networks[0].Via = false
	onConnRow(c, "net", 0)
	ck(c, "d")
	if v := ansi.Strip(c.View()); !strings.Contains(v, "Remove Tailscale?") || !strings.Contains(v, "admin console until you delete it there") {
		t.Errorf("tailscale:\n%s", v)
	}
	ck(c, "n")
	onConnRow(c, "net", 2)
	ck(c, "d")
	if v := ansi.Strip(c.View()); !strings.Contains(v, "deletes its file, /etc/wireguard/office.conf") {
		t.Errorf("profile:\n%s", v)
	}
	ck(c, "y")
	if len(acts) != 1 || acts[0] != "remove wg:office" {
		t.Errorf("remove: %v", acts)
	}
	// Logged in: enter asks nothing. Not set up: nothing to remove;
	// enter leaves to set it up.
	onConnRow(c, "net", 0)
	ck(c, "enter")
	if c.ended || c.dialog != nil {
		t.Errorf("enter on a connected Tailscale: %+v", c.result)
	}
	onConnRow(c, "net", 1)
	ck(c, "d")
	if c.dialog != nil {
		t.Error("d on NetBird not set up asked")
	}
	ck(c, "enter")
	if !c.ended || c.result != (ConnResult{Do: "login", ID: "netbird"}) {
		t.Errorf("enter: %+v", c.result)
	}
	c.ended = false
	ck(c, "n")
	if c.result.Do != "profile" {
		t.Errorf("n: %+v", c.result)
	}
	// The local network isn't yardo's to stop.
	c.ended, acts = false, nil
	onConnRow(c, "net", 3)
	ck(c, "space")
	if len(acts) != 0 || !strings.Contains(c.Toast(), "isn't yardo's to stop") {
		t.Errorf("local: %v %q", acts, c.Toast())
	}
}

// Tailscale by link: the link on its row (c copies it), polled until
// the login is done.
func TestConnectionLink(t *testing.T) {
	var acts []string
	c := testConn(&acts)
	c.d.Networks[0] = Network{ID: "tailscale", Name: "Tailscale", State: "login"}
	reads := 0
	linkPoll = time.Millisecond
	c.h.Link = func() (string, error) { return "https://login.tailscale.com/a/abc123", nil }
	c.h.Read = func() []Network {
		reads++
		n := Network{ID: "tailscale", Name: "Tailscale", State: "login"}
		if reads > 1 {
			n.State, n.Addr = "up", "100.64.1.7"
		}
		return []Network{n}
	}
	c.LinkLogin()
	drain(c, c.Init())
	onConnRow(c, "net", 0)
	if v := ansi.Strip(c.View()); !strings.Contains(v, "Log in at https://login.tailscale.com/a/abc123") || !strings.Contains(v, "waiting for login") {
		t.Fatalf("link:\n%s", v)
	}
	ck(c, "c")
	if acts[len(acts)-1] != "copy https://login.tailscale.com/a/abc123" {
		t.Errorf("c: %v", acts)
	}
	for range 2 {
		_, cmd := c.Update(connTick{})
		drain(c, cmd)
	}
	if c.linking || c.d.Networks[0].State != "up" || !strings.Contains(c.Toast(), "Tailscale is connected") {
		t.Errorf("after the login: linking %v, %+v, %q", c.linking, c.d.Networks[0], c.Toast())
	}
	// Leaving stops the poll.
	c.linking = true
	ck(c, "q")
	if _, cmd := c.Update(connTick{}); c.linking || cmd != nil {
		t.Error("still polling after q")
	}
}

// Tokens: n names a new one, d revokes one (asked first); a key to add is
// dismissed with d.
func TestConnectionTokens(t *testing.T) {
	var acts []string
	c := testConn(&acts)
	onConnRow(c, "newtoken", 0)
	ck(c, "enter", "c", "i", " ", "!")
	if !c.Typing() || !strings.Contains(ansi.Strip(c.View()), "Name: ci▏") {
		t.Fatalf("naming:\n%s", c.View())
	}
	ck(c, "enter")
	if len(acts) != 1 || acts[0] != "token ci" || c.Typing() {
		t.Errorf("made: %v", acts)
	}
	onConnRow(c, "token", 0)
	ck(c, "d")
	if v := ansi.Strip(c.View()); !strings.Contains(v, "Revoke laptop's token?") {
		t.Errorf("revoke:\n%s", v)
	}
	ck(c, "y")
	if acts[len(acts)-1] != "revoke laptop" || !strings.Contains(c.Toast(), "revoked") {
		t.Errorf("revoked: %v %q", acts, c.Toast())
	}
	var dropped string
	c.h.DropKey = func(pub string) error { dropped = pub; return nil }
	onConnRow(c, "key", 0)
	ck(c, "d")
	if dropped != "ssh-ed25519 AAAA ada@vm" || len(c.keys) != 0 {
		t.Errorf("key: %q %v", dropped, c.keys)
	}
}

// OSC 52 puts the text on the terminal's clipboard; under tmux, wrapped.
func TestOSC52(t *testing.T) {
	b := base64.StdEncoding.EncodeToString([]byte("Host vm\n  HostName 1.2.3.4"))
	if got := OSC52("Host vm\n  HostName 1.2.3.4", false); got != "\x1b]52;c;"+b+"\a" {
		t.Errorf("plain: %q", got)
	}
	if got := OSC52("x", true); got != "\x1bPtmux;\x1b\x1b]52;c;eA==\a\x1b\\" {
		t.Errorf("tmux: %q", got)
	}
}

// The Connection tab opens it; q comes back home, onto the tab.
func TestAppConnection(t *testing.T) {
	a := testApp()
	a.Conn.d.Networks = []Network{{ID: "local", Name: "Local network", State: "up", Addr: "10.0.0.5"}}
	for _, row := range []string{"conn"} {
		onRow(a.Menu, row)
		ak(a, "enter")
		if !a.onConn || !strings.Contains(a.View(), "Home › Connection") || !strings.Contains(a.View(), "Local network") {
			t.Fatalf("%s: not on Connection:\n%s", row, a.View())
		}
		ak(a, "q")
		if a.onConn || a.Menu.screen != "home" || a.Menu.rows()[a.Menu.sel].id != row {
			t.Errorf("%s: q: on %s, row %s", row, a.Menu.screen, a.Menu.rows()[a.Menu.sel].id)
		}
	}
	// Leaving to type: the app ends with it.
	onRow(a.Menu, "conn")
	ak(a, "enter")
	if cmd := ak(a, "n"); !isQuit(cmd) || a.result.Conn == nil || a.result.Conn.Do != "profile" {
		t.Errorf("n: %+v", a.result)
	}
}
