package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testBoard(first bool) *Board {
	tabs := []Tab{
		{ID: "group:DevOps", Title: "DevOps", Sections: []Tab{{ID: "k8s", Title: "Containers & Kubernetes"}, {ID: "iac", Title: "IaC"}}},
		{ID: "group:System", Title: "System", Sections: []Tab{{ID: "system", Title: "System"}, {ID: "logins", Title: "VPN logins"}}},
	}
	items := []*BoardItem{
		{ID: "docker", Label: "Docker Engine", Tab: "k8s", Picked: !first, State: "down", Detail: "docker isn't running"},
		{ID: "terraform", Label: "Terraform", Tab: "iac", Picked: !first, State: "ok", Updates: []string{"terraform 1.9.8 → 1.10.1"}},
		{ID: "ansible", Label: "Ansible", Tab: "iac"},
		{ID: "wg", Label: "WireGuard server", Tab: "system"},
		{ID: "keys", Label: "Agent API keys", Tab: "logins", Picked: !first, State: "secret", Inputs: []string{"~/.config/yardo/agent.env: OPENAI_API_KEY"}, Changes: [][2]string{{"the keys", "keys"}}},
	}
	if first {
		for _, i := range items {
			i.State, i.Updates = "", nil
		}
		items[0].Pending = "add"
	}
	b := NewBoard(HomeInfo{Host: "vm", User: "ada", Checked: "10:00"}, first, tabs, items)
	b.Update(tea.WindowSizeMsg{Width: 110, Height: 26})
	return b
}

func press2(b *Board, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		var m tea.KeyMsg
		switch k {
		case "up":
			m = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			m = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			m = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			m = tea.KeyMsg{Type: tea.KeyRight}
		case "enter":
			m = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			m = tea.KeyMsg{Type: tea.KeyEsc}
		case "space":
			m = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		case "ctrl+a":
			m = tea.KeyMsg{Type: tea.KeyCtrlA}
		case "backspace":
			m = tea.KeyMsg{Type: tea.KeyBackspace}
		default:
			m = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd = b.Update(m)
	}
	return cmd
}

