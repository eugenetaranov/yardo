package addon

import (
	"bytes"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Subject is what the basic checks look at: a held add-on's files (a
// skill's folder, or an MCP spec as "spec"), and what's around it.
type Subject struct {
	Kind, Name string
	Files      map[string][]byte
	// Odd are the source's entries that weren't regular files.
	Odd []string
	// Pinned: the source was fixed to a commit or digest when fetched.
	Pinned bool
	// ImageUser is the user an image runs as ("" is root); Image: there is
	// an image.
	Image     bool
	ImageUser string
	// Taken are the names of the skills (or servers) there are already.
	Taken []string
}

// Limits of a skill, and of each file in it: past them it needs your
// OK, as a file that big can't be read through (a vendored compiler, a
// bundle).
const (
	maxFile  = 2 << 20
	maxTotal = 32 << 20
	maxCount = 1000
)

// Basic runs yardo's rules on s: blocks (can't be installed as it is)
// and notes, each with where and what to do.
func Basic(s Subject) []Finding {
	var out []Finding
	block := func(rule, file string, line int, text, next, fix string) {
		out = append(out, Finding{Layer: "basic", Rule: rule, Level: "block", File: file, Line: line, Text: text, Next: next, Fix: fix})
	}
	needsOK := func(rule, file string, line int, text, next, fix string) {
		out = append(out, Finding{Layer: "basic", Rule: rule, Level: "ok", File: file, Line: line, Text: text, Next: next, Fix: fix})
	}
	note := func(rule, file string, line int, text, next string) {
		out = append(out, Finding{Layer: "basic", Rule: rule, Level: "note", File: file, Line: line, Text: text, Next: next})
	}
	if !s.Pinned {
		block("pin", "", 0, "the source isn't fixed to a commit or an image digest", "add it again from a commit (owner/repo@<commit>) or an image by digest", "")
	}
	nameRule := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	switch {
	case len(s.Name) > 64 || !nameRule.MatchString(s.Name):
		block("name", "", 0, fmt.Sprintf("%q isn't a valid name (lowercase letters, digits and single hyphens, at most 64)", s.Name), "decline it, or rename it at its source", "")
	case slices.Contains(s.Taken, s.Name):
		block("name", "", 0, fmt.Sprintf("there's a %s named %s already", s.Kind, s.Name), "remove or rename the one there is first, then add this again", "")
	default:
		for _, t := range s.Taken {
			if d := distance(s.Name, t); d > 0 && d <= 2 && len(s.Name) >= 4 {
				note("near-name", "", 0, fmt.Sprintf("its name is %d letter%s from %s, which you have", d, map[bool]string{true: "", false: "s"}[d == 1], t), "make sure it isn't posing as "+t)
			}
		}
	}
	for _, o := range s.Odd {
		block("link", o, 0, "a link or special file, not a regular one", "remove it (it wasn't fetched), or decline", "remove:"+o)
	}
	total := 0
	for _, p := range slices.Sorted(maps.Keys(s.Files)) {
		b := s.Files[p]
		total += len(b)
		if !cleanRel(p) {
			block("path", p, 0, "a path that leaves its folder", "decline it", "")
			continue
		}
		if len(b) > maxFile {
			needsOK("size", p, 0, fmt.Sprintf("%.1f MB: too big to read through (%d MB a file)", float64(len(b))/(1<<20), maxFile>>20), "type its name to keep it as it is, or remove this file", "remove:"+p)
		}
		switch {
		case program(b) != "":
			block("program", p, 0, fmt.Sprintf("a program (%s): code that can't be read before it runs", program(b)), "remove this file", "remove:"+p)
		case archive(p, b) != "":
			block("archive", p, 0, fmt.Sprintf("an archive (%s): files that can't be read here", archive(p, b)), "remove this file", "remove:"+p)
		default:
			out = append(out, textRules(s, p, b)...)
		}
	}
	if total > maxTotal || len(s.Files) > maxCount {
		needsOK("size", "", 0, fmt.Sprintf("%d files, %d MB: more than can be read through (%d files, %d MB)", len(s.Files), total>>20, maxCount, maxTotal>>20), "type its name to keep it as it is, or decline it", "")
	}
	out = oneEach(out)
	if s.Kind == "skill" {
		out = append(out, frontmatter(s.Files["SKILL.md"])...)
		if s.Files["SKILL.md"] == nil {
			block("frontmatter", "SKILL.md", 0, "no SKILL.md: not a skill", "decline it", "")
		}
	}
	if s.Kind == "mcp" {
		out = append(out, specRules(s)...)
	}
	return out
}

// oneEach keeps one note per web address, where it's first, saying how
// many more places have it: a bundle names the same one a hundred times.
// A file with more than manyURLs gets one note for them all, by site.
func oneEach(fs []Finding) []Finding {
	var out []Finding
	seen := map[string]int{}
	more := map[string]int{}
	per := map[string][]string{} // a file's addresses, first-seen order
	for _, f := range fs {
		if f.Rule != "url" {
			out = append(out, f)
			continue
		}
		if n, ok := seen[f.Text]; ok {
			more[f.Text]++
			out[n].Next = fmt.Sprintf("check where it sends you, and what it fetches (%d more %s)", more[f.Text], map[bool]string{true: "place", false: "places"}[more[f.Text] == 1])
			continue
		}
		seen[f.Text] = len(out)
		per[f.File] = append(per[f.File], strings.TrimPrefix(f.Text, "a web address: "))
		out = append(out, f)
	}
	var kept []Finding
	said := map[string]bool{}
	for _, f := range out {
		us := per[f.File]
		if f.Rule != "url" || len(us) <= manyURLs {
			kept = append(kept, f)
			continue
		}
		if said[f.File] {
			continue
		}
		said[f.File] = true
		f.Text = fmt.Sprintf("%d web addresses: %s", len(us), sites(us))
		f.Next = "check where they send you, and what they fetch"
		kept = append(kept, f)
	}
	return kept
}

// manyURLs is how many web addresses a file has said one by one.
const manyURLs = 3

// sites are the hosts of us, the most named first, at most five.
func sites(us []string) string {
	n := map[string]int{}
	var hs []string
	for _, u := range us {
		h := strings.SplitN(strings.SplitN(u, "://", 2)[1], "/", 2)[0]
		if n[h] == 0 {
			hs = append(hs, h)
		}
		n[h]++
	}
	slices.SortStableFunc(hs, func(a, b string) int { return n[b] - n[a] })
	var out []string
	for _, h := range hs[:min(5, len(hs))] {
		if n[h] > 1 {
			h = fmt.Sprintf("%s ×%d", h, n[h])
		}
		out = append(out, h)
	}
	if len(hs) > 5 {
		out = append(out, fmt.Sprintf("%d more %s", len(hs)-5, map[bool]string{true: "site", false: "sites"}[len(hs) == 6]))
	}
	return strings.Join(out, ", ")
}

// program is the kind of program b is, by its first bytes, or "".
func program(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x7fELF")):
		return "ELF"
	case bytes.HasPrefix(b, []byte("MZ")):
		return "Windows"
	case bytes.HasPrefix(b, []byte("\x00asm")):
		return "WebAssembly"
	}
	for _, m := range [][]byte{{0xfe, 0xed, 0xfa, 0xce}, {0xfe, 0xed, 0xfa, 0xcf}, {0xce, 0xfa, 0xed, 0xfe}, {0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}} {
		if bytes.HasPrefix(b, m) {
			return "Mach-O"
		}
	}
	return ""
}

