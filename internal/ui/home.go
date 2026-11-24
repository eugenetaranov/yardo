package ui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The home is a board of panels that show: Needs you (when something
// does), Machine in two lines, Services & containers and MCP servers side
// by side, and Connection, filling the screen. Its cursor is on the tabs
// along the top (the app draws them; see navTabs), or in Needs you, on a
// row that goes where it's done; what's under it is said on the home's
// first line. Under homeWide columns, Services and MCP servers fold to a
// line each, and Needs you is one to a line.

const homeWide = 96

// homeView is the home's body, w by h.
func (m *Menu) homeView(w, h int) string {
	ns := m.needList()
	var b []string
	add := func(s ...string) { b = append(b, s...) }

	// One line, so nothing under it moves: how things are, then what's
	// under the cursor in Needs you, else what's going on: a queue on
	// Tools, a check for updates.
	status := fmt.Sprintf(" %s%s", green.Render(tui.Cur.Mark.OK+" All good"), muted.Render("  ·  nothing needs you"))
	if n := len(ns); n > 0 {
		say := "  ·  below: ↓ chooses one, enter goes there"
		if m.need >= 0 && m.need < len(ns) {
			say = fmt.Sprintf("  ·  %s: %s", ns[m.need].label, ns[m.need].note)
		}
		status = fmt.Sprintf(" %s%s", yellow.Render(fmt.Sprintf("%s %s", tui.Cur.Mark.Warn, plural(n, "thing needs you", "things need you"))), muted.Render(say))
	}
	if m.need < 0 {
		if q := m.queued(); q > 0 {
			status += accent.Render(fmt.Sprintf("  ·  %s on Tools", plural(q, "change queued", "changes queued")))
		}
		if m.wait != nil {
			status += muted.Render("  ·  checking for updates…")
		}
	}
	// The OS and address at the right, when they fit.
	if sub := m.sub; sub != "" && lipgloss.Width(status)+2+lipgloss.Width(sub) < w {
		status += strings.Repeat(" ", w-1-lipgloss.Width(status)-lipgloss.Width(sub)) + muted.Render(sub)
	}
	add(ansi.Truncate(status, w-1, "…"))
	add("")

	// fw: a panel the width of the screen; pw: one of two side by side.
	fw, pw := w-2, (w-3)/2
	wide := w >= homeWide
	if wide {
		fw = 2*pw + 2
	}
	if len(ns) > 0 {
		add(m.needBox(ns, fw, h, len(b))...)
		add("")
	}
	add(box("Machine", "", m.machineShort(), fw, 2, false)...)

	conL := m.connectLines(fw - 4)
	// What's left for Services and Connection.
	room := h - len(b)
	// The services' columns fit the panel they're in: half the width beside
	// MCP servers, else all of it.
	sw := fw - 4
	if wide && m.hasMCP() {
		sw = pw - 4
	}
	svcL, mcpL := m.serviceLines(sw), m.mcpLines()
	cn := 0
	if len(conL) > 0 {
		cn = min(len(conL), max(1, room-6)) + 3
	}
	// k: the panels, each a gap and its border's 2 lines more.
	k := 1
	if m.hasMCP() {
		k = 2
	}
	switch {
	case wide:
		// Services and MCP servers side by side, as tall as they need, up
		// to the rest of the screen.
		tall := len(svcL)
		if m.hasMCP() {
			tall = max(tall, len(mcpL))
		}
		if bh := min(tall, room-cn-3); bh >= 1 {
			add("")
			if m.hasMCP() {
				add(beside2(box("Services & containers", "", cut(svcL, bh), pw, bh, false), box("MCP servers", "", cut(mcpL, bh), pw, bh, false))...)
			} else {
				add(box("Services & containers", "", cut(svcL, bh), fw, bh, false)...)
			}
		}
	case (room-cn-3*k)/k >= 3:
		// Narrow but tall: one above the other, sharing the rest, each no
		// taller than it needs.
		left := room - cn - 3*k
		bh := min(len(svcL), left/k)
		add("")
		add(box("Services & containers", "", cut(svcL, bh), fw, bh, false)...)
		if m.hasMCP() {
			bh = min(len(mcpL), left-bh)
			add("")
			add(box("MCP servers", "", cut(mcpL, bh), fw, bh, false)...)
		}
	default:
		if s := services(m.info.Dash); len(s) > 0 {
			add(fmt.Sprintf(" %s%s", muted.Render(padTo("Services", 10)), fit(s, fw-12)))
		}
		if m.hasMCP() {
			add(fmt.Sprintf(" %s%s", muted.Render(padTo("MCP", 10)), m.mcpSummary()))
		}
	}
	if len(conL) > 0 {
		if n := min(len(conL), h-len(b)-3); n >= 1 {
			add("")
			add(box("Connection", "Local settings has the rest", cut(conL, n), fw, n, false)...)
		}
	}
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
		r = fmt.Sprintf(" %s ", right)
	}
	fill := max(1, w-5-lipgloss.Width(title)-lipgloss.Width(r))
	head := st.Render("┌─ ") + bold.Render(title) + st.Render(fmt.Sprintf(" %s%s┐", strings.Repeat("─", fill), r))
	out := []string{head}
	for n := 0; n < h; n++ {
		l := ""
		if n < len(lines) {
			l = lines[n]
		}
		if lipgloss.Width(l) > iw {
			l = ansi.Truncate(l, iw, "…")
		}
		out = append(out, fmt.Sprintf("%s %s%s %s", st.Render("│"), l, strings.Repeat(" ", iw-lipgloss.Width(l)), st.Render("│")))
	}
	return append(out, st.Render(fmt.Sprintf("└%s┘", strings.Repeat("─", w-2))))
}

