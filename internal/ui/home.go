package ui

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The home is a board of panels: Machine and Needs you on top, Services
// & containers and MCP servers under them, and a Go to strip. The cursor
// walks Needs you's rows, then Go to's; tab jumps between the two. Under
// homeWide columns, the panels stack and the bottom two fold to a line.

const homeWide = 96

// homeView is the home, w by h.
func (m *Menu) homeView(w, h int) string {
	todo := m.todo()
	rs := m.rows()
	var b []string
	add := func(s ...string) { b = append(b, s...) }

	left := " " + accentBd.Render(tui.Cur.Brand) + muted.Render("  ·  "+m.info.Host+"  ·  "+m.info.User)
	add(left + strings.Repeat(" ", max(2, w-1-lipgloss.Width(left)-lipgloss.Width(m.sub))) + muted.Render(m.sub))
	if n := len(todo); n > 0 {
		add(" " + yellow.Render("▲ "+plural(n, "thing needs you", "things need you")) + muted.Render("  ·  everything else is working"))
	} else {
		add(" " + green.Render(tui.Cur.Mark.OK+" All good") + muted.Render("  ·  nothing needs you"))
	}
	add("")

	needOn := m.sel < len(todo)
	// nw: the Needs you panel's inside.
	nw := w - 6
	if w >= homeWide {
		nw = (w-3)/2 - 4
	}
	machineL, needL := m.machineLines(), m.needLines(todo, nw)
	svcL, mcpL := m.serviceLines(), m.mcpLines()
	// What's left over: the header, the boxes' borders and gaps, Go to and
	// the keys.
	if w >= homeWide {
		pw := (w - 3) / 2
		th := max(len(machineL), len(needL))
		top := beside2(box("Machine", "", machineL, pw, th, false), box("Needs you", fmt.Sprint(len(todo)), needL, pw, th, needOn))
		add(top...)
		room := h - len(b) - 1 - 2 - 1 - 4 - 2
		if bh := min(max(len(svcL), len(mcpL)), room); bh >= 1 {
			add("")
			if m.hasMCP() {
				add(beside2(box("Services & containers", "", cut(svcL, bh), pw, bh, false), box("MCP servers", "", cut(mcpL, bh), pw, bh, false))...)
			} else {
				add(box("Services & containers", "", cut(svcL, bh), 2*pw+2, bh, false)...)
			}
		}
	} else {
		pw := w - 2
		add(box("Needs you", fmt.Sprint(len(todo)), needL, pw, len(needL), needOn)...)
		add(box("Machine", "", m.machineShort(), pw, 2, false)...)
		if s := services(m.info.Dash); len(s) > 0 {
			add(" " + muted.Render(padTo("Services", 10)) + fit(s, pw-12))
		}
		if m.hasMCP() {
			add(" " + muted.Render(padTo("MCP", 10)) + m.mcpSummary())
		}
	}
	add("")
	gw := w - 2
	if w >= homeWide {
		gw = (w-3)/2*2 + 2
	}
	add(box("Go to", "", m.gotoLines(rs[len(todo):], gw-4), gw, 2, !needOn)...)

	for len(b) < h-2 {
		b = append(b, "")
	}
	if len(b) > h-2 {
		b = b[:h-2]
	}
	if m.toast != "" {
		b = append(b, "  "+green.Render(m.toast))
	} else {
		b = append(b, "")
	}
	k := func(key, what string) string { return keyOn.Render(key) + " " + what }
	b = append(b, " "+strings.Join([]string{k("↑↓←→", "choose"), k("tab", "next"), k("enter", "open"), k("type", "search tools"), k("q", "quit")}, "   "))
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

// needLines are the Needs you panel's: a row for each thing to do (the
// cursor on one), then how the tools are and when updates were checked.
func (m *Menu) needLines(todo []menuRow, w int) []string {
	var out []string
	if len(todo) == 0 {
		out = append(out, green.Render("Nothing needs you."))
	}
	col := 8
	for _, r := range todo {
		col = max(col, len(r.label))
	}
	col = min(col, 32)
	for n, r := range todo {
		g, st := needMark(r.id)
		note, _, _ := strings.Cut(r.note, " · ")
		if n == m.sel {
			out = append(out, rowOn.Render(" "+g+" "+padTo(r.label, col)+" ")) // its note is under the rows
		} else {
			out = append(out, " "+st.Render(g)+" "+padTo(r.label, col)+"  "+muted.Render(note))
		}
	}
	// The row under the cursor, in full, in two lines kept for it.
	var more []string
	if m.sel < len(todo) {
		more = wrap(todo[m.sel].note, max(10, w-3))
		if len(more) > 2 {
			more = []string{more[0], ansi.Truncate(strings.Join(more[1:], " "), max(10, w-3), "…")}
		}
	}
	for n := 0; n < 3; n++ {
		l := ""
		if n < len(more) {
			l = "   " + muted.Render(more[n])
		}
		out = append(out, l)
	}
	picked := len(m.filter(func(i *BoardItem) bool { return i.Picked }))
	ok := len(m.filter(func(i *BoardItem) bool { return i.Picked && i.State == "ok" }))
	s := green.Render(tui.Cur.Mark.OK) + " " + fmt.Sprintf("%d of %d tools working", ok, picked)
	if n := len(m.filter(func(i *BoardItem) bool { return i.Picked && i.State == "unchecked" })); n > 0 {
		s += muted.Render(fmt.Sprintf(" · %d not checked", n))
	}
	out = append(out, s)
	// A machine set up from a team profile says how much is the team's.
	team, teams := 0, map[string]bool{}
	for _, i := range m.items {
		if i.Picked && i.Team != "" {
			team++
			teams[i.Team] = true
		}
	}
	if team > 0 {
		out = append(out, "  "+muted.Render(fmt.Sprintf("%d from the team profile %s", team, strings.Join(slices.Sorted(maps.Keys(teams)), ", "))))
	}
	var sub string
	switch us := len(m.filter(newer)); {
	case m.wait != nil:
		sub = "checking for updates…"
	case us > 0:
		sub = plural(us, "update", "updates")
	case m.checked:
		sub = "nothing newer"
	default:
		sub = "updates not checked"
	}
	if m.info.Checked != "" && m.wait == nil {
		sub += " · checked " + m.info.Checked
	}
	return append(out, "  "+muted.Render(sub))
}

// needMark is a Needs you row's mark, by what it does.
func needMark(id string) (string, lipgloss.Style) {
	mk := tui.Cur.Mark
	switch {
	case id == "updates":
		return mk.Update, yellow
	case strings.HasPrefix(id, "key:") || id == "secrets":
		return mk.Need, accent
	case strings.HasPrefix(id, "mcpkey:"):
		return mk.Need, accent
	case strings.HasPrefix(id, "mcp:"):
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
	if s := m.setting("mcp"); s != nil {
		out[0] += "  " + muted.Render(s.label(s.Value))
	}
	out = append(out, "")
	var ready []string
	var bad []string
	others := 0
	for _, s := range m.mcpServers() {
		switch {
		case !s.Yardo:
			others++
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
	if len(ready)+len(bad) == 0 {
		out = append(out, muted.Render("None yet: add GitHub, Jira, Grafana and more."))
	}
	if others > 0 {
		out = append(out, muted.Render(fmt.Sprintf("+ %d more in ~/.mcp", others)))
	}
	return out
}

// mcpSummary is the MCP servers in a line.
func (m *Menu) mcpSummary() string {
	ready, all := 0, 0
	for _, s := range m.mcpServers() {
		if s.Yardo {
			all++
			if s.State == "ok" || s.State == "unchecked" {
				ready++
			}
		}
	}
	s := fmt.Sprintf("%d of %d ready", ready, all)
	if m.info.MCPDown {
		s = red.Render("gateway stopped") + muted.Render(" · ") + s
	}
	return s
}

// gotoLines are Go to's: its entries in a line (the cursor on one), then
// what the one under the cursor holds.
func (m *Menu) gotoLines(nav []menuRow, w int) []string {
	count := map[string]string{}
	count["tools"] = fmt.Sprint(len(m.filter(func(i *BoardItem) bool { return i.Picked })))
	n := 0
	for _, s := range m.info.MCP {
		if s.Yardo {
			n++
		}
	}
	count["mcp"] = fmt.Sprint(n)
	var parts []string
	note := muted.Render("or just start typing to search the tools")
	at := m.sel - len(m.todo())
	for i, r := range nav {
		l := r.label
		if c := count[r.id]; c != "" {
			l += " " + c
		}
		if i == at {
			parts = append(parts, rowOn.Render(" "+l+" "))
			note = muted.Render(ansi.Truncate(r.note, w, "…"))
		} else {
			parts = append(parts, " "+l+" ")
		}
	}
	return []string{strings.Join(parts, "   "), note}
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
