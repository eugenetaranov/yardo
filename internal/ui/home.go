package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The home is a board of panels that show: Machine in two lines,
// Services & containers and MCP servers side by side, and Connection.
// Under them, the tabs: the only place the cursor goes, the one it's on
// a block of the accent colour. Under homeWide columns, Services and MCP
// servers fold to a line each.

const homeWide = 96

// homeView is the home's body, w by h.
func (m *Menu) homeView(w, h int) string {
	rs := m.rows()
	need := m.needs()
	var b []string
	add := func(s ...string) { b = append(b, s...) }

	n := 0
	var where []string
	for _, r := range rs {
		if len(need[r.id]) > 0 {
			n += len(need[r.id])
			where = append(where, r.label)
		}
	}
	status := " " + green.Render(tui.Cur.Mark.OK+" All good") + muted.Render("  ·  nothing needs you")
	if n > 0 {
		status = " " + yellow.Render("▲ "+plural(n, "thing needs you", "things need you")) + muted.Render("  ·  marked below: "+strings.Join(where, ", "))
	}
	add(status + strings.Repeat(" ", max(2, w-1-lipgloss.Width(status)-lipgloss.Width(m.sub))) + muted.Render(m.sub))
	add("")

	// fw: a panel the width of the screen; pw: one of two side by side.
	fw, pw := w-2, (w-3)/2
	wide := w >= homeWide
	if wide {
		fw = 2*pw + 2
	}
	add(box("Machine", "", m.machineShort(), fw, 2, false)...)

	conL := m.connectLines(fw - 4)
	// What's left for Services and Connection: the tabs (4 and a gap) and
	// a line under them.
	room := h - len(b) - 5
	svcL, mcpL := m.serviceLines(), m.mcpLines()
	if wide {
		cn := 0
		if len(conL) > 0 {
			cn = min(len(conL), max(1, room-6)) + 3
		}
		if bh := min(max(len(svcL), len(mcpL)), room-cn-3); bh >= 1 {
			add("")
			if m.hasMCP() {
				add(beside2(box("Services & containers", "", cut(svcL, bh), pw, bh, false), box("MCP servers", "", cut(mcpL, bh), pw, bh, false))...)
			} else {
				add(box("Services & containers", "", cut(svcL, bh), fw, bh, false)...)
			}
		}
	} else {
		if s := services(m.info.Dash); len(s) > 0 {
			add(" " + muted.Render(padTo("Services", 10)) + fit(s, fw-12))
		}
		if m.hasMCP() {
			add(" " + muted.Render(padTo("MCP", 10)) + m.mcpSummary())
		}
	}
	if len(conL) > 0 {
		n := min(len(conL), max(1, h-len(b)-5-3))
		add("")
		add(box("Connection", "from a laptop", cut(conL, n), fw, n, false)...)
	}
	add("")
	gy := len(b) + 1
	gl, xs := m.gotoLines(rs, fw-4)
	for n, x := range xs {
		m.hit(gy, 2+x[0], 2+x[1], m.selects(n))
	}
	add(box("Go to", "", gl, fw, 2, false)...)
	return strings.Join(b, "\n")
}

// box is a panel w wide: title in its top border (and right at its end),
// lines inside it, h of them; on, its border is in the accent colour.
func box(title, right string, lines []string, w, h int, on bool) []string {
	st := muted
	if on {
		st = accent
	}
	iw := max(4, w-4)
	r := ""
	if right != "" {
		r = " " + right + " "
	}
	fill := max(1, w-5-lipgloss.Width(title)-lipgloss.Width(r))
	head := st.Render("┌─ ") + bold.Render(title) + st.Render(" "+strings.Repeat("─", fill)+r+"┐")
	out := []string{head}
	for n := 0; n < h; n++ {
		l := ""
		if n < len(lines) {
			l = lines[n]
		}
		if lipgloss.Width(l) > iw {
			l = ansi.Truncate(l, iw, "…")
		}
		out = append(out, st.Render("│")+" "+l+strings.Repeat(" ", iw-lipgloss.Width(l))+" "+st.Render("│"))
	}
	return append(out, st.Render("└"+strings.Repeat("─", w-2)+"┘"))
}

// beside2 puts two boxes of the same height side by side.
func beside2(a, b []string) []string {
	out := make([]string, len(a))
	for n := range a {
		out[n] = a[n] + "  " + b[n]
	}
	return out
}

