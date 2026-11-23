package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/creds"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// The Connection screen's data: the machine's networks, read from the
// VPNs themselves and the profiles' files and units; and what to set up
// on a laptop, which `yardo connect` prints too.

// unitState is a unit's state as a network's (up, down or failed), and
// whether it starts at boot (on, off).
func unitState(ctx context.Context, x steps.Execer, unit string) (state, boot string) {
	out, _ := x.Run(ctx, steps.Cmd{Name: "systemctl", Args: []string{"is-active", unit}})
	switch strings.TrimSpace(string(out)) {
	case "active", "activating", "reloading":
		state = "up"
	case "failed":
		state = "failed"
	default:
		state = "down"
	}
	out, _ = x.Run(ctx, steps.Cmd{Name: "systemctl", Args: []string{"is-enabled", unit}})
	boot = "off"
	if strings.TrimSpace(string(out)) == "enabled" {
		boot = "on"
	}
	return state, boot
}

// networks are the machine's networks: Tailscale and NetBird when
// installed (has) or answering, the WireGuard and OpenVPN profiles under
// root, a VPN interface no profile accounts for, and the local network
// last. via is the address this session came to ("" for none known).
func networks(ctx context.Context, x steps.Execer, root string, has func(string) bool, addrs []address, via string) []ui.Network {
	var out []ui.Network
	if st, ok := steps.TailscaleStatus(ctx, x); ok || has("tailscale-install") {
		n := ui.Network{ID: steps.NetTailscale, Name: "Tailscale"}
		switch st.BackendState {
		case "Running":
			n.State = "up"
		case "Stopped":
			n.State = "down"
		case "NeedsMachineAuth":
			n.State, n.Note = "down", "logged in, waiting to be approved in the tailnet's admin console (Machines)"
		case "NeedsLogin", "NoState":
			n.State, n.Link = "login", st.AuthURL
		default:
			n.State, n.Note = "down", "tailscaled isn't answering: sudo systemctl start tailscaled"
		}
		for _, ip := range st.Self.TailscaleIPs {
			if p := net.ParseIP(ip); p != nil && p.To4() != nil && n.State != "login" {
				n.Addr = ip
				break
			}
		}
		if n.State != "login" {
			n.DNS = strings.TrimSuffix(st.Self.DNSName, ".")
		}
		if n.State == "up" && len(st.Self.Tags) == 0 && st.Self.KeyExpiry != nil {
			n.Note = "its key expires on " + st.Self.KeyExpiry.Format("2006-01-02") + ": the admin console's Disable key expiry keeps it"
		}
		out = append(out, n)
	}
	if text, _ := x.Run(ctx, steps.Cmd{Name: "netbird", Args: []string{"status"}}); len(text) > 0 || has("netbird-install") {
		n := ui.Network{ID: steps.NetNetbird, Name: "NetBird", State: "down"}
		switch s := string(text); {
		case strings.Contains(s, "NeedsLogin"):
			n.State = "login"
		case strings.Contains(s, "Management: Connected"):
			n.State = "up"
			if st, ok := steps.NetbirdStatus(ctx, x); ok {
				n.Addr, _, _ = strings.Cut(st.IP, "/")
				n.DNS = strings.TrimSuffix(st.FQDN, ".")
			}
		case len(text) == 0 || strings.Contains(s, "daemon"):
			n.Note = "its service isn't answering: sudo netbird service start"
		}
		out = append(out, n)
	}
	seen := map[string]bool{}
	for _, kind := range []string{steps.NetWireGuard, steps.NetOpenVPN} {
		files, _ := filepath.Glob(filepath.Join(root, steps.ProfileDir(kind), "*.conf"))
		for _, f := range files {
			name := strings.TrimSuffix(filepath.Base(f), ".conf")
			n := ui.Network{ID: steps.NetID(kind, name), Name: name, Kind: map[string]string{steps.NetWireGuard: "WireGuard", steps.NetOpenVPN: "OpenVPN"}[kind],
				File: filepath.Join(steps.ProfileDir(kind), name+".conf")}
			n.State, n.Boot = unitState(ctx, x, steps.ProfileUnit(kind, name))
			if kind == steps.NetWireGuard {
				seen[name] = true
				for _, a := range addrs {
					if a.iface == name {
						n.Addr = a.ip
					}
				}
			}
			if n.State == "failed" {
				n.Note = "it failed to start: sudo journalctl -u " + steps.ProfileUnit(kind, name)
			}
			out = append(out, n)
		}
	}
	// A VPN up some other way (wg-quick up by hand, an OpenVPN of yours):
	// shown, left be. OpenVPN's tun devices aren't matched to a profile.
	tuns := slices.ContainsFunc(out, func(n ui.Network) bool { return strings.HasPrefix(n.ID, "ovpn:") && n.State == "up" })
	for _, a := range addrs {
		k := a.kind()
		if k == vpnMesh || k == "" || seen[a.iface] || strings.HasPrefix(a.iface, "tun") && tuns {
			continue
		}
		out = append(out, ui.Network{ID: "if:" + a.iface, Name: a.iface, Kind: a.vpn, State: "up", Addr: a.ip, Note: "started outside yardo: left be"})
	}
	if i := slices.IndexFunc(addrs, func(a address) bool { return a.kind() == "" }); i >= 0 {
		out = append(out, ui.Network{ID: "local", Name: "Local network", Kind: addrs[i].iface, State: "up", Addr: addrs[i].ip})
	}
	if via != "" {
		for n := range out {
			if out[n].Addr == via {
				out[n].Via = true
			}
		}
		// Another local address (a second interface) is the local network.
		if !slices.ContainsFunc(out, func(n ui.Network) bool { return n.Via }) {
			for _, a := range addrs {
				if a.ip == via && a.kind() == "" {
					for n := range out {
						if out[n].ID == "local" {
							out[n].Via = true
						}
					}
				}
			}
		}
	}
	return out
}

