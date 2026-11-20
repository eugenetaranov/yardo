package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// testApp is the app on testMenu's items and testBoard's tabs.
func testApp() *App {
	m, b := testMenu(true), testBoard(false)
	d := AppData{Info: m.info, Sub: m.sub, Items: b.items, Settings: m.settings, Connect: m.connect, Checked: true, Tabs: rawTabs(b)}
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
	for n, r := range a.Menu.rows() {
		if r.id == id {
			a.Menu.sel = n
		}
	}
}

// Enter on Tools, then q: one program, the home back, no result.
func TestAppHop(t *testing.T) {
	a := testApp()
	onHome(a, "tools")
	if isQuit(ak(a, "enter")) || !a.onBoard || !strings.Contains(a.View(), "Home › Tools") {
		t.Fatalf("enter on Tools:\n%s", a.View())
	}
	if cmd := ak(a, "q"); isQuit(cmd) || a.onBoard || a.result.Board != nil || a.result.Quit {
		t.Fatalf("q on Tools: left %v, result %+v", isQuit(cmd), a.result)
	}
	if v := a.View(); !strings.Contains(v, "─ Needs you ") || !strings.Contains(v, "Home ") {
		t.Errorf("not the home:\n%s", v)
	}
}

// The board's queue stays across a hop home and back; the home's Tools
// row says so; q on the home asks once, then quits.
func TestAppQueueKept(t *testing.T) {
	a := testApp()
	onHome(a, "tools")
	ak(a, "enter", "right", "right", "space") // System: WireGuard
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
	ak(a, "enter", "right", "right", "space") // System: WireGuard
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
	ak(a, "enter", "right") // DevOps
	if v := a.View(); !strings.Contains(strings.Split(v, "\n")[0], "Home › Tools › DevOps") {
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
	ak(a, "enter", "right", "d") // DevOps: Docker
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
		a.Board.items = append(a.Board.items, &BoardItem{ID: "x" + string(rune('a'+n%26)) + string(rune('a'+n/26)), Label: "Tool " + string(rune('A'+n%26)) + string(rune('A'+n/26)), Tab: "iac", Picked: true, State: "ok"})
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
	for _, want := range []string{"This screen", "space", "apply the queue", "uninstall the row", "search", "←→ h l", "Every screen", "PgUp PgDn", "Home › Tools › Installed › Keys"} {
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
// on a footer key presses it; the wheel over Needs you moves its rows.
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
	// The wheel over Needs you.
	a.Menu.info.Dash = Dashboard{}
	a.Board.items[0].State = "down"
	onHome(a, "fix:docker")
	start := a.Menu.sel
	a.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if a.Menu.sel == start {
		t.Errorf("wheel didn't move: %d", a.Menu.sel)
	}
}