// archive is the kind of archive b is, or "".
func archive(p string, b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("PK\x03\x04")):
		return "zip"
	case bytes.HasPrefix(b, []byte{0x1f, 0x8b}):
		return "gzip"
	case len(b) > 262 && string(b[257:262]) == "ustar":
		return "tar"
	case bytes.HasPrefix(b, []byte("BZh")), bytes.HasPrefix(b, []byte{0xfd, '7', 'z', 'X', 'Z'}), bytes.HasPrefix(b, []byte("7z\xbc\xaf")):
		return "compressed"
	}
	switch strings.ToLower(p[strings.LastIndex(p, ".")+1:]) {
	case "whl", "jar", "tgz", "zip", "gz", "xz", "7z", "rar":
		return strings.ToLower(p[strings.LastIndex(p, ".")+1:])
	}
	return ""
}

// warning is a rule that notes each line its pattern matches.
type warning struct {
	rule, next string
	re         *regexp.Regexp
	say        func(m string) string
}

var warnings = []warning{
	{"url", "check where it sends you, and what it fetches", regexp.MustCompile(`https?://[^\s"'<>)\]]+`),
		func(m string) string { return "a web address: " + m }},
	{"fetch-run", "an agent would run what this fetches: read what it is first", regexp.MustCompile(`(?i)\b(curl|wget)\b|\|\s*(ba|z)?sh\b|base64\s+(-d|--decode)\b|\beval\b`),
		func(m string) string { return "fetch or run: " + m }},
	{"sudo", "an agent would act as root, or leave something running later", regexp.MustCompile(`\b(sudo|crontab)\b`),
		func(m string) string { return "root or later: " + m }},
	{"secrets", "a skill rarely needs your keys: make sure this one does", regexp.MustCompile(`(?i)~/\.(ssh|aws|kube|docker|gnupg|netrc|git-credentials|npmrc)\b|\bid_(rsa|ed25519|ecdsa)\b|(^|[\s/"'])\.env\b|\b(api[_-]?key|access[_-]?token|secret[_-]?key|auth[_-]?token)s?\b`),
		func(m string) string { return "your secrets: " + strings.TrimSpace(m) }},
	{"hide", "a skill that tells an agent to hide things, or drop its rules, is working against you", regexp.MustCompile(`(?i)ignore (all |any |your |the )?(previous|prior|above|earlier|system) (instructions|rules|prompt)|(do not|don't|never) (tell|inform|show|mention|reveal)( this)?( to)? the user|without (telling|asking|informing) the user|hide (this|it|what you)|keep (this|it) (secret|hidden)`),
		func(m string) string { return "hiding or overriding: " + m }},
}

