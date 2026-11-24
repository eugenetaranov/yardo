package addon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

// VeilBin is skill-veil (the catalog's skillveil item installs it).
var VeilBin = "/usr/local/bin/skill-veil"

// VeilSum is the sha256 the binary must have (the pinned release's, for
// this machine), or "" not to check (tests).
var VeilSum = func() string {
	return map[string]string{
		"amd64": "12d113a12532e40860dbd59326a371585c730e18a67e58c1597229fa76b77ea6",
		"arm64": "36e0aacd646fe3ea421786aba5c81ff94e5aee216c27e5c96d6b96ca7e95b989",
	}[runtime.GOARCH]
}

// veilLimit is how long a scan may take.
const veilLimit = 60 * time.Second

// Veil scans h's files with skill-veil, offline: as nobody, with no
// network, no home and a time limit. Its high and critical need the
// user's OK (it can raise false alarms); medium and low are notes. A scan
// that doesn't finish says so, as a note.
func Veil(ctx context.Context, kind, name string, files map[string][]byte) []Finding {
	fs, err := veil(ctx, kind, name, files)
	if err != nil {
		return []Finding{{Layer: "skill-veil", Rule: "not-scanned", Level: "note", Text: "not scanned by skill-veil: " + err.Error(), Next: "the other checks still count; read the files yourself"}}
	}
	return fs
}

func veil(ctx context.Context, kind, name string, files map[string][]byte) ([]Finding, error) {
	if err := veilOK(); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "yardo-veil-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	// nobody reads the copy, and writes only its empty home.
	pkg := filepath.Join(tmp, "pkg", name)
	home := filepath.Join(tmp, "home")
	for _, d := range []string{tmp, filepath.Join(tmp, "pkg"), pkg, home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
		if err := os.Chmod(d, 0o755); err != nil {
			return nil, err
		}
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(home, 65534, 65534); err != nil {
			return nil, err
		}
	}
	back := map[string]string{} // the copy's path → the held one's
	for rel, b := range files {
		to := rel
		if kind == "mcp" && rel == "spec" {
			to = "spec.sh" // read as shell
		}
		back[to] = rel
		p := filepath.Join(pkg, filepath.FromSlash(to))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return nil, err
		}
	}
	// skill-veil scans packages with an entrypoint: a server's gets a
	// stub one, which nothing it finds in counts against.
	if kind == "mcp" {
		stub := fmt.Sprintf("---\nname: %s\ndescription: An MCP server's spec, in spec.sh.\n---\n", name)
		if err := os.WriteFile(filepath.Join(pkg, "SKILL.md"), []byte(stub), 0o644); err != nil {
			return nil, err
		}
		back["SKILL.md"] = ""
	}
	_ = filepath.WalkDir(pkg, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o755)
		}
		return nil
	})
	ctx, cancel := context.WithTimeout(ctx, veilLimit+30*time.Second)
	defer cancel()
	c := VeilCmd(ctx, pkg, home)
	out, err := c.Output()
	if ctx.Err() != nil {
		return nil, errors.New("it took longer than a minute")
	}
	if err != nil && len(out) == 0 {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%s", lastLine(ee.Stderr, err))
		}
		return nil, err
	}
	return parseVeil(out, pkg, back, files)
}

// veilOK: skill-veil is there, and is the pinned release.
func veilOK() error {
	f, err := os.Open(VeilBin)
	if err != nil {
		return errors.New("skill-veil isn't installed (it comes with the first add-on from elsewhere)")
	}
	defer func() { _ = f.Close() }()
	want := VeilSum()
	if want == "" {
		return nil
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return fmt.Errorf("%s isn't the release yardo pins: not run (apply skill-veil again from Tools)", VeilBin)
	}
	return nil
}

// VeilCmd is the scan of pkg: as root, under systemd-run as nobody with
// no network and no homes, for a minute at most; else (tests, a dry run)
// skill-veil itself.
var VeilCmd = func(ctx context.Context, pkg, home string) *exec.Cmd {
	args := []string{VeilBin, "scan-package", pkg, "--format", "json", "--color", "never"}
	if os.Geteuid() == 0 {
		args = append([]string{"systemd-run", "--quiet", "--pipe", "--wait", "--collect",
			"--uid=65534", "--gid=65534", "--working-directory=" + filepath.Dir(pkg),
			"-p", "PrivateNetwork=yes", "-p", "ProtectHome=yes", "-p", "NoNewPrivileges=yes",
			"-p", "ProtectSystem=strict", "-p", "ReadWritePaths=" + home, "-p", "PrivateDevices=yes",
			"-p", fmt.Sprintf("RuntimeMaxSec=%d", int(veilLimit.Seconds())),
			"-E", "HOME=" + home, "-E", "XDG_CACHE_HOME=" + home, "-E", "PATH=/usr/bin:/bin", "--"}, args...)
	}
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "HOME=" + home}
	return c
}

