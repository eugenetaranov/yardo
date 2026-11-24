package addon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
func Basic(s Subject) []Finding { return Scan(s).Findings }

// Scan runs yardo's rules on s: its findings, one per rule and file,
// what it can do (Caps), each file's role, and what it read.
func Scan(s Subject) Report {
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
	rl := roles(s.Files)
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
			out = append(out, big(p, b, rl[p]))
		}
		switch {
		case program(b) != "":
			block("program", p, 0, fmt.Sprintf("a program (%s): code that can't be read before it runs", program(b)), "remove this file", "remove:"+p)
		case archive(p, b) != "":
			block("archive", p, 0, fmt.Sprintf("an archive (%s): files that can't be read here", archive(p, b)), "remove this file", "remove:"+p)
		default:
			out = append(out, textRules(p, b, rl[p])...)
		}
	}
	if total > maxTotal || len(s.Files) > maxCount {
		needsOK("size", "", 0, fmt.Sprintf("%d files, %d MB in all: too much for Claude's review; yardo's rules read every line", len(s.Files), total>>20), "install it as it is (you type its name), or decline it", "")
	}
	caps, cfs := capsOf(s, rl)
	out = append(out, cfs...)
	if s.Kind == "skill" {
		out = append(out, frontmatter(s.Files["SKILL.md"])...)
		if s.Files["SKILL.md"] == nil {
			block("frontmatter", "SKILL.md", 0, "no SKILL.md: not a skill", "decline it", "")
		}
	}
	if s.Kind == "mcp" {
		out = append(out, specRules(s)...)
	}
	return Report{Findings: onePer(out, rl), Caps: caps, Files: rl,
		Coverage: []Coverage{{Layer: "basic", Files: len(s.Files), All: len(s.Files)}}}
}

// big is the finding of file p, too big for Claude's review: it needs
// your OK when it's the add-on's own; bundled code or an asset is a note
// worth a look; a doc or licence, a note.
func big(p string, b []byte, role string) Finding {
	f := Finding{Layer: "basic", Rule: "size", Level: "note", File: p, Fix: "remove:" + p,
		Text: fmt.Sprintf("%.1f MB: too big for Claude's review; yardo's rules read every line", float64(len(b))/(1<<20))}
	first, _, _ := bytes.Cut(b[:min(len(b), 400)], []byte("\n"))
	if utf8.Valid(first) && len(bytes.TrimSpace(first)) > 0 {
		f.Why = append(f.Why, fmt.Sprintf("It starts %q.", clean(string(first), 100)))
	}
	switch {
	case own(role):
		f.Level = "ok"
		f.Next = "install it as it is (you type its name), or remove the file: the skill may not work without it"
	case role == "vendored" || role == "asset":
		f.Look = true
		f.Why = append(f.Why, "It's bundled from elsewhere: Claude's review skips it.")
		f.Next = "install it as it is, or remove it from the held copy: what uses it then won't work"
	default:
		f.Next = "nothing to do: the agent isn't told to read it"
	}
	return f
}

// onePer keeps one finding per layer, rule and file: the worst, where
// it's first, with More the other places. Each gets its file's role, its
// kind in words and an ID: its layer, rule, file and the keys of what it
// matched (a finding's ID, as made, is its key).
func onePer(fs []Finding, rl map[string]string) []Finding {
	type key struct{ layer, rule, file string }
	var out []Finding
	at := map[key]int{}
	keys := map[int][]string{}
	for _, f := range fs {
		k := key{f.Layer, f.Rule, f.File}
		n, ok := at[k]
		if !ok || f.File == "" {
			at[k] = len(out)
			keys[len(out)] = []string{f.ID}
			out = append(out, f)
			continue
		}
		keys[n] = append(keys[n], f.ID)
		o := &out[n]
		more, look := o.More+f.More+1, o.Look || f.Look
		if rank(f.Level) > rank(o.Level) {
			*o = f
		}
		o.More, o.Look = more, look
	}
	for n := range out {
		f := &out[n]
		if f.Role == "" && f.File != "" {
			f.Role = rl[f.File]
		}
		if f.Kind == "" {
			f.Kind = ruleWords[f.Rule]
		}
		f.Look = f.Look || f.Level != "note"
		f.ID = findingID(f.Layer, f.Rule, f.File, keys[n]...)
	}
	return out
}

// findingID names a finding across reports: by layer, rule, file and the
// keys of what it matched (not the line).
func findingID(layer, rule, file string, keys ...string) string {
	keys = slices.Compact(slices.Sorted(slices.Values(keys)))
	s := sha256.Sum256([]byte(strings.Join(append([]string{layer, rule, file}, keys...), "\x00")))
	return hex.EncodeToString(s[:6])
}

