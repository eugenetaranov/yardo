package tui

import "testing"

// Every look has a key, a name and all its marks; the keys differ.
func TestLooks(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range Looks {
		m := l.Mark
		if l.Key == "" || l.Name == "" || l.Brand == "" || l.Sep == "" || seen[l.Key] ||
			m.OK == "" || m.Down == "" || m.Partial == "" || m.Need == "" || m.Update == "" || m.Unchecked == "" {
			t.Errorf("look %+v", l)
		}
		seen[l.Key] = true
	}
	if Looks[0].Key != "classic" || Looks[0].Accent != "" {
		t.Errorf("Classic isn't first, on the user's accent")
	}
}

func TestTitle(t *testing.T) {
	t.Cleanup(func() { Use("classic", "#D97757") })
	if got := Title("Settings  ›  Look"); got != "Settings  ›  Look" {
		t.Errorf("classic: %q", got)
	}
	Use("covert", "#D97757")
	if got := Title("Settings  ›  Look"); got != "SETTINGS  //  LOOK" {
		t.Errorf("covert: %q", got)
	}
	if Accent != "#FF9F1C" {
		t.Errorf("covert's accent is %q", Accent)
	}
	Use("classic", "#2BB3C0")
	if Accent != "#2BB3C0" {
		t.Errorf("classic takes the user's accent, not %q", Accent)
	}
}
