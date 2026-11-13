package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestValidExtra(t *testing.T) {
	for q, want := range map[string]bool{
		"pulumi": true, "pipx:pytest": true, "github:tackhq/tack@1.1.4": true, "node@22": true,
		"-rf": false, "a b": false, "x;rm": false, "": false, "$(id)": false,
	} {
		if ValidExtra(q) != want {
			t.Errorf("ValidExtra(%q) = %v", q, !want)
		}
	}
}

func TestWrap(t *testing.T) {
	got := wrap("Kubernetes CLIs (kubectl, kubectx, helm 3, k9s)", 16)
	for _, l := range got {
		if len(l) > 16 {
			t.Errorf("line %q wider than 16", l)
		}
	}
	if strings.Join(got, " ") != "Kubernetes CLIs (kubectl, kubectx, helm 3, k9s)" {
		t.Errorf("wrap lost words: %q", got)
	}
}

// A rune wider than the width can't be cut: it takes a line of its own
// (it used to loop forever).
func TestWrapWideRune(t *testing.T) {
	if got, want := wrap("日本語", 1), []string{"日", "本", "語"}; !reflect.DeepEqual(got, want) {
		t.Errorf("wrap(日本語, 1) = %q, want %q", got, want)
	}
}

// A theme changes the name, the titles and the marks; Classic is what
// yardo always looked like.
func TestSetLook(t *testing.T) {
	t.Cleanup(func() { SetLook("classic", "#D97757") })
	SetLook("bridge", "#D97757")
	m := testMenu(false)
	onRow(m, "settings")
	mk(m, "enter")
	v := ansi.Strip(m.View())
	for _, want := range []string{"◢◤ YARDO ◥◣", "HOME  ▸  SETTINGS"} {
		if !strings.Contains(v, want) {
			t.Errorf("bridge lacks %q:\n%s", want, v)
		}
	}
	if g, _ := mark(&BoardItem{Picked: true, State: "down"}); g != "✕" {
		t.Errorf("bridge marks down %q", g)
	}
	SetLook("nope", "#D97757")
	m = testMenu(false)
	if v := ansi.Strip(m.View()); !strings.Contains(v, " yardo  ·  vm") {
		t.Errorf("an unknown theme isn't Classic:\n%s", v)
	}
	if g, _ := mark(&BoardItem{Picked: true, State: "ok"}); g != "✓" {
		t.Errorf("classic marks ok %q", g)
	}
}
