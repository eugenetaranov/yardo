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
		cs := []string{"none running"}
		if len(d.Containers) > 0 {
			cs = nil
			for _, c := range d.Containers {
				s := c.Name
				if len(c.Ports) > 0 {
					s += " :" + strings.Join(c.Ports, ",")
				}
				cs = append(cs, s)
			}
		}
		line("Containers", cs)
	}
	var ss []string
	for _, s := range d.Services {
		if s.Up {
			ss = append(ss, s.Name+" "+green.Render("✓"))
		} else {
			ss = append(ss, s.Name+" "+red.Render("✗ failed"))
		}
	}
	line("Services", ss)
	var ls []string
	local := 0
	for _, l := range d.Listening {
		if l.Scope == "local" {
			local++
			continue
		}
		s := l.Port
		if l.Proto == "udp" {
			s += "/udp"
		}
		if l.Name != "" {
			s += " " + l.Name
		}
		switch l.Scope {
		case "all":
		case "vpn":
			s += muted.Render(" (VPN)")
		default:
			s += muted.Render(" (" + l.Scope + ")")
		}
		ls = append(ls, s)
	}
	if local > 0 {
		ls = append(ls, muted.Render(fmt.Sprintf("+ %d on this machine only", local)))
	}
	line("Listening", ls)
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