// beside2 puts two boxes of the same height side by side.
func beside2(a, b []string) []string {
	out := make([]string, len(a))
	for n := range a {
		out[n] = fmt.Sprintf("%s  %s", a[n], b[n])
	}
	return out
}

// cut is lines, at most n of them: the last says how many more (with
// room for one, the first says it after itself).
func cut(lines []string, n int) []string {
	switch {
	case len(lines) <= n:
		return lines
	case n <= 1:
		return []string{lines[0] + muted.Render(fmt.Sprintf("  ↓ %d more", len(lines)-1))}
	}
	return append(lines[:n-1:n-1], muted.Render(fmt.Sprintf("↓ %d more", len(lines)-n+1)))
}

// machineLines are the Machine panel's: CPU (its average while yardo is
// open, and a sparkline), load against the cores, memory, disk, uptime.
func (m *Menu) machineLines() []string {
	x := machine.m
	if !x.Read {
		return []string{muted.Render("Reading the machine…")}
	}
	label := func(s string) string { return muted.Render(padTo(s, max(6, len(s)+1))) }
	cpu := muted.Render("…")
	if len(x.CPU) > 0 {
		avg := 0.0
		for _, v := range x.CPU {
			avg += v
		}
		avg /= float64(len(x.CPU))
		cpu = fmt.Sprintf("%s%s  %s", padTo(fmt.Sprintf("%d%%", int(math.Round(avg*100))), 5), accent.Render(spark(x.CPU, 12)), muted.Render(cpuSpan(len(x.CPU))))
	}
	load := fmt.Sprintf("%.2f %.2f %.2f", x.Load[0], x.Load[1], x.Load[2])
	if x.Load[0] > float64(x.Cores) {
		load = yellow.Render(load)
	}
	out := []string{
		label("CPU") + cpu,
		fmt.Sprintf("%s%s  %s", label("Load"), padTo(load, 16), muted.Render("on "+plural(x.Cores, "core", "cores"))),
	}
	if x.MemTotal > 0 {
		f := float64(x.MemUsed) / float64(x.MemTotal)
		out = append(out, fmt.Sprintf("%s%s %s  %s", label("Mem"), bar(f, 10), padTo(fmt.Sprintf("%d%%", int(math.Round(f*100))), 4), muted.Render(fmt.Sprintf("%s of %s", gb(x.MemUsed), gb(x.MemTotal)))))
	}
	if x.DiskTotal > 0 {
		f := float64(x.DiskUsed) / float64(x.DiskTotal)
		out = append(out, fmt.Sprintf("%s%s %s  %s", label("Disk"), bar(f, 10), padTo(fmt.Sprintf("%d%%", int(math.Round(f*100))), 4), muted.Render("of "+gb(x.DiskTotal))))
	}
	if x.Up > 0 {
		out = append(out, label("Uptime")+uptime(x.Up))
	}
	return out
}

