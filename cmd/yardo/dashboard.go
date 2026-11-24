package main

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/ui"
)

// The home's dashboard: the containers running, the services up (or
// failed), and what listens on the network. Each comes from one quick
// command; a command missing or slow leaves its part out.

// dashboard gathers the home's dashboard; user's MCP servers' containers
// are said by server.
func dashboard(ctx context.Context, user string) ui.Dashboard {
	var d ui.Dashboard
	if ids, ok := quick(ctx, "docker", "ps", "-q"); ok {
		d.Docker = true
		if strings.TrimSpace(ids) != "" {
			out, ok := quick(ctx, "docker", append([]string{"inspect", "--format", inspectFormat}, strings.Fields(ids)...)...)
			d.Docker = ok
			d.Containers = parseContainers(out, filepath.Join(homeOf(user), ".mcp"))
		}
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

// inspectFormat is what docker inspect says of a container, a line each:
// its name, the image it was run as, the image's ID, its /mcp mount's
// source, and the host address, host port and port of each it publishes.
const inspectFormat = "{{.Name}}\t{{.Config.Image}}\t{{.Image}}\t" +
	`{{range .Mounts}}{{if eq .Destination "/mcp"}}{{.Source}}{{end}}{{end}}` + "\t" +
	`{{range $p, $b := .NetworkSettings.Ports}}{{range $b}}{{.HostIp}}|{{.HostPort}}|{{$p}} {{end}}{{end}}`

// parseContainers reads docker inspect's lines: each container, called by
// the image it was run as (by its ID only when that's all there is), with
// its name when it has one of its own (not one docker made up:
// clever_franklin), the MCP server in dir it runs, and the host ports it
// publishes, to whom; the same ones once, counted.
func parseContainers(out, dir string) []ui.Container {
	var cs []ui.Container
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, "\t")
		if len(f) < 5 || f[0] == "" {
			continue
		}
		name, image := strings.TrimPrefix(f[0], "/"), shortImage(f[1])
		c := ui.Container{Image: image, Count: 1}
		if id := strings.TrimPrefix(f[2], "sha256:"); image == "" || imageID.MatchString(f[1]) {
			c.Image = "image " + id[:min(7, len(id))]
		}
		if c.MCP, _ = mcpDirName(f[3], dir); c.MCP == "" && name != image && !strings.HasPrefix(image, name+":") &&
			(!madeUp.MatchString(name) || strings.HasPrefix(c.Image, "image ")) {
			c.Name = name
		}
		// 0.0.0.0|5432|5432/tcp ::|5432|5432/tcp
		for _, p := range strings.Fields(f[4]) {
			b := strings.Split(p, "|")
			if len(b) != 3 || b[1] == "" {
				continue
			}
			if !slices.Contains(c.Ports, b[1]) {
				c.Ports = append(c.Ports, b[1])
			}
			s := "all"
			if b[0] != "" {
				s = scopeOf(b[0])
			}
			if c.Reach == "" || reach(s) > reach(c.Reach) {
				c.Reach = s
			}
		}
		if n := slices.IndexFunc(cs, func(x ui.Container) bool {
			x.Count = 1
			return fmt.Sprint(x) == fmt.Sprint(c)
		}); n >= 0 {
			cs[n].Count++
			continue
		}
		cs = append(cs, c)
	}
	return cs
}

// madeUp is a name docker made up for a container: adjective_surname, a
// digit after it when the first was taken.
var madeUp = regexp.MustCompile(`^[a-z]+_[a-z]+[0-9]?$`)

// imageID is an image named by its ID: run by it, or its tag gone.
var imageID = regexp.MustCompile(`^(sha256:)?[0-9a-f]{12,64}$`)

// shortImage is an image without its registry, path, :latest or digest:
// postgres:16 for docker.io/library/postgres:16, mcpjungle for
// ghcr.io/mcpjungle/mcpjungle@sha256:….
func shortImage(image string) string {
	image, _, _ = strings.Cut(image, "@")
	if i := strings.LastIndex(image, "/"); i >= 0 {
		image = image[i+1:]
	}
	return strings.TrimSuffix(image, ":latest")
}

// services are the services the dashboard shows, by unit (a template's
// prefix ends in @), and their names.
var services = []struct{ unit, name string }{
	{"ssh.service", "ssh"}, {"sshd.service", "ssh"}, {"docker.service", "docker"},
	{"tailscaled.service", "tailscale"}, {"netbird.service", "netbird"},
	{"openvpn-server@", "openvpn"}, {"openvpn@", "openvpn"}, {"openvpn-client@", "openvpn"},
	{"wg-quick@", "wireguard"}, {"nfs-server.service", "nfs"}, {"smbd.service", "samba"},
	{"fail2ban.service", "fail2ban"}, {"k3s.service", "k3s"}, {"ollama.service", "ollama"},
	{"yardo-pages@", "pages"}, {"caddy.service", "caddy"},
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
	"dockerd": "docker", "k3s-server": "k3s", "ollama": "ollama", "mcpjungle": "mcp",
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
		case name == "docker-proxy":
			continue // a container's published port: on its row
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