// cut is lines, at most n of them: the last says how many more.
func cut(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return append(lines[:n-1:n-1], muted.Render(fmt.Sprintf("+ %d more", len(lines)-n+1)))
}

// machineLines are the Machine panel's: CPU (its average while yardo is
// open, and a sparkline), load against the cores, memory, disk, uptime.
func (m *Menu) machineLines() []string {
	x := machine.m
	if !x.Read {
		return []string{muted.Render("Reading the machine…")}
	}
	label := func(s string) string { return muted.Render(padTo(s, 6)) }
	cpu := muted.Render("…")
	if len(x.CPU) > 0 {
		avg := 0.0
		for _, v := range x.CPU {
			avg += v
		}
		avg /= float64(len(x.CPU))
		cpu = padTo(fmt.Sprintf("%d%%", int(math.Round(avg*100))), 5) + accent.Render(spark(x.CPU, 12)) + "  " + muted.Render(cpuSpan(len(x.CPU)))
	}
	load := fmt.Sprintf("%.2f %.2f %.2f", x.Load[0], x.Load[1], x.Load[2])
	if x.Load[0] > float64(x.Cores) {
		load = yellow.Render(load)
	}
	out := []string{
		label("CPU") + cpu,
		label("Load") + padTo(load, 16) + "  " + muted.Render("on "+plural(x.Cores, "core", "cores")),
	}
	if x.MemTotal > 0 {
		f := float64(x.MemUsed) / float64(x.MemTotal)
		out = append(out, label("Mem")+bar(f, 10)+" "+padTo(fmt.Sprintf("%d%%", int(math.Round(f*100))), 4)+"  "+muted.Render(fmt.Sprintf("%s of %s", gb(x.MemUsed), gb(x.MemTotal))))
	}
	if x.DiskTotal > 0 {
		f := float64(x.DiskUsed) / float64(x.DiskTotal)
		out = append(out, label("Disk")+bar(f, 10)+" "+padTo(fmt.Sprintf("%d%%", int(math.Round(f*100))), 4)+"  "+muted.Render("of "+gb(x.DiskTotal)))
	}
	if x.Up > 0 {
		out = append(out, label("Up")+uptime(x.Up))
	}
	return out
}

// machineShort is the Machine panel in two lines, for a narrow screen.
func (m *Menu) machineShort() []string {
	l := m.machineLines()
	if len(l) < 2 {
		return l
	}
	return []string{l[0] + muted.Render("  ·  ") + strings.TrimSpace(ansi.Strip(l[1])), strings.Join(l[2:], muted.Render("  ·  "))}
}

// cpuSpan says what n readings average over.
func cpuSpan(n int) string {
	d := time.Duration(n) * machineEvery
	switch {
	case n >= machineKeep:
		return "15-min average"
	case d < time.Minute:
		return "now"
	}
	return fmt.Sprintf("average, last %d min", int(d.Minutes()))
}

// spark is the last n of vs (0 to 1) as a sparkline.
func spark(vs []float64, n int) string {
	const ticks = "▁▂▃▄▅▆▇█"
	rs := []rune(ticks)
	vs = vs[max(0, len(vs)-n):]
	var b strings.Builder
	for _, v := range vs {
		b.WriteRune(rs[min(len(rs)-1, max(0, int(v*float64(len(rs)))))])
	}
	return b.String()
}

// bar is f (0 to 1) as n cells: green, amber past 70%, red past 90%.
func bar(f float64, n int) string {
	on := min(n, max(0, int(math.Round(f*float64(n)))))
	st := green
	switch {
	case f >= 0.9:
		st = red
	case f >= 0.7:
		st = yellow
	}
	return st.UnsetBold().Render(strings.Repeat("▰", on)) + muted.Render(strings.Repeat("▱", n-on))
}

func gb(b uint64) string {
	g := float64(b) / (1 << 30)
	if g < 10 {
		return fmt.Sprintf("%.1f GB", g)
	}
	return fmt.Sprintf("%.0f GB", g)
}

