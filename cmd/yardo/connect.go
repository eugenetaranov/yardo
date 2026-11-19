package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// address is one of this machine's IPv4 addresses, and which interface
// (and VPN) it's on; name is the VPN's DNS name for it, when it has one.
type address struct{ ip, iface, vpn, name string }

// VPN kinds: a mesh (Tailscale, NetBird) finds the direct path at home;
// yardo's servers have one address everywhere; a client is this machine's
// connection out to another network, not a way in from your laptop.
const (
	vpnMesh   = "mesh"
	vpnServer = "server"
	vpnClient = "client"
)

// vpnOf names the VPN an interface and address are on, and its kind
// ("" for neither). yardo's OpenVPN server is 10.8.0.1 on a tun device.
func vpnOf(iface, ip string) (name, kind string) {
	switch {
	case iface == "tailscale0":
		return "Tailscale", vpnMesh
	case iface == "wt0":
		return "NetBird", vpnMesh
	case iface == "yardo-wg":
		return "WireGuard (yardo's server)", vpnServer
	case strings.HasPrefix(iface, "tun") && ip == "10.8.0.1":
		return "OpenVPN (yardo's server)", vpnServer
	case strings.HasPrefix(iface, "tun"):
		return "OpenVPN client (this machine's connection out)", vpnClient
	case strings.HasPrefix(iface, "wg"):
		return "WireGuard client (this machine's connection out)", vpnClient
	}
	return "", ""
}

// rank orders addresses for a laptop: a mesh VPN's, yardo's servers', the
// local network's, then connections out.
func (x address) rank() int {
	return map[string]int{vpnMesh: 0, vpnServer: 1, "": 2, vpnClient: 3}[x.kind()]
}

// kind is the VPN kind of x: Tailscale's, whatever the interface, is a mesh.
func (x address) kind() string {
	if x.vpn == "Tailscale" {
		return vpnMesh
	}
	_, k := vpnOf(x.iface, x.ip)
	return k
}

// tsUserspace stands for the interface of a Tailscale address with none:
// tailscaled's userspace networking (an LXC container without /dev/net/tun).
const tsUserspace = "userspace"

// addresses are this machine's IPv4 addresses a laptop could reach, in
// rank order; not loopback or Docker's bridges.
func addresses() []address {
	ifs, _ := net.Interfaces()
	var out []address
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || i.Name == "docker0" ||
			strings.HasPrefix(i.Name, "br-") || strings.HasPrefix(i.Name, "veth") {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				name, _ := vpnOf(i.Name, n.IP.String())
				out = append(out, address{ip: n.IP.String(), iface: i.Name, vpn: name})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b address) int { return a.rank() - b.rank() })
	return out
}

// withNames fills in the VPNs' DNS names for their addresses (Tailscale's
// MagicDNS name), adds Tailscale's address when no interface has it
// (userspace networking), and returns Tailscale's status when it's up.
func withNames(ctx context.Context, x steps.Execer, addrs []address) ([]address, *steps.TSStatus) {
	st, ok := steps.TailscaleStatus(ctx, x)
	if !ok || st.BackendState != "Running" {
		return addrs, nil
	}
	name := strings.TrimSuffix(st.Self.DNSName, ".")
	found := false
	for i := range addrs {
		if addrs[i].iface == "tailscale0" || slices.Contains(st.Self.TailscaleIPs, addrs[i].ip) {
			addrs[i].vpn, addrs[i].name = "Tailscale", name
			found = true
		}
	}
	if !found {
		for _, ip := range st.Self.TailscaleIPs {
			if p := net.ParseIP(ip); p != nil && p.To4() != nil {
				addrs = append(addrs, address{ip: ip, iface: tsUserspace, vpn: "Tailscale", name: name})
				break
			}
		}
	}
	slices.SortStableFunc(addrs, func(a, b address) int { return a.rank() - b.rank() })
	return addrs, &st
}

