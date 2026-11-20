package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ptyRun runs argv in a pseudo-terminal (Python's pty: the module has
// none), typing keys once the prompt shows "Session:"; it returns what
// the terminal showed.
const ptyRun = `
import os, pty, select, sys
keys = sys.argv[1].encode()
pid, fd = pty.fork()
if pid == 0:
    os.execv(sys.argv[2], sys.argv[2:])
out, sent = b"", False
while True:
    r, _, _ = select.select([fd], [], [], 5)
    if not r:
        break
    try:
        d = os.read(fd, 1024)
    except OSError:
        break
    if not d:
        break
    out += d
    if keys and not sent and b"Session:" in out:
        os.write(fd, keys)
        sent = True
os.waitpid(pid, 0)
sys.stdout.buffer.write(out)
`

// sessionCase is one login: the environment, what's installed and
// running, the saved choice, the key typed.
type sessionCase struct {
	env          []string
	tools        []string // fakes on PATH: tmux, zellij
	tmuxUp       bool
	zellijList   string
	saved, keys  string
	tmuxRC       string
	noSSH, notty bool
}

// login runs session.zsh as an ssh login would, with fake tmux and
// zellij; it returns what they were asked to do, the terminal's output
// and the saved choice ("<none>" when there's no file).
func login(t *testing.T, c sessionCase) (ran, out, saved string) {
	t.Helper()
	z := needZsh(t)
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	log := filepath.Join(home, "ran")
	for _, d := range []string{bin, filepath.Join(home, ".config/yardo")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tool := range []string{"mkdir"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	fakes := map[string]string{
		"tmux":   "#!/bin/sh\necho \"tmux $*\" >>\"$LOG\"\n[ \"$1\" = has-session ] && { [ -n \"$TMUX_UP\" ]; exit; }\nexit ${TMUX_RC:-0}\n",
		"zellij": "#!/bin/sh\necho \"zellij $*\" >>\"$LOG\"\n[ \"$1\" = list-sessions ] && { printf '%b' \"$ZELLIJ_LIST\"; exit 0; }\nexit 0\n",
	}
	for _, tool := range c.tools {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(fakes[tool]), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	session := filepath.Join(home, ".config/yardo/session")
	if c.saved != "" {
		if err := os.WriteFile(session, []byte(c.saved+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(home, "session.zsh")
	if err := os.WriteFile(script, []byte(readEmbedded(t, "conf/session.zsh")), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "PATH=" + bin, "TERM=xterm", "LOG=" + log, "ZELLIJ_LIST=" + c.zellijList, "TMUX_RC=" + c.tmuxRC}
	if !c.noSSH {
		env = append(env, "SSH_CONNECTION=10.0.0.1 5000 10.0.0.2 22")
	}
	if c.tmuxUp {
		env = append(env, "TMUX_UP=1")
	}
	env = append(env, c.env...)
	var cmd *exec.Cmd
	if c.notty {
		cmd = exec.Command(z, "-f", "-i", "-c", "source "+script)
	} else {
		cmd = exec.Command(py, "-I", "-c", ptyRun, c.keys, z, "-f", "-i", "-c", "source "+script)
	}
	cmd.Env = env
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh: %v\n%s", err, b)
	}
	r, _ := os.ReadFile(log)
	s, err := os.ReadFile(session)
	if err != nil {
		s = []byte("<none>")
	}
	return strings.TrimSpace(string(r)), string(b), strings.TrimSpace(string(s))
}

// What a login starts, attaches or leaves alone: the multiplexer's
// commands besides the looks (has-session, list-sessions).
func started(ran string) string {
	var out []string
	for _, l := range strings.Split(ran, "\n") {
		if l != "" && !strings.Contains(l, "has-session") && !strings.Contains(l, "list-sessions") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func TestSessionOnLogin(t *testing.T) {
	both := []string{"tmux", "zellij"}
	for name, tc := range map[string]struct {
		c    sessionCase
		want string
	}{
		"attach to tmux":         {sessionCase{tools: both, saved: "tmux", tmuxUp: true}, "tmux attach"},
		"start tmux":             {sessionCase{tools: both, saved: "tmux"}, "tmux new -s main"},
		"attach to zellij":       {sessionCase{tools: both, saved: "zellij", zellijList: "work [Created 1h ago]\\nold [Created 2d ago] (EXITED - attach to resurrect)\\n"}, "zellij attach work"},
		"start zellij":           {sessionCase{tools: both, saved: "zellij", zellijList: "old (EXITED - attach to resurrect)\\n"}, "zellij attach -c main"},
		"inside tmux":            {sessionCase{tools: both, saved: "tmux", env: []string{"TMUX=/tmp/tmux-1/default,1,0"}}, ""},
		"inside zellij":          {sessionCase{tools: both, saved: "tmux", env: []string{"ZELLIJ=0"}}, ""},
		"not over ssh":           {sessionCase{tools: both, saved: "tmux", noSSH: true}, ""},
		"no terminal":            {sessionCase{tools: both, saved: "tmux", notty: true}, ""},
		"vscode":                 {sessionCase{tools: both, saved: "tmux", env: []string{"TERM_PROGRAM=vscode"}}, ""},
		"cursor":                 {sessionCase{tools: both, saved: "tmux", env: []string{"CURSOR_TRACE_ID=x"}}, ""},
		"skipped once":           {sessionCase{tools: both, saved: "tmux", env: []string{"YARDO_SESSION=off"}}, ""},
		"set off":                {sessionCase{tools: both, saved: "off"}, ""},
		"only tmux":              {sessionCase{tools: []string{"tmux"}}, "tmux new -s main"},
		"only zellij":            {sessionCase{tools: []string{"zellij"}}, "zellij attach -c main"},
		"neither":                {sessionCase{}, ""},
		"one running, no choice": {sessionCase{tools: both, tmuxUp: true}, "tmux attach"},
		"saved one missing":      {sessionCase{tools: []string{"tmux"}, saved: "zellij"}, "tmux new -s main"},
	} {
		t.Run(name, func(t *testing.T) {
			ran, out, _ := login(t, tc.c)
			if got := started(ran); got != tc.want {
				t.Errorf("ran %q, want %q\n%s", got, tc.want, out)
			}
			if strings.Contains(out, "Session:") {
				t.Errorf("asked:\n%s", out)
			}
		})
	}
}

// tmux failing leaves the plain shell, with a line saying so.
func TestSessionFails(t *testing.T) {
	_, out, _ := login(t, sessionCase{tools: []string{"tmux"}, tmuxRC: "1"})
	if !strings.Contains(out, "tmux didn't start (exit 1): this is the plain shell.") {
		t.Errorf("output:\n%s", out)
	}
}

// The question, with both installed and nothing set: t, z and n are saved
// and acted on; another key is "not now".
func TestSessionAsks(t *testing.T) {
	both := []string{"tmux", "zellij"}
	for key, want := range map[string][2]string{
		"t": {"tmux new -s main", "tmux"},
		"z": {"zellij attach -c main", "zellij"},
		"n": {"", "off"},
		"x": {"", "<none>"},
	} {
		t.Run(key, func(t *testing.T) {
			ran, out, saved := login(t, sessionCase{tools: both, keys: key})
			if !strings.Contains(out, "Session: start [t]mux  [z]ellij  [n]ever") {
				t.Errorf("not asked:\n%s", out)
			}
			if got := started(ran); got != want[0] || saved != want[1] {
				t.Errorf("ran %q, saved %q; want %q, %q", got, saved, want[0], want[1])
			}
		})
	}
	// Both running: "attach to".
	_, out, _ := login(t, sessionCase{tools: both, tmuxUp: true, zellijList: "w\\n", keys: "x"})
	if !strings.Contains(out, "Session: attach to [t]mux") {
		t.Errorf("both running:\n%s", out)
	}
}

// yardo's zsh setup sources the session file first, before the prompt.
func TestSessionSourcedFirst(t *testing.T) {
	tmpl := readEmbedded(t, "conf/zshrc.tmpl")
	s := strings.Index(tmpl, "~/.config/yardo/session.zsh")
	ip := strings.Index(tmpl, "p10k-instant-prompt-")
	if s < 0 || s > ip {
		t.Errorf("session.zsh must be sourced before the instant prompt (session=%d ip=%d)", s, ip)
	}
}