func uptime(d time.Duration) string {
	days, hours, mins := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

// needMark is the mark of something that needs you, by what it is: a
// tab carries its first one's.
func needMark(id string) (string, lipgloss.Style) {
	mk := tui.Cur.Mark
	switch {
	case id == "updates":
		return mk.Update, yellow
	case strings.HasPrefix(id, "key:") || id == "secrets" || id == "keys":
		return mk.Need, accent
	case strings.HasPrefix(id, "mcpkey:"):
		return mk.Need, accent
	case strings.HasPrefix(id, "mcp:") || id == "mcporphans":
		return "▲", yellow
	}
	return mk.Down, red
}

// serviceLines are the Services & containers panel's: each service, the
// ports it listens on and who can reach them, then the containers.
func (m *Menu) serviceLines() []string {
	d := m.info.Dash
	reach := map[string]string{"all": "anyone", "vpn": "VPN", "local": "this machine"}
	type svc struct {
		name  string
		up    bool
		ports []string
		who   string
	}
	var ss []*svc
	find := func(n string) *svc {
		for _, s := range ss {
			if s.name == n {
				return s
			}
		}
		return nil
	}
	for _, s := range d.Services {
		ss = append(ss, &svc{name: s.Name, up: s.Up})
	}
	local := 0
	for _, l := range d.Listening {
		if l.Scope == "local" {
			local++
			continue
		}
		s := find(l.Name)
		if s == nil {
			s = &svc{name: l.Name, up: true}
			if s.name == "" {
				s.name = "port " + l.Port
			}
			ss = append(ss, s)
		}
		s.ports = append(s.ports, l.Port+"/"+l.Proto)
		if s.who == "" {
			s.who = reach[l.Scope]
			if s.who == "" {
				s.who = "on " + l.Scope
			}
		}
	}
	var out []string
	for _, s := range ss {
		g := green.Render("●")
		if !s.up {
			g = red.Render(tui.Cur.Mark.Down)
		}
		port := "up"
		if !s.up {
			port = red.Render("failed")
		}
		if len(s.ports) > 0 {
			port = s.ports[0]
			if len(s.ports) > 1 {
				port += fmt.Sprintf(" +%d", len(s.ports)-1)
			}
		}
		out = append(out, g+" "+padTo(s.name, 12)+" "+padTo(port, 14)+" "+muted.Render(s.who))
	}
	if local > 0 {
		out = append(out, muted.Render("  + "+plural(local, "port", "ports")+" on this machine only"))
	}
	if d.Docker {
		out = append(out, muted.Render("─ containers ─────────────"))
		if len(d.Containers) == 0 {
			out = append(out, muted.Render("  none running"))
		}
		for _, c := range d.Containers {
			p := ""
			if len(c.Ports) > 0 {
				p = ":" + strings.Join(c.Ports, ",")
			}
			out = append(out, green.Render("●")+" "+padTo(c.Image, 14)+" "+padTo(c.Name, 14)+" "+p)
		}
	}
	if len(out) == 0 {
		out = append(out, muted.Render("Nothing running yet."))
	}
	return out
}

// mcpLines are the MCP servers panel's: the gateway, the ready servers
// two to a line, then each one that isn't, and why.
func (m *Menu) mcpLines() []string {
	gw := green.Render("● up")
	if m.info.MCPDown {
		gw = red.Render(tui.Cur.Mark.Down + " stopped")
	}
	out := []string{muted.Render("Gateway ") + gw}
	if m.info.MCPAllOff {
		out[0] += "  " + accent.Render("○ all servers off")
	} else if s := m.setting("mcp"); s != nil {
		out[0] += "  " + muted.Render(s.label(s.Value))
	}
	out = append(out, "")
	var ready, bad, off []string
	others := 0
	for _, s := range m.mcpServers() {
		switch {
		case !s.Yardo:
			others++
		case s.State == "off":
			off = append(off, muted.Render("○ "+padTo(s.Name, 16)+" off"))
		case s.State == "ok" || s.State == "unchecked":
			ready = append(ready, s.Name)
		default:
			g, st := "▲", yellow
			if s.State == "secret" || s.State == "pending" {
				g, st = "○", accent
			}
			bad = append(bad, st.Render(g)+" "+padTo(s.Name, 16)+" "+muted.Render(strings.TrimPrefix(ansi.Strip(mcpWord(s)), g+" ")))
		}
	}
	for n := 0; n < len(ready); n += 2 {
		l := green.Render("●") + " " + padTo(ready[n], 18)
		if n+1 < len(ready) {
			l += green.Render("●") + " " + ready[n+1]
		}
		out = append(out, l)
	}
	out = append(out, bad...)
	if n := len(m.info.MCPOrphans); n > 0 {
		out = append(out, yellow.Render("▲")+" "+plural(n, "orphaned container", "orphaned containers")+muted.Render(" · MCP servers › c"))
	}
	out = append(out, off...)
	if len(ready)+len(bad)+len(off) == 0 {
		out = append(out, muted.Render("None yet: add GitHub, Jira, Grafana and more."))
	}
	if others > 0 {
		out = append(out, muted.Render(fmt.Sprintf("+ %d more in ~/.mcp", others)))
	}
	return out
}

// connectLines are the How to connect panel's, w wide: what to type on
// the laptop, then where it goes or where its secret is kept.
func (m *Menu) connectLines(w int) []string {
	var out []string
	for _, r := range m.info.Connect {
		l := muted.Render(padTo(r.What, 8)) + " " + keyOn.Render(r.Use)
		if r.Note != "" {
			l += muted.Render("  ·  " + r.Note)
		}
		out = append(out, ansi.Truncate(l, w, "…"))
	}
	if n := len(m.info.Keys); n > 0 {
		var fors []string
		for _, k := range m.info.Keys {
			fors = append(fors, k.For())
		}
		out = append(out, accent.Render(tui.Cur.Mark.Need+" "+plural(n, "key", "keys")+" to add: "+strings.Join(fors, ", ")))
	}
	return out
}

// mcpSummary is the MCP servers in a line.
func (m *Menu) mcpSummary() string {
	ready, all, off := 0, 0, 0
	for _, s := range m.mcpServers() {
		switch {
		case !s.Yardo:
		case s.State == "off":
			off++
		default:
			all++
			if s.State == "ok" || s.State == "unchecked" {
				ready++
			}
		}
	}
	s := fmt.Sprintf("%d of %d ready", ready, all)
	if m.info.MCPAllOff {
		s = "all off"
	} else if off > 0 {
		s += fmt.Sprintf(" · %d off", off)
	}
	if m.info.MCPDown {
		s = red.Render("gateway stopped") + muted.Render(" · ") + s
	}
	return s
}

// gotoLines are the tabs': the tabs in a line, each with its count and
// the mark of what on it needs you, the cursor's a block of the accent
// colour; then what the one under the cursor holds; and where each tab is
// on the line (from, to).
func (m *Menu) gotoLines(nav []menuRow, w int) ([]string, [][2]int) {
	count := map[string]string{}
	count["tools"] = fmt.Sprint(len(m.filter(func(i *BoardItem) bool { return i.Picked })))
	n := 0
	for _, s := range m.info.MCP {
		if s.Yardo {
			n++
		}
	}
	count["mcp"] = fmt.Sprint(n)
	if us := len(m.filter(func(i *BoardItem) bool { return problem(i) || secret(i) || newer(i) })); us > 0 {
		count["updates"] = fmt.Sprint(us)
	}
	need := m.needs()
	var parts []string
	var xs [][2]int
	x := 0
	note := muted.Render("or just start typing to search the tools")
	for i, r := range nav {
		l := r.label
		if c := count[r.id]; c != "" {
			l += " " + c
		}
		mark := ""
		if ns := need[r.id]; len(ns) > 0 {
			g, st := needMark(ns[0].id)
			mark, l = st.Render(g), l+" "+g
		}
		if i > 0 {
			x += 3
		}
		xs = append(xs, [2]int{x, x + lipgloss.Width(l) + 2})
		x += lipgloss.Width(l) + 2
		if i == m.sel {
			parts = append(parts, rowOn.Render(" "+l+" "))
			note = muted.Render(ansi.Truncate(r.note, w, "…"))
		} else if mark != "" {
			parts = append(parts, " "+strings.TrimSuffix(l, ansi.Strip(mark))+mark+" ")
		} else {
			parts = append(parts, " "+l+" ")
		}
	}
	return []string{strings.Join(parts, "   "), note}, xs
}

// setting is the setting with key, if there is one.
func (m *Menu) setting(key string) *BoardSetting {
	for _, s := range m.settings {
		if s.Key == key {
			return s
		}
	}
	return nil
}