// sessionVia is the machine's address this ssh session came to: the
// third of SSH_CONNECTION's fields, from yardo's environment or, under a
// sudo that dropped it, its parents'. unsure: no ssh session found, but
// the terminal is a pseudo-terminal (likely ssh, maybe not).
func sessionVia(environ func(pid int) []string, tty string) (addr string, unsure bool) {
	conn := os.Getenv("SSH_CONNECTION")
	for pid, n := os.Getppid(), 0; conn == "" && pid > 1 && n < 20; n++ {
		ppid := 0
		for _, kv := range environ(pid) {
			if v, ok := strings.CutPrefix(kv, "SSH_CONNECTION="); ok {
				conn = v
			}
		}
		if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
			// pid (comm) state ppid …: comm may hold spaces.
			if i := strings.LastIndexByte(string(b), ')'); i > 0 {
				if f := strings.Fields(string(b)[i+1:]); len(f) > 1 {
					ppid, _ = strconv.Atoi(f[1])
				}
			}
		}
		pid = ppid
	}
	if f := strings.Fields(conn); len(f) >= 3 {
		return f[2], false
	}
	return "", strings.HasPrefix(tty, "/dev/pts/")
}

// procEnviron is pid's environment.
func procEnviron(pid int) []string {
	b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	return strings.Split(string(b), "\x00")
}

// ttyName is stdin's terminal.
func ttyName() string {
	p, _ := os.Readlink("/proc/self/fd/0")
	return p
}

// wayBack is how user gets in again when the network the session came by
// stops: a VPN of the mesh kind up, else the local network.
func wayBack(nets []ui.Network, user string) string {
	for _, pass := range []func(ui.Network) bool{
		func(n ui.Network) bool { return n.ID == steps.NetTailscale || n.ID == steps.NetNetbird },
		func(n ui.Network) bool { return n.ID == "local" },
	} {
		for _, n := range nets {
			if pass(n) && !n.Via && n.State == "up" && n.Addr != "" {
				return "ssh " + user + "@" + n.Addr + " (" + strings.ToLower(n.Name[:1]) + n.Name[1:] + ")"
			}
		}
	}
	return ""
}

