package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
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
// of its own, the MCP server it runs (from its /mcp mount), the host
// ports it publishes and the widest reach of them; Count of them the same.
type Container struct {
	Image, Name, MCP string
	Ports            []string
	Reach            string
	Count            int
}

// Service is a service that runs (Up), or failed.
type Service struct {
	Name string
	Up   bool
}

// Listener is a port something listens on, and who can reach it: all,
// vpn, local (this machine only), or one address.
type Listener struct{ Port, Proto, Name, Scope string }

// svc is a service as the dashboard shows it: whether it runs, the ports
// it listens on, and the widest reach of them.
type svc struct {
	name, scope string
	up          bool
	ports       []string
}

// svcs are d's services, the failed first, then what else listens, by
// name; what listens unnamed is "other", once for each reach.
func svcs(d Dashboard) []*svc {
	var ss []*svc
	find := func(n, scope string) *svc {
		for _, s := range ss {
			if s.name == n && (n != "other" || s.scope == scope) {
				return s
			}
		}
		return nil
	}
	for _, s := range d.Services {
		ss = append(ss, &svc{name: s.Name, up: s.Up})
	}
	for _, l := range d.Listening {
		name := cmp.Or(l.Name, "other")
		s := find(name, l.Scope)
		if s == nil {
			s = &svc{name: name, up: true}
			ss = append(ss, s)
		}
		s.ports = append(s.ports, fmt.Sprintf("%s/%s", l.Port, l.Proto))
		if rank(l.Scope) > rank(s.scope) {
			s.scope = l.Scope
		}
	}
	slices.SortStableFunc(ss, func(a, b *svc) int {
		switch {
		case a.up == b.up:
			return 0
		case a.up:
			return 1
		}
		return -1
	})
	return ss
}

// rank orders scopes: all, then one address, then vpn, then local.
func rank(scope string) int {
	switch scope {
	case "":
		return -1
	case "local":
		return 0
	case "vpn":
		return 1
	case "all":
		return 3
	}
	return 2
}

// reachWord says who can reach a scope.
func reachWord(scope string) string {
	switch scope {
	case "":
		return ""
	case "all":
		return "any network"
	case "vpn":
		return "VPN only"
	case "local":
		return "local"
	}
	return scope + " only"
}

// meantOpen are the services there to be reached: their any network
// isn't a warning.
var meantOpen = map[string]bool{"ssh": true, "tailscale": true, "wireguard": true, "openvpn": true, "netbird": true}

// reachSay is reachWord, each word the same colour wherever it is (this
// machine's only muted), and a yellow ▲ after it when name is open to any
// network and isn't meant to be.
func reachSay(name, scope string) string {
	if scope == "local" {
		return muted.Render(reachWord(scope))
	}
	if scope == "all" && !meantOpen[name] {
		return reachWord(scope) + " " + yellow.Render(tui.Cur.Mark.Warn)
	}
	return reachWord(scope)
}

// portList is ports joined in at most w cells: those that fit, then how
// many more ("22/tcp, 80/tcp +2").
func portList(ports []string, w int) string {
	all := strings.Join(ports, ", ")
	if runewidth.StringWidth(all) <= w || len(ports) < 2 {
		return all
	}
	out := ports[0]
	for n := 1; n < len(ports); n++ {
		next := fmt.Sprintf("%s, %s", out, ports[n])
		if runewidth.StringWidth(fmt.Sprintf("%s +%d", next, len(ports)-n-1)) > w {
			return fmt.Sprintf("%s +%d", out, len(ports)-n)
		}
		out = next
	}
	return out
}

// middle is s in w cells, its middle cut when it's longer: mcp-pr…aws.
func middle(s string, w int) string {
	rs := []rune(s)
	if runewidth.StringWidth(s) <= w || w < 3 || len(rs) != runewidth.StringWidth(s) {
		return padTo(s, w)
	}
	head := (w - 1) / 2
	return fmt.Sprintf("%s…%s", string(rs[:head]), string(rs[len(rs)-(w-1-head):]))
}

// mcpCounts are the MCP servers' containers in a line: aws ×4, github.
func mcpCounts(cs []Container) string {
	var names []string
	n := map[string]int{}
	for _, c := range cs {
		if c.MCP == "" {
			continue
		}
		if n[c.MCP] == 0 {
			names = append(names, c.MCP)
		}
		n[c.MCP] += max(1, c.Count)
	}
	for i, x := range names {
		if n[x] > 1 {
			names[i] = fmt.Sprintf("%s ×%d", x, n[x])
		}
	}
	return strings.Join(names, ", ")
}

// services are the dashboard's services as a line's parts: the failed
// first, then each with the ports it listens on and who can reach them
// ("ssh 22/tcp any network"), then how many containers run.
func services(d Dashboard) []string {
	var out []string
	for _, s := range svcs(d) {
		if !s.up {
			out = append(out, red.Render(s.name+" failed"))
			continue
		}
		p := s.name
		if len(s.ports) > 0 {
			p += fmt.Sprintf("%s %s", muted.Render(" "+portList(s.ports, 20)), reachSay(s.name, s.scope))
		}
		out = append(out, p)
	}
	if d.Docker {
		n := 0
		for _, c := range d.Containers {
			n += max(1, c.Count)
		}
		out = append(out, muted.Render(plural(n, "container", "containers")))
	}
	return out
}

// fit joins parts with " · " in at most w cells; when they don't all
// fit, "+ N more" says how many are left out before the last, which is
// kept (the dashboard's total).
func fit(parts []string, w int) string {
	join := func(ps []string) string { return strings.Join(ps, muted.Render(" · ")) }
	if len(parts) < 3 || lipgloss.Width(join(parts)) <= w {
		return ansi.Truncate(join(parts), w, "…")
	}
	head, last := parts[:len(parts)-1], parts[len(parts)-1]
	for n := len(head) - 1; n >= 1; n-- {
		if s := fmt.Sprintf("%s%s%s", join(head[:n]), muted.Render(fmt.Sprintf(" + %d more · ", len(head)-n)), last); lipgloss.Width(s) <= w {
			return s
		}
	}
	return ansi.Truncate(fmt.Sprintf("%s%s%s", head[0], muted.Render(fmt.Sprintf(" + %d more · ", len(head)-1)), last), w, "…")
}
