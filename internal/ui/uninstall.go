package ui

import "strings"

// UninstallPlan is what uninstalling one tool does, for the question
// (the board's, yardo remove's).
type UninstallPlan struct {
	// Say is what its own removal does (the catalog's remove_say).
	Say string
	// Off is everything that goes: packages, mise tools, files, lines.
	Off []string
	// Stays is what's kept, and why: "fzf (Shell helpers uses it)".
	Stays []string
	// Also are the tools that go with it (they need it), Forgets the ones
	// that need it and yardo can't uninstall (it stops checking them),
	// Back a queued install that needs it (it comes back): short names.
	Also, Forgets, Back []string
	// Err is why yardo couldn't work it out.
	Err string
}

// Lines are the plan as lines under the tool's name; full lists Off.
func (p UninstallPlan) Lines(full bool) []string {
	if p.Err != "" {
		return []string{"Can't work out what goes: " + p.Err}
	}
	var ls []string
	if p.Say != "" {
		ls = append(ls, capital(p.Say)+".")
	}
	if full || p.Say == "" {
		switch {
		case len(p.Off) > 0:
			ls = append(ls, "Goes: "+strings.Join(p.Off, ", "))
		case p.Say == "":
			ls = append(ls, "Nothing of it is left to take off.")
		}
	}
	if len(p.Stays) > 0 {
		ls = append(ls, "Stays: "+strings.Join(p.Stays, ", "))
	}
	if len(p.Also) > 0 {
		ls = append(ls, "Also uninstalls "+strings.Join(p.Also, ", ")+": they need it.")
	}
	if len(p.Forgets) > 0 {
		ls = append(ls, "⚠ "+strings.Join(p.Forgets, ", ")+" need it and will stop working: yardo forgets them (they stay installed).")
	}
	for _, b := range p.Back {
		ls = append(ls, b+", queued to install, needs it: it comes back.")
	}
	return ls
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
