package steps

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/creds"
)

func TestDockerDaemonFreshEngineOnly(t *testing.T) {
	haveDockerd := false
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "sh" && strings.HasSuffix(c.Args[1], " dockerd") && !haveDockerd {
			return nil, errors.New("not found")
		}
		return nil, nil
	}}
	e := shellEnv(t)
	e.Exec = x
	st := step(t, catalog.Step{Type: "go:docker_daemon"})
	if _, ok := st.(BeforeApt); !ok {
		t.Fatal("docker_daemon must run before the apt batch")
	}
	apply(t, e, st)
	if !strings.Contains(read(t, e.Path(dockerDaemon)), "log-opts") {
		t.Error("daemon.json not written for a fresh engine")
	}
	// An engine that is already there keeps its config.
	if err := os.Remove(e.Path(dockerDaemon)); err != nil {
		t.Fatal(err)
	}
	haveDockerd = true
	if !check(t, e, st) {
		t.Error("Check = false with dockerd installed")
	}
	apply(t, e, st)
	if _, err := os.Stat(e.Path(dockerDaemon)); err == nil {
		t.Error("daemon.json written for an existing engine")
	}
}

func TestSSHHardenSkipsWithoutKey(t *testing.T) {
	x := &fakeExec{}
	e := shellEnv(t)
	e.Exec = x
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	st := step(t, catalog.Step{Type: "go:sshharden"})
	if err := st.Apply(t.Context(), e); err != nil || len(notes) != 1 || !strings.Contains(notes[0], "lock ada out") {
		t.Fatalf("err = %v, notes = %q", err, notes)
	}
	if _, err := os.Stat(e.Path(sshdDropin)); err == nil {
		t.Error("config written with no SSH key")
	}
	put(t, e.Path("~/.ssh/authorized_keys"), "ssh-ed25519 AAAA ada@laptop\n")
	apply(t, e, st)
	if got := strings.Join(x.commands(), "; "); got != "sshd -t; systemctl try-reload-or-restart ssh" {
		t.Errorf("ran %s", got)
	}
	if !check(t, e, st) {
		t.Error("Check after Apply = false")
	}
}

func TestSSHHardenRollsBack(t *testing.T) {
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "sshd" {
			return []byte("bad option"), errors.New("exit 255")
		}
		return nil, nil
	}}
	e := shellEnv(t)
	e.Exec = x
	put(t, e.Path("~/.ssh/authorized_keys"), "ssh-ed25519 AAAA\n")
	err := step(t, catalog.Step{Type: "go:sshharden"}).Apply(t.Context(), e)
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(e.Path(sshdDropin)); err == nil {
		t.Error("a rejected config was left in place")
	}
	for _, c := range x.commands() {
		if strings.HasPrefix(c, "systemctl") {
			t.Errorf("ssh reloaded after a rejected config: %s", c)
		}
	}
}

func TestTimezone(t *testing.T) {
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) { return []byte("Etc/UTC\n"), nil }}
	e := testEnv(t, x)
	st := step(t, catalog.Step{Type: "go:timezone"})
	if !check(t, e, st) {
		t.Error("no timezone chosen: Check = false")
	}
	e.Answers = &answers.Answers{TZ: "Europe/Berlin"}
	if check(t, e, st) {
		t.Error("Check = true with UTC set and Berlin chosen")
	}
	apply(t, e, st)
	if got := x.commands(); got[len(got)-1] != "timedatectl set-timezone Europe/Berlin" {
		t.Errorf("ran %q", got)
	}
}

func TestTailscaleUpKeyFromFile(t *testing.T) {
	var keyFile string
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "tailscale" && c.Args[0] == "up" {
			keyFile = strings.TrimPrefix(c.Args[1], "--auth-key=file:")
			b, _ := os.ReadFile(keyFile)
			if string(b) != "tskey-auth-1" {
				t.Errorf("key file holds %q", b)
			}
		}
		return nil, nil
	}}
	e := testEnv(t, x)
	e.State = t.TempDir()
	st := step(t, catalog.Step{Type: "go:tailscale_up"})
	if !check(t, e, st) {
		t.Error("no key typed: Check = false")
	}
	e.Secrets = creds.New()
	e.Secrets.TSAuthKey = "tskey-auth-1"
	apply(t, e, st)
	if strings.Contains(strings.Join(x.commands(), " "), "tskey") {
		t.Error("the key is on the command line")
	}
	if _, err := os.Stat(keyFile); err == nil {
		t.Error("key file left behind")
	}
	if got := answers.ReadCreated(e.State); len(got) != 1 || got[0].Kind != "tailscale" {
		t.Errorf("created = %+v", got)
	}
	if got := x.commands(); !strings.HasSuffix(got[len(got)-1], " --accept-routes=false --operator=ada") {
		t.Errorf("up = %q", got[len(got)-1])
	}
}

// A joined VM isn't joined again (a key typed is noted, not used); its
// settings are kept with `tailscale set`, only when they've drifted.
func TestTailscaleJoined(t *testing.T) {
	prefs := `{"RouteAll": true, "OperatorUser": ""}`
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		switch {
		case c.Name == "tailscale" && c.Args[0] == "status":
			return []byte(`{"BackendState": "Running", "Self": {"DNSName": "payzoff.tail1234.ts.net."}}`), nil
		case c.Name == "tailscale" && c.Args[0] == "debug":
			return []byte(prefs), nil
		case c.Name == "tailscale" && c.Args[0] == "set":
			prefs = `{"RouteAll": false, "OperatorUser": "ada"}`
		}
		return nil, nil
	}}
	e := testEnv(t, x)
	e.State = t.TempDir()
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	e.Secrets = creds.New()
	e.Secrets.TSAuthKey = "tskey-auth-1"
	st := step(t, catalog.Step{Type: "go:tailscale_up"})
	if check(t, e, st) {
		t.Fatal("drifted prefs: Check = true")
	}
	apply(t, e, st)
	cmds := strings.Join(x.commands(), "\n")
	if strings.Contains(cmds, "tailscale up") || !strings.Contains(cmds, "tailscale set --accept-routes=false --operator=ada") {
		t.Errorf("ran %s", cmds)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "already on the tailnet as payzoff.tail1234.ts.net,") {
		t.Errorf("notes = %q", notes)
	}
	e.Secrets.TSAuthKey = ""
	if !check(t, e, st) {
		t.Error("joined, prefs kept: Check = false")
	}
}

