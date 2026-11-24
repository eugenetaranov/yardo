package addon

import (
	"fmt"
	"slices"
	"strings"
)

// Fix makes fix (a finding's) to h's held files: remove:<file> deletes a
// file (or forgets an odd entry), drop-key:<key> drops a frontmatter key
// of SKILL.md or a setting of a data spec, drop-opt:<option> drops a
// Docker option. The files changed, h needs checking again.
func (h *Held) Fix(fix string) error {
	files, err := h.Files()
	if err != nil {
		return err
	}
	kind, arg, _ := strings.Cut(fix, ":")
	switch kind {
	case "remove":
		if i := slices.Index(h.Odd, arg); i >= 0 {
			h.Odd = slices.Delete(h.Odd, i, i+1)
			return h.Save()
		}
		if _, ok := files[arg]; !ok {
			return fmt.Errorf("%s: no such file", arg)
		}
		delete(files, arg)
	case "drop-key":
		if h.Kind == "skill" {
			b, ok := dropFront(files["SKILL.md"], arg)
			if !ok {
				return fmt.Errorf("SKILL.md has no %s", arg)
			}
			files["SKILL.md"] = b
			break
		}
		sp := ParseSpec(string(files["spec"]))
		if !sp.Data() {
			return fmt.Errorf("the spec has code: yardo can't edit it")
		}
		sp.Drop(arg)
		files["spec"] = []byte(sp.Text("Written by yardo from the spec as given, " + arg + " dropped."))
	case "drop-opt":
		sp := ParseSpec(string(files["spec"]))
		if !sp.Data() {
			return fmt.Errorf("the spec has code: yardo can't edit it")
		}
		sp.Set("DOCKER_ARGS", dropOpt(sp.Fields["DOCKER_ARGS"], arg))
		files["spec"] = []byte(sp.Text("Written by yardo from the spec as given, " + arg + " dropped."))
	default:
		return fmt.Errorf("%q: not a fix yardo makes", fix)
	}
	if err := h.setFiles(files); err != nil {
		return err
	}
	h.Report = Report{}
	return h.Save()
}

// dropFront is b (a SKILL.md) without frontmatter key, and the lines
// indented under it.
func dropFront(b []byte, key string) ([]byte, bool) {
	front, ok := frontLines(b)
	if !ok {
		return b, false
	}
	var kept []string
	dropping, found := false, false
	for _, l := range front {
		indented := strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")
		switch {
		case !indented && strings.TrimSpace(strings.SplitN(l, ":", 2)[0]) == key:
			dropping, found = true, true
			continue
		case dropping && (indented || strings.TrimSpace(l) == ""):
			continue
		}
		dropping = false
		kept = append(kept, l)
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	rest := s[4+len(strings.Join(front, "\n")):]
	return []byte("---\n" + strings.Join(kept, "\n") + rest), found
}

// dropOpt is args without option o ("--opt=value", as disallowed names
// it, given either way in args).
func dropOpt(args, o string) string {
	name, val, _ := strings.Cut(o, "=")
	f := strings.Fields(args)
	var out []string
	for i := 0; i < len(f); i++ {
		n, v, hasV := strings.Cut(f[i], "=")
		if n == name && val != "" && !hasV && i+1 < len(f) && f[i+1] == val {
			i++
			continue
		}
		if n == name && (v == val || !hasV && val == "") {
			continue
		}
		out = append(out, f[i])
	}
	return strings.Join(out, " ")
}
