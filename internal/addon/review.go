package addon

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The AI review: Claude Code (`claude -p`, the user's own login) reads a
// held add-on with no tools, no MCP servers, no hooks or settings of the
// user's, from an empty folder, the content on its input. It can only add
// findings, and its answer is checked before any of it is shown.

//go:embed review-prompt.md
var reviewPrompt string

// PromptVersion and SchemaVersion name the reviewer's instructions and
// answer format: a change to either misses the cache.
var (
	PromptVersion = "2"
	SchemaVersion = "1"
)

// Models: the quick review, and Check again carefully.
const (
	QuickModel   = "haiku"
	CarefulModel = "sonnet"
)

var (
	// ReviewTimeout is how long a review may take.
	ReviewTimeout = 90 * time.Second
	// Raw serves a repository's files at a ref (a Docker image's source).
	Raw = "https://raw.githubusercontent.com"
	// ClaudeCmd is claude with args, run as user from the empty folder
	// dir: as root through runuser, with an environment of its own.
	ClaudeCmd = func(ctx context.Context, user, dir string, args ...string) *exec.Cmd {
		if os.Geteuid() != 0 {
			c := exec.CommandContext(ctx, "claude", args...)
			c.Dir = dir
			return c
		}
		home := homeOf(user)
		c := exec.CommandContext(ctx, "runuser", append([]string{"-u", user, "--", "env", "-i",
			"HOME=" + home, "USER=" + user, "LOGNAME=" + user, "LANG=C.UTF-8",
			fmt.Sprintf("PATH=%s/.local/bin:%s/.local/share/mise/shims:/usr/local/bin:/usr/bin:/bin", home, home),
			"claude"}, args...)...)
		c.Dir = dir
		return c
	}
)

func homeOf(name string) string {
	if u, err := user.Lookup(name); err == nil {
		return u.HomeDir
	}
	return "/home/" + name
}

// reviewFlags are the flags a locked-down review needs: a Claude Code
// without one of them isn't run.
var reviewFlags = []string{"--tools", "--restricted", "--strict-mcp-config", "--mcp-config",
	"--disable-slash-commands", "--permission-mode", "--no-session-persistence", "--settings",
	"--system-prompt", "--output-format", "--json-schema", "--max-budget-usd", "--model"}

// reviewSchema is the answer's format.
const reviewSchema = `{"type":"object","additionalProperties":false,"required":["nonce","verdict","summary","findings"],"properties":{
"nonce":{"type":"string"},
"verdict":{"type":"string","enum":["ok","warn","block"]},
"summary":{"type":"string","maxLength":200},
"findings":{"type":"array","maxItems":20,"items":{"type":"object","additionalProperties":false,"required":["severity","category","file","line","explain","next"],"properties":{
"severity":{"type":"string","enum":["warn","block"]},
"category":{"type":"string","enum":["secret_access","exfiltration","remote_code","agent_instruction","reviewer_injection","install_script","docker_privilege","unpinned","obfuscation","other"]},
"file":{"type":"string"},
"line":{"type":"integer","minimum":0},
"explain":{"type":"string","maxLength":300},
"next":{"type":"string","enum":["remove_file","remove_lines","pin_version","drop_docker_flag","decline","check_by_hand"]}}}}}}`

// nextSteps are the fixed next steps Claude picks from.
var nextSteps = map[string]string{
	"remove_file":      "remove this file, or decline",
	"remove_lines":     "these lines would have to go: decline, or fix it at its source",
	"pin_version":      "what it fetches isn't pinned: decline, or pin it at its source",
	"drop_docker_flag": "drop that Docker option, or decline",
	"decline":          "decline it",
	"check_by_hand":    "read it yourself before deciding",
}

// categoryWords say each category, in a few words.
var categoryWords = map[string]string{
	"secret_access": "Reads your secrets", "exfiltration": "Sends data out", "remote_code": "Runs what it downloads",
	"agent_instruction": "Steers the agent", "reviewer_injection": "Tries to steer the review", "install_script": "Runs code when installed",
	"docker_privilege": "Docker privileges", "unpinned": "Not pinned", "obfuscation": "Hard-to-read code", "other": "Worth a second read",
}

var categories = []string{"secret_access", "exfiltration", "remote_code", "agent_instruction",
	"reviewer_injection", "install_script", "docker_privilege", "unpinned", "obfuscation", "other"}

// Limits on what Claude is given.
const (
	reviewFileMax  = 32 << 10
	reviewTotalMax = 150 << 10
)

// Review is Claude's read of h's files with model: its findings (layer
// claude), and the model that read it ("" when it wasn't read: then a
// note says why). Earlier are the other layers' findings, given as hints.
func Review(ctx context.Context, h *Held, files map[string][]byte, earlier []Finding, model string) ([]Finding, string) {
	in, extra := reviewInput(ctx, h, files, earlier)
	key := reviewKey(files, in.extraHash, model)
	if fs, m, ok := cached(key); ok {
		return append(fs, extra...), m
	}
	fs, m, why := review(ctx, h.User, in, model)
	if why != "" {
		return append(extra, notChecked(why)), ""
	}
	cache(key, fs, m)
	return append(fs, extra...), m
}

func notChecked(why string) Finding {
	return Finding{Layer: "claude", Rule: "not-checked", Level: "note", Text: "Not checked by AI: " + why,
		Next: "the other checks still count; read it yourself"}
}

// input is what Claude reads: the text, its marker, the lines each file
// has (to check the answer's) and each file's role.
type input struct {
	text, nonce string
	lines       map[string]int
	roles       map[string]string
	extraHash   string
}

// reviewInput builds Claude's input from h's files (and, for an image,
// its settings and source), with the notes on what it didn't read.
func reviewInput(ctx context.Context, h *Held, files map[string][]byte, earlier []Finding) (input, []Finding) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	in := input{nonce: hex.EncodeToString(b), lines: map[string]int{}}
	var notes []Finding
	all := files
	if h.Kind == "mcp" && h.Image {
		ex, n := imageInput(ctx, h, files)
		notes = append(notes, n...)
		if len(ex) > 0 {
			all = map[string][]byte{}
			maps.Copy(all, files)
			maps.Copy(all, ex)
			in.extraHash = Hash(ex)
		}
	}
	pl := planRead(h, all, earlier)
	in.roles = pl.roles
	var t strings.Builder
	kind := map[string]string{"skill": "agent skill", "mcp": "MCP server"}[h.Kind]
	fmt.Fprintf(&t, "Add-on: %s %q from %s @ %s\n", kind, h.Name, h.Source, h.Pin)
	fmt.Fprintf(&t, "This review's marker value: %s\n\nFiles (path, bytes, sha256, role):\n", in.nonce)
	for _, p := range slices.Sorted(maps.Keys(all)) {
		s := sha256.Sum256(all[p])
		fmt.Fprintf(&t, "- %s %d %s %s\n", p, len(all[p]), hex.EncodeToString(s[:]), pl.roles[p])
	}
	if pl.changes != nil {
		t.WriteString("\nThis is an update of a version the user approved. Changes since then:\n")
		for _, c := range pl.changes {
			fmt.Fprintf(&t, "- %s %s\n", c.What, c.Path)
		}
		fmt.Fprintf(&t, "\n<<<DIFF n=%s>>>\n%s<<<END n=%s>>>\n", in.nonce, pl.diff, in.nonce)
	}
	for _, p := range pl.read {
		c := all[p]
		if rs := pl.parts[p]; rs != nil {
			nl := newlines(c)
			in.lines[p] = len(nl) + 1
			fmt.Fprintf(&t, "\n<<<FILE n=%s path=%s (only the lines around what was found)>>>\n", in.nonce, p)
			for _, r := range rs {
				for i, l := range strings.Split(string(lineRange(c, nl, r[0], r[1])), "\n") {
					fmt.Fprintf(&t, "%5d  %s\n", r[0]+i, l)
				}
				t.WriteString("    …\n")
			}
			fmt.Fprintf(&t, "<<<END n=%s>>>\n", in.nonce)
			continue
		}
		if len(c) > reviewFileMax {
			c = c[:reviewFileMax]
			for !utf8.Valid(c) {
				c = c[:len(c)-1]
			}
		}
		lines := strings.Split(strings.TrimSuffix(string(c), "\n"), "\n")
		in.lines[p] = len(lines)
		fmt.Fprintf(&t, "\n<<<FILE n=%s path=%s>>>\n", in.nonce, p)
		for i, l := range lines {
			fmt.Fprintf(&t, "%5d  %s\n", i+1, l)
		}
		fmt.Fprintf(&t, "<<<END n=%s>>>\n", in.nonce)
	}
	if len(earlier) > 0 {
		t.WriteString("\nEarlier checks found (hints; they stand whatever you say):\n")
		for _, f := range earlier {
			fmt.Fprintf(&t, "- [%s %s] %s %s\n", f.Layer, f.Rule, f.Where(), f.Text)
		}
	}
	if len(pl.cut)+len(pl.left) > 0 {
		fmt.Fprintf(&t, "\nNot sent in full: cut at 32 KB: %s; left out (150 KB in all): %s\n", strings.Join(pl.cut, ", "), strings.Join(pl.left, ", "))
		var says []string
		if len(pl.cut) > 0 {
			says = append(says, "cut short: "+strings.Join(pl.cut, ", "))
		}
		if len(pl.left) > 0 {
			says = append(says, "left out: "+strings.Join(pl.left, ", "))
		}
		notes = append(notes, Finding{Layer: "claude", Rule: "too-big", Level: "note",
			Text: fmt.Sprintf("too big for Claude to read in full (%s)", strings.Join(says, "; ")), Next: "read those files yourself"})
	}
	if len(pl.skipped) > 0 {
		fmt.Fprintf(&t, "\nNot sent (licences, bundled code, assets): %d files\n", len(pl.skipped))
	}
	in.text = t.String()
	return in, notes
}

