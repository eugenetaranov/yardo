package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// testApp is the app on testMenu's items and testBoard's tabs.
func testApp() *App {
	m, b := testMenu(true), testBoard(false)
	d := AppData{Info: m.info, Sub: m.sub, Items: b.items, Settings: m.settings, Checked: true, Tabs: rawTabs(b)}
	a := NewApp(d, nil)
	a.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	return a
}

// rawTabs are b's tabs as the catalog has them, without Installed.
func rawTabs(b *Board) []Tab {
	var out []Tab
	for _, t := range b.tabs {
		if t.ID != InstalledTab {
			out = append(out, t)
		}
	}
	return out
}

// ak sends keys to the app, as press2 names them.
func ak(a *App, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "shift+tab":
			msg = tea.KeyMsg{Type: tea.KeyShiftTab}
		case "pgdown":
			msg = tea.KeyMsg{Type: tea.KeyPgDown}
		case "ctrl+c":
			msg = tea.KeyMsg{Type: tea.KeyCtrlC}
		default:
			msg = keyMsg(k)
		}
		_, cmd = a.Update(msg)
	}
	return cmd
}

// isQuit: cmd ends the program.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// onHome moves the home's cursor onto row id.
func onHome(a *App, id string) {
	a.Menu.need = -1
	for n, r := range a.Menu.rows() {
		if r.id == id {
			a.Menu.sel = n
		}
	}
}

// onNeedA moves the home's cursor onto Needs you's row id.
func onNeedA(a *App, id string) {
	a.Menu.need = slices.IndexFunc(a.Menu.needList(), func(r menuRow) bool { return r.id == id })
}

// Enter on Tools, then q: one program, the home back, no result.
func TestAppHop(t *testing.T) {
	a := testApp()
	onHome(a, "tools")
	if isQuit(ak(a, "enter")) || !a.onBoard || !strings.Contains(ansi.Strip(a.View()), "[Tools 3 ✗]") {
		t.Fatalf("enter on Tools:\n%s", a.View())
	}
	if cmd := ak(a, "q"); isQuit(cmd) || a.onBoard || a.result.Board != nil || a.result.Quit {
		t.Fatalf("q on Tools: left %v, result %+v", isQuit(cmd), a.result)
	}
	if v := a.View(); !strings.Contains(v, "─ Machine ") || !strings.Contains(v, tabOn.Render("[Home]")) {
		t.Errorf("not the home:\n%s", v)
	}
}