// textRules are the warnings in a text file: each line that matches.
func textRules(s Subject, p string, b []byte) []Finding {
	var out []Finding
	hidden := false
	for n, line := range strings.Split(string(b), "\n") {
		for i, r := range line {
			if n == 0 && i == 0 && r == 0xFEFF {
				continue // a byte order mark
			}
			if hiddenRune(r) && code(p) && !steering(r) {
				// In code, a zero-width character or a direction mark in
				// a string is data (an entity table's &lrm;): said, not
				// blocked. What reorders or hides text still blocks.
				out = append(out, Finding{Layer: "basic", Rule: "unicode", Level: "note", File: p, Line: n + 1,
					Text: fmt.Sprintf("invisible character U+%04X in code", r), Next: "check it's in a string, as data"})
				break
			}
			if hiddenRune(r) {
				out = append(out, Finding{Layer: "basic", Rule: "unicode", Level: "block", File: p, Line: n + 1,
					Text: fmt.Sprintf("hidden character U+%04X: text an agent reads and you don't see", r), Next: "remove this file, or decline", Fix: "remove:" + p})
				hidden = true
				break
			}
		}
		if !utf8.ValidString(line) || hidden {
			continue
		}
		for _, w := range warnings {
			if m := w.re.FindString(line); m != "" && (w.rule != "url" || !xmlName.MatchString(m)) {
				out = append(out, Finding{Layer: "basic", Rule: w.rule, Level: "note", File: p, Line: n + 1, Text: w.say(m), Next: w.next})
			}
		}
		if p == "SKILL.md" && strings.Contains(line, "<!--") {
			out = append(out, Finding{Layer: "basic", Rule: "html-comment", Level: "note", File: p, Line: n + 1,
				Text: "an HTML comment: the agent reads it, a rendered page doesn't show it", Next: "read what it says"})
		}
	}
	switch path := p[strings.LastIndex(p, "/")+1:]; {
	case path == "package.json" && regexp.MustCompile(`"(pre|post)?install"\s*:|"prepare"\s*:`).Match(b):
		out = append(out, Finding{Layer: "basic", Rule: "install-script", Level: "note", File: p,
			Text: "package.json has an install script: npm install runs it", Next: "read the script before installing its packages"})
	case path == "setup.py":
		out = append(out, Finding{Layer: "basic", Rule: "install-script", Level: "note", File: p,
			Text: "setup.py: pip runs it to install", Next: "read it before installing the package"})
	}
	return out
}

