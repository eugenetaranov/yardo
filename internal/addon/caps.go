package addon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// The capability pass: what an add-on's code can do (start programs,
// reach the network, read your home, write outside its folder) and what
// its instructions have the agent do, from the code itself, not what it
// says it does.

// lang is the language p (content b) is in, for the capability pass: js,
// py, sh, go, or "" (not code it reads).
func lang(p string, b []byte) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".mts", ".cts":
		return "js"
	case ".py":
		return "py"
	case ".sh", ".bash", ".zsh":
		return "sh"
	case ".go":
		return "go"
	}
	if p == "spec" {
		return "sh"
	}
	if bytes.HasPrefix(b, []byte("#!")) {
		l, _, _ := bytes.Cut(b[:min(len(b), 120)], []byte("\n"))
		switch s := string(l); {
		case strings.Contains(s, "node"), strings.Contains(s, "deno"), strings.Contains(s, "bun"):
			return "js"
		case strings.Contains(s, "python"):
			return "py"
		case strings.Contains(s, "sh"):
			return "sh"
		}
	}
	return ""
}

// strip blanks b's comments to spaces, keeping its lines: text. code has
// its strings' (and regular expressions') contents blanked too, but in
// shell, where they're commands: what's left in code is a call it makes,
// not one it mentions.
func strip(b []byte, lang string) (code, text []byte) {
	code, text = bytes.Clone(b), bytes.Clone(b)
	blank := func(i, j int, both bool) {
		for k := i; k < j && k < len(b); k++ {
			if b[k] != '\n' {
				code[k] = ' '
				if both {
					text[k] = ' '
				}
			}
		}
	}
	hash := lang == "py" || lang == "sh"
	for i := 0; i < len(b); {
		c, next := b[i], byte(0)
		if i+1 < len(b) {
			next = b[i+1]
		}
		switch {
		case !hash && c == '/' && next == '/', hash && c == '#' && (lang == "py" || i == 0 || b[i-1] == ' ' || b[i-1] == '\t' || b[i-1] == '\n' || b[i-1] == ';'):
			j := i + bytes.IndexByte(b[i:], '\n')
			if j < i {
				j = len(b)
			}
			blank(i, j, true)
			i = j
		case !hash && c == '/' && next == '*':
			j := bytes.Index(b[i+2:], []byte("*/"))
			if j < 0 {
				j = len(b)
			} else {
				j += i + 4
			}
			blank(i, j, true)
			i = j
		case c == '\'' || c == '"' || c == '`' && lang != "py":
			j := strEnd(b, i, lang)
			if lang != "sh" {
				blank(i+1, j-1, false)
			}
			i = j
		case lang == "js" && c == '/' && regexStart(code, i):
			j := regexEnd(b, i)
			if j < 0 {
				i++
				continue
			}
			blank(i+1, j-1, false)
			i = j
		default:
			i++
		}
	}
	return code, text
}

// strEnd is where the string at b[i] ends, after its quote (or at its
// line's end, unclosed).
func strEnd(b []byte, i int, lang string) int {
	q := b[i]
	if lang == "py" && bytes.HasPrefix(b[i:], []byte{q, q, q}) {
		j := bytes.Index(b[i+3:], []byte{q, q, q})
		if j < 0 {
			return len(b)
		}
		return i + 6 + j
	}
	for j := i + 1; j < len(b); j++ {
		switch {
		case b[j] == '\\' && (lang != "sh" || q != '\''):
			j++
		case b[j] == q:
			return j + 1
		case b[j] == '\n' && q != '`' && lang != "sh":
			return j
		}
	}
	return len(b)
}

// regexStart: the / at b[i] starts a regular expression, not a division,
// by what's before it (in code, comments blanked).
func regexStart(code []byte, i int) bool {
	k := i - 1
	for k >= 0 && (code[k] == ' ' || code[k] == '\t' || code[k] == '\n' || code[k] == '\r') {
		k--
	}
	if k < 0 || strings.IndexByte("(,=:[!&|?{};+-*%<>~^", code[k]) >= 0 {
		return true
	}
	e := k + 1
	for k >= 0 && (code[k] == '_' || code[k] == '$' || code[k] >= 'a' && code[k] <= 'z' || code[k] >= 'A' && code[k] <= 'Z' || code[k] >= '0' && code[k] <= '9') {
		k--
	}
	switch string(code[k+1 : e]) {
	case "return", "typeof", "case", "in", "of", "new", "delete", "void", "throw", "yield", "await", "else", "do":
		return true
	}
	return false
}

// regexEnd is where the regular expression at b[i] ends, after its flags;
// -1 when it isn't one (no closing / on its line).
func regexEnd(b []byte, i int) int {
	class := false
	for j := i + 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			j++
		case '\n':
			return -1
		case '[':
			class = true
		case ']':
			class = false
		case '/':
			if !class {
				return j + 1
			}
		}
	}
	return -1
}

// lineOf is the line (from 1) of offset off, nl being b's newlines.
func lineOf(nl []int, off int) int { return sort.SearchInts(nl, off) + 1 }

func newlines(b []byte) []int {
	var nl []int
	for i, c := range b {
		if c == '\n' {
			nl = append(nl, i)
		}
	}
	return nl
}

// use is one place a capability is used: what (a program, a host), and
// whether it's worth the user's eyes, at what level.
type use struct {
	kind, what string
	line       int
	look       bool
	level      string
}

// detail says more of a file's capability, for its finding.
type detail map[string]string

