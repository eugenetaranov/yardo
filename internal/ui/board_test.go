package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testBoard(first bool) *Board {
	tabs := []Tab{
		{ID: "group:DevOps", Title: "DevOps", Sections: []Tab{{ID: "k8s", Title: "Containers & Kubernetes"}, {ID: "iac", Title: "IaC"}}},
		{ID: "system", Title: "System"},
		{ID: "__vm__", Title: "This VM", Sections: []Tab{{ID: "logins", Title: "Logins & keys"}, {ID: accessSection, Title: "Who can reach it"}, {ID: machineSection, Title: "This machine"}}},
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
	settings := []*BoardSetting{{Key: "mcp", Label: "MCP gateway", Value: "127.0.0.1", Choices: [][2]string{{"127.0.0.1", "this machine only"}, {"vpn", "this machine and your VPNs"}}}}
	actions := []BoardAction{{ID: "connect", Label: "Connect from a laptop"}, {ID: "refresh", Label: "Check again"}}
	b := NewBoard(HomeInfo{Host: "vm", User: "ada", Checked: "10:00"}, first, tabs, items, settings, actions, []string{"docker context create vm …"})
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

// Something needs attention: the board opens on Needs you, with the broken,
// the values to fill in and the updates; enter fixes, or fills in.
func TestBoardNeedsYou(t *testing.T) {
	b := testBoard(false)
	v := b.View()
	for _, want := range []string{"Needs you 3", "Broken", "Needs a value", "Updates", "✗ 1 to fix", "? 1 to fill in", "stopped: docker isn't running", "enter fix"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	press2(b, "enter") // Docker: the fix panel
	if b.panel == nil || !strings.Contains(b.View(), "Fix it now") {
		t.Fatalf("no fix panel:\n%s", b.View())
	}
	press2(b, "enter")
	if b.result.Kind != BoardFix || b.result.ID != "docker" {
		t.Errorf("result %+v", b.result)
	}
	b = testBoard(false)
	press2(b, "down", "enter") // Agent API keys: fill in first, or change
	if b.panel == nil || !strings.Contains(b.View(), "Fill in 1 value") || !strings.Contains(b.View(), "Change the keys") {
		t.Fatalf("no fill-in panel:\n%s", b.View())
	}
	press2(b, "enter")
	if b.result.Kind != BoardEdit || b.result.ID != "keys" || len(b.result.Inputs) != 1 {
		t.Errorf("fill in: result %+v", b.result)
	}
	// A value left empty doesn't hide the item's answers (the MCP servers).
	b = testBoard(false)
	press2(b, "down", "enter", "down", "enter")
	if b.result.Kind != BoardChange || b.result.ID != "keys" || b.result.Change != "keys" {
		t.Errorf("change: result %+v", b.result)
	}
}

// Space queues adding (and removing); a setting changes through enter;
// Review lists the queue; a asks, then applies.
func TestBoardQueueAndApply(t *testing.T) {
	b := testBoard(false)
	press2(b, "right", "right") // System
	if !strings.Contains(b.View(), "WireGuard server") {
		t.Fatalf("not on System:\n%s", b.View())
	}
	press2(b, "space")
	press2(b, "right", "down", "enter") // This VM: Who can reach it, MCP gateway
	if b.panel == nil {
		t.Fatalf("no setting panel:\n%s", b.View())
	}
	press2(b, "down", "enter")
	if b.queued() != 2 || !strings.Contains(b.View(), "2 change(s) queued") || !strings.Contains(b.View(), "APPLY 2") {
		t.Errorf("queue:\n%s", b.View())
	}
	press2(b, "right") // Review
	if v := b.View(); !strings.Contains(v, "[+]") || !strings.Contains(v, "this machine only → this machine and your VPNs") {
		t.Errorf("review:\n%s", v)
	}
	press2(b, "a")
	if b.panel == nil || !strings.Contains(b.View(), "+ WireGuard server") {
		t.Fatalf("no confirm:\n%s", b.View())
	}
	press2(b, "enter")
	if r := b.result; r.Kind != BoardApply || strings.Join(r.Adds, " ") != "wg" || r.Settings["mcp"] != "vpn" {
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
	if b.result.Kind != BoardQuit || b.out || !strings.Contains(b.View(), "queued and not applied") {
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

// This VM's actions: connect shows how, in a panel.
func TestBoardConnect(t *testing.T) {
	b := testBoard(false)
	b.tab = len(b.visibleTabs()) - 2 // This VM
	press2(b, "down", "down", "enter")
	if b.panel == nil || !strings.Contains(b.View(), "docker context create vm") {
		t.Errorf("connect:\n%s", b.View())
	}
	press2(b, "esc")
	if b.panel != nil {
		t.Error("esc didn't close the panel")
	}
}

// A setting of another section (Look): listed there, its own panel title,
// queued like the rest.
func TestBoardLook(t *testing.T) {
	b := testBoard(false)
	secs := b.tabs[2].Sections // Look goes before This machine, as yardo puts it
	b.tabs[2].Sections = append(secs[:len(secs)-1:len(secs)-1], Tab{ID: "__look__", Title: "Look"}, secs[len(secs)-1])
	b.settings = append(b.settings, &BoardSetting{Key: "accent", Label: "Accent colour", Section: "__look__", Value: "coral",
		Choices: [][2]string{{"coral", "Coral"}, {"lagoon", "Lagoon (aqua blue)"}}})
	b.tab = len(b.visibleTabs()) - 2 // This VM
	v := b.View()
	if !strings.Contains(v, "Look") || !strings.Contains(v, "Accent colour") || strings.Index(v, "Accent colour") < strings.Index(v, "MCP gateway") {
		t.Fatalf("This VM:\n%s", v)
	}
	press2(b, "down", "down", "enter")
	if b.panel == nil || b.panel.title != "Accent colour" {
		t.Fatalf("panel %+v", b.panel)
	}
	press2(b, "down", "enter", "a", "enter")
	if b.result.Kind != BoardApply || b.result.Settings["accent"] != "lagoon" {
		t.Errorf("result %+v", b.result)
	}
}

// Enter on a picked item with answers offers to change them; choosing it
// leaves the board with the item and the answer's key.
func TestBoardChange(t *testing.T) {
	tabs := []Tab{{ID: "git", Title: "Git & SSH"}}
	items := []*BoardItem{{ID: "gitid", Label: "Git identity", Tab: "git", Picked: true, State: "ok", Changes: [][2]string{{"git name and email", "git"}}}}
	b := NewBoard(HomeInfo{Host: "vm", User: "ada"}, false, tabs, items, nil, nil, nil)
	b.Update(tea.WindowSizeMsg{Width: 110, Height: 26})
	if !strings.Contains(b.View(), "enter change") {
		t.Errorf("footer lacks enter change:\n%s", b.View())
	}
	press2(b, "enter")
	if b.panel == nil || !strings.Contains(b.View(), "Change git name and email") {
		t.Fatalf("no change option:\n%s", b.View())
	}
	press2(b, "enter")
	if r := b.result; r.Kind != BoardChange || r.ID != "gitid" || r.Change != "git" {
		t.Errorf("result %+v", r)
	}
}