// printConnect writes how to reach Docker and the MCP gateway from a
// laptop, for user a; ts is Tailscale's status, when it's up.
// clipboardHint is how copying in Neovim or tmux here reaches the
// laptop's clipboard: through the terminal (OSC 52), which iTerm2 lets
// through only once allowed. term is the laptop's terminal when it says
// (iTerm2 sends LC_TERMINAL over ssh), else "".
func clipboardHint(term string) []string {
	out := []string{
		"Copy and paste (y in Neovim, a drag in tmux) reach your laptop's clipboard through the terminal.",
		"  iTerm2, once on your laptop (then quit and reopen it):",
		ui.Paste + "    defaults write com.googlecode.iterm2 AllowClipboardAccess -bool true",
	}
	if term != "iTerm2" {
		out = append(out, "  Ghostty, kitty, WezTerm, Alacritty: nothing to do · Terminal.app can't (hold fn to select).")
	}
	return out
}

// paste writes lines to type, or to put in a file, indented; marked
// ui.Paste, the screen shows them apart from what's said about them.
func paste(w io.Writer, indent string, lines ...string) {
	for _, l := range lines {
		fmt.Fprintln(w, ui.Paste+indent+l)
	}
}

// printConnect says how to reach this machine from a laptop; term is the
// laptop's terminal when it says (see clipboardHint).
func printConnect(w io.Writer, a *answers.Answers, addrs []address, ts *steps.TSStatus, term string, tokens []string) {
	host, _ := os.Hostname()
	ctxName := strings.SplitN(host, ".", 2)[0]
	fmt.Fprintf(w, "Connecting to %s from your laptop (as %s)\n\n", host, a.User)
	fmt.Fprintln(w, "This machine's addresses:")
	addr := host
	for i, x := range addrs {
		where := x.iface
		switch {
		case x.kind() == vpnClient:
			where += " · " + x.vpn
		case x.vpn != "":
			where += " · " + x.vpn + " (bring it up on your laptop too)"
		}
		if x.name != "" {
			where = x.name + " · " + where
		}
		fmt.Fprintf(w, "  %-15s %s\n", x.ip, where)
		if i == 0 {
			addr = cmp.Or(x.name, x.ip)
		}
	}
	if len(addrs) == 0 {
		fmt.Fprintln(w, "  (none found)")
	}
	if len(addrs) > 0 && (addrs[0].kind() == vpnMesh || addrs[0].kind() == vpnServer) {
		fmt.Fprintf(w, "\nUse %s at home and away: ", addr)
		if addrs[0].kind() == vpnMesh {
			fmt.Fprintf(w, "at home %s takes the direct path over your network, and\n", addrs[0].vpn)
		} else {
			fmt.Fprintf(w, "keep %s connected on your laptop at home too, and\n", addrs[0].vpn)
		}
		fmt.Fprintln(w, "the address stays the same when your laptop changes network. Not the local address:")
		fmt.Fprintln(w, "VPN-only services (the MCP gateway, container ports) aren't open there.")
		fmt.Fprintln(w, "\nSSH: add to your laptop's ~/.ssh/config (then: ssh "+ctxName+", and VS Code Remote-SSH to "+ctxName+"):")
		paste(w, "  ", "Host "+ctxName, "  HostName "+addr, "  User "+a.User, "  HostKeyAlias "+ctxName,
			"  ServerAliveInterval 15", "  ServerAliveCountMax 8")
		if addrs[0].vpn == "Tailscale" && addrs[0].name == "" {
			fmt.Fprintln(w, "  (Tailscale gave no name: MagicDNS off? Its address works the same.)")
		}
	} else if len(addrs) > 0 {
		fmt.Fprintf(w, "\nThe commands below use %s, which works only on its own network; a VPN (Tailscale:\n", addr)
		fmt.Fprintln(w, "in 'sudo yardo', Tools → Tailscale → Log in) gives one name for home and away.")
	}
	if ts != nil && len(ts.Self.Tags) == 0 && ts.Self.KeyExpiry != nil {
		fmt.Fprintf(w, "\nTailscale: this machine's key expires on %s, and then it drops off the tailnet. Turn that off in\n",
			ts.Self.KeyExpiry.Format("2006-01-02"))
		fmt.Fprintln(w, "the admin console (Machines → … → Disable key expiry), or join with a tagged auth key.")
	}

	fmt.Fprintln(w, "\nDocker, over SSH (no port to open; your SSH key is the login). On the laptop:")
	paste(w, "  ", fmt.Sprintf("docker context create %s --docker \"host=ssh://%s@%s\"", ctxName, a.User, addr),
		fmt.Sprintf("docker context use %s        # back: docker context use default", ctxName))
	fmt.Fprintf(w, "  docker context use makes it the default: every docker command, in every terminal, until you\n")
	fmt.Fprintf(w, "  switch back. Just one terminal: export DOCKER_CONTEXT=%s. Just one command: docker --context %s ps.\n", ctxName, ctxName)
	ports := map[string]string{"": "everywhere (Docker's way)", "vpn": "this machine and the VPNs", "local": "this machine only"}[a.DockerPorts]
	fmt.Fprintf(w, "  -v paths are this machine's; -p ports open here, reachable from %s.\n", ports)

	fmt.Fprintln(w)
	for _, l := range clipboardHint(term) {
		fmt.Fprintln(w, l)
	}

	port := mcpPort(a)
	fmt.Fprintln(w, "\nMCP gateway (mcpjungle; the servers' own tokens stay here):")
	if url, who, ok := gatewayURL(a, addr); ok {
		fmt.Fprintf(w, "  reachable from %s, with the gateway's token for your laptop.\n", who)
		if len(tokens) > 0 {
			fmt.Fprintln(w, "  1. Read the token here (it isn't shown):")
			for _, t := range tokens {
				paste(w, "       ", "sudo cat "+t)
			}
		} else {
			fmt.Fprintln(w, "  1. yardo makes the token on its next run (sudo yardo --yes), in a file, not on the screen; read it here:")
			paste(w, "       ", "sudo cat "+laptopToken(a.User, "client"))
		}
		fmt.Fprintln(w, "  2. On the laptop, for Claude Code (<token> is the token):")
		paste(w, "       ", fmt.Sprintf("claude mcp add --transport http %s %s --header \"Authorization: Bearer <token>\"", ctxName, url))
		fmt.Fprintln(w, "     or for Codex, add to ~/.codex/config.toml:")
		paste(w, "       ", "[mcp_servers."+ctxName+"]", fmt.Sprintf("url = %q", url), `http_headers = { "Authorization" = "Bearer <token>" }`)
	} else {
		fmt.Fprintln(w, "  this machine only: in 'sudo yardo', Settings → Network access opens it to your VPNs or")
		fmt.Fprintln(w, "  the internet. Or, with nothing to change, through SSH, on the laptop:")
		paste(w, "    ", fmt.Sprintf("ssh -N -L %d:127.0.0.1:%d %s@%s", port, port, a.User, addr),
			fmt.Sprintf("claude mcp add --transport http %s http://127.0.0.1:%d/mcp", ctxName, port))
	}
}