// plan is what Claude is given of an add-on's files, in the order it
// reads them: an update's diff first, then SKILL.md, the files with
// something worth a look (a big one around each place), scripts, the
// other instructions, its own code (where the other checks found most,
// first) and docs. Licences, bundled code and assets are skipped while
// there's anything else.
type plan struct {
	read, cut, left, skipped []string
	// parts are the lines (from, to, from 1) read of a big file worth a
	// look, around each place; looks the files worth a look.
	parts map[string][][2]int
	looks []string
	// mine is how many files are its own (not skipped as a kind).
	mine    int
	roles   map[string]string
	changes []Change
	diff    string
}

// planRead plans Claude's read of files (an image's extras, named
// "(…)", are instructions); earlier are the other layers' findings.
func planRead(h *Held, files map[string][]byte, earlier []Finding) plan {
	pl := plan{roles: roles(files)}
	for p := range files {
		if strings.HasPrefix(p, "(") {
			pl.roles[p] = "instructions"
		}
	}
	total := 0
	changed := map[string]bool{}
	if h.Update {
		if old, err := ApprovedFiles(h.User, h.Kind, h.Name); err == nil {
			pl.changes = Changes(old, files)
			for _, c := range pl.changes {
				changed[c.Path] = c.What != "removed"
				pl.diff += c.Diff
			}
			if len(pl.diff) > reviewFileMax {
				pl.diff, pl.cut = pl.diff[:reviewFileMax], append(pl.cut, "(the diff)")
				for !utf8.ValidString(pl.diff) {
					pl.diff = pl.diff[:len(pl.diff)-1]
				}
			}
			total += len(pl.diff)
		}
	}
	hits := map[string]int{}
	at := map[string][]int{} // places worth a look, by file
	for _, f := range earlier {
		hits[f.File] += 1 + f.More
		if (f.Look || f.Level != "note") && f.File != "" && (own(pl.roles[f.File]) || pl.roles[f.File] == "doc") {
			at[f.File] = append(at[f.File], f.Line)
		}
	}
	// An update's SKILL.md and new or changed scripts come first.
	first := func(p string) int {
		switch {
		case !h.Update:
			return 0
		case path.Base(p) == "SKILL.md" || p == "spec":
			return 0
		case changed[p] && isScript(p, files[p]):
			return 1
		}
		return 2
	}
	order := map[string]int{"script": 2, "instructions": 3, "code": 4, "doc": 5}
	weight := func(p string) int {
		switch o, ok := order[pl.roles[p]]; {
		case path.Base(p) == "SKILL.md" && !strings.Contains(p, "/") || p == "spec":
			return 0
		case at[p] != nil:
			return 1
		case ok:
			return o
		}
		return 6
	}
	paths := slices.Sorted(maps.Keys(files))
	slices.SortStableFunc(paths, func(a, b string) int {
		if d := first(a) - first(b); d != 0 {
			return d
		}
		if d := weight(a) - weight(b); d != 0 {
			return d
		}
		return hits[b] - hits[a]
	})
	for _, p := range paths {
		if weight(p) < 6 {
			pl.mine++
		}
		if at[p] != nil {
			pl.looks = append(pl.looks, p)
		}
	}
	for _, p := range paths {
		c := files[p]
		switch {
		case !utf8.Valid(c) || bytes.IndexByte(c, 0) >= 0:
			pl.skipped = append(pl.skipped, p) // a binary file: listed only
			continue
		case weight(p) == 6 && pl.mine > 0:
			pl.skipped = append(pl.skipped, p)
			continue
		}
		n := min(len(c), reviewFileMax)
		if at[p] != nil && len(c) > reviewFileMax {
			rs := around(c, at[p])
			n = partSize(c, rs)
			if total+n <= reviewTotalMax {
				if pl.parts == nil {
					pl.parts = map[string][][2]int{}
				}
				pl.parts[p] = rs
				total += n
				pl.read = append(pl.read, p)
				continue
			}
		}
		if total+n > reviewTotalMax {
			pl.left = append(pl.left, p)
			continue
		}
		if len(c) > reviewFileMax {
			pl.cut = append(pl.cut, p)
		}
		total += n
		pl.read = append(pl.read, p)
	}
	return pl
}

