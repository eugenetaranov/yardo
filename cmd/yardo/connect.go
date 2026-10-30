package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/steps"
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
	_, kind := vpnOf(x.iface, x.ip)
	return map[string]int{vpnMesh: 0, vpnServer: 1, "": 2, vpnClient: 3}[kind]
}

func (x address) kind() string {
	_, k := vpnOf(x.iface, x.ip)
	return k
}

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
// MagicDNS name), and returns Tailscale's status when it's up.
func withNames(ctx context.Context, x steps.Execer, addrs []address) *steps.TSStatus {
	for i := range addrs {
		if addrs[i].iface != "tailscale0" {
			continue
		}
		st, ok := steps.TailscaleStatus(ctx, x)
		if !ok || st.BackendState != "Running" {
			return nil
		}
		addrs[i].name = strings.TrimSuffix(st.Self.DNSName, ".")
		return &st
	}
	return nil
}

// printConnect writes how to reach Docker and the MCP gateway from a
// laptop, for user a; ts is Tailscale's status, when it's up.
func printConnect(w io.Writer, a *answers.Answers, addrs []address, ts *steps.TSStatus) {
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
		fmt.Fprintln(w, "\nYour laptop's ~/.ssh/config (then: ssh "+ctxName+", and VS Code Remote-SSH to "+ctxName+"):")
		fmt.Fprintf(w, "  Host %s\n    HostName %s\n    User %s\n    HostKeyAlias %s\n    ServerAliveInterval 15\n    ServerAliveCountMax 8\n",
			ctxName, addr, a.User, ctxName)
		if addrs[0].iface == "tailscale0" && addrs[0].name == "" {
			fmt.Fprintln(w, "  (Tailscale gave no name: MagicDNS off? Its address works the same.)")
		}
	} else if len(addrs) > 0 {
		fmt.Fprintf(w, "\nThe commands below use %s, which works only on its own network; a VPN (Tailscale:\n", addr)
		fmt.Fprintln(w, "This VM → VPN logins in 'sudo yardo') gives one name for home and away.")
	}
	if ts != nil && len(ts.Self.Tags) == 0 && ts.Self.KeyExpiry != nil {
		fmt.Fprintf(w, "\nTailscale: this machine's key expires on %s, and then it drops off the tailnet. Turn that off in\n",
			ts.Self.KeyExpiry.Format("2006-01-02"))
		fmt.Fprintln(w, "the admin console (Machines → … → Disable key expiry), or join with a tagged auth key.")
	}

	fmt.Fprintln(w, "\nDocker, over SSH (no port to open; your SSH key is the login):")
	fmt.Fprintf(w, "  docker context create %s --docker \"host=ssh://%s@%s\"\n", ctxName, a.User, addr)
	fmt.Fprintf(w, "  docker context use %s        # back: docker context use default\n", ctxName)
	ports := map[string]string{"": "everywhere (Docker's way)", "vpn": "this machine and the VPNs", "local": "this machine only"}[a.DockerPorts]
	fmt.Fprintf(w, "  -v paths are this machine's; -p ports open here, reachable from %s.\n", ports)
	if a.DockerAPI != "" {
		who := map[string]string{"vpn": "the VPNs", "lan": "the local network and the VPNs"}[a.DockerAPI]
		fmt.Fprintf(w, "\nDocker, directly (TLS on port 2376, from %s; for tools that can't use SSH):\n", who)
		fmt.Fprintln(w, "  sudo yardo-docker client laptop     # here, once: the laptop's certificate, and how to use it")
		fmt.Fprintf(w, "  then: docker context create %s-tls --docker \"host=tcp://%s:2376,ca=…,cert=…,key=…\"\n", ctxName, addr)
	}

	port := a.MCPPort
	if port == 0 {
		port = answers.FirstMCPPort
	}
	fmt.Fprintln(w, "\nMCP gateway:")
	switch a.MCPListen {
	case "vpn", "tailscale", "0.0.0.0", "public":
		who := "this machine and the VPNs"
		url := fmt.Sprintf("http://%s:%d/mcp", addr, port)
		switch a.MCPListen {
		case "0.0.0.0":
			who = "everywhere (plain HTTP)"
		case "public":
			who, url = "the internet, over HTTPS", "https://"+a.MCPDomain+"/mcp"
		}
		fmt.Fprintf(w, "  reachable from %s, with a token. Make your laptop's (once, here):\n", who)
		fmt.Fprintln(w, "    mcp-sync client laptop")
		fmt.Fprintln(w, "  then on the laptop:")
		fmt.Fprintf(w, "    claude mcp add --transport http %s %s --header \"Authorization: Bearer <token>\"\n", ctxName, url)
		fmt.Fprintf(w, "    Codex, in ~/.codex/config.toml:\n      [mcp_servers.%s]\n      url = %q\n      http_headers = { \"Authorization\" = \"Bearer <token>\" }\n", ctxName, url)
	default:
		fmt.Fprintln(w, "  this machine only: in 'sudo yardo', This VM → MCP gateway opens it to your VPNs or")
		fmt.Fprintln(w, "  the internet. Or, with nothing to change, through SSH:")
		fmt.Fprintf(w, "    ssh -N -L %d:127.0.0.1:%d %s@%s\n", port, port, a.User, addr)
		fmt.Fprintf(w, "    claude mcp add --transport http %s http://127.0.0.1:%d/mcp\n", ctxName, port)
	}
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
	addrs := addresses()
	printConnect(stdout, a, addrs, withNames(context.Background(), steps.OSExec{}, addrs))
	return nil
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
