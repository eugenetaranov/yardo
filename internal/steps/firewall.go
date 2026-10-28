package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// yardo's firewall: nftables tables of its own, one per file in
// /etc/yardo/firewall, loaded at boot by yardo-firewall.service. They only
// drop (what a service's access setting keeps out); they never flush or
// touch Docker's, the VPNs' or anyone else's rules.
const (
	fwDir  = "/etc/yardo/firewall"
	fwUnit = "/etc/systemd/system/yardo-firewall.service"
)

// VPNInterfaces are the interfaces "VPN only" lets in: Tailscale, NetBird,
// yardo's WireGuard and OpenVPN servers, and a WireGuard tunnel of your own.
var VPNInterfaces = []string{"tailscale0", "wt0", "yardo-wg", "tun0", "wg0"}

const fwUnitText = `[Unit]
Description=yardo's firewall tables (/etc/yardo/firewall)
Wants=network-pre.target
After=network-pre.target
Before=network.target docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -c 'for f in /etc/yardo/firewall/*.nft; do [ -e "$f" ] && /usr/sbin/nft -f "$f"; done; exit 0'

[Install]
WantedBy=multi-user.target
`

// nftName is s as an nftables name: letters, digits and _.
func nftName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, s)
}

// nftSet is an anonymous nftables set of interface names.
func nftSet(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	return "{ " + strings.Join(q, ", ") + " }"
}

// fwTable is one of yardo's tables: its file, and its chains ("" means the
// table shouldn't be there).
type fwTable struct{ name, chains string }

func (t fwTable) file() string { return filepath.Join(fwDir, t.name+".nft") }

// text is the table's file: the table made (so it can be deleted), deleted,
// and defined again, so loading it twice changes nothing.
func (t fwTable) text() []byte {
	return []byte(fmt.Sprintf("# yardo rewrites this file.\ntable inet %[1]s\ndelete table inet %[1]s\ntable inet %[1]s {\n%[2]s}\n", t.name, t.chains))
}

func (t fwTable) Check(ctx context.Context, e *Env) bool {
	cur, err := os.ReadFile(e.Path(t.file()))
	if t.chains == "" {
		return os.IsNotExist(err)
	}
	if err != nil || string(cur) != string(t.text()) || !same(e.Path(fwUnit), wantFile{b: []byte(fwUnitText), mode: 0o644}) {
		return false
	}
	_, err = e.Exec.Run(ctx, Cmd{Name: "nft", Args: []string{"list", "table", "inet", t.name}})
	return err == nil
}

func (t fwTable) Apply(ctx context.Context, e *Env) error {
	if t.chains == "" {
		if err := os.Remove(e.Path(t.file())); err != nil && !os.IsNotExist(err) {
			return err
		}
		_, _ = e.Exec.Run(ctx, Cmd{Name: "nft", Args: []string{"delete", "table", "inet", t.name}})
		return nil
	}
	if _, err := e.Exec.Run(ctx, Cmd{Name: "sh", Args: []string{"-c", "command -v nft"}}); err != nil {
		if err := AptInstall(ctx, e, []string{"nftables"}); err != nil {
			return err
		}
	}
	if p := e.Path(fwUnit); !same(p, wantFile{b: []byte(fwUnitText), mode: 0o644}) {
		if err := writeFile(p, []byte(fwUnitText), 0o644); err != nil {
			return err
		}
		for _, args := range [][]string{{"daemon-reload"}, {"enable", "yardo-firewall"}} {
			if _, err := e.run(ctx, Cmd{Name: "systemctl", Args: args}); err != nil {
				return err
			}
		}
	}
	if err := os.MkdirAll(e.Path(fwDir), 0o755); err != nil {
		return err
	}
	if err := writeFile(e.Path(t.file()), t.text(), 0o644); err != nil {
		return err
	}
	_, err := e.run(ctx, Cmd{Name: "nft", Args: []string{"-f", t.file()}})
	return err
}

// mcpTable is the user's gateway port, let in only from this machine and
// the VPN interfaces when its access is "vpn" (no table otherwise).
func mcpTable(e *Env) fwTable {
	t := fwTable{name: "yardo_mcp_" + nftName(e.User)}
	if listen(e) == "vpn" {
		port := MCPPort(e)
		t.chains = fmt.Sprintf(`	chain input {
		type filter hook input priority filter - 1; policy accept;
		tcp dport %[1]d iifname "lo" accept
		tcp dport %[1]d iifname %[2]s accept
		tcp dport %[1]d drop
	}
`, port, nftSet(VPNInterfaces))
	}
	return t
}

// DockerPorts is where containers' published ports can be reached from:
// "" (everywhere, as Docker does), "vpn" or "local" (this machine only).
func DockerPorts(e *Env) string {
	if e.Answers == nil {
		return ""
	}
	switch e.Answers.DockerPorts {
	case "vpn", "local":
		return e.Answers.DockerPorts
	}
	return ""
}

// dockerTable keeps new connections to published ports (Docker forwards
// them: ct status dnat) out, but from this machine, its containers and (for
// "vpn") the VPN interfaces. Docker itself needn't restart.
func dockerTable(e *Env) fwTable {
	t := fwTable{name: "yardo_docker_ports"}
	mode := DockerPorts(e)
	if mode == "" {
		return t
	}
	vpn := ""
	if mode == "vpn" {
		vpn = fmt.Sprintf("\t\tct state new ct status dnat iifname %s accept\n", nftSet(VPNInterfaces))
	}
	t.chains = `	chain forward {
		type filter hook forward priority filter - 1; policy accept;
		ct state new ct status dnat iifname { "lo", "docker0" } accept
		ct state new ct status dnat iifname "br-*" accept
` + vpn + `		ct state new ct status dnat drop
	}
`
	return t
}
