// Package mcp reads the MCP server specs yardo offers (the shell
// files mcp-sync uses: DESCRIPTION, IMAGE or URL, REQUIRED, and a
// skeleton() that prints the env file template).
package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Spec is one MCP server.
type Spec struct {
	Name, Description, Image, URL string
	Required                      []string
	// Keys are the skeleton's active KEY= lines; Optional its commented
	// #KEY= ones. Defaults has a value the skeleton sets.
	Keys, Optional []string
	Defaults       map[string]string
	// Help is the comment right above a KEY= line (how to get its value),
	// without the "# ".
	Help map[string]string
	// Skeleton is the env file a new server starts with.
	Skeleton string
}

// Dirs are where the specs are, first match wins: the installed catalog,
// then the copies yardo ships (tack-roles', then yardo's own).
func Dirs(share string) []string {
	return []string{"/usr/local/share/mcp-catalog", filepath.Join(share, "mcp-catalog"), filepath.Join(share, "conf", "mcp-catalog")}
}

// Names lists every spec name in dirs, sorted.
func Names(dirs []string) []string {
	seen := map[string]bool{}
	for _, d := range dirs {
		m, _ := filepath.Glob(filepath.Join(d, "*.spec"))
		for _, f := range m {
			seen[strings.TrimSuffix(filepath.Base(f), ".spec")] = true
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Load reads spec name from the first dir that has it (bash sources it).
func Load(ctx context.Context, dirs []string, name string) (*Spec, error) {
	path := ""
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, name+".spec")); err == nil {
			path = filepath.Join(d, name+".spec")
			break
		}
	}
	if path == "" {
		return nil, fmt.Errorf("no MCP spec %q", name)
	}
	const script = `set -eu
. "$1"
printf '%s\0%s\0%s\0%s\0' "${DESCRIPTION:-}" "${IMAGE:-}" "${URL:-}" "${REQUIRED:-}"
skeleton 2>/dev/null || true`
	out, err := exec.CommandContext(ctx, "bash", "-c", script, "_", path).Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f := strings.SplitN(string(out), "\x00", 5)
	if len(f) < 5 {
		return nil, fmt.Errorf("%s: unreadable", path)
	}
	s := &Spec{Name: name, Description: f[0], Image: f[1], URL: f[2], Required: strings.Fields(f[3]), Defaults: map[string]string{}, Help: map[string]string{}, Skeleton: f[4]}
	var comment []string // the comment lines since the last other line
	for _, line := range strings.Split(f[4], "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, "=")
		if opt, isOpt := strings.CutPrefix(k, "#"); ok && isOpt && validKey(opt) {
			s.Optional = append(s.Optional, opt)
			comment = nil
			continue
		}
		if c, isComment := strings.CutPrefix(line, "#"); isComment {
			comment = append(comment, strings.TrimPrefix(c, " "))
			continue
		}
		if ok && validKey(k) {
			s.Keys = append(s.Keys, k)
			if v != "" {
				s.Defaults[k] = v
			}
			if len(comment) > 0 {
				s.Help[k] = strings.Join(comment, "\n")
			}
		}
		comment = nil
	}
	return s, nil
}

func validKey(k string) bool {
	if k == "" || k[0] < 'A' || k[0] > 'Z' {
		return false
	}
	for _, c := range k {
		if c != '_' && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// IsSecret says whether a key holds a secret (read without echo).
func IsSecret(k string) bool {
	return strings.Contains(k, "TOKEN") || strings.Contains(k, "SECRET") || strings.Contains(k, "PASSWORD") ||
		strings.HasSuffix(k, "_KEY") || strings.HasSuffix(k, "_KEY_ID") || strings.Contains(k, "APIKEY")
}

// Instance splits a picked server into its spec and its name. "grafana" is
// the catalog's Grafana (spec and name grafana); "grafana:prod" is another
// one of the same spec, named grafana-prod: its ~/.mcp directory, the
// gateway's server name and its tools' prefix (grafana-prod__<tool>).
func Instance(entry string) (spec, name string, err error) {
	spec, label, ok := strings.Cut(entry, ":")
	if !ValidName(spec) {
		return "", "", fmt.Errorf("%q: not an MCP server", entry)
	}
	if !ok {
		return spec, spec, nil
	}
	if !ValidLabel(label) {
		return "", "", fmt.Errorf("%q: name it with lowercase letters, digits and '-' (e.g. %s:prod)", entry, spec)
	}
	return spec, spec + "-" + label, nil
}

// ValidLabel says whether l can name another instance of a server: short,
// lowercase letters, digits and inner dashes.
func ValidLabel(l string) bool {
	if l == "" || len(l) > 24 || l[0] == '-' || l[len(l)-1] == '-' || strings.Contains(l, "--") {
		return false
	}
	for _, c := range l {
		if c != '-' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// ValidName says whether n is a server's name, as mcp-sync takes them: a
// directory in ~/.mcp, never a path.
func ValidName(n string) bool {
	if n == "" || len(n) > 40 || n[0] == '.' || n[0] == '-' || strings.Contains(n, "..") || strings.Contains(n, "__") {
		return false
	}
	for _, c := range n {
		if c != '.' && c != '_' && c != '-' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// EntryFor is the entry for a new server of spec, named typed: "jira"
// (the catalog's), or "work" or "jira-work" for another one (jira:work);
// or why it can't be. isSpec says which names are the catalog's, taken
// which servers there are.
func EntryFor(spec, typed string, isSpec func(string) bool, taken []string) (string, error) {
	typed = strings.TrimSpace(typed)
	entry := spec
	switch {
	case typed == "":
		return "", errors.New("give it a name")
	case typed != spec:
		label := strings.TrimPrefix(typed, spec+"-")
		switch {
		case label == spec:
			return "", fmt.Errorf("that's %s itself: name it %s", spec, spec)
		case !ValidLabel(label):
			return "", errors.New("use lowercase letters, digits and '-', such as work")
		case isSpec(spec + "-" + label):
			return "", fmt.Errorf("%s-%s is a server of its own; name it otherwise", spec, label)
		}
		entry = spec + ":" + label
	}
	if _, name, _ := Instance(entry); slices.Contains(taken, name) {
		return "", fmt.Errorf("there's a %s already", name)
	}
	return entry, nil
}

// RenameEntry is entry renamed to typed, as EntryFor names a new one; a
// server with its own spec (not the catalog's) takes any name.
func RenameEntry(entry, typed string, isSpec func(string) bool, taken []string) (string, error) {
	spec, name, err := Instance(entry)
	if err != nil {
		return "", err
	}
	typed = strings.TrimSpace(typed)
	if typed == name {
		return "", errors.New("that's its name already")
	}
	taken = slices.DeleteFunc(slices.Clone(taken), func(n string) bool { return n == name })
	if isSpec(spec) {
		return EntryFor(spec, typed, isSpec, taken)
	}
	switch {
	case typed == "":
		return "", errors.New("give it a name")
	case !ValidName(typed) || strings.Contains(typed, ":"):
		return "", errors.New("use letters, digits, '.', '_' and '-'")
	case isSpec(typed) || slices.Contains(taken, typed):
		return "", fmt.Errorf("there's a %s already", typed)
	}
	return typed, nil
}