// aroundLines is how many lines either side of a place Claude reads of a
// big file.
const aroundLines = 40

// around are the line ranges of b around each of lines (0: its start),
// joined where they meet.
func around(b []byte, lines []int) [][2]int {
	n := bytes.Count(b, []byte("\n")) + 1
	var rs [][2]int
	for _, l := range slices.Sorted(slices.Values(lines)) {
		r := [2]int{max(1, l-aroundLines), min(n, max(l, 1)+aroundLines)}
		if k := len(rs) - 1; k >= 0 && r[0] <= rs[k][1]+1 {
			rs[k][1] = max(rs[k][1], r[1])
			continue
		}
		rs = append(rs, r)
	}
	return rs
}

// partSize is how many bytes the ranges rs of b are.
func partSize(b []byte, rs [][2]int) int {
	nl := newlines(b)
	n := 0
	for _, r := range rs {
		n += len(lineRange(b, nl, r[0], r[1])) + 1
	}
	return n
}

// coverage is what pl has Claude read, in words.
func (pl plan) coverage() Coverage {
	c := Coverage{Layer: "claude", Files: len(pl.read), All: pl.mine}
	if len(pl.skipped) > 0 {
		n := map[string]int{}
		for _, p := range pl.skipped {
			n[pl.roles[p]]++
		}
		var kinds []string
		for _, r := range []string{"license", "vendored", "asset", "doc", "code", "script", "instructions"} {
			if n[r] > 0 {
				kinds = append(kinds, map[string]string{"license": "licences", "vendored": "vendored", "asset": "assets", "doc": "docs"}[r])
			}
		}
		c.Skipped = fmt.Sprintf("%d %s", len(pl.skipped), strings.Join(slices.DeleteFunc(kinds, func(s string) bool { return s == "" }), ", "))
	}
	if len(pl.looks) > 0 {
		var read, part []string
		for _, p := range pl.looks {
			switch {
			case pl.parts[p] != nil:
				part = append(part, p)
			case slices.Contains(pl.read, p):
				read = append(read, p)
			}
		}
		c.Looks = fmt.Sprintf("%d of the %d files worth a look", len(read)+len(part), len(pl.looks))
		if len(read) > 0 {
			c.Looks += "; whole: " + strings.Join(read, ", ")
		}
		if len(part) > 0 {
			c.Looks += fmt.Sprintf("; %d lines around each place: %s", aroundLines, strings.Join(part, ", "))
		}
	}
	var un []string
	if len(pl.left) > 0 {
		un = append(un, fmt.Sprintf("%d own files past the budget (%s/…)", len(pl.left), path.Dir(pl.left[0])))
	}
	if len(pl.cut) > 0 {
		un = append(un, fmt.Sprintf("%d cut at 32 KB", len(pl.cut)))
	}
	c.Unread = strings.Join(un, "; ")
	return c
}

