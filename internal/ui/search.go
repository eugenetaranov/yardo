// Package ui is yardo's terminal UI: the tabbed picker with
// type-to-search across every tab, and the prompts.
package ui

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// MaxResults caps the search results.
const MaxResults = 200

// Entry is one row the picker can show: a catalog item (or an MCP server).
type Entry struct {
	Tab, ID, Label, Source string
	On                     bool
}

var wordRe = regexp.MustCompile(`[\w.+-]+`)

// toolName: "pipx:aider-chat[x]@latest" → "aider-chat".
func toolName(spec string) string {
	spec = strings.SplitN(spec, "@", 2)[0]
	spec = strings.SplitN(spec, "[", 2)[0]
	if i := strings.LastIndexByte(spec, ':'); i >= 0 {
		spec = spec[i+1:]
	}
	if i := strings.LastIndexByte(spec, '/'); i >= 0 {
		spec = spec[i+1:]
	}
	return strings.ToLower(spec)
}

// MiseTools returns the tools a source installs with mise ("mise use -g a
// b" or "mise pipx:x", between "·" separators).
func MiseTools(source string) map[string]bool {
	out := map[string]bool{}
	for _, seg := range strings.Split(source, "·") {
		w := strings.Fields(seg)
		if len(w) == 0 || w[0] != "mise" {
			continue
		}
		if len(w) >= 3 && w[1] == "use" && w[2] == "-g" {
			w = w[3:]
		} else {
			w = w[1:]
		}
		for _, t := range w {
			out[toolName(t)] = true
		}
	}
	return out
}

func alnum(b byte) bool {
	r := rune(b)
	return r >= 0x80 || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// fuzzy scores q as a substring, else (when allowed) an in-order
// subsequence, of s (both lower case); ok=false when it doesn't match.
func fuzzy(q, s string, subsequence bool) (int, bool) {
	l := len(s)
	if l > 200 {
		l = 200
	}
	if i := strings.Index(s, q); i >= 0 {
		if i == 0 || !alnum(s[i-1]) {
			return 300 - l/20, true
		}
		return 200 - l/20, true
	}
	if !subsequence {
		return 0, false
	}
	pos, prev, score := 0, -2, 0
	for i := 0; i < len(q); i++ {
		j := strings.IndexByte(s[pos:], q[i])
		if j < 0 {
			return 0, false
		}
		j += pos
		if j == prev+1 {
			score += 5
		}
		if j == 0 || !alnum(s[j-1]) {
			score += 8
		}
		prev, pos = j, j+1
	}
	return score - l/20, true
}

func namesOf(e *Entry) (primary, secondary []string) {
	secondary = wordRe.FindAllString(strings.ToLower(e.Label), -1)
	var tools []string
	for t := range MiseTools(e.Source) {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	return []string{strings.ToLower(e.ID)}, append(secondary, tools...)
}

func nameScore(q, n string) (int, bool) {
	switch {
	case n == q:
		return 1000, true
	case strings.HasPrefix(n, q):
		return 900 - len(n), true
	case strings.Contains(n, q):
		return 600 - len(n), true
	}
	return 0, false
}

// Score ranks e for q (higher is better; ok=false: no match). Primary
// names first (exact, prefix, substring), then secondary names, then a
// fuzzy match on the label (a mise tool's names), then the source or
// description as a substring. Catalog items rank ahead of mise tools.
func Score(e *Entry, q string) (int, bool) {
	q = strings.ToLower(q)
	primary, secondary := namesOf(e)
	best, ok := 0, false
	take := func(s int, m bool) {
		if m && (!ok || s > best) {
			best, ok = s, true
		}
	}
	for _, n := range primary {
		take(nameScore(q, n))
	}
	for _, n := range secondary {
		if s, m := nameScore(q, n); m {
			take(s-150, true)
		}
	}
	take(fuzzy(q, strings.ToLower(e.Label), true))
	if s, m := fuzzy(q, strings.ToLower(e.Source), false); m {
		take(s-50, true)
	}
	return best, ok
}

// strict reports whether q is a substring of e's names, label, source or
// description (not only an in-order subsequence of them).
func strict(e *Entry, q string) bool {
	q = strings.ToLower(q)
	primary, secondary := namesOf(e)
	for _, n := range append(primary, secondary...) {
		if strings.Contains(n, q) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(e.Label), q) || strings.Contains(strings.ToLower(e.Source), q)
}

// Search returns the entries matching q, best first. Subsequence-only
// matches ("lzgt" for lazygit) count only when nothing has q as a
// substring, so "ssh" doesn't list every label with s…s…h in it.
func Search(entries []*Entry, q string) []*Entry {
	type hit struct {
		s, n int
		e    *Entry
	}
	var hits, loose []hit
	for n, e := range entries {
		if s, ok := Score(e, q); ok {
			if strict(e, q) {
				hits = append(hits, hit{s, n, e})
			} else {
				loose = append(loose, hit{s, n, e})
			}
		}
	}
	if len(hits) == 0 {
		hits = loose
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.s != b.s {
			return a.s > b.s
		}
		if len(a.e.Label) != len(b.e.Label) {
			return len(a.e.Label) < len(b.e.Label)
		}
		return a.n < b.n
	})
	if len(hits) > MaxResults {
		hits = hits[:MaxResults]
	}
	out := make([]*Entry, len(hits))
	for i, h := range hits {
		out[i] = h.e
	}
	return out
}
