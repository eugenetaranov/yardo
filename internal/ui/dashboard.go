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

// Container is a running container and the host ports it publishes.
type Container struct {
	Name  string
	Ports []string
}

// Service is a service that runs (Up), or failed.
type Service struct {
	Name string
	Up   bool
}

// Listener is a port something listens on, and who can reach it: all,
// vpn, local (this machine), or one address.
type Listener struct{ Port, Proto, Name, Scope string }

// dashLines are the dashboard's lines, w wide: a label, then as much as
// fits, and how many more when not all of it does.
func (m *Menu) dashLines(w int) []string {
	d := m.info.Dash
	var out []string
	line := func(label string, parts []string) {
		if len(parts) == 0 {
			return
		}
		out = append(out, "  "+muted.Render(padTo(label, 12))+fit(parts, w-16))
	}
	var tools []string
	for _, i := range m.items {
		if i.Picked && i.Tab != ExtraTab {
			tools = append(tools, short(i.Label))
		}
	}
	for _, i := range m.items {
		if i.Picked && i.Tab == ExtraTab {
			tools = append(tools, short(i.Label))
		}
	}
	if len(tools) > 0 {
		tools[0] = fmt.Sprintf("%d: %s", len(tools), tools[0])
	}
	line("Installed", tools)
	if d.Docker {
		var cs []string
		for _, c := range d.Containers {
			s := c.Name
			if len(c.Ports) > 0 {
				s += " :" + strings.Join(c.Ports, ",")
			}
			cs = append(cs, s)
		}
		if len(cs) == 0 {
			cs = []string{"0"}
		} else {
			cs[0] = fmt.Sprintf("%d: %s", len(cs), cs[0])
		}
		line("Containers", cs)
	}
	line("Services", services(d))
	return out
}

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
		st := green.Render("✓")
		if !s.Up {
			st = red.Render("✗ failed")
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
			p += muted.Render(" (VPN)")
		default:
			p += muted.Render(" (" + l.Scope + ")")
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
		s := g.name
		if g.state != "" {
			s += " " + g.state
		}
		if len(g.ports) > 0 {
			s += " " + strings.Join(g.ports, ", ")
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