// xmlName is an XML namespace's name: a web address nothing fetches.
var xmlName = regexp.MustCompile(`^https?://www\.w3\.org/(2000/svg|1999/xhtml|1999/xlink|1998/Math/MathML|XML/1998/namespace|2000/xmlns/?)$`)

// code: p is a program's source, not text an agent reads as words.
func code(p string) bool {
	switch strings.ToLower(p[strings.LastIndex(p, ".")+1:]) {
	case "js", "cjs", "mjs", "ts", "tsx", "jsx", "css", "go", "rs", "java", "py":
		return true
	}
	return false
}

// steering: r reorders text (direction overrides and isolates) or hides
// it in tags, so what's seen isn't what's there, in code too.
func steering(r rune) bool {
	return r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 || r >= 0xE0000 && r <= 0xE007F
}

// hiddenRune: r is a character that changes or hides text without being
// seen (direction controls, zero-width ones, tags).
func hiddenRune(r rune) bool {
	switch {
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r >= 0x200B && r <= 0x200F,
		r == 0x2060, r == 0xFEFF, r >= 0xE0000 && r <= 0xE007F:
		return true
	}
	return false
}

// frontmatter checks a SKILL.md's frontmatter: no hooks, no shell or
// write tools granted.
func frontmatter(b []byte) []Finding {
	front, ok := frontLines(b)
	if !ok {
		if b != nil {
			return []Finding{{Layer: "basic", Rule: "frontmatter", Level: "note", File: "SKILL.md", Line: 1, Text: "no frontmatter (name, description): agents may not load it", Next: "check it's a skill"}}
		}
		return nil
	}
	var out []Finding
	grants := regexp.MustCompile(`\b(Bash|Write|Edit|MultiEdit|NotebookEdit|Shell|Exec)\b`)
	for n, l := range front {
		k, v, _ := strings.Cut(l, ":")
		if strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") {
			continue
		}
		switch strings.TrimSpace(k) {
		case "hooks":
			out = append(out, Finding{Layer: "basic", Rule: "frontmatter", Level: "block", File: "SKILL.md", Line: n + 2,
				Text: "hooks: commands the agent runs on its own when the skill is used", Next: "drop the hooks (yardo edits its copy)", Fix: "drop-key:hooks"})
		case "allowed-tools", "tools":
			all := v
			for _, more := range front[n+1:] {
				if !strings.HasPrefix(more, " ") && !strings.HasPrefix(more, "\t") {
					break
				}
				all += " " + more
			}
			if m := grants.FindString(all); m != "" {
				out = append(out, Finding{Layer: "basic", Rule: "frontmatter", Level: "block", File: "SKILL.md", Line: n + 2,
					Text: fmt.Sprintf("it grants itself %s without asking you", m), Next: "drop the grant (yardo edits its copy): the agent asks as usual", Fix: "drop-key:" + strings.TrimSpace(k)})
			}
		}
	}
	return out
}

// frontLines are b's frontmatter lines, between its --- lines.
func frontLines(b []byte) ([]string, bool) {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return nil, false
	}
	front, _, ok := strings.Cut(s[4:], "\n---")
	if !ok {
		return nil, false
	}
	return strings.Split(front, "\n"), true
}

