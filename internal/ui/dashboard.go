package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Dashboard is what the home shows of the machine, under how it is: the
// tools installed (from the items), the containers running, the services
// up or failed, and what listens on the network.
type Dashboard struct {
	// Docker: docker answered (so no containers means none running).
	Docker     bool
	Containers []Container
	Services   []Service
	Listening  []Listener
}

// Container is a running container: its image, its name when it has one
// of its own, and the host ports it publishes.
type Container struct {
	Image, Name string
	Ports       []string
}

// Service is a service that runs (Up), or failed.
type Service struct {
	Name string
	Up   bool
}

// Listener is a port something listens on, and who can reach it: all,
// vpn, local (this machine), or one address.
type Listener struct{ Port, Proto, Name, Scope string }

// services are the dashboard's services: each with its state and the
// ports it listens on beyond this machine ("tailscale ✓ 80/tcp,
// 41641/udp"), then what else listens, by name (or port); the ports open
// to this machine only are counted.
func services(d Dashboard) []string {
	type group struct {
		name, state string
		ports       []string
	}
	var gs []*group
	find := func(name string) *group {
		for _, g := range gs {
			if g.name == name {
				return g
			}
		}
		return nil
	}
	for _, s := range d.Services {
		st := ""
		if !s.Up {
			st = red.Render("failed")
		}
		gs = append(gs, &group{name: s.Name, state: st})
	}
	local := 0
	for _, l := range d.Listening {
		if l.Scope == "local" {
			local++
			continue
		}
		p := l.Port + "/" + l.Proto
		switch l.Scope {
		case "all":
		case "vpn":
			p += " VPN only"
		default:
			p += " on " + l.Scope
		}
		name := l.Name
		if name == "" {
			name = "port"
		}
		g := find(name)
		if g == nil {
			g = &group{name: name}
			gs = append(gs, g)
		}
		g.ports = append(g.ports, p)
	}
	var out []string
	for _, g := range gs {
		in := g.ports
		if g.state != "" {
			in = append([]string{g.state}, in...)
		}
		s := g.name
		if len(in) > 0 {
			s += muted.Render(" (") + strings.Join(in, muted.Render(", ")) + muted.Render(")")
		}
		out = append(out, s)
	}
	if local > 0 {
		out = append(out, muted.Render(fmt.Sprintf("+ %d on this machine only", local)))
	}
	return out
}

// fit joins parts with " · " in at most w cells, ending with "+ N more"
// when they don't all fit.
func fit(parts []string, w int) string {
	out := ""
	for n, p := range parts {
		next := p
		if out != "" {
			next = out + muted.Render(" · ") + p
		}
		more := ""
		if n < len(parts)-1 {
			more = muted.Render(fmt.Sprintf(" + %d more", len(parts)-n-1))
		}
		if lipgloss.Width(next)+lipgloss.Width(more) > w && out != "" {
			return out + muted.Render(fmt.Sprintf(" + %d more", len(parts)-n))
		}
		out = next
	}
	if lipgloss.Width(out) > w {
		return ansi.Truncate(out, w, "…")
	}
	return out
}