// laptopSetups are what to set up on a laptop to reach this machine, as
// ctxName at addrs[0] (its VPN address first): SSH, Docker, and with the
// gateway picked the MCP clients; then the script that does it all.
func laptopSetups(a *answers.Answers, addrs []address, tokens []string, gateway bool, ctxName string) []ui.Setup {
	host, _ := os.Hostname()
	addr := host
	if len(addrs) > 0 {
		addr = addrs[0].ip // a name needs the laptop to resolve it; the address doesn't
	}
	out := []ui.Setup{
		{ID: "ssh", What: "SSH", How: "ssh " + ctxName + "  ·  a Host in the laptop's ~/.ssh/config; your SSH key signs you in",
			Lines: []string{"Host " + ctxName, "  HostName " + addr, "  User " + a.User, "  HostKeyAlias " + ctxName, "  ServerAliveInterval 15", "  ServerAliveCountMax 8"},
			Say: []string{"Add it to ~/.ssh/config on the laptop; then ssh " + ctxName + ", and VS Code's Remote-SSH to " + ctxName + ".",
				"You sign in with your SSH key: the one this machine has in ~/.ssh/authorized_keys."}},
		{ID: "docker", What: "Docker", How: "docker context use " + ctxName + "  ·  over SSH, no port to open",
			Lines: []string{fmt.Sprintf("docker context create %s --docker \"host=ssh://%s@%s\"", ctxName, a.User, addr),
				fmt.Sprintf("docker context use %s        # back: docker context use default", ctxName)},
			Say: []string{"Your SSH key signs you in. With the Host above, \"host=ssh://" + ctxName + "\" does too.",
				"Just one terminal: export DOCKER_CONTEXT=" + ctxName + ". Just one command: docker --context " + ctxName + " ps."}},
	}
	if gateway {
		port := mcpPort(a)
		if url, who, ok := gatewayURL(a, addr); ok {
			tok := laptopToken(a.User, "client")
			if len(tokens) > 0 {
				tok = tokens[0]
			}
			sign := "You sign in with the gateway's token for this client: " + tildeOf(a.User, tok) + " (sudo cat it here). c copies the line with it filled in; it's never shown."
			if len(tokens) == 0 {
				sign = "You sign in with a token for this client: none yet. n makes one (or sudo yardo --yes makes " + tildeOf(a.User, tok) + ")."
			}
			reach := "The gateway is reachable from " + who + "."
			out = append(out,
				ui.Setup{ID: "mcp-claude", What: "Claude Code", How: "claude mcp add " + ctxName + " " + url, Token: tok,
					Lines: []string{fmt.Sprintf("claude mcp add --transport http %s %s --header \"Authorization: Bearer <token>\"", ctxName, url)},
					Say:   []string{"Run it on the laptop. " + reach, sign}},
				ui.Setup{ID: "mcp-codex", What: "Codex", How: "~/.codex/config.toml  ·  [mcp_servers." + ctxName + "]", Token: tok,
					Lines: []string{"[mcp_servers." + ctxName + "]", fmt.Sprintf("url = %q", url), `http_headers = { "Authorization" = "Bearer <token>" }`},
					Say:   []string{"Add it to ~/.codex/config.toml on the laptop. " + reach, sign}},
				ui.Setup{ID: "mcp-other", What: "Other clients", How: url + "  ·  streamable HTTP, a bearer token", Token: tok,
					Lines: []string{url, "Authorization: Bearer <token>"},
					Say:   []string{"Any MCP client that speaks streamable HTTP: the URL, and the header. " + reach, sign}})
		} else {
			tunnel := fmt.Sprintf("ssh -N -L %d:127.0.0.1:%d %s@%s", port, port, a.User, addr)
			local := fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
			say := []string{"The gateway is this machine's only: on the laptop, the ssh tunnel first (it stays in the foreground), then the client. The tunnel is the login: no token.",
				"Settings › Network access opens it to your VPNs, with a token per client."}
			out = append(out,
				ui.Setup{ID: "mcp-claude", What: "Claude Code", How: "through an ssh tunnel  ·  " + local,
					Lines: []string{tunnel, fmt.Sprintf("claude mcp add --transport http %s %s", ctxName, local)}, Say: say},
				ui.Setup{ID: "mcp-codex", What: "Codex", How: "through an ssh tunnel  ·  ~/.codex/config.toml",
					Lines: []string{tunnel, "[mcp_servers." + ctxName + "]", fmt.Sprintf("url = %q", local)}, Say: say},
				ui.Setup{ID: "mcp-other", What: "Other clients", How: "through an ssh tunnel  ·  " + local,
					Lines: []string{tunnel, local}, Say: say})
		}
	}
	out = append(out, ui.Setup{ID: "clipboard", What: "Clipboard", How: "iTerm2  ·  let copies here (y in Neovim, a drag in tmux) reach the laptop",
		Lines: []string{iterm2Clipboard},
		Say: []string{"Run it once on the laptop, then quit and reopen iTerm2. Copies here travel through the terminal (OSC 52), which iTerm2 blocks until then.",
			"Ghostty, kitty, WezTerm, Alacritty: nothing to do. Terminal.app can't (hold fn to select)."}})
	file := "yardo-" + ctxName + ".sh"
	return append(out, ui.Setup{ID: "script", What: "All of it", How: "one script for the laptop: " + file,
		Lines: []string{"ssh " + a.User + "@" + addr + " sudo yardo connect --script > " + file, "sh " + file},
		Say: []string{"On the laptop: save it, read it, then run it. It adds the Host block, the Docker context, the gateway to Claude Code when claude is there, and lets iTerm2 take copies, each once; nothing else.",
			"It holds no token: it reads the token over ssh when it runs."}})
}