// machineShort is the Machine panel in two lines, as short as they say
// it: CPU and Load, then memory, disk and uptime; they fit 76 columns.
func (m *Menu) machineShort() []string {
	x := machine.m
	if !x.Read {
		return m.machineLines()
	}
	dot := muted.Render(" · ")
	cpu := muted.Render("CPU ") + "…"
	if len(x.CPU) > 0 {
		avg := 0.0
		for _, v := range x.CPU {
			avg += v
		}
		avg /= float64(len(x.CPU))
		cpu = fmt.Sprintf("%s%d%% %s %s", muted.Render("CPU "), int(math.Round(avg*100)), accent.Render(spark(x.CPU, 8)), muted.Render(cpuShort(len(x.CPU))))
	}
	load := fmt.Sprintf("%.2f %.2f %.2f", x.Load[0], x.Load[1], x.Load[2])
	if x.Load[0] > float64(x.Cores) {
		load = yellow.Render(load)
	}
	one := cpu + dot + muted.Render("Load ") + load + muted.Render(" on "+plural(x.Cores, "core", "cores"))
	var two []string
	if x.MemTotal > 0 {
		f := float64(x.MemUsed) / float64(x.MemTotal)
		two = append(two, fmt.Sprintf("%s%s %d%% %s", muted.Render("Mem "), bar(f, 6), int(math.Round(f*100)), muted.Render(gb(x.MemUsed)+" of "+gb(x.MemTotal))))
	}
	if x.DiskTotal > 0 {
		f := float64(x.DiskUsed) / float64(x.DiskTotal)
		two = append(two, fmt.Sprintf("%s%s %d%% %s", muted.Render("Disk "), bar(f, 6), int(math.Round(f*100)), muted.Render("of "+gb(x.DiskTotal))))
	}
	if x.Up > 0 {
		two = append(two, muted.Render("Uptime ")+uptime(x.Up))
	}
	return []string{one, strings.Join(two, dot)}
}

// cpuShort is cpuSpan in a word or two.
func cpuShort(n int) string {
	if s := cpuSpan(n); s != "now" {
		return fmt.Sprintf("%d-min avg", max(1, int((time.Duration(n)*machineEvery).Minutes())))
	}
	return "now"
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
	case strings.HasPrefix(id, "mcp:") || id == "mcporphans" || strings.HasPrefix(id, "skill:") || strings.HasPrefix(id, "held:"):
		return mk.Warn, yellow
	}
	return mk.Down, red
}

// serviceLines are the Services & containers panel's, w wide: each
// service (the failed first), the ports it listens on and who can reach
// them; then the containers, theirs, and the MCP servers' counted.
func (m *Menu) serviceLines(w int) []string {
	d := m.info.Dash
	ss := svcs(d)
	var out []string
	if len(ss) > 0 {
		nw, pw, rw := len("service"), len("port"), 0
		for _, s := range ss {
			nw = max(nw, runewidth.StringWidth(s.name))
			pw = max(pw, runewidth.StringWidth(strings.Join(s.ports, ", ")))
			rw = max(rw, runewidth.StringWidth(reachWord(s.scope)))
		}
		nw = min(nw, 14)
		// The ports take what the names and the reach leave.
		pw = max(len("port"), min(pw, w-nw-rw-6))
		out = append(out, muted.Render(fmt.Sprintf("  %s  %s  reachable from", padTo("service", nw), padTo("port", pw))))
		for _, s := range ss {
			switch {
			case !s.up:
				out = append(out, fmt.Sprintf("%s %s  %s", red.Render(tui.Cur.Mark.Down), padTo(s.name, nw), red.Render("failed to start")))
			case len(s.ports) == 0:
				out = append(out, fmt.Sprintf("%s %s  %s", green.Render("●"), padTo(s.name, nw), muted.Render("running")))
			default:
				out = append(out, fmt.Sprintf("%s %s  %s  %s", green.Render("●"), padTo(s.name, nw), padTo(portList(s.ports, pw), pw), reachSay(s.name, s.scope)))
			}
		}
	}
	if d.Docker {
		out = append(out, muted.Render("─ containers "+strings.Repeat("─", max(0, w-13))))
		if len(d.Containers) == 0 {
			out = append(out, muted.Render("  none running"))
		}
		// Each column as wide as what's in it; the image takes what the
		// others leave, its middle cut.
		var cs []Container
		var iw, nw, pw, rw int
		for _, c := range d.Containers {
			if c.MCP != "" {
				continue
			}
			cs = append(cs, c)
			iw = max(iw, runewidth.StringWidth(c.Image+times(c.Count)))
			nw = max(nw, min(16, runewidth.StringWidth(c.Name)))
			pw = max(pw, runewidth.StringWidth(colons(c.Ports)))
			rw = max(rw, runewidth.StringWidth(reachWord(c.Reach)))
		}
		rest := 2
		for _, x := range []int{nw, pw, rw} {
			if x > 0 {
				rest += x + 2
			}
		}
		iw = max(8, min(iw, w-rest))
		for _, c := range cs {
			x := times(c.Count)
			l := fmt.Sprintf("%s %s", green.Render("●"), middle(c.Image, iw-runewidth.StringWidth(x))+x)
			if nw > 0 {
				l += "  " + padTo(c.Name, nw)
			}
			if pw > 0 {
				l += "  " + padTo(colons(c.Ports), pw)
			}
			if c.Reach != "" {
				l += "  " + reachSay("", c.Reach)
			}
			out = append(out, strings.TrimRight(l, " "))
		}
		if s := mcpCounts(d.Containers); s != "" {
			out = append(out, muted.Render(ansi.Truncate("MCP servers: "+s, w, "…")))
		}
	}
	if len(out) == 0 {
		out = append(out, muted.Render("Nothing running yet."))
	}
	return out
}

