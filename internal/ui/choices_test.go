package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

func ckKeys(c *checklist, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		c.Update(msg)
	}
}

// One screen: a toggle's box, a multi's boxes, a one's radios (None
// first); the cursor skips headings.
func TestChecklist(t *testing.T) {
	prompt := &Question{Title: "zsh prompt", Kind: "pick", Options: [][2]string{{"p10k", "Powerlevel10k"}, {"starship", "Starship"}}, Picked: []string{"p10k"}}
	lang := &Question{Title: "Languages", Note: "Python, Node.js, Go", Kind: "toggle", Picked: []string{"on"}}
	cloud := &Question{Title: "Cloud", Kind: "multi", Options: [][2]string{{"aws", "AWS"}, {"doctl", "DigitalOcean"}, {"gcloud", "Google Cloud"}}, Picked: []string{"gcloud"}}
	vpn := &Question{Title: "VPN", Kind: "one", Options: [][2]string{{"tailscale", "Tailscale"}, {"netbird", "NetBird"}}, Picked: []string{"tailscale"}}
	c := newChecklist([]*Question{prompt, lang, cloud, vpn})
	c.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	v := ansi.Strip(c.View())
	for _, want := range []string{"Choices", "(•) Powerlevel10k", "( ) Starship", "[x] Languages", "Python, Node.js, Go", "[ ] AWS", "[x] Google Cloud", "( ) None", "(•) Tailscale"} {
		if !strings.Contains(v, want) {
			t.Errorf("lacks %q:\n%s", want, v)
		}
	}
	// Starship; Languages off; AWS on (before Google Cloud); None.
	ckKeys(c, "down", "space", "down", "space", "down", "space", "down", "down", "down", "space")
	if !slices.Equal(prompt.Picked, []string{"starship"}) || len(lang.Picked) != 0 ||
		!slices.Equal(cloud.Picked, []string{"aws", "gcloud"}) || len(vpn.Picked) != 0 {
		t.Errorf("prompt %v lang %v cloud %v vpn %v", prompt.Picked, lang.Picked, cloud.Picked, vpn.Picked)
	}
	// The cursor stops at the ends.
	ckKeys(c, "down", "down", "down", "down", "down")
	if r := c.rows[c.sel]; r.q != vpn || r.key != "netbird" {
		t.Errorf("at the end on %+v", r)
	}
	ckKeys(c, "q")
	if !c.quit {
		t.Error("q doesn't cancel")
	}
}

// Every box, a toggle's or an option's, selected or not, is in one column;
// headings are left of it.
func TestChecklistLinesUp(t *testing.T) {
	lang := &Question{Title: "Languages", Kind: "multi", Options: [][2]string{{"py", "Python"}, {"go", "Go"}}, Picked: []string{"py"}}
	docker := &Question{Title: "Docker", Note: "and the MCP gateway", Kind: "toggle", Picked: []string{"on"}}
	jira := &Question{Title: "Jira", Kind: "toggle"}
	vpn := &Question{Title: "VPN", Kind: "one", Options: [][2]string{{"tailscale", "Tailscale"}}}
	c := newChecklist([]*Question{lang, docker, jira, vpn})
	ckKeys(c, "down", "down")
	lines, _ := c.lines(100)
	col := -1
	for _, l := range lines {
		l = ansi.Strip(l)
		i := strings.IndexAny(l, "[(")
		if i < 0 {
			if h := strings.TrimSpace(l); h != "" && col >= 0 && strings.Index(l, h) >= col {
				t.Errorf("heading %q not left of the boxes", h)
			}
			continue
		}
		i = runewidth.StringWidth(l[:i]) // › is one column, three bytes
		if col < 0 {
			col = i
		}
		if i != col {
			t.Errorf("box at %d, not %d: %q", i, col, l)
		}
	}
}