// setupLines are the lines of the setup with id.
func setupLines(ss []ui.Setup, id string) []string {
	for _, s := range ss {
		if s.ID == id {
			return s.Lines
		}
	}
	return nil
}

// tokenRows are the gateway's client tokens, by name and file.
func tokenRows(user string) []ui.Token {
	var out []ui.Token
	for _, f := range laptopTokens(user) {
		out = append(out, ui.Token{Name: strings.TrimSuffix(filepath.Base(f), ".token"), File: tildeOf(user, f)})
	}
	return out
}

// copyHint is how to let the terminal copy, in a line.
func copyHint(term string) string {
	if term == "iTerm2" {
		return "iTerm2 needs it allowed, once, on the laptop: defaults write com.googlecode.iterm2 AllowClipboardAccess -bool true (then quit and reopen it). The lines stay here to select by hand."
	}
	return "Ghostty, kitty, WezTerm and Alacritty copy as is; iTerm2 needs it allowed (defaults write com.googlecode.iterm2 AllowClipboardAccess -bool true); Terminal.app can't (hold fn to select). The lines stay here to select by hand."
}

// connData is the Connection screen's data, for user a.
func (o *options) connData(ctx context.Context, a *answers.Answers, addrs []address, picked func(string) bool) ui.ConnData {
	host, _ := os.Hostname()
	ctxName := strings.SplitN(host, ".", 2)[0]
	tokens := laptopTokens(a.User)
	via, unsure := sessionVia(procEnviron, ttyName())
	nets := networks(ctx, steps.OSExec{}, "", picked, addrs, via)
	_, _, open := gatewayURL(a, "")
	return ui.ConnData{Networks: nets, Setups: laptopSetups(a, addrs, tokens, picked("mcpjungle"), ctxName), Tokens: tokenRows(a.User),
		Gateway: picked("mcpjungle") && open, WayBack: wayBack(nets, a.User), Unsure: unsure, CopyHint: copyHint(os.Getenv("LC_TERMINAL"))}
}

// tsLink is a login link in what tailscale said.
var tsLink = regexp.MustCompile(`https://\S+`)