// ruleWords say what each rule finds, in a few words.
var ruleWords = map[string]string{
	"pin": "Not fixed to a version", "name": "Its name", "near-name": "A name like one you have", "link": "A link, not a file",
	"path": "A path out of its folder", "program": "A program", "archive": "An archive", "size": "Not read through",
	"unicode": "Hidden characters", "url": "Web addresses", "fetch-run": "Runs what it fetches", "sudo": "Acts as root, or later",
	"secrets": "Names your secrets", "hide": "Hides things from you", "html-comment": "A hidden comment",
	"frontmatter": "Hooks or tools it grants itself", "code": "Code in its spec", "docker": "Docker options",
	"kind": "Not a kind yardo runs", "root-image": "Runs as root",
}

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

// Patterns of the text rules.
var (
	// Fetched and run: piped to a shell (in shell, a spec, markdown).
	pipeRun = regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^|\n]*\|\s*(?:sudo\s+)?(?:ba|z|da|k)?sh\b|\bbase64\s+(?:-d|--decode|-D)\b[^|\n]*\|\s*(?:sudo\s+)?(?:ba|z)?sh\b|\b(?:ba)?sh\s+-c\s+["']?\$\(\s*(?:curl|wget)\b|\bsource\s+<\(\s*(?:curl|wget)\b`)
	// Text run as code: a call in code, not in a string or comment.
	jsEval = regexp.MustCompile(`(?:^|[^.\w$])eval\s*\(|\bnew\s+Function\s*\(`)
	pyEval = regexp.MustCompile(`(?:^|[^.\w])(?:eval|exec)\s*\(`)
	sudoRe = regexp.MustCompile(`\b(sudo|crontab)\b`)
	// Secrets, by path or name.
	secretWords = regexp.MustCompile(`(?i)~/\.(ssh|aws|kube|docker|gnupg|netrc|git-credentials|npmrc)\b|\bid_(rsa|ed25519|ecdsa)\b|(^|[\s/"'])\.env\b|\b(api[_-]?key|access[_-]?token|secret[_-]?key|auth[_-]?token)s?\b`)
	// Hiding from the user, or overriding the agent's rules: not "do not
	// tell the user a link is clickable", nor "do not hide its target".
	hideRe      = regexp.MustCompile(`(?i)\b(?:ignore|disregard|forget) (?:all |any |your |the )?(?:previous|prior|above|earlier|system|other) (?:instructions|rules|prompts?)\b|\b(?:do not|don't|never) (?:tell|inform|show|mention|reveal|disclose)(?: this| it| that| any of this)?(?: to)? the user(?:\s*$|\s*[.,;:!)]|\s+(?:about|that|what|where|how|why|anything|this|it)\b)|\bwithout (?:telling|asking|informing|notifying) the user\b|\bhide (?:this|it|that|what you\w*|your \w+) from (?:the user|them|your user)\b|\bkeep (?:this|it) (?:secret|hidden) from (?:the user|them)\b|\bkeep (?:this|it) (?:secret|hidden)\s*[.!]?\s*$`)
	htmlComment = regexp.MustCompile(`<!--(.*?)(?:-->|$)`)
)

