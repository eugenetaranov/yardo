package ui

import (
	"reflect"
	"strings"
	"testing"
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