// Patterns of each language's calls, matched in code (strings blanked).
var (
	jsExec    = regexp.MustCompile(`(?:^|[^.\w$])(?:spawn|spawnSync|exec|execSync|execFile|execFileSync|fork)\s*\(|\b(?:child_process|childProcess|cp)\.(?:spawn|spawnSync|exec|execSync|execFile|execFileSync|fork)\s*\(|\bBun\.spawn(?:Sync)?\s*\(|\bDeno\.(?:Command|run)\b`)
	pyExec    = regexp.MustCompile(`\bsubprocess\.(?:run|call|check_call|check_output|Popen)\s*\(|\bos\.(?:system|popen|exec\w*|spawn\w*)\s*\(`)
	goExec    = regexp.MustCompile(`\bexec\.Command(?:Context)?\s*\(`)
	shExec    = regexp.MustCompile("\\$\\(|`")
	jsNet     = regexp.MustCompile(`(?:^|[^.\w$])fetch\s*\(|\b(?:globalThis|window|self)\.fetch\b|\bhttps?\.(?:request|get)\s*\(|\baxios\b|\bnew\s+WebSocket\s*\(|\bXMLHttpRequest\b|\bnet\.(?:connect|createConnection)\s*\(|\btls\.connect\s*\(|\bundici\b`)
	pyNet     = regexp.MustCompile(`\brequests\.(?:get|post|put|patch|delete|head|request|Session)\b|\burllib\.request\b|\burlopen\s*\(|\bhttpx\.|\baiohttp\.|\bhttp\.client\b|\bsocket\.(?:socket|create_connection)\s*\(`)
	goNet     = regexp.MustCompile(`\bhttp\.(?:Get|Post|Head|PostForm|NewRequest\w*|DefaultClient)\b|\bnet\.Dial\w*\s*\(`)
	shNet     = regexp.MustCompile(`\b(?:curl|wget|nc|ncat)\b`)
	jsEnv     = regexp.MustCompile(`\bprocess\.env\b|\b(?:Deno|Bun)\.env\b`)
	pyEnv     = regexp.MustCompile(`\bos\.environ\b|\bgetenv\s*\(`)
	goEnv     = regexp.MustCompile(`\bos\.(?:Getenv|LookupEnv|Environ)\s*\(`)
	shEnv     = regexp.MustCompile(`\$\{?[A-Z_][A-Z0-9_]*\b`)
	jsHome    = regexp.MustCompile(`\bhomedir\s*\(\s*\)|\b(?:process\.)?env\.(?:HOME|USERPROFILE)\b`)
	pyHome    = regexp.MustCompile(`\bexpanduser\s*\(|\bPath\.home\s*\(`)
	goHome    = regexp.MustCompile(`\bos\.UserHomeDir\s*\(`)
	shHome    = regexp.MustCompile(`\$\{?HOME\b|(?:^|[\s=:"'])~/`)
	tildeHome = regexp.MustCompile("['\"`]~/")
	jsWrite   = regexp.MustCompile(`\b\w*\.(?:writeFile|writeFileSync|appendFile|appendFileSync|mkdir|mkdirSync|rm|rmSync|rmdir|rmdirSync|rename|renameSync|copyFile|copyFileSync|cp|cpSync|createWriteStream|symlink|symlinkSync|unlink|unlinkSync)\s*\(|(?:^|[^.\w$])(?:writeFile|writeFileSync|appendFile|appendFileSync|mkdir|mkdirSync|createWriteStream)\s*\(`)
	pyWrite   = regexp.MustCompile(`\.write_(?:text|bytes)\s*\(|\bshutil\.(?:copy\w*|move|rmtree)\s*\(|\bos\.(?:makedirs|mkdir|remove|unlink|rename|replace|rmdir)\s*\(`)
	pyOpenW   = regexp.MustCompile(`\bopen\s*\([^\n]*?,\s*(?:mode\s*=\s*)?['"][wax]b?\+?['"]`)
	goWrite   = regexp.MustCompile(`\bos\.(?:WriteFile|Create|OpenFile|Mkdir|MkdirAll|Remove|RemoveAll|Rename|Symlink)\s*\(`)
	shWrite   = regexp.MustCompile(`(?:^|[^0-9&>])>>?\s*["']?(?:~/|\$HOME|\$\{HOME\})|\b(?:mkdir|cp|mv|rm|tee|ln|touch)\s`)
	tokenName = regexp.MustCompile(`\b[A-Z][A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|PASSWD|API_KEY|APIKEY|ACCESS_KEY|PRIVATE_KEY|CREDENTIALS?)\b`)
	secretRe  = regexp.MustCompile("(?:^|[^\\w.])\\.(?:ssh|aws|gnupg|netrc|kube/config|git-credentials|npmrc|pypirc|docker/config\\.json)\\b|\\.config/gh\\b|\\b[Kk]eychain\\b|\\bsecurity\\s+find-(?:generic|internet)-password\\b|['\"`/]\\.env['\"`]")
	// What's outside its folder that's the agent's, or starts on its own.
	targetRe = regexp.MustCompile(`(?i)~/\.claude\b|['"/]\.claude['"/]|\.codex\b|\bCLAUDE\.md\b|\bAGENTS\.md\b|\bGEMINI\.md\b|settings(?:\.local)?\.json|\.git/hooks|\.bashrc|\.zshrc|\.bash_profile|\.zprofile|LaunchAgents|\bcrontab\b|\.config/autostart|systemd/user|['"]/etc/`)
	envRead  = regexp.MustCompile(`\benv\b|environ|getenv|Getenv|\$\{?[A-Z_]`)
	relative = regexp.MustCompile("(?:fetch|get|request)\\s*\\(\\s*['\"`]/")
	cacheRe  = regexp.MustCompile(`(?i)cache`)
	urlRe    = regexp.MustCompile(`https?://[^\s"'<>)\]` + "`" + `]+`)
)

