package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
)

// address is one of this machine's IPv4 addresses, and which interface
// (and VPN) it's on.
type address struct{ ip, iface, vpn string }

// vpnNames name the VPN interfaces.
var vpnNames = map[string]string{"tailscale0": "Tailscale", "wt0": "NetBird", "yardo-wg": "WireGuard (yardo's server)", "tun0": "OpenVPN", "wg0": "WireGuard"}

// addresses are this machine's IPv4 addresses a laptop could reach: VPNs
// first; not loopback or Docker's bridges.
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
				out = append(out, address{n.IP.String(), i.Name, vpnNames[i.Name]})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b address) int {
		switch {
		case a.vpn != "" && b.vpn == "":
			return -1
		case a.vpn == "" && b.vpn != "":
			return 1
		}
		return 0
	})
	return out
}

// printConnect writes how to reach Docker and the MCP gateway from a
// laptop, for user a.
func printConnect(w io.Writer, a *answers.Answers, addrs []address) {
	host, _ := os.Hostname()
	ctxName := strings.SplitN(host, ".", 2)[0]
	fmt.Fprintf(w, "Connecting to %s from your laptop (as %s)\n\n", host, a.User)
	fmt.Fprintln(w, "This machine's addresses:")
	addr := host
	for i, x := range addrs {
		where := x.iface
		if x.vpn != "" {
			where += " · " + x.vpn + " (bring it up on your laptop too)"
		}
		fmt.Fprintf(w, "  %-15s %s\n", x.ip, where)
		if i == 0 {
			addr = x.ip
		}
	}
	if len(addrs) == 0 {
		fmt.Fprintln(w, "  (none found)")
	}
	fmt.Fprintf(w, "\nThe commands below use %s; use another address from the list if that's how your laptop reaches it.\n", addr)

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
	printConnect(stdout, a, addresses())
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