// specRules check an MCP spec from elsewhere: its code, its kind and pin,
// and its Docker options against what servers from elsewhere may have.
func specRules(s Subject) []Finding {
	var out []Finding
	block := func(rule string, line int, text, next, fix string) {
		out = append(out, Finding{Layer: "basic", Rule: rule, Level: "block", File: "spec", Line: line, Text: text, Next: next, Fix: fix})
	}
	sp := ParseSpec(string(s.Files["spec"]))
	if !sp.Data() {
		out = append(out, Finding{Layer: "basic", Rule: "code", Level: "ok", File: "spec", Line: sp.Code[0],
			Text: fmt.Sprintf("%d line%s of code, not just settings: it runs when the gateway reads it", len(sp.Code), map[bool]string{true: "", false: "s"}[len(sp.Code) == 1]),
			Next: "read the marked lines; installing needs your OK for this spec"})
	}
	line := func(k string) int {
		for n, l := range sp.Lines {
			if strings.HasPrefix(strings.TrimSpace(l), k+"=") {
				return n + 1
			}
		}
		return 0
	}
	switch sp.Fields["KIND"] {
	case "docker":
		if img := sp.Fields["IMAGE"]; !strings.Contains(img, "@sha256:") {
			block("pin", line("IMAGE"), fmt.Sprintf("IMAGE %s isn't pinned to a digest", img), "use the image by its digest (repo@sha256:…)", "")
		}
		if s.Image && (s.ImageUser == "" || s.ImageUser == "root" || s.ImageUser == "0" || strings.HasPrefix(s.ImageUser, "0:")) {
			out = append(out, Finding{Layer: "basic", Rule: "root-image", Level: "note", File: "spec", Line: line("IMAGE"),
				Text: "the image runs as root inside its container", Next: "yardo runs it as nobody: if its files are root's alone, it won't start"})
		}
	case "http":
		if _, err := CheckURL(sp.Fields["URL"]); err != nil && sp.Fields["URL_VAR"] == "" {
			block("pin", line("URL"), err.Error(), "give an https address", "")
		}
	default:
		if sp.Data() {
			block("kind", line("KIND"), fmt.Sprintf("KIND %q: neither docker nor http", sp.Fields["KIND"]), "decline it", "")
		}
	}
	for _, k := range []string{"HOST_ACCESS", "MOUNT_AWS", "MOUNT_KUBE"} {
		if v, ok := sp.Fields[k]; ok && v != "" && v != "0" {
			block("docker", line(k), fmt.Sprintf("%s=%s: access to the machine, or your cloud keys, that only yardo's own servers get", k, v), "drop it (yardo edits its copy)", "drop-key:"+k)
		}
	}
	if v, ok := sp.Fields["LIMITS"]; ok && v != "" {
		block("docker", line("LIMITS"), fmt.Sprintf("LIMITS=%s: a server from elsewhere runs with every limit", v), "drop it (yardo edits its copy)", "drop-key:LIMITS")
	}
	for _, o := range disallowed(sp.Fields["DOCKER_ARGS"]) {
		block("docker", line("DOCKER_ARGS"), fmt.Sprintf("Docker option %s: not one a server from elsewhere may have", o), "drop it (yardo edits its copy)", "drop-opt:"+o)
	}
	if !sp.Data() {
		for _, k := range []string{"DOCKER_ARGS", "HOST_ACCESS", "MOUNT_AWS", "MOUNT_KUBE", "LIMITS", "NETWORK"} {
			if regexp.MustCompile(fmt.Sprintf("\\b%s\\b", k)).Match(s.Files["spec"]) && sp.Fields[k] == "" {
				block("docker", 0, k+" is set by code: yardo can't tell what it is", "decline it, or ask its author for a spec of settings only", "")
			}
		}
	}
	return out
}

// disallowed are the options in args a server from elsewhere may not
// have: everything but -e/--env, --tmpfs, --memory, --cpus, --network
// none, --read-only, --cap-drop and no-new-privileges.
func disallowed(args string) []string {
	var out []string
	f := strings.Fields(args)
	for i := 0; i < len(f); i++ {
		o, v, hasV := strings.Cut(f[i], "=")
		// A flag alone; any other option takes the next word as its value.
		flag := map[string]bool{"--read-only": true, "--privileged": true, "--init": true, "--rm": true, "-i": true, "-t": true, "-it": true, "-P": true, "--publish-all": true}
		if !flag[o] && !hasV && i+1 < len(f) && !strings.HasPrefix(f[i+1], "-") {
			v = f[i+1]
			i++
		}
		ok := false
		switch o {
		case "-e", "--env", "--tmpfs", "-m", "--memory", "--cpus", "--cap-drop", "--read-only":
			ok = true
		case "--network", "--net":
			ok = v == "none"
		case "--security-opt":
			ok = v == "no-new-privileges" || v == "no-new-privileges:true" || v == "no-new-privileges=true"
		}
		if !ok {
			if v != "" {
				o += "=" + v
			}
			out = append(out, o)
		}
	}
	return out
}

// distance is the edit distance between a and b.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
