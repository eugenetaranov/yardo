package steps

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
)

const wgProfile = "[Interface]\nPrivateKey = c2VjcmV0LWtleS1kby1ub3QtbG9n\nAddress = 10.9.0.2/32\n\n[Peer]\nPublicKey = abc\nEndpoint = vpn.example.com:51820\nAllowedIPs = 10.9.0.0/24\n"

// Each network's start, stop, boot and remove are its own commands; a
// profile's remove deletes its file, a mesh's drops its join's record.
func TestNetAct(t *testing.T) {
	x := &fakeExec{}
	e := testEnv(t, x)
	e.State = t.TempDir()
	put(t, e.Path("/etc/wireguard/office.conf"), wgProfile)
	if err := answers.AddCreated(e.State, answers.Created{Kind: "tailscale"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ id, act, want string }{
		{"tailscale", "stop", "tailscale down"},
		{"tailscale", "start", "tailscale up --accept-routes=false --operator=ada"},
		{"netbird", "stop", "netbird down"},
		{"netbird", "remove", "netbird down\nnetbird logout"},
		{"wg:office", "stop", "systemctl stop wg-quick@office"},
		{"ovpn:home", "start", "systemctl start openvpn-client@home"},
		{"wg:office", "noboot", "systemctl disable wg-quick@office"},
		{"wg:office", "remove", "systemctl disable --now wg-quick@office"},
		{"tailscale", "remove", "tailscale logout"},
	} {
		x.ran = nil
		if err := NetAct(context.Background(), e, c.id, c.act); err != nil {
			t.Fatalf("%s %s: %v", c.act, c.id, err)
		}
		if got := strings.Join(x.commands(), "\n"); got != c.want {
			t.Errorf("%s %s ran %q, want %q", c.act, c.id, got, c.want)
		}
	}
	if exists(e.Path("/etc/wireguard/office.conf")) {
		t.Error("the profile is still there")
	}
	if len(answers.ReadCreated(e.State)) != 0 {
		t.Errorf("tailscale's record stays: %v", answers.ReadCreated(e.State))
	}
	if err := NetAct(context.Background(), e, "tailscale", "boot"); err == nil {
		t.Error("boot on Tailscale")
	}
}

// Adding a profile: checked, stored root's only, started at boot and now,
// then waited for; a bad one or a name taken stores nothing; the profile
// never reaches the log.
func TestAddProfile(t *testing.T) {
	ProfileWait = 0
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "wg" {
			return []byte("pubkey\t1731000000\n"), nil
		}
		return nil, nil
	}}
	e := testEnv(t, x)
	var log bytes.Buffer
	e.Log = &log
	ok, err := AddProfile(context.Background(), e, NetWireGuard, "office", []byte(wgProfile))
	if err != nil || !ok {
		t.Fatalf("add: %v %v", ok, err)
	}
	p := e.Path("/etc/wireguard/office.conf")
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 || read(t, p) != wgProfile {
		t.Errorf("stored: %v %v", fi, err)
	}
	if got := strings.Join(x.commands(), "\n"); !strings.Contains(got, "systemctl enable --now wg-quick@office") {
		t.Errorf("ran %q", got)
	}
	if strings.Contains(log.String(), "c2VjcmV0") {
		t.Errorf("the key is in the log:\n%s", log.String())
	}
	for name, c := range map[string]struct{ kind, name, conf, want string }{
		"taken":        {NetWireGuard, "office", wgProfile, "already"},
		"no interface": {NetWireGuard, "home", "[Peer]\nPublicKey = x\n", "no [Interface]"},
		"no peer":      {NetWireGuard, "home", "[Interface]\nPrivateKey = x\n", "no [Peer]"},
		"no remote":    {NetOpenVPN, "home", "client\ndev tun\n", "no remote line"},
		"bad name":     {NetWireGuard, "Home Office", wgProfile, "lowercase"},
	} {
		if _, err := AddProfile(context.Background(), e, c.kind, c.name, []byte(c.conf)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
	if exists(e.Path("/etc/wireguard/home.conf")) || exists(e.Path("/etc/openvpn/client/home.conf")) {
		t.Error("a refused profile was stored")
	}
	// OpenVPN, up but nothing answered yet.
	ok, err = AddProfile(context.Background(), e, NetOpenVPN, "home", []byte("client\nremote vpn.example.com 1194\nredirect-gateway def1\n"))
	if err != nil || ok {
		t.Errorf("openvpn: %v %v", ok, err)
	}
	if !FullTunnel(NetOpenVPN, []byte("redirect-gateway def1\n")) || FullTunnel(NetWireGuard, []byte(wgProfile)) || !FullTunnel(NetWireGuard, []byte("AllowedIPs = 0.0.0.0/0, ::/0\n")) {
		t.Error("FullTunnel")
	}
}
