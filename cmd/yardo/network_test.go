package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// replyExec answers each command line from its map ("" for the rest).
type replyExec map[string]string

func (r replyExec) Run(_ context.Context, c steps.Cmd) ([]byte, error) {
	return []byte(r[c.String()]), nil
}

// The networks, read from the VPNs and the profiles: Tailscale running,
// NetBird needing its login, one profile up and one down, a tunnel no
// profile accounts for, then the local network; the session's is marked.
func TestNetworks(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"/etc/wireguard/office.conf", "/etc/wireguard/lab.conf"} {
		if err := os.MkdirAll(filepath.Dir(root+f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(root+f, []byte("[Interface]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	x := replyExec{
		"tailscale status --json":              `{"BackendState":"Running","Self":{"DNSName":"vm.tail1234.ts.net.","TailscaleIPs":["100.64.1.7","fd7a::1"]}}`,
		"netbird status":                       "Daemon status: NeedsLogin\n",
		"systemctl is-active wg-quick@lab":     "active\n",
		"systemctl is-enabled wg-quick@lab":    "enabled\n",
		"systemctl is-active wg-quick@office":  "inactive\n",
		"systemctl is-enabled wg-quick@office": "disabled\n",
	}
	addrs := []address{{"100.64.1.7", "tailscale0", "Tailscale", ""}, {"192.168.0.5", "eth0", "", ""}, {"10.9.0.2", "lab", "", ""}, {"10.135.0.2", "tun0", "OpenVPN client", ""}}
	nets := networks(context.Background(), x, root, func(string) bool { return false }, addrs, "100.64.1.7")
	var got []string
	for _, n := range nets {
		got = append(got, strings.Join([]string{n.ID, n.State, n.Addr, n.DNS, n.Boot}, "|")+map[bool]string{true: " via"}[n.Via])
	}
	want := []string{"tailscale|up|100.64.1.7|vm.tail1234.ts.net| via", "netbird|login|||", "wg:lab|up|10.9.0.2||on", "wg:office|down|||off",
		"if:tun0|up|10.135.0.2||", "local|up|192.168.0.5||"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("networks:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if wb := wayBack(nets, "ada"); wb != "ssh ada@192.168.0.5 (local network)" {
		t.Errorf("way back = %q", wb)
	}
	// Over the local network: it's marked, and Tailscale is the way back.
	nets = networks(context.Background(), x, root, func(string) bool { return false }, addrs, "192.168.0.5")
	if !nets[len(nets)-1].Via || nets[0].Via || wayBack(nets, "ada") != "ssh ada@100.64.1.7 (tailscale)" {
		t.Errorf("local: %+v", nets)
	}
	// Installed, the daemon not answering: listed, stopped, saying why.
	nets = networks(context.Background(), replyExec{}, t.TempDir(), func(id string) bool { return id == "tailscale-install" }, nil, "")
	if len(nets) != 1 || nets[0].State != "down" || !strings.Contains(nets[0].Note, "tailscaled") {
		t.Errorf("not answering: %+v", nets)
	}
	// Waiting for its login: the link.
	nets = networks(context.Background(), replyExec{"tailscale status --json": `{"BackendState":"NeedsLogin","AuthURL":"https://login.tailscale.com/a/x"}`}, t.TempDir(), func(string) bool { return false }, nil, "")
	if nets[0].State != "login" || nets[0].Link != "https://login.tailscale.com/a/x" {
		t.Errorf("login: %+v", nets)
	}
}

// The session's address: SSH_CONNECTION's third field, here or in a
// parent's environment; none on a pseudo-terminal is unsure.
func TestSessionVia(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "192.168.0.10 50000 100.64.1.7 22")
	if a, unsure := sessionVia(func(int) []string { return nil }, "/dev/pts/0"); a != "100.64.1.7" || unsure {
		t.Errorf("here: %q %v", a, unsure)
	}
	t.Setenv("SSH_CONNECTION", "")
	parent := func(int) []string { return []string{"HOME=/root", "SSH_CONNECTION=10.0.0.9 1 10.0.0.5 22"} }
	if a, _ := sessionVia(parent, ""); a != "10.0.0.5" {
		t.Errorf("parent: %q", a)
	}
	none := func(int) []string { return nil }
	if a, unsure := sessionVia(none, "/dev/pts/3"); a != "" || !unsure {
		t.Errorf("pts: %q %v", a, unsure)
	}
	if _, unsure := sessionVia(none, "/dev/tty1"); unsure {
		t.Error("the console is unsure")
	}
}

// The screen's Claude Code row and yardo connect have the same line; with
// the gateway this machine's only, the tunnel.
func TestLaptopSetups(t *testing.T) {
	host, _ := os.Hostname()
	short := strings.SplitN(host, ".", 2)[0]
	addrs := []address{{"100.64.1.7", "tailscale0", "Tailscale", ""}}
	a := &answers.Answers{User: "ada", MCPListen: "vpn", MCPPort: 8081}
	ss := laptopSetups(a, addrs, nil, true, short)
	var out bytes.Buffer
	printConnect(&out, a, addrs, nil, "", nil)
	for _, id := range []string{"ssh", "docker", "mcp-claude", "mcp-codex", "clipboard"} {
		for _, l := range setupLines(ss, id) {
			if !strings.Contains(ui.Unmark(out.String()), l) {
				t.Errorf("%s: yardo connect lacks %q", id, l)
			}
		}
	}
	claude := setupLines(ss, "mcp-claude")
	if len(claude) != 1 || claude[0] != `claude mcp add --transport http `+short+` http://100.64.1.7:8081/mcp --header "Authorization: Bearer <token>"` {
		t.Errorf("claude: %q", claude)
	}
	for _, s := range ss {
		if strings.HasPrefix(s.ID, "mcp") && s.Token != laptopToken("ada", "client") {
			t.Errorf("%s: token file %q", s.ID, s.Token)
		}
	}
	a.MCPListen = ""
	ss = laptopSetups(a, addrs, nil, true, short)
	if l := setupLines(ss, "mcp-claude"); len(l) != 2 || !strings.HasPrefix(l[0], "ssh -N -L 8081:127.0.0.1:8081 ada@100.64.1.7") || strings.Contains(l[1], "Bearer") {
		t.Errorf("tunnel: %q", l)
	}
	if ss = laptopSetups(a, addrs, nil, false, short); len(ss) != 4 || ss[2].ID != "clipboard" || ss[3].ID != "script" || !strings.Contains(ss[3].Lines[0], "sudo yardo connect --script > yardo-"+short+".sh") {
		t.Errorf("no gateway: %+v", ss)
	}
}

// The laptop's script: run twice against a fake laptop, the second run
// adds nothing and exits 0; it holds no token, reading it over ssh.
func TestConnectScript(t *testing.T) {
	home, bin := t.TempDir(), t.TempDir()
	log := filepath.Join(home, "log")
	fake := map[string]string{
		"ssh":    `echo "ssh $*" >> ` + log + `; printf 'f00dfeed\r\n'`,
		"docker": `echo "docker $*" >> ` + log + `; [ "$2" = inspect ] && [ -f ` + home + `/ctx ] && exit 0; [ "$2" = create ] && touch ` + home + `/ctx; [ "$2" = inspect ] && exit 1; exit 0`,
		"claude": `echo "claude $*" >> ` + log + `; [ "$2" = get ] && [ -f ` + home + `/mcp ] && exit 0; [ "$2" = add ] && touch ` + home + `/mcp; [ "$2" = get ] && exit 1; exit 0`,
		// iTerm2 is installed; its clipboard leave lands in a file.
		"defaults": `echo "defaults $*" >> ` + log + `; [ "$1" = write ] && echo 1 > ` + home + `/clip; [ "$3" = AllowClipboardAccess ] && cat ` + home + `/clip 2>/dev/null; exit 0`,
	}
	for n, body := range fake {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := &answers.Answers{User: "ada", MCPListen: "vpn", MCPPort: 8081}
	var script bytes.Buffer
	connectScript(&script, a, []address{{"100.64.1.7", "tailscale0", "Tailscale", ""}}, nil, true, "vm")
	if strings.Contains(script.String(), "f00dfeed") {
		t.Fatal("a token in the script")
	}
	path := filepath.Join(home, "yardo-vm.sh")
	if err := os.WriteFile(path, script.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func() string {
		c := exec.Command("sh", path)
		c.Env = []string{"HOME=" + home, "PATH=" + bin + ":/usr/bin:/bin"}
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s\n%s", err, out, script.String())
		}
		return string(out)
	}
	out := run()
	conf, _ := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if !strings.Contains(string(conf), "Host vm\n  HostName 100.64.1.7\n  User ada") || !strings.Contains(out, "added Host vm") ||
		!strings.Contains(out, "made the context vm") || !strings.Contains(out, "added the MCP server vm") ||
		!strings.Contains(out, "iTerm2: takes copies from vm (") {
		t.Errorf("first run:\n%s\nconfig:\n%s", out, conf)
	}
	l, _ := os.ReadFile(log)
	if !strings.Contains(string(l), `claude mcp add --transport http vm http://100.64.1.7:8081/mcp --header Authorization: Bearer f00dfeed`) ||
		!strings.Contains(string(l), "ssh -t vm sudo cat ~/.mcp/clients/client.token") {
		t.Errorf("commands:\n%s", l)
	}
	out = run()
	conf2, _ := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if string(conf2) != string(conf) || strings.Contains(out, "added") || strings.Contains(out, "made") || !strings.Contains(out, "iTerm2: takes copies from vm already") {
		t.Errorf("second run changed something:\n%s", out)
	}
}
