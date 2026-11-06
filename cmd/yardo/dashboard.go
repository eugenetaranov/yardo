package main

import (
	"context"
	"net"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/ui"
)

// The home's dashboard: the containers running, the services up (or
// failed), and what listens on the network. Each comes from one quick
// command; a command missing or slow leaves its part out.

// dashboard gathers the home's dashboard.
func dashboard(ctx context.Context) ui.Dashboard {
	var d ui.Dashboard
	if out, ok := quick(ctx, "docker", "ps", "--format", "{{.Names}}\t{{.Ports}}"); ok {
		d.Docker, d.Containers = true, parseContainers(out)
	}
	if out, ok := quick(ctx, "systemctl", "list-units", "--type=service", "--all", "--no-legend", "--plain"); ok {
		d.Services = parseServices(out)
	}
	if out, ok := quick(ctx, "ss", "-H", "-tulnp"); ok {
		d.Listening = parseListening(out)
	}
	return d
}

// quick runs a command for at most 3 seconds: its output, and whether it
// ran.
func quick(ctx context.Context, name string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err == nil
}

// parseContainers reads docker ps' names and ports: each container, with
// the host ports it publishes.
func parseContainers(out string) []ui.Container {
	var cs []ui.Container
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		name, ports, _ := strings.Cut(l, "\t")
		if name == "" {
			continue
		}
		c := ui.Container{Name: name}
		// 0.0.0.0:5432->5432/tcp, [::]:5432->5432/tcp
		for _, p := range strings.Split(ports, ", ") {
			host, _, ok := strings.Cut(p, "->")
			if !ok {
				continue
			}
			if i := strings.LastIndex(host, ":"); i >= 0 && !slices.Contains(c.Ports, host[i+1:]) {
				c.Ports = append(c.Ports, host[i+1:])
			}
		}
		cs = append(cs, c)
	}
	return cs
}

// services are the services the dashboard shows, by unit (a template's
// prefix ends in @), and their names.
var services = []struct{ unit, name string }{
	{"ssh.service", "ssh"}, {"sshd.service", "ssh"}, {"docker.service", "docker"},
	{"tailscaled.service", "tailscale"}, {"netbird.service", "netbird"},
	{"openvpn-server@", "openvpn"}, {"openvpn@", "openvpn"}, {"openvpn-client@", "openvpn"},
	{"wg-quick@", "wireguard"}, {"nfs-server.service", "nfs"}, {"smbd.service", "samba"},
	{"fail2ban.service", "fail2ban"}, {"k3s.service", "k3s"}, {"ollama.service", "ollama"},
}

// parseServices reads systemctl list-units: the services above that run,
// or failed, in that order; the others (stopped on purpose, or not there)
// are left out.
func parseServices(out string) []ui.Service {
	state := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 4 {
			continue
		}
		for _, s := range services {
			if f[0] == s.unit || strings.HasSuffix(s.unit, "@") && strings.HasPrefix(f[0], s.unit) {
				switch {
				case f[2] == "active" || f[2] == "activating" || f[2] == "reloading":
					state[s.name] = "up"
				case f[2] == "failed" && state[s.name] == "":
					state[s.name] = "failed"
				}
			}
		}
	}
	var out2 []ui.Service
	for _, s := range services {
		if st, ok := state[s.name]; ok && !slices.ContainsFunc(out2, func(x ui.Service) bool { return x.Name == s.name }) {
			out2 = append(out2, ui.Service{Name: s.name, Up: st == "up"})
		}
	}
	return out2
}

// listenNames are what a listening process is called on the dashboard.
var listenNames = map[string]string{
	"sshd": "ssh", "tailscaled": "tailscale", "netbird": "netbird", "openvpn": "openvpn", "smbd": "samba",
	"docker-proxy": "docker", "dockerd": "docker", "k3s-server": "k3s", "ollama": "ollama", "mcpjungle": "mcp",
	"rpcbind": "rpcbind", "nginx": "nginx", "caddy": "caddy",
}

// quietPorts are listeners nobody needs told about: DHCP, mDNS, chrony.
var quietPorts = map[string]bool{"68": true, "546": true, "5353": true, "323": true}

// parseListening reads ss -tulnp: what listens, on which port, to whom
// (everywhere, your VPNs, this machine, or one address); the same listener
// on IPv4 and IPv6 once, the widest reach kept.
func parseListening(out string) []ui.Listener {
	var ls []ui.Listener
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 5 {
			continue
		}
		proto, local := f[0], f[4]
		i := strings.LastIndex(local, ":")
		if i < 0 {
			continue
		}
		addr, port := strings.Trim(local[:i], "[]"), local[i+1:]
		addr, _, _ = strings.Cut(addr, "%")
		if quietPorts[port] {
			continue
		}
		name := ""
		if len(f) > 6 {
			// users:(("sshd",pid=900,fd=4))
			if _, rest, ok := strings.Cut(f[6], `(("`); ok {
				name, _, _ = strings.Cut(rest, `"`)
			}
		}
		switch {
		case listenNames[name] != "":
			name = listenNames[name]
		case name == "" && port == "2049":
			name = "nfs"
		case name == "systemd-resolve", name == "chronyd":
			continue
		}
		x := ui.Listener{Port: port, Proto: proto, Name: name, Scope: scopeOf(addr)}
		if n := slices.IndexFunc(ls, func(y ui.Listener) bool { return y.Port == x.Port && y.Proto == x.Proto && y.Name == x.Name }); n >= 0 {
			if reach(x.Scope) > reach(ls[n].Scope) {
				ls[n].Scope = x.Scope
			}
			continue
		}
		ls = append(ls, x)
	}
	return ls
}

// scopeOf is who can reach a listening address: all, vpn, local, or the
// address itself.
func scopeOf(addr string) string {
	switch {
	case addr == "0.0.0.0" || addr == "::" || addr == "*":
		return "all"
	case strings.HasPrefix(addr, "127.") || addr == "::1" || addr == "localhost":
		return "local"
	case strings.HasPrefix(addr, "fd7a:115c:a1e0"):
		return "vpn"
	}
	// Tailscale's (and NetBird's) 100.64.0.0/10.
	if ip := net.ParseIP(addr); ip != nil && cgnat.Contains(ip) {
		return "vpn"
	}
	return addr
}

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// reach orders scopes: all, then one address, then vpn, then local.
func reach(scope string) int {
	switch scope {
	case "all":
		return 3
	case "vpn":
		return 1
	case "local":
		return 0
	}
	return 2
}
