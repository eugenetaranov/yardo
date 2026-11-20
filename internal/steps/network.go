package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
)

// The machine's networks, as the Connection screen starts, stops and
// removes them: Tailscale and NetBird (kind and id alike), and the
// WireGuard and OpenVPN client profiles (id wg:<name>, ovpn:<name>).
const (
	NetTailscale = "tailscale"
	NetNetbird   = "netbird"
	NetWireGuard = "wireguard"
	NetOpenVPN   = "openvpn"
)

// ProfileDir is where a client kind's profiles live, root's only.
func ProfileDir(kind string) string {
	if kind == NetOpenVPN {
		return "/etc/openvpn/client"
	}
	return "/etc/wireguard"
}

// ProfileUnit is the systemd unit that runs profile name of kind.
func ProfileUnit(kind, name string) string {
	if kind == NetOpenVPN {
		return "openvpn-client@" + name
	}
	return "wg-quick@" + name
}

// NetID is a network's id: the kind for a mesh, kind's prefix and the
// name for a profile.
func NetID(kind, name string) string {
	switch kind {
	case NetWireGuard:
		return "wg:" + name
	case NetOpenVPN:
		return "ovpn:" + name
	}
	return kind
}

// ParseNetID is NetID's kind and name.
func ParseNetID(id string) (kind, name string) {
	if n, ok := strings.CutPrefix(id, "wg:"); ok {
		return NetWireGuard, n
	}
	if n, ok := strings.CutPrefix(id, "ovpn:"); ok {
		return NetOpenVPN, n
	}
	return id, ""
}

// netCmds are the commands act (start, stop, boot, noboot, remove) runs
// on network id. A profile's remove also deletes its file (NetAct).
func netCmds(e *Env, id, act string) ([]Cmd, error) {
	kind, name := ParseNetID(id)
	sys := func(args ...string) Cmd { return Cmd{Name: "systemctl", Args: args} }
	switch kind {
	case NetTailscale:
		switch act {
		case "stop":
			return []Cmd{{Name: "tailscale", Args: []string{"down"}}}, nil
		case "start":
			// yardo's settings, as up needs every one that isn't the default.
			return []Cmd{{Name: "tailscale", Args: append([]string{"up"}, tsFlags(e)...)}}, nil
		case "remove":
			return []Cmd{{Name: "tailscale", Args: []string{"logout"}}}, nil
		}
	case NetNetbird:
		switch act {
		case "stop":
			return []Cmd{{Name: "netbird", Args: []string{"down"}}}, nil
		case "start":
			return []Cmd{{Name: "netbird", Args: []string{"up"}}}, nil
		case "remove":
			return []Cmd{{Name: "netbird", Args: []string{"down"}}, {Name: "netbird", Args: []string{"logout"}}}, nil
		}
	case NetWireGuard, NetOpenVPN:
		u := ProfileUnit(kind, name)
		switch act {
		case "stop":
			return []Cmd{sys("stop", u)}, nil
		case "start":
			return []Cmd{sys("start", u)}, nil
		case "boot":
			return []Cmd{sys("enable", u)}, nil
		case "noboot":
			return []Cmd{sys("disable", u)}, nil
		case "remove":
			return []Cmd{sys("disable", "--now", u)}, nil
		}
	}
	return nil, fmt.Errorf("%s: can't %s it", id, act)
}

// NetAct does act (start, stop, boot, noboot, remove) to network id. A
// failure says its last line. Removing a mesh logs it out (its join's
// record goes, so yardo forget leaves it); a profile's file goes too.
func NetAct(ctx context.Context, e *Env, id, act string) error {
	cmds, err := netCmds(e, id, act)
	if err != nil {
		return err
	}
	for i, c := range cmds {
		out, err := e.run(ctx, c)
		// NetBird's logout after its down: one already down still logs out.
		downFirst := act == "remove" && i == 0 && len(cmds) > 1
		if err != nil && !downFirst {
			return fmt.Errorf("%s: %s", c, lastLine(out, err))
		}
	}
	kind, name := ParseNetID(id)
	if act != "remove" {
		return nil
	}
	switch kind {
	case NetWireGuard, NetOpenVPN:
		return rmIfThere(e.Path(filepath.Join(ProfileDir(kind), name+".conf")))
	}
	return dropCreated(e, kind)
}

// lastLine is what a command said last, or its error.
func lastLine(out []byte, err error) string {
	s := strings.TrimSpace(string(out))
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if s == "" {
		return err.Error()
	}
	return s
}