// netbird up reads the setup key from a file (not the command line, which
// ps shows), with the management URL when one was typed.
func TestNetbirdUp(t *testing.T) {
	var keyFile string
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "netbird" {
			keyFile = c.Args[2]
			b, _ := os.ReadFile(keyFile)
			if string(b) != "NB-KEY-1" {
				t.Errorf("key file holds %q", b)
			}
		}
		return nil, nil
	}}
	e := testEnv(t, x)
	st := step(t, catalog.Step{Type: "go:netbird_up"})
	if !check(t, e, st) {
		t.Error("no key typed: Check = false")
	}
	e.Secrets = creds.New()
	e.Secrets.NBSetupKey, e.Secrets.NBManagementURL = "NB-KEY-1", "https://netbird.example.com"
	apply(t, e, st)
	cmds := strings.Join(x.commands(), " ")
	if strings.Contains(cmds, "NB-KEY") || !strings.Contains(cmds, "--setup-key-file") || !strings.Contains(cmds, "--management-url https://netbird.example.com") {
		t.Errorf("commands: %s", cmds)
	}
	if _, err := os.Stat(keyFile); err == nil {
		t.Error("key file left behind")
	}
}

func TestNoteWhen(t *testing.T) {
	cond := false
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if cond {
			return nil, nil
		}
		return nil, errors.New("false")
	}}
	e := testEnv(t, x)
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	st := step(t, catalog.Step{Type: "note", Text: "see {{home}}", When: "test -e {{home}}/x"})
	apply(t, e, st)
	cond = true
	apply(t, e, st)
	if len(notes) != 1 || notes[0] != "see /home/ada" {
		t.Errorf("notes = %q", notes)
	}
	if x.ran[0].Args[1] != "test -e /home/ada/x" {
		t.Errorf("when ran as %q", x.ran[0].Args[1])
	}
}

func TestGitConfig(t *testing.T) {
	cfg := map[string]string{}
	haveDelta := false
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		switch {
		case c.Name == "sh":
			if haveDelta {
				return nil, nil
			}
			return nil, errors.New("no delta")
		case c.Name == "git" && c.Args[2] == "--get":
			if v, ok := cfg[c.Args[3]]; ok {
				return []byte(v + "\n"), nil
			}
			return nil, errors.New("unset")
		case c.Name == "git":
			cfg[c.Args[2]] = c.Args[3]
		}
		return nil, nil
	}}
	e := testEnv(t, x)
	e.Answers = &answers.Answers{Git: answers.Git{Email: "ada@example.com"}}
	id := step(t, catalog.Step{Type: "git_config", Values: map[string]string{"user.name": "{{git_name}}", "user.email": "{{git_email}}"}})
	apply(t, e, id)
	// No name in the answers: the user's own (or GitHub's) stays.
	if _, ok := cfg["user.name"]; ok || cfg["user.email"] != "ada@example.com" {
		t.Errorf("git config = %v", cfg)
	}
	if !check(t, e, id) {
		t.Error("Check after Apply = false")
	}
	delta := step(t, catalog.Step{Type: "git_config", IfTool: "delta", Values: map[string]string{"core.pager": "delta"}})
	if !check(t, e, delta) {
		t.Error("without delta: Check = false")
	}
	haveDelta = true
	if check(t, e, delta) {
		t.Error("with delta, unset: Check = true")
	}
	apply(t, e, delta)
	if cfg["core.pager"] != "delta" {
		t.Errorf("git config = %v", cfg)
	}
}

func TestFail2banPublicIPv4(t *testing.T) {
	old := interfaceAddrs
	defer func() { interfaceAddrs = old }()
	set := func(cidrs ...string) {
		interfaceAddrs = func() ([]net.Addr, error) {
			var out []net.Addr
			for _, c := range cidrs {
				ip, n, err := net.ParseCIDR(c)
				if err != nil {
					t.Fatal(err)
				}
				n.IP = ip
				out = append(out, n)
			}
			return out, nil
		}
	}
	e := shellEnv(t)
	x := &fakeExec{}
	e.Exec = x
	st := step(t, catalog.Step{Type: "go:fail2ban"})
	set("127.0.0.1/8", "10.0.0.5/24", "192.168.1.9/24", "172.17.0.1/16", "100.101.102.103/32", "169.254.1.1/16", "fe80::1/64", "2001:db8::5/64")
	if publicIPv4() != "" || !check(t, e, st) {
		t.Errorf("private only: public = %q, Check should be true", publicIPv4())
	}
	apply(t, e, st)
	if got := x.commands(); len(got) != 0 {
		t.Errorf("ran %q without a public IPv4", got)
	}
	set("10.0.0.5/24", "203.0.113.7/24")
	if got := publicIPv4(); got != "203.0.113.7" {
		t.Errorf("public = %q", got)
	}
	if plan := fmt.Sprint(st.Plan(e)); !strings.Contains(plan, "fail2ban python3-systemd (public IPv4 203.0.113.7)") {
		t.Errorf("plan = %s", plan)
	}
}