func mcpPort(a *answers.Answers) int {
	if a.MCPPort == 0 {
		return answers.FirstMCPPort
	}
	return a.MCPPort
}

// gatewayURL is the MCP gateway's URL from a laptop, at addr, and who can
// reach it; ok is false when it's this machine's only.
func gatewayURL(a *answers.Answers, addr string) (url, who string, ok bool) {
	switch a.MCPListen {
	case "vpn", "tailscale":
		return fmt.Sprintf("http://%s:%d/mcp", addr, mcpPort(a)), "this machine and the VPNs", true
	case "0.0.0.0":
		return fmt.Sprintf("http://%s:%d/mcp", addr, mcpPort(a)), "everywhere (plain HTTP)", true
	case "public":
		return "https://" + a.MCPDomain + "/mcp", "the internet, over HTTPS", true
	}
	return "", "", false
}

// laptopToken is where mcp-sync client keeps name's gateway token.
func laptopToken(user, name string) string {
	return filepath.Join(homeOf(user), ".mcp", "clients", name+".token")
}

// laptopTokens are the gateway tokens mcp-sync client made, by file.
func laptopTokens(user string) []string {
	ts, _ := filepath.Glob(laptopToken(user, "*"))
	return ts
}

// connectRows are the home's How to connect panel: what to type on the
// laptop, and where it goes or where its secret is kept (never the
// secret); gateway: the MCP gateway is picked.
func connectRows(a *answers.Answers, addrs []address, tokens []string, gateway bool) []ui.ConnectRow {
	host, _ := os.Hostname()
	ctxName := strings.SplitN(host, ".", 2)[0]
	addr := host
	if len(addrs) > 0 {
		addr = cmp.Or(addrs[0].name, addrs[0].ip)
	}
	ssh := a.User + "@" + addr
	if len(addrs) > 0 && addrs[0].vpn != "" && addrs[0].kind() != vpnClient {
		ssh += " over " + addrs[0].vpn
	}
	ports := map[string]string{"": "-p ports open everywhere", "vpn": "-p ports from the VPNs", "local": "-p ports here only"}[a.DockerPorts]
	rows := []ui.ConnectRow{
		{What: "SSH", Use: "ssh " + ctxName, Note: ssh + ", your SSH key (Host " + ctxName + " in ~/.ssh/config)"},
		{What: "Docker", Use: "docker context use " + ctxName, Note: "host=ssh://" + a.User + "@" + addr + " · " + ports},
	}
	if !gateway {
		return rows
	}
	mcp := ui.ConnectRow{What: "MCP"}
	if url, _, ok := gatewayURL(a, addr); ok {
		mcp.Use = url
		switch {
		case len(tokens) > 0:
			mcp.Note = "mcpjungle, with a token: sudo cat " + tildeOf(a.User, tokens[0])
		default:
			mcp.Note = "mcpjungle, with a token: sudo yardo --yes makes it, in ~/.mcp/clients/client.token"
		}
	} else {
		p := mcpPort(a)
		mcp.Use = fmt.Sprintf("ssh -N -L %d:127.0.0.1:%d %s", p, p, ctxName)
		mcp.Note = fmt.Sprintf("mcpjungle, then http://127.0.0.1:%d/mcp · no token: it's this machine's only, the tunnel is the login (Settings › Network access opens it, with a token)", p)
	}
	return append(rows, mcp)
}