// prog is the program a call starts, from what follows its "(": a name
// given as it is (process.execPath is node), "" when worked out.
var prog = regexp.MustCompile(`^\s*\[?\s*(?:\w+\s*,\s*)?(?:(process\.execPath|sys\.executable)|['"` + "`" + `]([^'"` + "`" + `\s$]+))`)

func progName(rest []byte) string {
	m := prog.FindSubmatch(rest[:min(len(rest), 200)])
	switch {
	case m == nil:
		return workedProg
	case string(m[1]) == "process.execPath":
		return "node"
	case string(m[1]) == "sys.executable":
		return "python"
	}
	return path.Base(string(m[2]))
}

// constURL is a web address a file names a constant for.
type constURL struct{ file, url string }

var (
	jsConst = regexp.MustCompile("(?m)^\\s*(?:export\\s+)?(?:const|let|var)\\s+([A-Za-z_$][\\w$]*)\\s*=\\s*['\"`](https?://[^'\"`\\s]+)['\"`]")
	pyConst = regexp.MustCompile(`(?m)^([A-Za-z_]\w*)\s*(?::\s*\w+\s*)?=\s*['"](https?://[^'"\s]+)['"]`)
	urlName = regexp.MustCompile(`(?i)url|uri|endpoint|host|api|base|server|origin`)
)

// consts are the web addresses own files give names to, by name: where a
// call's address comes from when it isn't in the call.
func consts(files map[string][]byte, rl map[string]string) map[string][]constURL {
	out := map[string][]constURL{}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		re := map[string]*regexp.Regexp{"js": jsConst, "py": pyConst}[lang(p, files[p])]
		if re == nil || rl[p] == "vendored" {
			continue
		}
		b := files[p]
		nl := newlines(b)
		for _, m := range re.FindAllSubmatchIndex(b, -1) {
			if n := string(b[m[2]:m[3]]); urlName.MatchString(n) {
				out[n] = append(out[n], constURL{fmt.Sprintf("%s:%d", p, lineOf(nl, m[2])), string(b[m[4]:m[5]])})
			}
		}
	}
	return out
}