// connHooks are what the Connection screen does, for user a.
func (o *options) connHooks(ctx context.Context, a *answers.Answers) ui.ConnHooks {
	picked := func(id string) bool { return slices.Contains(a.Picks, id) }
	read := func() []ui.Network {
		addrs, _ := withNames(ctx, steps.OSExec{}, addresses())
		via, _ := sessionVia(procEnviron, ttyName())
		return networks(ctx, steps.OSExec{}, "", picked, addrs, via)
	}
	env := func() (*steps.Env, error) {
		log, _ := os.OpenFile(o.log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		e, err := o.env(a, nil, logOrDiscard(log))
		return e, err
	}
	mcpSync := func(args ...string) error {
		out, err := mcpSyncCmd(ctx, o.share(), a, args...).CombinedOutput()
		if err != nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				return errors.New(strings.TrimPrefix(lastOfLines(s), "mcp-sync: "))
			}
		}
		return err
	}
	return ui.ConnHooks{
		Act: func(id, act string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return steps.NetAct(ctx, e, id, act)
		},
		Read: read,
		Link: func() (string, error) {
			e, err := env()
			if err != nil {
				return "", err
			}
			// up waits for the login; the link's in its output, and in
			// status meanwhile (the screen's poll).
			out, err := e.Exec.Run(ctx, steps.Cmd{Name: "tailscale", Args: append([]string{"up", "--timeout=20s"}, "--accept-routes=false", "--operator="+a.User)})
			if l := tsLink.FindString(string(out)); l != "" {
				return l, nil
			}
			if err != nil {
				return "", errors.New(lastOfLines(string(out)))
			}
			return "", nil
		},
		NewToken:    func(name string) error { return mcpSync("client", name) },
		RevokeToken: func(name string) error { return mcpSync("client-rm", name) },
		Tokens:      func() []ui.Token { return tokenRows(a.User) },
		ReadToken: func(file string) (string, error) {
			if rest, ok := strings.CutPrefix(file, "~/"); ok {
				file = filepath.Join(homeOf(a.User), rest)
			}
			b, err := os.ReadFile(file)
			return string(b), err
		},
		Copy: func(s string) error {
			tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			defer tty.Close()
			_, err = io.WriteString(tty, ui.OSC52(s, os.Getenv("TMUX") != ""))
			return err
		},
		DropKey: func(pub string) error { return dropKey(o.state, pub) },
	}
}

// logOrDiscard is f, or nowhere when it couldn't be opened.
func logOrDiscard(f *os.File) io.Writer {
	if f == nil {
		return io.Discard
	}
	return f
}