// imageInput are an image's settings, the docker run yardo would use and
// its source's Dockerfile and README, as files for Claude; with the notes
// on what it can't read.
func imageInput(ctx context.Context, h *Held, files map[string][]byte) (map[string][]byte, []Finding) {
	ex := map[string][]byte{}
	notes := []Finding{{Layer: "claude", Rule: "image-program", Level: "note",
		Text: "the image's program wasn't read: only its settings, and its Dockerfile and README when it names its source",
		Next: "trust it as far as you trust who publishes it"}}
	sp := ParseSpec(string(files["spec"]))
	run := []string{"docker", "run", "-i", "--rm", "--env-file", fmt.Sprintf("~/.mcp/%s/env", h.Name), "-v", fmt.Sprintf("~/.mcp/%s:/mcp:ro", h.Name)}
	run = append(run, strings.Fields(sp.Fields["DOCKER_ARGS"])...)
	run = append(run, h.Pin)
	run = append(run, strings.Fields(sp.Fields["IMAGE_ARGS"])...)
	ex["(docker run)"] = []byte(strings.Join(run, " ") + "\n")
	out, err := exec.CommandContext(ctx, Docker, "image", "inspect", "--format", "{{json .Config}}", h.Pin).Output()
	if err != nil {
		return ex, notes
	}
	var cfg struct {
		Entrypoint, Cmd []string
		User            string
		Env             []string
		ExposedPorts    map[string]any
		Labels          map[string]string
	}
	if json.Unmarshal(out, &cfg) != nil {
		return ex, notes
	}
	var envNames []string
	for _, e := range cfg.Env {
		envNames = append(envNames, strings.SplitN(e, "=", 2)[0]) // names only
	}
	settings, _ := json.MarshalIndent(map[string]any{"entrypoint": cfg.Entrypoint, "cmd": cfg.Cmd, "user": cfg.User,
		"env (names)": envNames, "ports": slices.Sorted(maps.Keys(cfg.ExposedPorts)), "labels": cfg.Labels}, "", "  ")
	ex["(image settings)"] = append(settings, '\n')
	src := cfg.Labels["org.opencontainers.image.source"]
	ver := cfg.Labels["org.opencontainers.image.version"]
	m := regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`).FindStringSubmatch(src)
	if m == nil {
		notes = append(notes, Finding{Layer: "claude", Rule: "no-source", Level: "note",
			Text: "the image names no GitHub source repository: its Dockerfile wasn't read", Next: "find where it's built before trusting it"})
		return ex, notes
	}
	refs := []string{"HEAD"}
	if ver != "" && regexp.MustCompile(`^[A-Za-z0-9._-]+$`).MatchString(ver) {
		refs = []string{"v" + ver, ver}
	}
	for _, ref := range refs {
		got := false
		for _, f := range []string{"Dockerfile", "README.md"} {
			b, err := get(ctx, fmt.Sprintf("%s/%s/%s/%s/%s", Raw, m[1], m[2], ref, f), "", reviewFileMax)
			if err == nil {
				ex[fmt.Sprintf("(source %s/%s@%s)/%s", m[1], m[2], ref, f)] = b
				got = true
			}
		}
		if got {
			return ex, notes
		}
	}
	notes = append(notes, Finding{Layer: "claude", Rule: "no-source", Level: "note",
		Text: fmt.Sprintf("its source %s/%s has no Dockerfile or README at %s", m[1], m[2], strings.Join(refs, " or ")), Next: "find where it's built before trusting it"})
	return ex, notes
}

// review runs Claude on in; why is set when it wasn't read.
func review(ctx context.Context, usr string, in input, model string) (fs []Finding, read string, why string) {
	if why := flagsOK(ctx, usr); why != "" {
		return nil, "", why
	}
	dir, err := os.MkdirTemp("", "yardo-review-")
	if err != nil {
		return nil, "", err.Error()
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if os.Geteuid() == 0 {
		if u, err := user.Lookup(usr); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			_ = os.Chown(dir, uid, gid)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, ReviewTimeout)
	defer cancel()
	out, err := runClaude(ctx, usr, dir, in.text, model)
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return nil, "", fmt.Sprintf("Claude took longer than %d seconds", int(ReviewTimeout.Seconds()))
	case ctx.Err() != nil:
		return nil, "", "stopped before Claude answered"
	case errors.Is(err, exec.ErrNotFound) || isNotFound(err, out):
		return nil, "", "Claude Code isn't installed"
	}
	var env struct {
		IsError    bool            `json:"is_error"`
		Result     string          `json:"result"`
		Structured json.RawMessage `json:"structured_output"`
		Usage      map[string]any  `json:"modelUsage"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil {
		if err != nil {
			return nil, "", "Claude Code failed: " + clean(lastLine(out, err), 160)
		}
		return nil, "", "the answer didn't hold up (not Claude Code's JSON)"
	}
	if env.IsError {
		r := strings.ToLower(env.Result)
		switch {
		case strings.Contains(r, "not logged in") || strings.Contains(r, "/login") || strings.Contains(r, "invalid api key"):
			return nil, "", "Claude Code isn't logged in (claude, then /login)"
		case model != "" && strings.Contains(r, "model"):
			return review(ctx, usr, in, "") // the alias refused: Claude Code's default
		}
		return nil, "", "Claude Code said: " + clean(env.Result, 160)
	}
	for m := range env.Usage {
		read = m
	}
	if read == "" {
		read = cmpOr(model, "Claude Code's default model")
	}
	raw := env.Structured
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage(env.Result)
	}
	fs, err = checkAnswer(raw, in)
	if err != nil {
		return nil, "", fmt.Sprintf("the answer didn't hold up (%s)", err.Error())
	}
	return fs, read, ""
}