// host is a web address's host, "" if none.
func host(u string) string {
	_, rest, ok := strings.Cut(u, "://")
	if !ok {
		return ""
	}
	end := strings.IndexFunc(rest, func(r rune) bool {
		return !strings.ContainsRune(".-:[]", r) && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	if end >= 0 {
		rest = rest[:end]
	}
	if h, _, ok := strings.Cut(rest, ":"); ok && !strings.HasPrefix(rest, "[") {
		rest = h
	}
	return strings.ToLower(rest)
}

// local: h is this machine.
func local(h string) bool {
	return h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasPrefix(h, "127.") || h == "0.0.0.0" || h == "[::1]" || h == "::1" || h == ""
}

const (
	thisMachine = "this machine"
	workedProg  = "programs it works out when it runs"
	workedOut   = "an address it works out when it runs"
)

// capsIn are file p's uses of what it can do, and the details of them.
func capsIn(p string, b []byte, lg string, cs map[string][]constURL) ([]use, detail) {
	code, text := strip(b, lg)
	nl := newlines(b)
	var out []use
	d := detail{}
	at := func(off int) int { return lineOf(nl, off) }
	each := func(re *regexp.Regexp, buf []byte, f func(s, e int)) { matches(re, buf, f) }
	add := func(kind, what string, line int) { out = append(out, use{kind: kind, what: what, line: line}) }
	pick := func(m map[string]*regexp.Regexp) *regexp.Regexp { return m[lg] }

	// Starts programs.
	if re := pick(map[string]*regexp.Regexp{"js": jsExec, "py": pyExec, "go": goExec, "sh": shExec}); re != nil &&
		(lg != "js" || bytes.Contains(b, []byte("child_process")) || bytes.Contains(b, []byte("Bun.spawn")) || bytes.Contains(b, []byte("Deno."))) {
		each(re, code, func(s, e int) {
			what := "shell commands"
			if lg != "sh" {
				what = progName(b[e:])
			}
			add("exec", what, at(s))
		})
	}
	// Reaches the network: the hosts of addresses on its lines, of the
	// constants it uses, or this machine for a path alone.
	if re := pick(map[string]*regexp.Regexp{"js": jsNet, "py": pyNet, "go": goNet, "sh": shNet}); re != nil {
		var lines []int
		each(re, code, func(s, _ int) { lines = append(lines, at(s)) })
		if len(lines) > 0 {
			hosts := map[string]string{} // host → its address, and where it's from
			for _, l := range lines {
				ln := lineText(b, nl, l)
				for _, u := range urlRe.FindAllString(ln, -1) {
					hosts[host(u)] = strings.TrimRight(u, ".,;")
				}
				if relative.MatchString(ln) {
					hosts[""] = ""
				}
			}
			var uses []int // where a constant's address it has is used
			for _, n := range slices.Sorted(maps.Keys(cs)) {
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(n) + `\b`)
				if !re.Match(code) {
					continue
				}
				here, has := false, false
				for _, c := range cs[n] {
					f, _, _ := strings.Cut(c.file, ":")
					if f == p || bytes.Contains(b, []byte(strings.TrimSuffix(path.Base(f), path.Ext(f)))) {
						here, has = here || f == p, true
						if _, ok := hosts[host(c.url)]; !ok {
							hosts[host(c.url)] = fmt.Sprintf("%s (%s, from %s)", c.url, n, c.file)
						}
					}
				}
				if !has {
					continue
				}
				ms := re.FindAllIndex(code, -1)
				if !here {
					ms = ms[1:] // its import
				}
				for _, m := range ms {
					uses = append(uses, at(m[0]))
				}
			}
			var whats, says []string
			for _, h := range slices.Sorted(maps.Keys(hosts)) {
				switch {
				case local(h):
					whats = append(whats, thisMachine)
				default:
					whats = append(whats, h)
					says = append(says, hosts[h])
				}
			}
			if len(whats) == 0 {
				whats = []string{workedOut}
			}
			whats = slices.Compact(whats)
			d["network"] = strings.Join(says, "; ")
			// Called through a variable (fetchImpl = globalThis.fetch): the
			// call nearest where the address is used comes first.
			if l := nearest(code, nl, uses); l > 0 && !slices.Contains(lines, l) {
				lines = append([]int{l}, lines...)
			}
			for _, l := range lines {
				for _, w := range whats {
					u := use{kind: "network", what: w, line: l}
					if w != thisMachine {
						u.look, u.level = true, "ok"
					}
					out = append(out, u)
				}
			}
		}
	}
	// Reads the environment, and the tokens in it.
	if re := pick(map[string]*regexp.Regexp{"js": jsEnv, "py": pyEnv, "go": goEnv, "sh": shEnv}); re != nil {
		env := false
		each(re, code, func(s, _ int) { env = true; add("env", "environment variables", at(s)) })
		if env {
			// A token's name read from it: worth a look where the file
			// reaches a host elsewhere, where it can send it.
			remote := slices.ContainsFunc(out, func(u use) bool { return u.kind == "network" && u.look })
			each(tokenName, text, func(s, e int) {
				if envRead.MatchString(lineText(text, nl, at(s))) {
					out = append(out, use{kind: "env", what: string(b[s:e]), line: at(s), look: remote, level: map[bool]string{true: "note"}[remote]})
				}
			})
		}
	}
	// Uses the home folder; names secrets in it.
	homeRe := pick(map[string]*regexp.Regexp{"js": jsHome, "py": pyHome, "go": goHome, "sh": shHome})
	home := false
	if homeRe != nil {
		each(homeRe, code, func(s, _ int) { home = true; add("home", "your home folder", at(s)) })
	}
	if lg != "sh" {
		each(tildeHome, text, func(s, _ int) { home = true; add("home", "your home folder", at(s)) })
	}
	if home {
		each(secretRe, text, func(s, e int) {
			out = append(out, use{kind: "secrets", what: secretName(string(b[s:e])), line: at(s), look: true, level: "ok"})
		})
	}
	// Writes: next to a path of the agent's or one that starts on its
	// own, that; else a cache or the home folder it uses; else where it's
	// run.
	writes := []*regexp.Regexp{pick(map[string]*regexp.Regexp{"js": jsWrite, "py": pyWrite, "go": goWrite, "sh": shWrite})}
	if lg == "py" {
		writes = append(writes, pyOpenW)
	}
	cache := cacheRe.Match(text)
	for i, re := range writes {
		if re == nil {
			continue
		}
		buf := code
		if i > 0 {
			buf = text
		}
		each(re, buf, func(s, _ int) {
			l := at(s)
			near := lineRange(text, nl, l-3, l+3)
			if ms := targetRe.FindAllString(string(near), -1); ms != nil {
				// The most telling: settings over the folder they're in.
				what := targetName(ms[0])
				for _, m := range ms {
					if t := targetName(m); t != "files in your agent's folder" {
						what = t
						break
					}
				}
				out = append(out, use{kind: "writes", what: what, line: l, look: true, level: "ok"})
				return
			}
			switch {
			case home && cache:
				add("writes", "a cache in your home folder", l)
			case home:
				add("writes", "files in your home folder", l)
			default:
				add("writes", "files where it's run", l)
			}
		})
	}
	return out, d
}

// keywords are what a line has to have for a pattern to match it: only
// those lines are matched, as bundled code runs to megabytes.
var keywords = map[*regexp.Regexp][]string{
	jsExec: {"spawn", "exec", "fork", "Bun.", "Deno."}, pyExec: {"subprocess.", "os."}, goExec: {"exec.Command"}, shExec: {"$(", "`"},
	fetchCall: {"etch"}, jsNet: {"fetch", "http.", "https.", "axios", "WebSocket", "XMLHttpRequest", "net.", "tls.", "undici"},
	pyNet: {"requests.", "urllib", "urlopen", "httpx", "aiohttp", "http.client", "socket."}, goNet: {"http.", "net.Dial"},
	shNet: {"curl", "wget", "nc"}, jsEnv: {".env"}, pyEnv: {"environ", "getenv"}, goEnv: {"os."}, shEnv: {"$"},
	jsHome: {"homedir", "HOME", "USERPROFILE"}, pyHome: {"expanduser", "Path.home"}, goHome: {"UserHomeDir"}, shHome: {"HOME", "~/"},
	tildeHome: {"~/"}, jsWrite: {"writeFile", "appendFile", "mkdir", "rm(", "rmSync", "rmdir", "rename", "copyFile", "cp(", "cpSync", "createWriteStream", "symlink", "unlink"},
	pyWrite: {"write_", "shutil.", "os."}, pyOpenW: {"open"}, goWrite: {"os."}, shWrite: {">", "mkdir", "cp", "mv", "rm", "tee", "ln", "touch"},
	tokenName: {"TOKEN", "SECRET", "PASSW", "API_KEY", "APIKEY", "ACCESS_KEY", "PRIVATE_KEY", "CREDENTIAL"},
	secretRe:  {".ssh", ".aws", ".gnupg", ".netrc", ".kube", ".git-credentials", ".npmrc", ".pypirc", ".docker", ".config/gh", "eychain", "security", ".env"},
}

// matches calls f with each match of re in b (its start and end), line
// by line, on the lines that have one of its keywords.
func matches(re *regexp.Regexp, b []byte, f func(s, e int)) {
	kw := keywords[re]
	for off := 0; off < len(b); {
		l := b[off:]
		if i := bytes.IndexByte(l, '\n'); i >= 0 {
			l = l[:i]
		}
		if kw == nil || slices.ContainsFunc(kw, func(k string) bool { return bytes.Contains(l, []byte(k)) }) {
			for _, m := range re.FindAllIndex(l, -1) {
				f(off+m[0], off+m[1])
			}
		}
		off += len(l) + 1
	}
}

// fetchCall is a call of something named for fetching (fetchImpl(…)),
// not its definition.
var fetchCall = regexp.MustCompile(`(?:^|[^\w$.])\w*[fF]etch\w*\s*\(`)

// nearest is the line of the fetch-named call in code nearest one of
// lines (0: none, or no lines).
func nearest(code []byte, nl []int, lines []int) int {
	best, dist := 0, 0
	if len(lines) == 0 {
		return 0
	}
	matches(fetchCall, code, func(s, _ int) {
		l := lineOf(nl, s)
		if strings.Contains(lineText(code, nl, l), "function") {
			return
		}
		for _, u := range lines {
			if d := max(l-u, u-l); best == 0 || d < dist {
				best, dist = l, d
			}
		}
	})
	return best
}

// lineText is line l (from 1) of b, nl its newlines.
func lineText(b []byte, nl []int, l int) string { return string(lineRange(b, nl, l, l)) }

// lineRange are lines from to to (from 1, inclusive) of b.
func lineRange(b []byte, nl []int, from, to int) []byte {
	from, to = max(from, 1), min(to, len(nl)+1)
	s, e := 0, len(b)
	if from > 1 {
		s = nl[from-2] + 1
	}
	if to <= len(nl) {
		e = nl[to-1]
	}
	if s > e {
		return nil
	}
	return b[s:e]
}

// secretName says a secret a match names.
func secretName(m string) string {
	m = strings.Trim(m, "'\"`/ \t")
	switch {
	case strings.Contains(strings.ToLower(m), "keychain"), strings.HasPrefix(m, "security"):
		return "your keychain"
	case m == ".env":
		return ".env files"
	}
	if i := strings.Index(m, "."); i >= 0 {
		m = m[i:]
	}
	return "~/" + m
}

// targetName says what a path of the agent's (or one that starts on its
// own) is.
func targetName(m string) string {
	l := strings.ToLower(strings.Trim(m, `'"/`))
	switch {
	case strings.Contains(l, "settings"):
		return "your agent's settings"
	case strings.Contains(l, "hooks"):
		return "git hooks"
	case strings.Contains(l, "bashrc"), strings.Contains(l, "zshrc"), strings.Contains(l, "profile"):
		return "your shell profile"
	case strings.Contains(l, "launchagents"), strings.Contains(l, "crontab"), strings.Contains(l, "autostart"), strings.Contains(l, "systemd"):
		return "what runs at login"
	case strings.HasPrefix(l, "/etc"), l == "etc/":
		return "the system's settings (/etc)"
	case strings.HasSuffix(l, ".md"):
		return "your global rules file"
	}
	return "files in your agent's folder"
}

// Instructions that have the agent change what's outside the project:
// its rules, settings, hooks, your shell profile.
var (
	agentVerb   = regexp.MustCompile(`(?i)\b(?:write|writes|edit|edits|add|adds|append|appends|replace|replaces|update|updates|modify|modifies|insert|inserts|create|creates|change|changes|overwrite|overwrites|delete|deletes|remove|removes)\b`)
	agentTarget = regexp.MustCompile(`(?i)global rules|rules file|~/\.claude\b|~/\.codex\b|\bCLAUDE\.md\b|\bAGENTS\.md\b|\bGEMINI\.md\b|\.cursorrules|settings(?:\.local)?\.json|\.claude/hooks|\.bashrc|\.zshrc|\.bash_profile|\.zprofile`)
	agentAsked  = regexp.MustCompile(`(?i)\b(?:only )?(?:when|if) the user asks|\bwhen asked\b|\bunless (?:the user )?asks\b|\bonly on request\b`)
	homePath    = regexp.MustCompile(`~/[\w./-]+`)
	// A description that has it act unasked.
	unasked = regexp.MustCompile(`(?i)\bby default\b|\bunasked\b|\balways\b|\bproactive(?:ly)?\b|\bautomatically\b|\bwithout (?:being )?asked\b|\bwhenever\b|\bevery (?:session|time|response|answer)\b`)
)

// agentLine says what a line has the agent change (with the paths it
// names), and whether only when asked.
type agentLine struct {
	say   string
	asked bool
}

// agentUses are what instructions b have the agent change outside the
// project, and each line's say, by line.
func agentUses(b []byte) ([]use, map[int]agentLine) {
	var out []use
	say := map[int]agentLine{}
	for n, l := range strings.Split(string(b), "\n") {
		t := ""
		for _, sn := range strings.SplitAfter(l, ". ") {
			if m := agentTarget.FindStringIndex(sn); m != nil && slices.ContainsFunc(agentVerb.FindAllStringIndex(sn[:m[0]], -1), func(v []int) bool { return m[0]-v[1] <= 160 }) {
				t = sn[m[0]:m[1]]
				break
			}
		}
		if t == "" {
			continue // no verb leads to it, in a sentence
		}
		what := "your global rules file"
		switch tl := strings.ToLower(t); {
		case strings.Contains(tl, "settings"):
			what = "your agent's settings"
		case strings.Contains(tl, "hooks"):
			what = "your agent's hooks"
		case strings.Contains(tl, "rc"), strings.Contains(tl, "profile"):
			what = "your shell profile"
		}
		u := use{kind: "agent", what: "edits " + what, line: n + 1, look: true, level: "ok"}
		if agentAsked.MatchString(l) {
			u.what += " when you ask"
		}
		out = append(out, u)
		paths := slices.Compact(slices.Sorted(slices.Values(homePath.FindAllString(l, -1))))
		s := what
		if len(paths) > 0 {
			s = fmt.Sprintf("%s (%s)", what, strings.Join(paths, ", "))
		}
		say[n+1] = agentLine{s, agentAsked.MatchString(l)}
	}
	return out, say
}

// description is a SKILL.md's frontmatter description.
func description(b []byte) string {
	front, _ := frontLines(b)
	for _, l := range front {
		if k, v, ok := strings.Cut(l, ":"); ok && strings.TrimSpace(k) == "description" {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// unaskedIn says what in d has it act unasked ("by default unasked"), or "".
func unaskedIn(d string) string {
	m := unasked.FindAllStringIndex(d, -1)
	switch {
	case m == nil:
		return ""
	case len(m) > 1 && m[len(m)-1][1]-m[0][0] <= 40:
		return d[m[0][0]:m[len(m)-1][1]]
	}
	return d[m[0][0]:m[0][1]]
}

// capKinds are the kinds, in the order they're said; capWords each kind
// as a finding says it.
var (
	capKinds = []string{"agent", "grants", "hooks", "install", "network", "secrets", "writes", "exec", "env", "home"}
	capWords = map[string]string{
		"agent": "Changes your agent's global rules", "grants": "Grants itself tools", "hooks": "Runs commands on its own",
		"install": "Runs code when installed", "network": "Contacts the network", "secrets": "Reads your secrets",
		"writes": "Writes outside its folder", "exec": "Starts programs", "env": "Reads a token from the environment",
		"home": "Uses your home folder",
	}
)

// whyRole says why a finding in a file of role r matters.
func whyRole(r, p string) string {
	switch {
	case p == "spec":
		return "The gateway runs the spec's code when it reads it."
	case r == "instructions":
		return fmt.Sprintf("The agent reads %s each time the skill is used: this is an instruction it follows, not code.", p)
	case r == "script":
		return "The agent is told to run this script, so it happens when you use the skill."
	case r == "code":
		return "It's the skill's own code: it runs when a script that loads it does."
	case r == "vendored":
		return "It's bundled code from elsewhere: it runs when what uses it does."
	}
	return ""
}

// capsOf are what s's files can do (Caps, each kind's own and bundled
// apart) and a finding for each worth the user's eyes, a file each.
func capsOf(s Subject, rl map[string]string) ([]Cap, []Finding) {
	type at struct {
		p string
		u use
	}
	var all []at
	var fs []Finding
	cs := consts(s.Files, rl)
	details := map[string]detail{}
	says := map[string]map[int]agentLine{}
	for _, p := range slices.Sorted(maps.Keys(s.Files)) {
		b, r := s.Files[p], rl[p]
		if r == "instructions" && isMD(p) {
			us, say := agentUses(b)
			says[p] = say
			for _, u := range us {
				all = append(all, at{p, u})
			}
		}
		lg := lang(p, b)
		if lg == "" || !own(r) && r != "vendored" || r == "instructions" && p != "spec" {
			continue
		}
		us, d := capsIn(p, b, lg, cs)
		details[p] = d
		for _, u := range us {
			if r == "vendored" && (!u.look || u.kind != "secrets" && u.kind != "writes") {
				u.what, u.look, u.level = "bundled code", false, ""
			}
			all = append(all, at{p, u})
		}
	}
	if sk := s.Files["SKILL.md"]; s.Kind == "skill" && sk != nil {
		if d := unaskedIn(description(sk)); d != "" {
			all = append(all, at{"SKILL.md", use{kind: "agent", what: fmt.Sprintf("acts unasked (its description says %q)", d), line: 3}})
		}
		g, h := grantsOf(sk)
		for _, u := range g {
			all = append(all, at{"SKILL.md", u})
		}
		if h > 0 {
			all = append(all, at{"SKILL.md", use{kind: "hooks", what: "commands on the agent's events", line: h, look: true, level: "block"}})
		}
	}
	for _, p := range slices.Sorted(maps.Keys(s.Files)) {
		if rl[p] == "vendored" {
			continue
		}
		switch path.Base(p) {
		case "package.json":
			for _, u := range installScripts(s.Files[p]) {
				all = append(all, at{p, u})
			}
		case "setup.py":
			all = append(all, at{p, use{kind: "install", what: "setup.py", line: 1, look: true, level: "note"}})
		}
	}

	// Caps: a kind's uses, its own worth a look, its own, and bundled.
	type key struct {
		kind       string
		look, vend bool
	}
	caps := map[key]*Cap{}
	places := map[key]map[string]bool{}
	for _, a := range all {
		k := key{a.u.kind, a.u.look, rl[a.p] == "vendored"}
		c := caps[k]
		if c == nil {
			c = &Cap{Kind: a.u.kind, Look: a.u.look}
			caps[k], places[k] = c, map[string]bool{}
		}
		if !slices.Contains(strings.Split(c.What, ", "), a.u.what) {
			c.What = strings.TrimPrefix(c.What+", "+a.u.what, ", ")
		}
		w := fmt.Sprintf("%s:%d", a.p, a.u.line)
		if !places[k][w] {
			places[k][w] = true
			c.Count++
			if len(c.Where) < 3 {
				c.Where = append(c.Where, w)
			}
		}
	}
	keys := slices.Collect(maps.Keys(caps))
	slices.SortFunc(keys, func(a, b key) int {
		if d := slices.Index(capKinds, a.kind) - slices.Index(capKinds, b.kind); d != 0 {
			return d
		}
		if a.look != b.look {
			return map[bool]int{true: -1, false: 1}[a.look]
		}
		return map[bool]int{true: 1, false: -1}[a.vend]
	})
	// What's worth a look isn't said again in the kind's other caps;
	// programs it works out come last.
	var out []Cap
	for _, k := range keys {
		c := *caps[k]
		items := strings.Split(c.What, ", ")
		if l := caps[key{k.kind, true, false}]; !k.look && l != nil {
			items = slices.DeleteFunc(items, func(w string) bool { return slices.Contains(strings.Split(l.What, ", "), w) })
		}
		if i := slices.Index(items, workedProg); i >= 0 {
			items = append(slices.Delete(items, i, i+1), workedProg)
		}
		if len(items) == 0 {
			continue
		}
		c.What = strings.Join(items, ", ")
		out = append(out, c)
	}

	// A finding for each file's uses worth a look (of a kind, the worst).
	type fk struct{ p, kind string }
	var order []fk
	group := map[fk][]use{}
	for _, a := range all {
		if !a.u.look || a.u.level == "block" {
			continue // a block is the frontmatter rule's
		}
		k := fk{a.p, a.u.kind}
		if group[k] == nil {
			order = append(order, k)
		}
		group[k] = append(group[k], a.u)
	}
	for _, k := range order {
		us := group[k]
		level, whats, lines := "note", []string{}, map[int]bool{}
		for _, u := range us {
			if rank(u.level) > rank(level) {
				level = u.level
			}
			if !slices.Contains(whats, u.what) {
				whats = append(whats, u.what)
			}
			lines[u.line] = true
		}
		f := Finding{Layer: "basic", Rule: "cap-" + k.kind, Level: level, File: k.p, Line: us[0].line, More: len(lines) - 1,
			Kind: capWords[k.kind], Look: true, ID: strings.Join(slices.Sorted(slices.Values(whats)), ",")}
		if w := whyRole(rl[k.p], k.p); w != "" {
			f.Why = append(f.Why, w)
		}
		list := and(whats)
		switch k.kind {
		case "network":
			f.Text = fmt.Sprintf("Contacts %s.", list)
			if d := details[k.p]["network"]; d != "" {
				f.Text = fmt.Sprintf("Contacts %s: %s.", list, d)
			}
			f.Why = append(f.Why, "That host sees each request: that you use it, and when.")
			if slices.Contains(whats, workedOut) {
				f.Why = append(f.Why, "yardo can't tell where to: the address is worked out when it runs.")
			}
			f.Next = fmt.Sprintf("Fine if you expect it to reach %s; else decline, or remove the file from the held copy (what uses it may stop working).", list)
		case "secrets":
			f.Text = fmt.Sprintf("Uses your home folder and names %s: it may read your keys.", list)
			f.Why = append(f.Why, "A skill rarely needs your keys.")
			f.Next = "Read what it does with them before installing."
		case "writes":
			f.Text = fmt.Sprintf("Writes files, next to where it names %s: it may change them.", list)
			f.Why = append(f.Why, "What it changes there stays after you remove it, and acts in every session.")
			f.Next = "Read the lines around it: fine if it's what the skill is for."
		case "env":
			f.Kind = capWords["env"]
			f.Text = fmt.Sprintf("Reads %s from the environment.", list)
			f.Why = append(f.Why, "With it, it can act as you where the token works.")
			f.Next = "Read where it sends it."
		case "install":
			f.Text = fmt.Sprintf("Installing its packages runs code: %s.", list)
			f.Next = "Read the script before installing its packages."
		case "grants":
			f.Text = fmt.Sprintf("Its frontmatter grants it %s without asking you.", list)
			f.Next = "Drop the grant (yardo edits its copy): the agent asks as usual."
			f.Fix = "drop-key:" + grantKey(s.Files["SKILL.md"])
		case "agent":
			al := says[k.p][us[0].line]
			if !strings.Contains(al.say, "global rules") {
				f.Kind = "Changes your agent's settings"
			}
			f.Text = fmt.Sprintf("Tells the agent to write to %s.", al.say)
			if al.asked {
				f.Why = append(f.Why, "It says to do it only when you ask.")
			}
			f.Why = append(f.Why, "What it writes there stays after you remove the skill, and the agent reads it in every session.")
			f.Next = "Fine if you want it to manage that; else decline: yardo can't take out one instruction."
		}
		fs = append(fs, f)
	}
	return out, fs
}

// grantsOf are the tools a SKILL.md's frontmatter grants, and the line
// of its hooks (0 none).
func grantsOf(b []byte) ([]use, int) {
	front, _ := frontLines(b)
	var out []use
	hooks := 0
	risky := regexp.MustCompile(`^(?:Bash|Write|Edit|MultiEdit|NotebookEdit|Shell|Exec|WebFetch|WebSearch)\b|^mcp__`)
	shell := regexp.MustCompile(`\b(Bash|Write|Edit|MultiEdit|NotebookEdit|Shell|Exec)\b`)
	for n, l := range front {
		if strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") {
			continue
		}
		k, v, _ := strings.Cut(l, ":")
		switch strings.TrimSpace(k) {
		case "hooks":
			hooks = n + 2
		case "allowed-tools", "tools":
			all := v
			for _, more := range front[n+1:] {
				if !strings.HasPrefix(more, " ") && !strings.HasPrefix(more, "\t") {
					break
				}
				all += "," + strings.TrimPrefix(strings.TrimSpace(more), "- ")
			}
			blocked := shell.MatchString(all) // the frontmatter rule's
			for _, t := range strings.FieldsFunc(all, func(r rune) bool { return r == ',' || r == '[' || r == ']' }) {
				t = strings.Trim(strings.TrimSpace(t), `"'`)
				if t == "" {
					continue
				}
				u := use{kind: "grants", what: t, line: n + 2}
				if risky.MatchString(t) {
					u.look, u.level = true, "note"
					if blocked {
						u.level = "block"
					}
				}
				out = append(out, u)
			}
		}
	}
	return out, hooks
}

// grantKey is the key a SKILL.md grants tools with.
func grantKey(b []byte) string {
	front, _ := frontLines(b)
	for _, l := range front {
		if k, _, _ := strings.Cut(l, ":"); strings.TrimSpace(k) == "tools" {
			return "tools"
		}
	}
	return "allowed-tools"
}

// installScripts are the scripts npm install runs of a package.json.
func installScripts(b []byte) []use {
	var pk struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(b, &pk) != nil {
		return nil
	}
	var out []use
	for _, k := range []string{"preinstall", "install", "postinstall", "prepare"} {
		if _, ok := pk.Scripts[k]; ok {
			line := 1
			if i := bytes.Index(b, []byte(`"`+k+`"`)); i >= 0 {
				line = bytes.Count(b[:i], []byte("\n")) + 1
			}
			out = append(out, use{kind: "install", what: "package.json " + k, line: line, look: true, level: "note"})
		}
	}
	return out
}

// and is items as a list in words: "a", "a and b", "a, b and c".
func and(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// Say is what c says it can do, as a phrase after "it": its kind's verb
// and its things ("contacts api.github.com and tt-a1i.github.io"); an
// agent's carry their own verbs ("edits your global rules file when you
// ask").
func (c Cap) Say() string {
	items := strings.Split(c.What, ", ")
	list := and(items)
	if c.What == "bundled code" {
		return map[string]string{
			"exec": "runs programs from its bundled code", "network": "contacts the network from its bundled code",
			"env": "reads environment variables in its bundled code", "home": "uses your home folder in its bundled code",
			"writes": "writes files from its bundled code",
		}[c.Kind]
	}
	switch c.Kind {
	case "exec", "hooks":
		return "runs " + list
	case "network":
		return "contacts " + list
	case "env":
		tokens := slices.DeleteFunc(slices.Clone(items), func(w string) bool { return w == "environment variables" })
		switch {
		case len(tokens) == 0:
			return "reads environment variables"
		case len(tokens) < len(items):
			return "reads environment variables, " + and(tokens) + " among them"
		}
		return "reads " + list + " from the environment"
	case "home":
		return "uses " + list
	case "secrets":
		return "reads " + list
	case "writes":
		return "writes " + list
	case "grants":
		return "grants itself " + list
	case "install":
		return "runs " + list + " when its packages are installed"
	}
	return list // agent: each says its verb
}

// KindCount is n findings of kind as words, for a fold: "1 hidden
// character", "3 web addresses"; a kind that's a phrase, "contacts the
// network (3)".
func KindCount(kind string, n int) string {
	if f, ok := kindNouns[kind]; ok {
		return fmt.Sprintf("%d %s", n, f[min(n, 2)-1])
	}
	return fmt.Sprintf("%s (%d)", strings.ToLower(kind[:min(1, len(kind))])+kind[min(1, len(kind)):], n)
}

// kindNouns are kinds as nouns, one and many.
var kindNouns = map[string][2]string{
	"Hidden characters": {"hidden character", "hidden characters"}, "Web addresses": {"web address", "web addresses"},
	"A hidden comment": {"hidden comment", "hidden comments"}, "Runs what it fetches": {"fetch-and-run line", "fetch-and-run lines"},
	"Acts as root, or later": {"sudo or crontab line", "sudo or crontab lines"}, "Names your secrets": {"mention of your secrets", "mentions of your secrets"},
	"Hides things from you": {"line that hides things", "lines that hide things"}, "Not read through": {"file not read through", "files not read through"},
	"A program": {"program", "programs"}, "An archive": {"archive", "archives"}, "A link, not a file": {"link", "links"},
	"A path out of its folder": {"path out of its folder", "paths out of its folder"}, "A name like one you have": {"name like one you have", "names like ones you have"},
	"Docker options": {"Docker option", "Docker options"}, "Code in its spec": {"spec with code", "specs with code"},
	"Hooks or tools it grants itself": {"hook or grant", "hooks or grants"}, "Not fixed to a version": {"unpinned source", "unpinned sources"},
}