// parseVeil reads skill-veil's JSON: its findings, by the held file's
// path, one per rule and file. High and critical need your OK, medium and
// low are notes; a finding with no line is put where the file first does
// what the rule is about; in bundled code, docs or assets it's a note.
func parseVeil(out []byte, pkg string, back map[string]string, files map[string][]byte) ([]Finding, error) {
	var res []struct {
		Findings []struct {
			RuleID            string `json:"rule_id"`
			Severity          string `json:"severity"`
			RecommendedAction string `json:"recommended_action"`
			Line              *int   `json:"line_number"`
			Path              string `json:"artifact_path"`
			Reason            string `json:"reason"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("its report didn't read: %w", err)
	}
	rl := roles(files)
	var fs []Finding
	for _, r := range res {
		for _, f := range r.Findings {
			level := map[string]string{"critical": "ok", "high": "ok", "medium": "note", "low": "note"}[strings.ToLower(f.Severity)]
			if level == "" || f.RecommendedAction == "log" {
				continue // hygiene, not a risk
			}
			file := strings.TrimPrefix(filepath.ToSlash(strings.TrimPrefix(f.Path, pkg)), "/")
			if b, ok := back[file]; ok {
				if b == "" {
					continue // the stub's
				}
				file = b
			}
			if file != "" {
				file = path.Clean(file)
			}
			if dropVeil(f.RuleID, file) {
				continue
			}
			line, more := 0, 0
			if f.Line != nil {
				line = *f.Line
			} else {
				line, more = locate(f.RuleID, files[file])
			}
			look := level == "ok"
			if file != "" && !own(rl[file]) {
				level, look = "note", false
			}
			next := "read the line: is it what the skill is for?"
			if level == "ok" {
				next = "read the line; installing needs your OK (skill-veil can be wrong)"
			}
			fs = append(fs, Finding{Layer: "skill-veil", Rule: f.RuleID, Level: level, File: file, Line: line, More: more, Look: look,
				Kind: veilWords(f.RuleID), Text: fmt.Sprintf("%s (%s)", strings.TrimSuffix(f.Reason, "."), f.Severity), Next: next})
		}
	}
	return onePer(fs, rl), nil
}

// dropVeil: rule is a false alarm on file, by yardo's overrides: a
// "multilingual harmful verb" in code is a word in an identifier.
func dropVeil(rule, file string) bool {
	switch rule {
	case "OFFICIAL_OBFUSCATION_MULTILINGUAL_HARMFUL_VERB":
		switch path.Ext(file) {
		case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx":
			return true
		}
	}
	return false
}

// veilSigns are what each kind of rule is about, to find in a file: the
// first that a rule's ID has a word of.
var veilSigns = []struct {
	words []string
	re    *regexp.Regexp
}{
	{[]string{"CHILD_PROCESS", "EXEC", "SPAWN", "SHELL"}, regexp.MustCompile(`child_process|\bspawn\w*\s*\(|\bexec\w*\s*\(|subprocess\.|os\.system`)},
	{[]string{"NETWORK", "FETCH", "HTTP", "EXFIL", "CURL", "DOWNLOAD"}, regexp.MustCompile(`\bfetch\s*\(|\bhttps?\.(?:request|get)\b|\b(?:curl|wget)\b|\brequests\.|urlopen`)},
	{[]string{"SECRET", "CREDENTIAL", "FS", "FILE"}, regexp.MustCompile(`\bprocess\.env\b|\bfs\.|os\.homedir|os\.environ|\bopen\s*\(|readFile`)},
	{[]string{"ENV"}, regexp.MustCompile(`\bprocess\.env\b|os\.environ|getenv`)},
	{[]string{"EVAL", "DYNAMIC"}, regexp.MustCompile(`\beval\s*\(|new\s+Function\s*\(`)},
}

// locate is where in b rule's kind of thing is first, and how many more
// lines have it (0, 0: not found).
func locate(rule string, b []byte) (int, int) {
	for _, s := range veilSigns {
		if !slices.ContainsFunc(s.words, func(w string) bool { return strings.Contains(rule, w) }) {
			continue
		}
		first, n := 0, 0
		for i, l := range strings.Split(string(b), "\n") {
			if s.re.MatchString(l) {
				if first == 0 {
					first = i + 1
				}
				n++
			}
		}
		if first > 0 {
			return first, n - 1
		}
	}
	return 0, 0
}

// veilWords say what a skill-veil rule is about, in a few words.
func veilWords(rule string) string {
	for _, w := range []struct{ word, say string }{
		{"REMOTE_EXEC", "Runs what it downloads"}, {"EXFIL", "Sends data out"}, {"SECRET", "Reads secrets or files"},
		{"CREDENTIAL", "Reads secrets or files"}, {"CHILD_PROCESS", "Starts programs"}, {"EXEC", "Starts programs"},
		{"SPAWN", "Starts programs"}, {"NETWORK", "Contacts the network"}, {"FETCH", "Contacts the network"},
		{"CURL", "Contacts the network"}, {"OBFUSCAT", "Hard-to-read text"}, {"INJECTION", "Steers the agent"},
		{"PROMPT", "Steers the agent"}, {"PERMISSION", "Asks for permissions"}, {"PERSIST", "Stays after it's used"},
		{"FS", "Reads or writes files"}, {"ENV", "Reads the environment"},
	} {
		if strings.Contains(rule, w.word) {
			return w.say
		}
	}
	return "skill-veil: " + strings.ToLower(strings.ReplaceAll(rule, "_", " "))
}