// What needs attention shows on its own rows, the tabs' marks and the
// header (the home lists it too; the board has no tab for it); enter fixes,
// or fills in.
func TestBoardNeedsYou(t *testing.T) {
	b := testBoard(false)
	v := b.View()
	for _, want := range []string{"DevOps ✗", "System ○", "✗ 1 to fix", "○ 1 to fill in", "stopped: docker isn't running", "enter fix"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	for _, gone := range []string{"Needs you", "Review", "This VM"} {
		if strings.Contains(v, gone) {
			t.Errorf("view has %q:\n%s", gone, v)
		}
	}
	press2(b, "enter") // Docker: the fix panel
	if b.panel == nil || !strings.Contains(b.View(), "Fix it (start it again)") {
		t.Fatalf("no fix panel:\n%s", b.View())
	}
	press2(b, "enter")
	if b.result.Kind != BoardFix || b.result.ID != "docker" {
		t.Errorf("result %+v", b.result)
	}
	b = testBoard(false)
	press2(b, "right", "right", "down", "enter") // System, VPN logins: Agent API keys: fill in first, or change
	if b.panel == nil || !strings.Contains(b.View(), "Fill in 1 value") || !strings.Contains(b.View(), "Change the keys") {
		t.Fatalf("no fill-in panel:\n%s", b.View())
	}
	press2(b, "enter")
	if b.result.Kind != BoardEdit || b.result.ID != "keys" || len(b.result.Inputs) != 1 {
		t.Errorf("fill in: result %+v", b.result)
	}
	// A value left empty doesn't hide the item's answers (the MCP servers).
	b = testBoard(false)
	press2(b, "right", "right", "down", "enter", "down", "enter")
	if b.result.Kind != BoardChange || b.result.ID != "keys" || b.result.Change != "keys" {
		t.Errorf("change: result %+v", b.result)
	}
}

// Installed, the first tab, lists every tool installed, from all the
// tabs, each with its tab; d uninstalls one from there. The first run has
// none.
func TestBoardInstalled(t *testing.T) {
	b := testBoard(false)
	v := b.View()
	for _, want := range []string{"Installed ✗", "Docker Engine", "Terraform", "Agent API keys", "DevOps · ", "System · "} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	for _, gone := range []string{"Ansible", "WireGuard server"} {
		if strings.Contains(v, gone) {
			t.Errorf("Installed lists %q:\n%s", gone, v)
		}
	}
	b.items[1].Removable = true
	press2(b, "down", "d")
	if b.items[1].Pending != "uninstall" || !strings.Contains(b.View(), "Uninstall 1 tool?") {
		t.Fatalf("d didn't queue the uninstall:\n%s", b.View())
	}
	if strings.Contains(testBoard(true).View(), "Installed") {
		t.Errorf("first run has Installed:\n%s", testBoard(true).View())
	}
}

// Space queues adding (and removing); the queue shows on its own line; a
// asks, then applies.
func TestBoardQueueAndApply(t *testing.T) {
	b := testBoard(false)
	press2(b, "right", "right") // System
	if !strings.Contains(b.View(), "WireGuard server") {
		t.Fatalf("not on System:\n%s", b.View())
	}
	press2(b, "space")
	press2(b, "left", "down", "down", "space") // DevOps: Ansible
	if v := b.View(); b.queued() != 2 || !strings.Contains(v, "2 changes queued") || !strings.Contains(v, "APPLY 2") ||
		!strings.Contains(v, "Queued: install Ansible · install WireGuard server") {
		t.Errorf("queue:\n%s", v)
	}
	press2(b, "a")
	if b.panel == nil || !strings.Contains(b.View(), "+ Install WireGuard server") || strings.Contains(b.View(), "Queued:") {
		t.Fatalf("no confirm:\n%s", b.View())
	}
	press2(b, "enter")
	if r := b.result; r.Kind != BoardApply || strings.Join(r.Adds, " ") != "ansible wg" {
		t.Errorf("result %+v", r)
	}
}

// First run: nothing has a mark yet, the defaults come queued; esc with a
// queue asks once before quitting.
func TestBoardFirstRun(t *testing.T) {
	b := testBoard(true)
	v := b.View()
	if strings.Contains(v, "Needs you") || !strings.Contains(v, "nothing installed yet") || !strings.Contains(v, "[+]") {
		t.Errorf("first run:\n%s", v)
	}
	press2(b, "q")
	if b.result.Kind != BoardQuit || b.out || !strings.Contains(b.View(), "not applied yet") {
		t.Errorf("q with a queue quit at once:\n%s", b.View())
	}
	if cmd := press2(b, "q"); cmd == nil || b.result.Kind != BoardQuit {
		t.Error("q again didn't quit")
	}
}

// / searches every tab; space queues from the results; ctrl-a adds a tool
// the catalog lacks.
func TestBoardSearch(t *testing.T) {
	b := testBoard(false)
	press2(b, "/", "w", "i", "r", "e")
	if v := b.View(); !strings.Contains(v, "WireGuard server") || !strings.Contains(v, "System") {
		t.Errorf("search:\n%s", v)
	}
	press2(b, "space", "esc")
	if b.search != nil || b.queued() != 1 {
		t.Errorf("after search: queued %d", b.queued())
	}
	press2(b, "/", "p", "u", "l", "u", "m", "i")
	if !strings.Contains(b.View(), `ctrl-a adds "pulumi"`) {
		t.Errorf("no extra offer:\n%s", b.View())
	}
	press2(b, "ctrl+a")
	if b.queued() != 2 || !strings.Contains(b.tabBar(110), "Extra") {
		t.Errorf("extra not queued: %d", b.queued())
	}
}

// Enter on a picked item with answers offers to change them; choosing it
// leaves the board with the item and the answer's key.
func TestBoardChange(t *testing.T) {
	tabs := []Tab{{ID: "git", Title: "Git & SSH"}}
	items := []*BoardItem{{ID: "gitid", Label: "Git identity", Tab: "git", Picked: true, State: "ok", Changes: [][2]string{{"git name and email", "git"}}}}
	b := NewBoard(HomeInfo{Host: "vm", User: "ada"}, false, tabs, items)
	b.Update(tea.WindowSizeMsg{Width: 110, Height: 26})
	if !strings.Contains(b.View(), "enter change") {
		t.Errorf("footer lacks enter change:\n%s", b.View())
	}
	press2(b, "enter")
	if b.panel == nil || !strings.Contains(b.View(), "Change git name and email") {
		t.Fatalf("no change option:\n%s", b.View())
	}
	press2(b, "down") // past Reinstall
	press2(b, "enter")
	if r := b.result; r.Kind != BoardChange || r.ID != "gitid" || r.Change != "git" {
		t.Errorf("result %+v", r)
	}
}

// Labels take the screen's width, not a fixed column: a wide terminal
// shows a long label whole, a narrow one shortens it and keeps the note.
func TestBoardLabelWidth(t *testing.T) {
	long := "OpenVPN client (connect this machine to a VPN elsewhere; your .ovpn in /etc/openvpn/client)"
	tabs := []Tab{{ID: "net", Title: "Connectivity"}}
	items := []*BoardItem{
		{ID: "a", Label: "Tailscale", Tab: "net"},
		{ID: "b", Label: long, Tab: "net", Picked: true, State: "drifted", Detail: "file /etc/x"},
	}
	b := NewBoard(HomeInfo{Host: "vm", User: "ada"}, false, tabs, items)
	b.Update(tea.WindowSizeMsg{Width: 200, Height: 26})
	if v := b.View(); !strings.Contains(v, long) || !strings.Contains(v, "edited by hand: file /etc/x") {
		t.Errorf("wide: label or note cut:\n%s", v)
	}
	b.Update(tea.WindowSizeMsg{Width: 90, Height: 26})
	if v := b.View(); strings.Contains(v, long) || !strings.Contains(v, "edited by hand") {
		t.Errorf("narrow: want the label shortened and the note kept:\n%s", v)
	}
}

// Enter on a ready item offers to apply it again (a newer yardo's files)
// or to remove it.
func TestBoardReadyItemActions(t *testing.T) {
	tabs := []Tab{{ID: "shell", Title: "Shell & CLI"}}
	mk := func() *Board {
		items := []*BoardItem{{ID: "helpers", Label: "Shell helpers", Tab: "shell", Picked: true, State: "ok"}}
		b := NewBoard(HomeInfo{Host: "vm", User: "ada"}, false, tabs, items)
		b.Update(tea.WindowSizeMsg{Width: 110, Height: 26})
		return b
	}
	b := mk()
	press2(b, "enter")
	if v := b.View(); !strings.Contains(v, "Reinstall") || !strings.Contains(v, "Forget it") {
		t.Fatalf("panel lacks the actions:\n%s", v)
	}
	press2(b, "enter")
	if r := b.result; r.Kind != BoardFix || r.ID != "helpers" {
		t.Errorf("apply again: result %+v", r)
	}
	b = mk()
	press2(b, "enter")
	press2(b, "down")
	press2(b, "enter")
	if b.items[0].Pending != "forget" {
		t.Errorf("forget: pending %q", b.items[0].Pending)
	}
}

// Open goes to a tab by its id; an unknown id leaves the tab as it was.
func TestBoardOpen(t *testing.T) {
	b := testBoard(false)
	b.Open("group:System")
	if b.key() != "group:System" || !strings.Contains(b.View(), "WireGuard server") {
		t.Fatalf("Open(group:System): on %q", b.key())
	}
	b.Open("nope")
	if b.key() != "group:System" {
		t.Errorf("Open(nope) moved to %q", b.key())
	}
}

// From the menu, the footer and the queue's warning say esc goes back.
func TestBoardFromMenu(t *testing.T) {
	b := testBoard(false)
	if f := b.footer(nil); !strings.Contains(f, "quit") || strings.Contains(f, "home") {
		t.Errorf("standalone footer: %s", f)
	}
	b.SetFromMenu(true)
	if f := b.footer(nil); !strings.Contains(f, "esc home") || strings.Contains(f, "quit") {
		t.Errorf("from the menu footer: %s", f)
	}
	b.Open("group:System")
	press2(b, "space", "esc")
	if !strings.Contains(b.flash, "esc again goes back without them") {
		t.Errorf("flash %q", b.flash)
	}
}

// ctrl-a adds the Extra tab; the tab shown stays the one it was.
func TestBoardExtraKeepsTab(t *testing.T) {
	b := testBoard(false)
	b.Open("group:System")
	press2(b, "/", "p", "u", "l", "u", "m", "i", "ctrl+a")
	if b.key() != "group:System" {
		t.Fatalf("on %q, want System", b.key())
	}
	if v := b.View(); !strings.Contains(v, "Queued: install pulumi") {
		t.Errorf("the queue lacks the extra:\n%s", v)
	}
}

// Search opens the board searching, as typing / does.
func TestBoardSearchOpen(t *testing.T) {
	b := testBoard(false)
	b.Search("wire")
	if v := b.View(); !strings.Contains(v, "WireGuard server") || !strings.Contains(v, "/wire_") {
		t.Errorf("search:\n%s", v)
	}
}

// A team profile's pick says so; uninstalling it says the profile then
// skips it.
func TestBoardTeam(t *testing.T) {
	tabs := []Tab{{ID: "shell", Title: "Shell & CLI"}}
	items := []*BoardItem{{ID: "zsh", Label: "zsh (oh-my-zsh, plugins)", Tab: "shell", Picked: true, State: "ok", Team: "platform", Removable: true}}
	b := NewBoard(HomeInfo{Host: "vm", User: "ada"}, false, tabs, items)
	b.Update(tea.WindowSizeMsg{Width: 110, Height: 26})
	if !strings.Contains(b.View(), "from the team profile platform") {
		t.Errorf("no team note:\n%s", b.View())
	}
	press2(b, "space")
	if items[0].Pending != "uninstall" || !strings.Contains(b.View(), "zsh is on the team profile platform: once uninstalled, applying the profile again skips it") {
		t.Errorf("no warning:\n%s", b.View())
	}
}

// Space unticks an installed tool: it's queued to uninstall; a asks,
// titled by what it does, with what goes, stays and goes with it; y
// applies. d asks at once. One yardo can't uninstall offers to forget
// it; a protected one only says why. A login is part of its tool's row:
// its details log out. After, the result stays until dismissed and
// offers a retry.
func TestBoardUninstall(t *testing.T) {
	tabs := []Tab{{ID: "net", Title: "Network"}}
	mk := func() (*Board, []*BoardItem) {
		items := []*BoardItem{
			{ID: "netbird-install", Label: "NetBird (mesh VPN)", Tab: "net", Picked: true, State: "ok", Removable: true,
				RemoveSay: "disconnects from NetBird and uninstalls it"},
			{ID: "netbird", Label: "NetBird login", Tab: "net", Picked: true, State: "ok", PartOf: "netbird-install", Removable: true},
			{ID: "aws", Label: "AWS tools (aws CLI)", Tab: "net", Picked: true, State: "ok", Removable: true},
			{ID: "ecc", Label: "ECC for Claude Code", Tab: "net", Picked: true, State: "ok", KeepSay: "its files sit among yours"},
			{ID: "zsh", Label: "zsh, ready to use", Tab: "net", Picked: true, State: "ok", Protected: true, KeepSay: "it's the shell you log in with"},
			{ID: "terraform", Label: "Terraform", Tab: "net"},
		}
		b := NewBoard(HomeInfo{Host: "vm", User: "ada"}, false, tabs, items)
		b.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
		return b, items
	}
	b, items := mk()
	v := b.View()
	if !strings.Contains(v, "space uninstall") || strings.Contains(v, "stop managing") {
		t.Errorf("footer:\n%s", v)
	}
	if strings.Contains(v, "NetBird login") {
		t.Errorf("the login is a row of its own:\n%s", v)
	}
	press2(b, "right") // Network: netbird-install, aws, ecc, zsh, terraform
	press2(b, "space", "down", "space")
	if items[0].Pending != "uninstall" || items[2].Pending != "uninstall" {
		t.Fatalf("space didn't untick: %q %q", items[0].Pending, items[2].Pending)
	}
	if v := b.View(); !strings.Contains(v, "[−]") || !strings.Contains(v, "will uninstall") || !strings.Contains(v, "Queued: uninstall NetBird · uninstall AWS tools") {
		t.Errorf("marks:\n%s", v)
	}
	press2(b, "down", "space") // ECC: can't; forget instead?
	if v := b.View(); b.panel == nil || !strings.Contains(v, "yardo can't uninstall ECC for Claude Code") || !strings.Contains(v, "Its files sit among yours.") {
		t.Fatalf("no forget offer:\n%s", v)
	}
	press2(b, "y")
	if items[3].Pending != "forget" || !strings.Contains(b.View(), "will forget it: stays installed") {
		t.Errorf("ecc: %q\n%s", items[3].Pending, b.View())
	}
	press2(b, "down", "space") // zsh: says why, nothing queued
	if items[4].Pending != "" || b.panel != nil || !strings.Contains(b.View(), "it's the shell you log in with") {
		t.Errorf("zsh: %q\n%s", items[4].Pending, b.View())
	}
	var asked []string
	b.SetPlanner(func(ids, adds []string) map[string]UninstallPlan {
		asked = ids
		return map[string]UninstallPlan{
			"netbird-install": {Say: "disconnects from NetBird and uninstalls it", Off: []string{"netbird"}, Also: []string{"NetBird login"}},
			"aws":             {Off: []string{"aws-cli", "mariadb-client"}, Stays: []string{"fzf (Shell helpers uses it)"}},
		}
	})
	press2(b, "a")
	v = b.View()
	for _, want := range []string{"Apply 3 changes?", "− Uninstall NetBird", "    Disconnects from NetBird and uninstalls it.", "    Also uninstalls NetBird login: they need it.",
		"− Uninstall AWS tools", "    Goes: aws-cli, mariadb-client", "    Stays: fzf (Shell helpers uses it)", "Forget ECC for Claude Code: it stays installed", "List everything that goes (3)"} {
		if !strings.Contains(v, want) {
			t.Errorf("question lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Goes: netbird") {
		t.Errorf("the full list shows before it's asked for:\n%s", v)
	}
	press2(b, "down", "enter") // List everything
	if !strings.Contains(b.View(), "Goes: netbird") {
		t.Errorf("no full list:\n%s", b.View())
	}
	if !slices.Equal(asked, []string{"netbird-install", "aws"}) {
		t.Errorf("planner asked %q", asked)
	}
	press2(b, "y")
	if r := b.result; r.Kind != BoardApply || !slices.Equal(r.Uninstalls, []string{"netbird-install", "aws"}) || !slices.Equal(r.Forgets, []string{"ecc"}) {
		t.Errorf("result %+v", r)
	}

	// d: asks at once; only uninstalls: titled so.
	b, items = mk()
	press2(b, "right", "down", "d")
	if v := b.View(); items[2].Pending != "uninstall" || !strings.Contains(v, "Uninstall 1 tool?") || !strings.Contains(v, "Uninstall (y)") {
		t.Errorf("d:\n%s", v)
	}
	press2(b, "n")
	if b.panel != nil || b.result.Kind == BoardApply {
		t.Error("n applied")
	}

	// The login: logged out from its tool's details.
	b, items = mk()
	press2(b, "right", "enter")
	if !strings.Contains(b.View(), "Log out (NetBird stays)") {
		t.Fatalf("no log out:\n%s", b.View())
	}
	for b.panel.opts[b.panel.sel][0] != "Log out (NetBird stays)" {
		press2(b, "down")
	}
	press2(b, "enter")
	if items[1].Pending != "uninstall" || items[0].Pending != "" {
		t.Errorf("log out: %q %q", items[1].Pending, items[0].Pending)
	}

	// After: the result stays, and retries what failed.
	b, items = mk()
	b.ShowResult("Done: 1 uninstalled, 1 failed", []string{"✓ AWS tools uninstalled.", "✗ NetBird: the uninstall stopped (exit 1)."}, []string{"netbird-install"})
	if v := b.View(); !strings.Contains(v, "Done: 1 uninstalled, 1 failed") || !strings.Contains(v, "Retry NetBird") {
		t.Fatalf("result:\n%s", v)
	}
	press2(b, "right") // a key outside the panel's doesn't close it
	if b.panel == nil {
		t.Fatal("the result went on a key")
	}
	press2(b, "down", "enter")
	if items[0].Pending != "uninstall" || !strings.Contains(b.View(), "Uninstall 1 tool?") {
		t.Errorf("retry:\n%s", b.View())
	}
}

// Typing filters; enter goes into the rows keeping the filter, where
// space and a work; esc clears the filter.
func TestBoardFilter(t *testing.T) {
	b := testBoard(false)
	b.Search("ansi")
	press2(b, "enter")
	if b.typing || b.search == nil || !strings.Contains(b.View(), "Ansible") {
		t.Fatalf("enter left the filter:\n%s", b.View())
	}
	press2(b, "space")
	if n := b.queued(); n != 1 {
		t.Errorf("space in the filtered list queued %d", n)
	}
	press2(b, "esc")
	if b.search != nil || b.out {
		t.Errorf("esc didn't clear the filter")
	}
}