// dropCreated takes kind's record (a mesh's join) off created.json.
func dropCreated(e *Env, kind string) error {
	if e.State == "" {
		return nil
	}
	return answers.DropCreated(e.State, answers.Created{Kind: kind})
}

// profileName is a profile's name: it's its interface's too, so short.
var profileName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,14}$`)

// CheckProfileName says why name can't name a profile, or nil.
func CheckProfileName(name string) error {
	if !profileName.MatchString(name) {
		return fmt.Errorf("%q: a name is lowercase letters, digits and -, 15 at most", name)
	}
	return nil
}

// CheckProfile says why conf isn't a profile of kind, or nil.
func CheckProfile(kind string, conf []byte) error {
	s := string(conf)
	switch kind {
	case NetWireGuard:
		if !regexp.MustCompile(`(?m)^\s*\[Interface\]`).MatchString(s) {
			return errors.New("it isn't a WireGuard profile: there's no [Interface] section")
		}
		if !regexp.MustCompile(`(?m)^\s*PrivateKey\s*=`).MatchString(s) {
			return errors.New("it isn't a WireGuard profile: its [Interface] has no PrivateKey")
		}
		if !regexp.MustCompile(`(?m)^\s*\[Peer\]`).MatchString(s) {
			return errors.New("it isn't a WireGuard profile: there's no [Peer] section")
		}
	case NetOpenVPN:
		if !regexp.MustCompile(`(?m)^\s*remote\s+\S`).MatchString(s) {
			return errors.New("it isn't an OpenVPN profile: there's no remote line")
		}
	default:
		return fmt.Errorf("%s: no such VPN client", kind)
	}
	return nil
}

// FullTunnel reports whether profile conf sends all the machine's
// traffic through it (its default route).
func FullTunnel(kind string, conf []byte) bool {
	if kind == NetOpenVPN {
		return regexp.MustCompile(`(?m)^\s*redirect-gateway`).Match(conf)
	}
	return regexp.MustCompile(`(?m)^\s*AllowedIPs\s*=.*(0\.0\.0\.0/0|::/0)`).Match(conf)
}

// ProfileWait is how long AddProfile waits for a new profile to connect.
var ProfileWait = 15 * time.Second

// AddProfile stores conf as profile name of kind (root's only), starts it
// at boot and now, and waits for it to connect. A bad name, a name taken
// or a bad profile is refused with nothing stored; connected is false when
// it's up but nothing answered yet. The profile itself is never logged.
func AddProfile(ctx context.Context, e *Env, kind, name string, conf []byte) (connected bool, err error) {
	if err := CheckProfileName(name); err != nil {
		return false, err
	}
	if err := CheckProfile(kind, conf); err != nil {
		return false, err
	}
	p := e.Path(filepath.Join(ProfileDir(kind), name+".conf"))
	if _, err := os.Lstat(p); err == nil {
		return false, fmt.Errorf("there's a profile called %s already", name)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return false, err
	}
	if err := writeFile(p, conf, 0o600); err != nil {
		return false, err
	}
	if e.Log != nil {
		fmt.Fprintf(e.Log, "wrote %s (0600, root)\n", p)
	}
	u := ProfileUnit(kind, name)
	if out, err := e.run(ctx, Cmd{Name: "systemctl", Args: []string{"enable", "--now", u}}); err != nil {
		return false, fmt.Errorf("%s didn't start: %s", u, lastLine(out, err))
	}
	for end := time.Now().Add(ProfileWait); ; {
		if profileUp(ctx, e, kind, name) {
			return true, nil
		}
		if !time.Now().Before(end) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// profileUp reports whether profile name answered: a WireGuard handshake,
// or OpenVPN's "Initialization Sequence Completed".
func profileUp(ctx context.Context, e *Env, kind, name string) bool {
	if kind == NetOpenVPN {
		out, _ := e.Exec.Run(ctx, Cmd{Name: "journalctl", Args: []string{"-u", ProfileUnit(kind, name), "-n", "50", "--no-pager", "-o", "cat"}})
		return bytes.Contains(out, []byte("Initialization Sequence Completed"))
	}
	out, _ := e.Exec.Run(ctx, Cmd{Name: "wg", Args: []string{"show", name, "latest-handshakes"}})
	for _, l := range strings.Split(string(out), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[1] != "0" {
			return true
		}
	}
	return false
}