// tildeOf is path with user's home as ~.
func tildeOf(user, path string) string {
	if rest, ok := strings.CutPrefix(path, homeOf(user)+"/"); ok {
		return "~/" + rest
	}
	return path
}

// connect is `yardo connect`: how to reach this machine from a laptop, for
// the user who ran sudo (or --user).
func (o *options) connect(stdout io.Writer, st *answers.Store) error {
	u, err := o.userFor(st)
	if err != nil {
		return err
	}
	a, _, err := st.Load(u)
	if err != nil {
		return err
	}
	a.User = u
	addrs, ts := withNames(context.Background(), steps.OSExec{}, addresses())
	var b strings.Builder
	printConnect(&b, a, addrs, ts, os.Getenv("LC_TERMINAL"), laptopTokens(u))
	_, err = io.WriteString(stdout, ui.Unmark(b.String()))
	return err
}

// userFor is whose answers a command is about: --user, the sudo user, or
// the one user set up.
func (o *options) userFor(st *answers.Store) (string, error) {
	u := o.user
	if u == "" {
		u = os.Getenv("SUDO_USER")
	}
	if u == "" || u == "root" {
		saved, _, err := st.Users()
		if err != nil {
			return "", err
		}
		if len(saved) != 1 {
			return "", errors.New("which user? give --user")
		}
		u = saved[0]
	}
	return u, nil
}