// The tabs along the top, on every screen: 1 to 9 jump to them (not
// while typing, or in a question), a click opens one; the body starts
// under them, its clicks too.
func TestAppNav(t *testing.T) {
	a := testApp()
	lines := func() []string { return strings.Split(ansi.Strip(a.View()), "\n") }
	if l := lines(); !strings.Contains(l[1], "Tools") || !strings.Contains(l[1], "Network") || !strings.Contains(l[1], "Settings") || !strings.HasPrefix(strings.TrimSpace(l[2]), "───") {
		t.Fatalf("no tabs along the top:\n%s", strings.Join(l, "\n"))
	}
	n := func(id string) string {
		_, ids := a.Menu.navTabs()
		return fmt.Sprint(slices.Index(ids, id) + 1)
	}
	ak(a, n("settings"))
	if a.Menu.screen != "settings" || !strings.Contains(lines()[1], "[Settings]") {
		t.Fatalf("%s: on %s:\n%s", n("settings"), a.Menu.screen, a.View())
	}
	ak(a, n(NetTab))
	a.Conn.focus("access")
	ak(a, "enter") // Network access: Home › Network › Network access
	if l := lines(); !strings.Contains(l[0], "Network › Network access") || strings.Contains(l[0], "Home") || !strings.Contains(l[1], "[Network]") {
		t.Errorf("a screen under a tab says where:\n%s", strings.Join(l[:2], "\n"))
	}
	ak(a, n("tools"))
	if !a.onBoard {
		t.Fatalf("%s: not on Tools:\n%s", n("tools"), a.View())
	}
	// Typing a search: the digit is the search's.
	ak(a, "/", n(NetTab))
	if !a.onBoard || a.Board.search == nil || *a.Board.search != n(NetTab) {
		t.Fatalf("typing: %v", a.onBoard)
	}
	ak(a, "esc", n(NetTab))
	if !a.onConn || a.Conn.tab != NetTab {
		t.Fatalf("%s: not on Network:\n%s", n(NetTab), a.View())
	}
	// A click on a tab along the top opens it.
	a.View()
	s := a.nav["tools"]
	a.Update(tea.MouseMsg{X: s.x0 + 1, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !a.onBoard || a.onConn {
		t.Fatalf("click on Tools: board %v", a.onBoard)
	}
	// In a question, digits are its own.
	ak(a, "q")
	onHome(a, "tools")
	ak(a, "enter", "right", "right", "right", "space", "q", "q")
	if a.dialog == nil {
		t.Fatalf("no question:\n%s", a.View())
	}
	ak(a, n("settings"))
	if a.dialog == nil || a.Menu.screen != "home" {
		t.Errorf("a digit left the question")
	}
}

// The tabs scroll to keep the selected one in view, saying some are
// hidden; only those shown are clicked. The selected one is in brackets,
// which show without colour.
func TestTabStrip(t *testing.T) {
	var tabs []tabItem
	for _, s := range []string{"Updates 3", "Tools 12", "MCP servers 5", "Skills", "Connection", "Settings", "Last run"} {
		tabs = append(tabs, tabItem{label: s})
	}
	l, spots := tabStrip(tabs, 0, 200)
	if strings.Contains(l, "‹") || strings.Contains(l, "›") || len(spots) != len(tabs) || !strings.Contains(ansi.Strip(l), "[Updates 3]") {
		t.Errorf("wide: %q %v", l, spots)
	}
	for sel := range tabs {
		l, spots := tabStrip(tabs, sel, 50)
		p := ansi.Strip(l)
		if ansi.StringWidth(l) > 50 || !strings.Contains(p, "["+tabs[sel].label+"]") {
			t.Errorf("%d: %q", sel, p)
		}
		if spots[0].n > 0 != strings.Contains(p, "‹") || spots[len(spots)-1].n < len(tabs)-1 != strings.HasSuffix(p, "›") {
			t.Errorf("%d: arrows: %q %v", sel, p, spots)
		}
		for _, s := range spots {
			if got := strings.TrimSpace(strings.Trim(string([]rune(p)[s.x0:s.x1]), "[]")); got != tabs[s.n].label {
				t.Errorf("%d: spot %v is %q", sel, s, got)
			}
		}
	}
}

// The header cuts the host and user, not the path, and says no empty
// one; a toast is coloured by its mark, in the look's marks.
func TestAppHeaderToast(t *testing.T) {
	h := ansi.Strip(header(HomeInfo{Host: "a-very-long-host-name.example.internal", User: "someone"}, "Settings › Network access", 60))
	if !strings.HasSuffix(h, "Settings › Network access ") || !strings.Contains(h, "…") || ansi.StringWidth(h) > 60 {
		t.Errorf("cut: %q", h)
	}
	if h := ansi.Strip(header(HomeInfo{}, "Home", 60)); strings.Contains(h, "·") {
		t.Errorf("empty host and user: %q", h)
	}
	t.Cleanup(func() { SetLook("classic", "#D97757") })
	SetLook("bridge", "")
	if got := ansi.Strip(toastLine("✗ it failed", 60)); got != " ✕ it failed" {
		t.Errorf("bridge's ✗: %q", got)
	}
	if got := toastLine("Saving…", 60); got != " Saving…" {
		t.Errorf("plain: %q", got)
	}
}

// esc always leaves a question; q picks its Q (quit) when it has one,
// which its footer says.
func TestDialogKeys(t *testing.T) {
	picked := ""
	d := &Dialog{Title: "Quit?", YN: true, Q: "yes", Opts: [][2]string{{"Quit without it (y)", "yes"}, {"Stay (n)", "no"}}, On: func(v string) tea.Cmd { picked = v; return nil }}
	keys := dialogKeys(d)
	if !slices.Contains(keys, Key{Key: "q", What: "quit without it", Keep: true}) || !slices.Contains(keys, Key{Key: "esc", What: "cancel", Keep: true}) {
		t.Errorf("keys: %v", keys)
	}
	p := d
	dialogKey(&p, "esc")
	if p != nil || picked != "" {
		t.Errorf("esc picked %q", picked)
	}
	p = d
	dialogKey(&p, "q")
	if picked != "yes" {
		t.Errorf("q picked %q", picked)
	}
	// The frame is closed, the options' labels at one place.
	ls := dialogLines(&Dialog{Title: "T", Lines: []string{"a line"}, Opts: [][2]string{{"One", "1"}, {"Two", "2"}}}, 60, 0, nil)
	for _, l := range ls {
		if ansi.StringWidth(l) != 59 {
			t.Errorf("width %d: %q", ansi.StringWidth(l), ansi.Strip(l))
		}
	}
	col := func(l, s string) int {
		p := ansi.Strip(l)
		return ansi.StringWidth(p[:strings.Index(p, s)])
	}
	if a, b := col(ls[2], "One"), col(ls[3], "Two"); a != b || !strings.HasSuffix(ansi.Strip(ls[1]), "│") {
		t.Errorf("labels at %d and %d:\n%s", a, b, ansi.Strip(strings.Join(ls, "\n")))
	}
}

// The board's queue stays across a hop home and back; the home's Tools
// row says so; q on the home asks once, then quits.
func TestAppQueueKept(t *testing.T) {
	a := testApp()
	onHome(a, "tools")
	ak(a, "enter", "right", "right", "right", "space") // System: WireGuard
	if a.Board.queued() != 1 {
		t.Fatalf("not queued:\n%s", a.View())
	}
	tab := a.Board.tabID
	ak(a, "q")
	onHome(a, "tools")
	if v := ansi.Strip(a.View()); !strings.Contains(v, "1 change queued") {
		t.Errorf("Tools' note:\n%s", v)
	}
	ak(a, "enter")
	if a.Board.queued() != 1 || a.Board.tabID != tab {
		t.Errorf("back on Tools: queued %d, tab %s (was %s)", a.Board.queued(), a.Board.tabID, tab)
	}
	ak(a, "q")
	if cmd := ak(a, "q"); isQuit(cmd) || a.dialog == nil || !strings.Contains(a.View(), "1 change isn't applied") {
		t.Fatalf("q with a queue:\n%s", a.View())
	}
	if cmd := ak(a, "q"); !isQuit(cmd) || !a.result.Quit {
		t.Errorf("q again: %+v", a.result)
	}
}

// Reload keeps the tab, the cursors and the queue but what the run did.
func TestAppReload(t *testing.T) {
	a := testApp()
	onHome(a, "tools")
	ak(a, "enter", "right", "right", "right", "space") // System: WireGuard
	tab := a.Board.tabID
	reload := func(installed bool) {
		m, b := testMenu(true), testBoard(false)
		b.items[3].Picked = installed
		a.Reload(AppData{Info: m.info, Items: b.items, Tabs: rawTabs(b), Checked: true})
	}
	reload(false) // a run of something else
	if a.Board.tabID != tab || a.Board.items[3].Pending != "add" {
		t.Errorf("reload: tab %s, wg %q", a.Board.tabID, a.Board.items[3].Pending)
	}
	reload(true) // the run installed it
	if a.Board.items[3].Pending != "" {
		t.Errorf("reload after installing: wg %q", a.Board.items[3].Pending)
	}
}

// Typing on the home is the board's filter, live.
func TestAppTypeOnHome(t *testing.T) {
	a := testApp()
	ak(a, "n")
	if !a.onBoard || a.Board.search == nil || *a.Board.search != "n" {
		t.Fatalf("n: %v", a.Board.search)
	}
	ak(a, "e")
	if *a.Board.search != "ne" || !strings.Contains(a.View(), "Search  ne") {
		t.Errorf("ne:\n%s", a.View())
	}
}

// The header's path and the footer, which keeps its keys while the
// board says something.
func TestAppChrome(t *testing.T) {
	a := testApp()
	onHome(a, "tools")
	ak(a, "enter", "right", "right") // DevOps
	if v := ansi.Strip(a.View()); strings.Contains(strings.Split(v, "\n")[0], "Tools") || !strings.Contains(v, "[DevOps") || !strings.Contains(v, "[Tools") {
		t.Errorf("header:\n%s", v)
	}
	ak(a, "a")
	v := a.View()
	ls := strings.Split(v, "\n")
	if !strings.Contains(ls[len(ls)-2], "Nothing queued") || !strings.Contains(ls[len(ls)-1], "? keys") || !strings.Contains(ls[len(ls)-1], "q home") {
		t.Errorf("flash took the footer:\n%s", v)
	}
	if strings.Contains(v, " >[") {
		t.Errorf("a > pointer:\n%s", v)
	}
}

// One dialog: d on a tool and on an MCP server open the same frame,
// with y and n in the footer.
func TestAppDialog(t *testing.T) {
	a := testApp()
	a.Board.items[0].Removable = true
	onHome(a, "tools")
	ak(a, "enter", "right", "right", "d") // DevOps: Docker
	v := a.View()
	if !strings.Contains(v, "┌ Uninstall 1 tool?") || !strings.Contains(v, "y yes") || !strings.Contains(v, "n no") {
		t.Fatalf("uninstall:\n%s", v)
	}
	ak(a, "q")
	if a.Board.dialog != nil {
		t.Error("q didn't close it")
	}
	m := mcpMenu()
	onRow(m, "mcp")
	mk(m, "enter", "d")
	v2 := m.View()
	if !strings.Contains(v2, "┌ Delete github?") || !strings.Contains(v2, "y yes") || !strings.Contains(v2, "n no") {
		t.Errorf("delete:\n%s", v2)
	}
}

// G goes to the last row, scrolled into view; PgDn a screenful.
func TestAppPaging(t *testing.T) {
	a := testApp()
	for n := range 40 {
		a.Board.items = append(a.Board.items, &BoardItem{ID: fmt.Sprintf("x%s%s", string(rune('a'+n%26)), string(rune('a'+n/26))), Label: fmt.Sprintf("Tool %s%s", string(rune('A'+n%26)), string(rune('A'+n/26))), Tab: "iac", Picked: true, State: "ok"})
	}
	onHome(a, "tools")
	ak(a, "enter") // Installed
	ak(a, "G")
	rs := a.Board.rows()
	c, _ := a.Board.selected(rs)
	if c != len(rs)-1 || !strings.Contains(a.View(), "› [x] ✓ Tool NB") {
		t.Errorf("G: on %d of %d\n%s", c, len(rs), a.View())
	}
	ak(a, "g")
	if c, _ := a.Board.selected(a.Board.rows()); c != 0 {
		t.Errorf("g: on %d", c)
	}
	ak(a, "pgdown")
	if c, _ := a.Board.selected(a.Board.rows()); c < 10 {
		t.Errorf("pgdown: on %d", c)
	}
}

// ? lists the screen's keys, then everyone's; q goes back, same row.
func TestAppKeys(t *testing.T) {
	a := testApp()
	onHome(a, "tools")
	ak(a, "enter", "down")
	c, _ := a.Board.selected(a.Board.rows())
	ak(a, "?")
	v := a.View()
	for _, want := range []string{"This screen", "space", "apply the queue", "uninstall the row", "search", "←→ h l", "Every screen", "PgUp PgDn", "Tools › Keys"} {
		if !strings.Contains(v, want) {
			t.Errorf("keys lack %q:\n%s", want, v)
		}
	}
	ak(a, "q")
	if c2, _ := a.Board.selected(a.Board.rows()); a.keys || !a.onBoard || c2 != c {
		t.Errorf("q: keys %v, board %v, row %d (was %d)", a.keys, a.onBoard, c2, c)
	}
}

// The mouse: a click on a tab opens it; on a row selects it, twice acts;
// on a footer key presses it; the wheel on the home moves along the tabs.
func TestAppMouse(t *testing.T) {
	a := testApp()
	at := time.Unix(1000, 0)
	a.now = func() time.Time { return at }
	click := func(x, y int) {
		a.View()
		a.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	}
	find := func(s string) (int, int) {
		for y, l := range strings.Split(ansi.Strip(a.View()), "\n") {
			if x := strings.Index(l, s); x >= 0 {
				return len([]rune(l[:x])), y
			}
		}
		t.Fatalf("no %q:\n%s", s, a.View())
		return 0, 0
	}
	onHome(a, "tools")
	ak(a, "enter")
	x, y := find(" System ")
	click(x+2, y)
	if a.Board.tabID != "group:System" {
		t.Errorf("tab click: on %s", a.Board.tabID)
	}
	x, y = find("WireGuard server")
	click(x, y)
	if _, r := a.Board.selected(a.Board.rows()); r == nil || r.item.ID != "wg" || r.item.Pending != "" {
		t.Fatalf("row click: %+v", r)
	}
	at = at.Add(100 * time.Millisecond)
	click(x, y)
	if a.Board.items[3].Pending != "add" {
		t.Errorf("double-click didn't act: %q", a.Board.items[3].Pending)
	}
	x, y = find("? keys")
	click(x, y)
	if !a.keys {
		t.Errorf("footer ? click:\n%s", a.View())
	}
	ak(a, "q", "q")
	// The wheel on the home.
	a.Menu.info.Dash = Dashboard{}
	onHome(a, "tools")
	start := a.Menu.sel
	a.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if a.Menu.sel == start {
		t.Errorf("wheel didn't move: %d", a.Menu.sel)
	}
}

// testUpdates is the app with three tools that have newer versions; one
// (Go) is pinned when pinned.
func testUpdates(pinned bool) *App {
	a := testApp()
	for _, i := range a.Board.items {
		switch i.ID {
		case "docker":
			i.State, i.Updates, i.Source = "ok", []string{"docker-ce 27.1 → 27.2"}, "apt docker-ce"
		case "keys":
			i.State, i.Updates, i.Source = "ok", []string{"gh 2.60 → 2.62"}, "mise use -g gh"
		case "terraform":
			i.Pinned, i.Source = pinned, "mise use -g terraform"
		}
	}
	return a
}

// The home's Install N updates opens Tools on the Updates tab with every
// update queued: a then y updates them all; space leaves one out; q goes
// home with the queue kept.
func TestAppUpdates(t *testing.T) {
	a := testUpdates(false)
	onNeedA(a, "updates")
	ak(a, "enter")
	v := ansi.Strip(a.View())
	if !a.onBoard || a.Board.tabID != UpdatesTab || !strings.Contains(v, "Updates 3") {
		t.Fatalf("enter on Install 3 updates:\n%s", v)
	}
	for _, want := range []string{"[↑]", "will update: terraform 1.9.8 → 1.10.1 · mise", "docker-ce 27.1 → 27.2 · apt", "Queued: 3 to update", "a apply 3"} {
		if !strings.Contains(v, want) {
			t.Errorf("tab lacks %q:\n%s", want, v)
		}
	}
	if cmd := ak(a, "a"); !isQuit(cmd) || a.result.Board == nil || a.result.Board.Kind != BoardApply ||
		strings.Join(a.result.Board.Updates, " ") != "docker terraform keys" || len(a.result.Board.Uninstalls) != 0 {
		t.Fatalf("result %+v", a.result.Board)
	}
	// Leave one out.
	a = testUpdates(false)
	onNeedA(a, "updates")
	ak(a, "enter", "space", "a")
	if got := strings.Join(a.result.Board.Updates, " "); got != "terraform keys" {
		t.Errorf("updates %q", got)
	}
	// q: home, the queue kept.
	a = testUpdates(false)
	onNeedA(a, "updates")
	ak(a, "enter", "q")
	if a.onBoard || a.Board.queued() != 3 {
		t.Errorf("q: on board %v, queued %d", a.onBoard, a.Board.queued())
	}
	// u, from the home or Network: the Updates tab, nothing queued.
	for _, from := range []string{"home", "network"} {
		a = testUpdates(false)
		a.Open(from)
		ak(a, "u")
		if !a.onBoard || a.Board.tabID != UpdatesTab || a.Board.queued() != 0 {
			t.Errorf("u from %s: board %v, tab %s, queued %d", from, a.onBoard, a.Board.tabID, a.Board.queued())
		}
	}
}

// A fix or a value in Needs you opens Tools' Updates tab on its tool's
// row, nothing queued; the keys open Network on them.
func TestAppNeedsYou(t *testing.T) {
	a := testApp()
	onNeedA(a, "key:keys:OPENAI_API_KEY")
	ak(a, "enter")
	if _, r := a.Board.selected(a.Board.rows()); !a.onBoard || a.Board.tabID != UpdatesTab || r == nil || r.item.ID != "keys" || a.Board.queued() != 0 {
		t.Fatalf("a value: board %v, tab %s, row %+v", a.onBoard, a.Board.tabID, r)
	}
	ak(a, "q")
	a.Conn.keys = []AddKey{{Pub: "ssh-ed25519 AAAA", Where: []string{"github.com: Settings › SSH keys"}}}
	a.Menu.info.Keys = a.Conn.keys
	onNeedA(a, "keys")
	ak(a, "enter")
	if !a.onConn || a.Conn.tab != NetTab || a.Conn.row().kind != "key" {
		t.Fatalf("keys: conn %v, %s", a.onConn, a.Conn.tab)
	}
	// Dismissed there, gone from Needs you.
	ak(a, "d", "q")
	if slices.ContainsFunc(a.Menu.needList(), func(r menuRow) bool { return r.id == "keys" }) {
		t.Errorf("a dismissed key still needs you")
	}
}

// Space on another tab still uninstalls; arriving by tab queues nothing.
func TestAppUpdatesByTab(t *testing.T) {
	a := testUpdates(false)
	a.Board.items[1].Removable = true
	onHome(a, "tools")
	ak(a, "enter", "right")
	if a.Board.tabID != UpdatesTab || a.Board.queued() != 0 {
		t.Fatalf("tab %s, queued %d", a.Board.tabID, a.Board.queued())
	}
	ak(a, "right", "down", "space") // DevOps: Terraform
	if a.Board.items[1].Pending != "uninstall" {
		t.Errorf("space on DevOps: %q", a.Board.items[1].Pending)
	}
}

// A pinned tool says so, and space only says why.
func TestAppUpdatesPinned(t *testing.T) {
	a := testUpdates(true)
	onNeedA(a, "updates")
	ak(a, "enter")
	if a.Board.queued() != 2 || a.Board.items[1].Pending != "" {
		t.Fatalf("queued %d, terraform %q", a.Board.queued(), a.Board.items[1].Pending)
	}
	if v := ansi.Strip(a.View()); !strings.Contains(v, "pinned by your profile") {
		t.Errorf("no pinned note:\n%s", v)
	}
	ak(a, "down", "space")
	if a.Board.items[1].Pending != "" || !strings.Contains(a.View(), "pinned by your profile: it stays") {
		t.Errorf("space on pinned: %q\n%s", a.Board.items[1].Pending, a.View())
	}
}

// While a check runs, the tab says so; when it's done, it fills in; with
// nothing newer, the fixes stay, and with nothing to fix either, it goes
// and Installed is up. u on the tab checks again.
func TestAppUpdatesCheck(t *testing.T) {
	a := testApp()
	done := make(chan struct{})
	started := 0
	a.Menu.Checker(func() func() UpdatesMsg {
		started++
		return func() UpdatesMsg {
			<-done
			return UpdatesMsg{ByID: map[string][]string{"terraform": {"terraform 1.9.8 → 1.11.0"}, "keys": {"gh 2.60 → 2.62"}}, At: "09:05"}
		}
	})
	onHome(a, "tools")
	ak(a, "enter", "right")
	if a.Board.tabID != UpdatesTab {
		t.Fatalf("tab %s", a.Board.tabID)
	}
	ak(a, "u")
	if started != 1 || !strings.Contains(a.View(), "Checking for updates…") {
		t.Fatalf("u: started %d\n%s", started, a.View())
	}
	close(done)
	a.Update(a.Menu.wait())
	v := ansi.Strip(a.View())
	if a.Board.tabID != UpdatesTab || !strings.Contains(v, "terraform 1.9.8 → 1.11.0") || !strings.Contains(v, "gh 2.60 → 2.62") {
		t.Fatalf("filled in:\n%s", v)
	}
	a.Update(UpdatesMsg{ByID: map[string][]string{}, At: "09:10"})
	if v := ansi.Strip(a.View()); a.Board.tabID != UpdatesTab || !strings.Contains(v, "No updates: everything is current.") || !strings.Contains(v, "stopped: docker isn't running") {
		t.Errorf("nothing newer, Docker to fix: tab %s\n%s", a.Board.tabID, v)
	}
	a.Board.items[0].State, a.Board.items[4].State = "ok", "ok"
	if a.View(); a.Board.tabID != InstalledTab {
		t.Errorf("nothing at all: tab %s", a.Board.tabID)
	}
}

// Updates holds the fixes too, all queued from the home: a applies them
// with the updates, in one run, without asking.
func TestAppFixes(t *testing.T) {
	a := testUpdates(false)
	a.Board.items[0].State = "drifted"
	onNeedA(a, "updates")
	ak(a, "enter")
	if a.Board.tabID != UpdatesTab || a.Board.items[0].Pending != "fix" {
		t.Fatalf("tab %s, docker %q", a.Board.tabID, a.Board.items[0].Pending)
	}
	if v := ansi.Strip(a.View()); !strings.Contains(v, "[~]") || !strings.Contains(v, "will fix: edited by hand") {
		t.Errorf("the fix:\n%s", v)
	}
	ak(a, "a")
	r := a.result.Board
	if r == nil || r.Kind != BoardApply || strings.Join(r.Fixes, " ") != "docker" || len(r.Updates) == 0 {
		t.Errorf("a: %+v", r)
	}
}

// Reload keeps a queued update only while the tool still has one.
func TestAppReloadUpdates(t *testing.T) {
	a := testUpdates(false)
	onNeedA(a, "updates")
	ak(a, "enter")
	reload := func(updated bool) {
		b := testBoard(false)
		if updated {
			b.items[1].Updates = nil
		}
		a.Reload(AppData{Info: a.Menu.info, Items: b.items, Tabs: rawTabs(b), Checked: true})
	}
	reload(false)
	if a.Board.items[1].Pending != "update" {
		t.Errorf("reload: terraform %q", a.Board.items[1].Pending)
	}
	reload(true)
	if a.Board.items[1].Pending != "" {
		t.Errorf("reload after updating: terraform %q", a.Board.items[1].Pending)
	}
}