func isNotFound(err error, out []byte) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == 127 ||
		err != nil && strings.Contains(string(out), "claude: not found") ||
		err != nil && strings.Contains(string(out), "No such file or directory")
}

func runClaude(ctx context.Context, usr, dir, text, model string) ([]byte, error) {
	args := []string{"-p", "--tools", "", "--restricted",
		"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`,
		"--disable-slash-commands", "--permission-mode", "dontAsk", "--no-session-persistence",
		"--settings", `{"hooks":{},"disableAllHooks":true}`,
		"--system-prompt", reviewPrompt,
		"--output-format", "json", "--json-schema", reviewSchema, "--max-budget-usd", "0.50"}
	if model != "" {
		args = append(args, "--model", model)
	}
	c := ClaudeCmd(ctx, usr, dir, args...)
	c.Stdin = strings.NewReader(text)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	if stdout.Len() == 0 {
		return stderr.Bytes(), err
	}
	return stdout.Bytes(), err
}

// flagsOK says why the user's Claude Code can't be run locked down: not
// installed, or a flag missing ("" when it can). Read once a version.
func flagsOK(ctx context.Context, usr string) string {
	dir, err := os.MkdirTemp("", "yardo-review-")
	if err != nil {
		return err.Error()
	}
	defer func() { _ = os.RemoveAll(dir) }()
	_ = os.Chmod(dir, 0o755)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	v, err := ClaudeCmd(ctx, usr, dir, "--version").Output()
	if err != nil {
		return "Claude Code isn't installed"
	}
	ver := strings.Fields(string(v))
	if len(ver) == 0 {
		return "Claude Code isn't installed"
	}
	mark := filepath.Join(Base, "review-cache", "flags-"+regexp.MustCompile(`[^0-9A-Za-z.-]`).ReplaceAllString(ver[0], "_"))
	if b, err := os.ReadFile(mark); err == nil && string(b) == "ok" {
		return ""
	}
	help, err := ClaudeCmd(ctx, usr, dir, "--help").Output()
	if err != nil {
		return "Claude Code's --help didn't run"
	}
	var missing []string
	for _, f := range reviewFlags {
		if !regexp.MustCompile(regexp.QuoteMeta(f) + `\b`).Match(help) {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return fmt.Sprintf("Claude Code %s is too old to be run with no tools (no %s): update it", ver[0], strings.Join(missing, ", "))
	}
	if os.MkdirAll(filepath.Dir(mark), 0o700) == nil {
		_ = os.WriteFile(mark, []byte("ok"), 0o600)
	}
	return ""
}

// answer is Claude's, in the schema.
type answer struct {
	Nonce    string `json:"nonce"`
	Verdict  string `json:"verdict"`
	Summary  string `json:"summary"`
	Findings []struct {
		Severity string `json:"severity"`
		Category string `json:"category"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Explain  string `json:"explain"`
		Next     string `json:"next"`
	} `json:"findings"`
}

// checkAnswer checks Claude's answer against the run (its marker, the
// format, the files and lines there are) and makes its findings. Claude's
// block needs the user's OK, and its warn in the instructions or a script
// the agent runs; elsewhere its warn is a note, worth a look in the
// add-on's own files. A finding of a file or line that isn't there is
// dropped.
func checkAnswer(raw []byte, in input) ([]Finding, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var a answer
	if err := d.Decode(&a); err != nil {
		return nil, errors.New("not the format asked for")
	}
	if a.Nonce != in.nonce {
		return nil, errors.New("its marker isn't this review's")
	}
	if !slices.Contains([]string{"ok", "warn", "block"}, a.Verdict) {
		return nil, errors.New("an unknown verdict")
	}
	if len(a.Findings) > 20 {
		a.Findings = a.Findings[:20]
	}
	var fs []Finding
	for _, f := range a.Findings {
		n, ok := in.lines[f.File]
		if !ok || f.Line < 0 || f.Line > n {
			continue // not in the add-on
		}
		level := map[string]string{"block": "ok", "warn": "note"}[f.Severity]
		next, nok := nextSteps[f.Next]
		if level == "" || !nok || !slices.Contains(categories, f.Category) {
			return nil, errors.New("a value outside the fixed lists")
		}
		r := in.roles[f.File]
		if r == "instructions" || r == "script" {
			level = "ok"
		}
		fd := Finding{Layer: "claude", Rule: f.Category, Level: level, File: f.File, Line: f.Line, Role: r,
			Kind: categoryWords[f.Category], Look: level == "ok" || own(r) || r == "doc", Text: clean(f.Explain, 300), Next: next}
		if f.Next == "remove_file" && !strings.HasPrefix(f.File, "(") {
			fd.Fix = "remove:" + f.File
		}
		fs = append(fs, fd)
	}
	if len(fs) == 0 && a.Verdict != "ok" {
		// A verdict with no finding that held up still counts.
		fs = append(fs, Finding{Layer: "claude", Rule: "verdict", Level: map[string]string{"block": "ok", "warn": "note"}[a.Verdict], Look: true, Kind: "Claude's verdict",
			Text: fmt.Sprintf("Claude judged it %s: %s", map[string]string{"block": "harmful", "warn": "worth a second read"}[a.Verdict], clean(a.Summary, 200)),
			Next: nextSteps["check_by_hand"]})
	}
	return fs, nil
}

// clean is s as plain text: no control characters, at most n runes.
func clean(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		s = string(r[:n-1]) + "…"
	}
	return s
}

// The cache: a review by its content, instructions and model.
func reviewKey(files map[string][]byte, extra, model string) string {
	s := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%s", Hash(files), extra, PromptVersion, SchemaVersion, model)))
	return hex.EncodeToString(s[:])
}

type cachedReview struct {
	Findings []Finding `json:"findings"`
	Model    string    `json:"model"`
}

func cached(key string) ([]Finding, string, bool) {
	b, err := os.ReadFile(filepath.Join(Base, "review-cache", key+".json"))
	if err != nil {
		return nil, "", false
	}
	var c cachedReview
	if json.Unmarshal(b, &c) != nil || c.Model == "" {
		return nil, "", false
	}
	return c.Findings, c.Model, true
}

func cache(key string, fs []Finding, model string) {
	d := filepath.Join(Base, "review-cache")
	if os.MkdirAll(d, 0o700) != nil {
		return
	}
	b, _ := json.Marshal(cachedReview{fs, model})
	_ = os.WriteFile(filepath.Join(d, key+".json"), b, 0o600)
}

// ReviewLayer is Check's Claude layer (set as Extra): the quick model.
func ReviewLayer(ctx context.Context, h *Held, files map[string][]byte, earlier []Finding) ([]Finding, string) {
	return Review(ctx, h, files, earlier, QuickModel)
}

// Recheck is Check again carefully: Claude's review again with the
// larger model, its findings replacing Claude's before.
func (h *Held) Recheck(ctx context.Context) error {
	files, err := h.Files()
	if err != nil {
		return err
	}
	found := h.Found
	if found == nil {
		found = h.Report.Findings // a report from before findings were kept as found
	}
	var rest []Finding
	for _, f := range found {
		if f.Layer != "claude" {
			f.Also = slices.DeleteFunc(f.Also, func(l string) bool { return l == "claude" })
			rest = append(rest, f)
		}
	}
	fs, m := Review(ctx, h, files, rest, CarefulModel)
	cov := slices.DeleteFunc(h.Report.Coverage, func(c Coverage) bool { return c.Layer == "claude" })
	h.Report.Coverage = append(cov, h.claudeCoverage(files, rest, fs, m, true))
	h.Found, h.Report.AI = append(rest, fs...), m
	h.Report.Hash = Hash(files)
	h.finish()
	return h.Save()
}
