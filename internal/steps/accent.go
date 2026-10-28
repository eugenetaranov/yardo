package steps

// Accents are the accent colours a user can pick: yardo's screen and their
// shell (the prompt, tmux's status bar, $YARDO_ACCENT) take it, so each
// machine's terminals can be told apart. The first is the default.
var Accents = []struct{ Key, Name, Hex string }{
	{"coral", "Coral", "#D97757"},
	{"lagoon", "Lagoon (aqua blue)", "#2BB3C0"},
	{"pistachio", "Pistachio", "#93C572"},
	{"lavender", "Lavender", "#A78BFA"},
	{"amber", "Amber", "#E0A030"},
	{"rose", "Rose", "#E36A8C"},
}

// AccentHex is accent's colour (a key of Accents; coral when unknown).
func AccentHex(accent string) string {
	for _, a := range Accents {
		if a.Key == accent {
			return a.Hex
		}
	}
	return Accents[0].Hex
}

// accentOf is the user's accent colour.
func accentOf(e *Env) string {
	if e.Answers == nil {
		return AccentHex("")
	}
	return AccentHex(e.Answers.Accent)
}