// times is " ×n" for more than one.
func times(n int) string {
	if n < 2 {
		return ""
	}
	return fmt.Sprintf(" ×%d", n)
}

// colons are host ports as docker says them: :5432,8080.
func colons(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	return ":" + strings.Join(ports, ",")
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
			off = append(off, muted.Render(fmt.Sprintf("○ %s off", padTo(s.Name, 16))))
		case s.State == "ok":
			ready = append(ready, s.Name)
		default:
			g, st := "▲", yellow
			switch s.State {
			case "secret", "pending":
				g, st = "○", accent
			case "unchecked":
				g, st = "·", muted
			}
			bad = append(bad, fmt.Sprintf("%s %s %s", st.Render(g), padTo(s.Name, 16), muted.Render(strings.TrimPrefix(ansi.Strip(mcpWord(s)), g+" "))))
		}
	}
	// Two to a line, the second where the others' words are.
	for n := 0; n < len(ready); n += 2 {
		l := fmt.Sprintf("%s %s", green.Render("●"), padTo(ready[n], 17))
		if n+1 < len(ready) {
			l += fmt.Sprintf("%s %s", green.Render("●"), ready[n+1])
		}
		out = append(out, l)
	}
	out = append(out, bad...)
	if n := len(m.info.MCPOrphans); n > 0 {
		out = append(out, fmt.Sprintf("%s %s%s", yellow.Render("▲"), plural(n, "orphaned container", "orphaned containers"), muted.Render(" · MCP servers › c")))
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

// everyday are the setups the home's Connection panel says, by id, and
// what it calls them ("": their own name); Local settings has the rest.
var everyday = [][2]string{{"ssh", ""}, {"docker", ""}, {"mcp-other", "MCP"}, {"pages", ""}}

// connectLines are the Connection panel's, w wide: of Local settings'
// setups, the everyday ones, each what to type on your computer, then
// the key fact (where it goes, where its secret is kept).
func (m *Menu) connectLines(w int) []string {
	// What to type in a column as wide as the widest, the notes lined up
	// after it: they're cut first, what to type only when it's that or
	// no note at all.
	type line struct{ what, use, note string }
	var ls []line
	uw := 0
	for _, e := range everyday {
		n := slices.IndexFunc(m.setups, func(s Setup) bool { return s.ID == e[0] })
		if n < 0 {
			continue
		}
		s := m.setups[n]
		use, note := s.use()
		ls = append(ls, line{cmpOr(e[1], s.What), use, note})
		uw = max(uw, runewidth.StringWidth(use))
	}
	uw = min(uw, w-9-2-12)
	var out []string
	for _, l := range ls {
		use := l.use
		if runewidth.StringWidth(use) > uw {
			use = middle(use, uw)
		}
		s := fmt.Sprintf("%s %s", muted.Render(padTo(l.what, 8)), keyOn.Render(use))
		if l.note != "" {
			s += strings.Repeat(" ", uw-runewidth.StringWidth(use)+2) + muted.Render(l.note)
		}
		out = append(out, ansi.Truncate(s, w, "…"))
	}
	return out
}

// needCols is how many of Needs you's rows go to a line, w wide: two on
// a wide screen.
func (m *Menu) needCols(w int) int {
	if w >= homeWide && len(m.needList()) > 1 {
		return 2
	}
	return 1
}

// needBox is the Needs you panel, w wide, at body line y, on a body h
// tall: each row its mark, what to do, and the tab it's on at its right;
// the cursor's in the accent colour. A quarter of h at most (the
// cursor's among them); its rows' hits are recorded.
func (m *Menu) needBox(ns []menuRow, w, h, y int) []string {
	cols := m.needCols(m.width)
	iw := max(4, w-4)
	cw := iw
	if cols == 2 {
		cw = (iw - 3) / 2
	}
	tabs := map[string]string{}
	for _, r := range m.rows() {
		tabs[r.id] = r.label
	}
	cell := func(n int) string {
		r := ns[n]
		mk, st := needMark(r.id)
		dest := tabs[needTab(r.id)]
		lw := max(4, cw-runewidth.StringWidth(dest)-4)
		label := runewidth.Truncate(r.label, lw, "…")
		if n == m.need {
			return rowOn.Render(padTo(fmt.Sprintf("›%s %s", mk, label), cw-runewidth.StringWidth(dest)-1)) + " " + accent.Render(dest)
		}
		return fmt.Sprintf(" %s %s%s", st.Render(mk), padTo(label, cw-runewidth.StringWidth(dest)-3), muted.Render(dest))
	}
	lines := (len(ns) + cols - 1) / cols
	start, shown := 0, min(lines, max(2, h/4))
	if m.need >= 0 {
		start = clamp(m.need/cols-shown+1, 0, lines-shown)
	}
	var out []string
	for l := start; l < start+shown; l++ {
		var cs []string
		for c := 0; c < cols && l*cols+c < len(ns); c++ {
			n := l*cols + c
			x := 2 + c*(cw+3)
			m.hit(y+1+len(out), x, x+cw, func() { m.need = n })
			cs = append(cs, cell(n))
		}
		out = append(out, strings.Join(cs, "   "))
	}
	right := "↓ chooses"
	switch {
	case shown < lines:
		right = fmt.Sprintf("%d of %d", min(len(ns), (start+shown)*cols)-start*cols, len(ns))
	case m.need >= 0:
		right = "enter goes there"
	}
	return box("Needs you", right, out, w, shown, m.need >= 0)
}

// mcpSummary is the MCP servers in a line.
func (m *Menu) mcpSummary() string {
	ready, all, off, unchecked := 0, 0, 0, 0
	for _, s := range m.mcpServers() {
		switch {
		case !s.Yardo:
		case s.State == "off":
			off++
		default:
			all++
			switch s.State {
			case "ok":
				ready++
			case "unchecked":
				unchecked++
			}
		}
	}
	s := fmt.Sprintf("%d of %d ready", ready, all)
	if unchecked > 0 {
		s += fmt.Sprintf(" · %d not checked", unchecked)
	}
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

// navTabs are the tabs along the top (the home's rows), and their ids:
// each with its count, and the mark of the first thing on it that needs
// you.
func (m *Menu) navTabs() ([]tabItem, []string) {
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
	tabs, ids := []tabItem{{label: tui.Title("Home")}}, []string{"home"}
	for _, r := range m.rows() {
		t := tabItem{label: tui.Title(r.label)}
		if c := count[r.id]; c != "" {
			t.label += " " + c
		}
		if ns := need[r.id]; len(ns) > 0 {
			t.mark, t.st = needMark(ns[0].id)
		}
		tabs, ids = append(tabs, t), append(ids, r.id)
	}
	return tabs, ids
}

// tabOf is the tab along the top the screen is under.
func (m *Menu) tabOf() string {
	switch m.screen {
	case "home":
		return ""
	case "access":
		return NetTab
	case "look", "shell":
		return "settings"
	}
	s, _, _ := strings.Cut(m.screen, ":")
	return s
}

// navAt is the tab along the top the menu is on: Home, or the screen's
// tab (-1 for none).
func (m *Menu) navAt() int {
	_, ids := m.navTabs()
	return slices.Index(ids, cmpOr(m.tabOf(), "home"))
}

// nav are the tabs along the top, w wide, as the menu alone draws them,
// and the name of the one it's on.
func (m *Menu) nav(w int) ([]string, string) {
	tabs, _ := m.navTabs()
	at := m.navAt()
	l, _ := navLines(tabs, at, max(50, w))
	if at <= 0 {
		return l, ""
	}
	return l, m.rows()[at-1].label
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