// textRules are the rules on text file p of role (see Roles): what's in
// the instructions needs your OK; in bundled code or docs, it's a note.
func textRules(p string, b []byte, role string) []Finding {
	var out []Finding
	add := func(rule, level string, line int, text, next, key string, look bool) {
		out = append(out, Finding{Layer: "basic", Rule: rule, Level: level, File: p, Line: line, Text: text, Next: next,
			Look: look || level != "note", ID: key})
	}
	lg := lang(p, b)
	md := isMD(p)
	var code []string
	if lg == "js" || lg == "py" {
		c, _ := strip(b, lg)
		code = strings.Split(string(c), "\n")
	}
	shellish := lg == "sh" || md
	words := shellish || strings.HasSuffix(p, ".txt")
	var urls []string
	urlLine := 0
	hidden, fence := false, false
	for n, line := range strings.Split(string(b), "\n") {
		for i, r := range line {
			if i == 0 && ascii(line) {
				break
			}
			if n == 0 && i == 0 && r == 0xFEFF || !hiddenRune(r) {
				continue // a byte order mark, or seen
			}
			key := fmt.Sprintf("%04X", r)
			switch {
			case steering(r):
				out = append(out, Finding{Layer: "basic", Rule: "unicode", Level: "block", File: p, Line: n + 1, ID: key,
					Text: fmt.Sprintf("hidden character U+%04X: text an agent reads and you don't see", r), Next: "remove this file, or decline", Fix: "remove:" + p})
				hidden = true
			case role == "instructions":
				add("unicode", "ok", n+1, fmt.Sprintf("invisible character U+%04X in what the agent reads", r), "read the line: it may say more than you see", key, true)
			case lg != "":
				// In code, a zero-width character or a direction mark in a
				// string is data (an entity table's &lrm;).
				add("unicode", "note", n+1, fmt.Sprintf("invisible character U+%04X in code", r), "check it's in a string, as data", key, false)
			default:
				add("unicode", "note", n+1, fmt.Sprintf("invisible character U+%04X", r), "the agent isn't told to read this file", key, false)
			}
			break
		}
		if !utf8.ValidString(line) || hidden {
			continue
		}
		if md && strings.HasPrefix(strings.TrimSpace(line), "```") {
			fence = !fence
		}
		if strings.Contains(line, "://") {
			for _, m := range urlRe.FindAllString(line, -1) {
				if m = strings.TrimRight(m, "`;,.:"); !xmlName.MatchString(m) {
					if urlLine == 0 {
						urlLine = n + 1
					}
					urls = append(urls, m)
				}
			}
		}
		// Fetched and run (piped to a shell); text run as code.
		m := ""
		switch {
		case shellish:
			m = pipeRun.FindString(line)
		case lg == "js" && n < len(code) && (strings.Contains(code[n], "eval") || strings.Contains(code[n], "Function")):
			m = jsEval.FindString(code[n])
		case lg == "py" && n < len(code) && (strings.Contains(code[n], "eval") || strings.Contains(code[n], "exec")):
			m = pyEval.FindString(code[n])
		}
		if m = strings.TrimLeft(m, " \t=(,;!&|"); m != "" {
			level := map[bool]string{true: "ok", false: "note"}[role == "instructions"]
			add("fetch-run", level, n+1, "fetch or run: "+strings.TrimSpace(m), "an agent would run what this fetches or makes: read what it is first",
				strings.Join(strings.Fields(strings.ToLower(m)), " "), role == "script" || role == "code")
		}
		if !words {
			continue
		}
		look := role == "instructions" || role == "script"
		if m := sudoRe.FindString(line); m != "" {
			add("sudo", "note", n+1, "root or later: "+m, "an agent would act as root, or leave something running later", m, look)
		}
		if m := secretWords.FindString(line); m != "" {
			m = strings.TrimSpace(m)
			add("secrets", "note", n+1, "your secrets: "+m, "a skill rarely needs your keys: make sure this one does", strings.ToLower(m), look)
		}
		if m := hideRe.FindString(line); m != "" && look {
			m = strings.TrimSpace(m)
			level := map[bool]string{true: "ok", false: "note"}[role == "instructions"]
			add("hide", level, n+1, "hiding or overriding: "+m, "read what it hides, and from whom", strings.ToLower(m), true)
		}
		if md {
			for _, c := range htmlComment.FindAllStringSubmatchIndex(line, -1) {
				if fence || strings.Count(line[:c[0]], "`")%2 == 1 {
					continue // shown as code, not hidden
				}
				// A marker (<!-- toc -->) is a note; words, or more lines,
				// in the instructions need your OK.
				long := !strings.HasSuffix(line[c[0]:c[1]], "-->") || len(strings.Fields(line[c[2]:c[3]])) >= 4
				level := map[bool]string{true: "ok", false: "note"}[role == "instructions" && long]
				add("html-comment", level, n+1, "an HTML comment: the agent reads it, a rendered page doesn't show it", "read what it says", "", false)
				break
			}
		}
	}
	switch u := slices.Compact(slices.Sorted(slices.Values(urls))); {
	case len(urls) == 0:
	case len(u) == 1 && len(urls) == 1:
		add("url", "note", urlLine, "a web address: "+urls[0], "check where it sends you, and what it fetches", "", false)
	case len(u) == 1:
		add("url", "note", urlLine, fmt.Sprintf("a web address: %s (%d places)", u[0], len(urls)), "check where it sends you, and what it fetches", "", false)
	default:
		add("url", "note", urlLine, fmt.Sprintf("%d web addresses: %s", len(urls), sites(urls)), "check where they send you; what its code calls is under what it can do", "", false)
	}
	return out
}

// xmlName is an XML namespace's name: a web address nothing fetches.
var xmlName = regexp.MustCompile(`^https?://www\.w3\.org/(2000/svg|1999/xhtml|1999/xlink|1998/Math/MathML|XML/1998/namespace|2000/xmlns/?)$`)

// ascii: s has no byte past ASCII (so no hidden character).
func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
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