// lastOfLines is s's last line.
func lastOfLines(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// connectScript is `yardo connect --script`: a POSIX sh script for the
// laptop that adds the Host block, the Docker context and (with claude
// there and the gateway picked) the gateway to Claude Code, each once. It
// holds no token: it reads it over ssh as it runs.
func connectScript(w io.Writer, a *answers.Answers, addrs []address, tokens []string, gateway bool, ctxName string) {
	ss := laptopSetups(a, addrs, tokens, gateway, ctxName)
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	fmt.Fprintf(w, `#!/bin/sh
# yardo connect --script: sets up this laptop to reach %s. It adds
# what's missing, once: the ssh Host block, the Docker context, the MCP
# gateway to Claude Code when claude is installed, and iTerm2's leave to
# take copies from Neovim and tmux there. Nothing else.
set -eu
name=%s

conf="$HOME/.ssh/config"
if [ -f "$conf" ] && grep -qix "host $name" "$conf"; then
  echo "ssh: Host $name is in $conf already."
else
  mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh"
  [ ! -s "$conf" ] || echo >> "$conf"
  cat >> "$conf" <<'EOF'
%s
EOF
  chmod 600 "$conf"
  echo "ssh: added Host $name to $conf (ssh $name)."
fi
`, ctxName, q(ctxName), strings.Join(setupLines(ss, "ssh"), "\n"))
	fmt.Fprintf(w, `
if ! command -v docker >/dev/null 2>&1; then
  echo "docker: not on this laptop; skipped."
elif docker context inspect "$name" >/dev/null 2>&1; then
  echo "docker: the context $name is there already (docker context use $name)."
else
  docker context create "$name" --docker "host=ssh://$name" >/dev/null
  echo "docker: made the context $name (docker context use $name)."
fi

if ! command -v defaults >/dev/null 2>&1 || ! defaults read com.googlecode.iterm2 >/dev/null 2>&1; then
  echo "iTerm2: not on this laptop; skipped."
elif [ "$(defaults read com.googlecode.iterm2 AllowClipboardAccess 2>/dev/null)" = 1 ]; then
  echo "iTerm2: takes copies from $name already."
else
  %s
  echo "iTerm2: takes copies from $name (y in Neovim, a drag in tmux) once you quit and reopen it."
fi
`, iterm2Clipboard)
	if !gateway {
		return
	}
	claude := setupLines(ss, "mcp-claude")
	add := claude[len(claude)-1]
	fmt.Fprintf(w, `
if ! command -v claude >/dev/null 2>&1; then
  echo "claude: not on this laptop; skipped."
elif claude mcp get "$name" >/dev/null 2>&1; then
  echo "claude: the MCP server $name is there already."
`)
	if strings.Contains(add, "<token>") {
		var tok string
		for _, s := range ss {
			if s.ID == "mcp-claude" {
				tok = tildeOf(a.User, s.Token)
			}
		}
		fmt.Fprintf(w, `else
  # The token, read now over ssh (sudo may ask for your password); never
  # kept in this script.
  token=$(ssh -t "$name" sudo cat %s | tr -d '\r' | tail -n 1)
  if [ -z "$token" ]; then
    echo "claude: no token from $name (in sudo yardo, Connection: n makes one); skipped." >&2
  else
    claude mcp add --transport http "$name" %s --header "Authorization: Bearer $token" >/dev/null
    echo "claude: added the MCP server $name."
  fi
fi
`, q(tok), q(strings.Fields(add)[6]))
	} else {
		fmt.Fprintf(w, `else
  %s >/dev/null
  echo "claude: added the MCP server $name; it needs the tunnel up: %s"
fi
`, add, claude[0])
	}
}

// newProfile is a VPN client profile to add: its kind, name and contents.
type newProfile struct {
	kind, name string
	conf       []byte
}

// connDone is what a Connection result came to: a toast, or a run (fix,
// with sec, pick picked first; then prof added); link: start Tailscale's
// login by link.
type connDone struct {
	toast string
	fix   []string
	sec   *creds.Secrets
	pick  string
	link  bool
	prof  *newProfile
}

// connDo does what Connection left to type for: a VPN's login (its key,
// then the login item's run; Tailscale with none, a login by link), or a
// profile to add.
func (o *options) connDo(ctx context.Context, a *answers.Answers, r ui.ConnResult, out io.Writer) (connDone, error) {
	sec := creds.New()
	var err error
	switch {
	case r.Do == "login" && r.ID == steps.NetTailscale:
		sec.TSAuthKey, err = ui.Password("Tailscale auth key: a tagged one (Settings → Keys → Tags) never expires (empty: log in with a link, in a browser)", "tskey-auth-…")
		switch {
		case errors.Is(err, ui.ErrQuit):
			return connDone{}, nil
		case err != nil:
			return connDone{}, err
		case sec.TSAuthKey == "":
			return connDone{link: true}, nil
		}
		return connDone{fix: []string{"tailscale"}, sec: sec, pick: "tailscale"}, nil
	case r.Do == "login" && r.ID == steps.NetNetbird:
		sec.NBSetupKey, err = ui.Password("NetBird setup key: a reusable one with an auto-group (Setup Keys → Auto-assigned groups) puts the machine in that group's policies", "")
		if errors.Is(err, ui.ErrQuit) || err == nil && sec.NBSetupKey == "" {
			return connDone{toast: "No key: NetBird isn't set up."}, nil
		} else if err != nil {
			return connDone{}, err
		}
		if sec.NBManagementURL, err = ui.Input("NetBird management URL", "Empty for NetBird's cloud; your own server's URL (https://netbird.example.com) if you self-host.", ""); err != nil && !errors.Is(err, ui.ErrQuit) {
			return connDone{}, err
		}
		return connDone{fix: []string{"netbird"}, sec: sec, pick: "netbird"}, nil
	case r.Do == "profile":
		return o.askProfile(ctx, a, out)
	}
	return connDone{}, nil
}

// clientItem is the item that installs kind's client.
var clientItem = map[string]string{steps.NetWireGuard: "wireguard-client", steps.NetOpenVPN: "openvpn-client"}

// askProfile asks for a profile to add: its VPN, name and contents (a
// file here, or pasted). A bad one is refused, nothing stored; one that
// routes everything says so first. With its client not installed, that's
// installed first, then the profile added.
func (o *options) askProfile(ctx context.Context, a *answers.Answers, out io.Writer) (connDone, error) {
	quit := func(err error) (connDone, error) {
		if errors.Is(err, ui.ErrQuit) {
			return connDone{toast: "No profile added."}, nil
		}
		return connDone{}, err
	}
	kinds := []string{"WireGuard", "OpenVPN"}
	sel := "WireGuard"
	if slices.Contains(a.Picks, "openvpn-client") && !slices.Contains(a.Picks, "wireguard-client") {
		sel = "OpenVPN"
	}
	k, err := ui.Choose("Add a profile for", kinds, sel)
	if err != nil {
		return quit(err)
	}
	kind := map[string]string{"WireGuard": steps.NetWireGuard, "OpenVPN": steps.NetOpenVPN}[k]
	name, err := ui.Input("Its name", "Lowercase letters, digits and -, 15 at most: it names its interface too (office, home).", "")
	if err != nil {
		return quit(err)
	}
	name = strings.TrimSpace(name)
	if err := steps.CheckProfileName(name); err != nil {
		return connDone{toast: "✗ " + err.Error() + ": nothing stored."}, nil
	}
	if _, err := os.Lstat(filepath.Join(steps.ProfileDir(kind), name+".conf")); err == nil {
		return connDone{toast: "✗ There's a profile called " + name + " already: nothing stored."}, nil
	}
	file, err := ui.Input("The profile", "A file on this machine ("+map[string]string{steps.NetWireGuard: "~/office.conf", steps.NetOpenVPN: "~/office.ovpn"}[kind]+"), or empty to paste it.", "")
	if err != nil {
		return quit(err)
	}
	var conf []byte
	if file = strings.TrimSpace(file); file == "" {
		s, err := ui.Text("Paste the profile", "Then alt+enter, or tab to leave the box and enter. It's stored root's only, never shown again.")
		if err != nil {
			return quit(err)
		}
		conf = []byte(strings.TrimSpace(s) + "\n")
	} else {
		if rest, ok := strings.CutPrefix(file, "~/"); ok {
			file = filepath.Join(homeOf(a.User), rest)
		}
		if conf, err = os.ReadFile(file); err != nil {
			return connDone{toast: "✗ " + err.Error() + ": nothing stored."}, nil
		}
	}
	if err := steps.CheckProfile(kind, conf); err != nil {
		return connDone{toast: "✗ " + strings.ToUpper(err.Error()[:1]) + err.Error()[1:] + ": nothing stored."}, nil
	}
	if steps.FullTunnel(kind, conf) {
		ok, err := ui.Confirm(name+" sends all of this machine's traffic through it",
			"Its default route changes when it starts. ssh over the local network keeps working (yardo keeps that way in), but a session that came in over another VPN could drop. Add it?", false)
		if err != nil || !ok {
			return quit(cmpErr(err))
		}
	}
	p := &newProfile{kind: kind, name: name, conf: conf}
	if it := clientItem[kind]; !slices.Contains(a.Picks, it) {
		fmt.Fprintf(out, "%s isn't installed: installing it first, then %s.\n", k, name)
		return connDone{fix: []string{it}, sec: creds.New(), pick: it, prof: p}, nil
	}
	return connDone{toast: o.addProfile(ctx, a, *p, out)}, nil
}

// cmpErr is err, or ErrQuit for a no.
func cmpErr(err error) error {
	if err == nil {
		return ui.ErrQuit
	}
	return err
}

// addProfile stores p, starts it and says how that went, in a line.
func (o *options) addProfile(ctx context.Context, a *answers.Answers, p newProfile, out io.Writer) string {
	log, _ := os.OpenFile(o.log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if log != nil {
		defer log.Close()
	}
	e, err := o.env(a, nil, logOrDiscard(log))
	if err != nil {
		return "✗ " + err.Error()
	}
	fmt.Fprintf(out, "Starting %s and waiting for it to connect…\n", p.name)
	ok, err := steps.AddProfile(ctx, e, p.kind, p.name, p.conf)
	switch {
	case err != nil:
		return "✗ " + err.Error()
	case !ok:
		return p.name + " is up, but nothing answered yet: its server, or its keys? (sudo journalctl -u " + steps.ProfileUnit(p.kind, p.name) + ")"
	}
	return "✓ " + p.name + " is connected, and starts at boot."
}
