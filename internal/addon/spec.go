package addon

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Spec is an MCP spec from elsewhere, read without running it: its
// KEY=value lines, and the lines that are anything else (code).
type Spec struct {
	Fields map[string]string
	Order  []string
	// Code are the line numbers (from 1) that aren't data: a function, a
	// command, a substitution, a here-doc.
	Code []int
	// Lines are the spec's lines, for showing it with the code marked.
	Lines []string
}

var (
	specKey = regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)=(.*)$`)
	// A bare value: nothing the shell would expand or split.
	bareValue = regexp.MustCompile(`^[A-Za-z0-9_./:@%+,=-]*$`)
)

// ParseSpec reads text as data: blank lines, comments, and KEY="value",
// KEY='value' or KEY=value lines whose value the shell takes literally.
// Every other line is code.
func ParseSpec(text string) Spec {
	s := Spec{Fields: map[string]string{}, Lines: strings.Split(strings.TrimRight(text, "\n"), "\n")}
	heredoc := ""
	for n, l := range s.Lines {
		t := strings.TrimSpace(l)
		if heredoc != "" {
			s.Code = append(s.Code, n+1)
			if t == heredoc {
				heredoc = ""
			}
			continue
		}
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		v, k, ok := specValue(t)
		if !ok {
			s.Code = append(s.Code, n+1)
			if _, end, found := strings.Cut(t, "<<"); found {
				heredoc = strings.Trim(strings.TrimPrefix(strings.TrimSpace(end), "-"), `'" `)
			}
			continue
		}
		if _, seen := s.Fields[k]; !seen {
			s.Order = append(s.Order, k)
		}
		s.Fields[k] = v
	}
	return s
}

// specValue is t's key and value, when t is a data line.
func specValue(t string) (value, key string, ok bool) {
	m := specKey.FindStringSubmatch(t)
	if m == nil {
		return "", "", false
	}
	v := m[2]
	switch {
	case len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"':
		in := v[1 : len(v)-1]
		if strings.ContainsAny(in, "$`\\\"") {
			return "", "", false
		}
		return in, m[1], true
	case len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'':
		in := v[1 : len(v)-1]
		if strings.Contains(in, "'") {
			return "", "", false
		}
		return in, m[1], true
	case bareValue.MatchString(v):
		return v, m[1], true
	}
	return "", "", false
}

// Data: the spec is only data.
func (s Spec) Data() bool { return len(s.Code) == 0 }

// Text is the spec's data written back by yardo, each value quoted so the
// shell takes it literally.
func (s Spec) Text(header string) string {
	var b strings.Builder
	if header != "" {
		for _, l := range strings.Split(strings.TrimRight(header, "\n"), "\n") {
			b.WriteString("# " + l + "\n")
		}
	}
	for _, k := range s.Order {
		fmt.Fprintf(&b, "%s=%s\n", k, quote(s.Fields[k]))
	}
	return b.String()
}

// quote is v, on one line, as the shell takes it literally: in double
// quotes, or single ones when it has what double quotes would expand.
func quote(v string) string {
	v = strings.Join(strings.Fields(strings.ReplaceAll(v, "\x00", "")), " ")
	if !strings.ContainsAny(v, "$`\\\"") {
		return `"` + v + `"`
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// Set sets key to v, in order.
func (s *Spec) Set(key, v string) {
	if s.Fields == nil {
		s.Fields = map[string]string{}
	}
	if _, ok := s.Fields[key]; !ok {
		s.Order = append(s.Order, key)
	}
	s.Fields[key] = v
}

// Drop removes key.
func (s *Spec) Drop(key string) {
	delete(s.Fields, key)
	s.Order = slices.DeleteFunc(s.Order, func(k string) bool { return k == key })
}

// Form is what the user says about a server from an image or a URL.
type Form struct {
	Name, Description string
	// Image is repo@sha256:… (a docker server) or URL an https address.
	Image, URL string
	// Required are the settings it needs, in its env file.
	Required []string
	// Network: it needs the network (an image; phase 5's limits).
	Network bool
}

// FormSpec is the data spec a form makes.
func FormSpec(f Form) Spec {
	var s Spec
	s.Set("DESCRIPTION", f.Description)
	if f.URL != "" {
		s.Set("KIND", "http")
		s.Set("URL", f.URL)
		s.Set("AUTH", "none")
	} else {
		s.Set("KIND", "docker")
		s.Set("IMAGE", f.Image)
		s.Set("NETWORK", map[bool]string{true: "1", false: "0"}[f.Network])
	}
	s.Set("REQUIRED", strings.Join(f.Required, " "))
	return s
}
