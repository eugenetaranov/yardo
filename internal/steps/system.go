package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

func init() {
	RegisterGo("docker_daemon", func(catalog.Step) Step { return dockerDaemonStep{} })
	RegisterGo("timezone", func(catalog.Step) Step { return timezoneStep{} })
	RegisterGo("tailscale_up", func(catalog.Step) Step { return tailscaleUpStep{} })
	RegisterGo("netbird_up", func(catalog.Step) Step { return netbirdUpStep{} })
	RegisterGo("sshharden", func(catalog.Step) Step { return sshHardenStep{} })
	RegisterGo("fail2ban", func(catalog.Step) Step { return fail2banStep{} })
}

// root reports whether a command is on root's PATH.
func root(ctx context.Context, e *Env, tool string) bool {
	_, err := e.Exec.Run(ctx, Cmd{Name: "sh", Args: []string{"-c", "command -v " + tool}})
	return err == nil
}

// --- go:docker_daemon --------------------------------------------------------

// dockerDaemonStep writes /etc/docker/daemon.json (log rotation, address
// pools) before the engine is installed, for a fresh engine only: a later
// change means a restart that stops running containers.
type dockerDaemonStep struct{}

const dockerDaemon = "/etc/docker/daemon.json"

func (dockerDaemonStep) BeforeApt() bool { return true }

func (dockerDaemonStep) Plan(*Env) []Action {
	return []Action{{"file", dockerDaemon + " (a fresh engine only)"}}
}

func (dockerDaemonStep) Check(ctx context.Context, e *Env) (bool, error) {
	if _, err := os.Stat(e.Path(dockerDaemon)); err == nil {
		return true, nil
	}
	return root(ctx, e, "dockerd"), nil
}

func (d dockerDaemonStep) Apply(ctx context.Context, e *Env) error {
	if done, _ := d.Check(ctx, e); done {
		return nil
	}
	b, err := fs.ReadFile(e.Files, "daemon.json")
	if err != nil {
		return err
	}
	return writeFile(e.Path(dockerDaemon), b, 0o644)
}

// --- go:timezone -------------------------------------------------------------

type timezoneStep struct{}

func tz(e *Env) string {
	if e.Answers == nil {
		return ""
	}
	return e.Answers.TZ
}

func (timezoneStep) Plan(e *Env) []Action {
	if tz(e) == "" {
		return nil
	}
	return []Action{{"run", "timedatectl set-timezone " + tz(e)}}
}

func (timezoneStep) Check(ctx context.Context, e *Env) (bool, error) {
	if tz(e) == "" {
		return true, nil
	}
	out, err := e.Exec.Run(ctx, Cmd{Name: "timedatectl", Args: []string{"show", "-p", "Timezone", "--value"}})
	return err == nil && strings.TrimSpace(string(out)) == tz(e), nil
}

func (timezoneStep) Apply(ctx context.Context, e *Env) error {
	if tz(e) == "" {
		return nil
	}
	_, err := e.run(ctx, Cmd{Name: "timedatectl", Args: []string{"set-timezone", tz(e)}})
	return err
}

// --- go:tailscale_up ---------------------------------------------------------

// tailscaleUpStep joins the tailnet with the auth key typed this run (none:
// the user runs `sudo tailscale up` later), and keeps two settings on a
// joined VM: no subnet routes accepted (a VM on the advertised LAN would
// send its own LAN traffic through the router) and the user as operator
// (`tailscale serve` without sudo). A joined VM isn't joined again.
type tailscaleUpStep struct{}

func authKey(e *Env) string {
	if e.Secrets == nil {
		return ""
	}
	return e.Secrets.TSAuthKey
}

// TSStatus is what `tailscale status --json` says of this machine.
type TSStatus struct {
	BackendState string
	Self         struct {
		DNSName   string
		Tags      []string
		KeyExpiry *time.Time
	}
}

// TailscaleStatus is this machine's Tailscale status; ok is false when
// tailscale isn't there or says nothing readable.
func TailscaleStatus(ctx context.Context, x Execer) (st TSStatus, ok bool) {
	out, err := x.Run(ctx, Cmd{Name: "tailscale", Args: []string{"status", "--json"}})
	if len(out) == 0 || json.Unmarshal(out, &st) != nil {
		return st, false
	}
	_ = err // logged out: status exits non-zero, with the JSON
	return st, true
}

func (tailscaleUpStep) joined(ctx context.Context, e *Env) (TSStatus, bool) {
	st, ok := TailscaleStatus(ctx, e.Exec)
	return st, ok && st.BackendState == "Running"
}

// tsFlags are the settings yardo keeps, for `up` and `set` alike.
func tsFlags(e *Env) []string {
	return []string{"--accept-routes=false", "--operator=" + e.User}
}

// prefsOK reports whether tailscale's prefs have yardo's settings; true
// when they can't be read (no loop of `set` every run).
func (tailscaleUpStep) prefsOK(ctx context.Context, e *Env) bool {
	out, err := e.Exec.Run(ctx, Cmd{Name: "tailscale", Args: []string{"debug", "prefs"}})
	var p struct {
		RouteAll     bool
		OperatorUser string
	}
	if err != nil || json.Unmarshal(out, &p) != nil {
		return true
	}
	return !p.RouteAll && p.OperatorUser == e.User
}

func (t tailscaleUpStep) Plan(e *Env) []Action {
	if authKey(e) == "" {
		return []Action{{"run", "tailscale set " + strings.Join(tsFlags(e), " ") + " (once joined)"}}
	}
	return []Action{{"run", "tailscale up --auth-key=file:… " + strings.Join(tsFlags(e), " ") + " (the key typed)"}}
}

func (t tailscaleUpStep) Check(ctx context.Context, e *Env) (bool, error) {
	if _, ok := t.joined(ctx, e); ok {
		return authKey(e) == "" && t.prefsOK(ctx, e), nil
	}
	return authKey(e) == "", nil
}

func (t tailscaleUpStep) Apply(ctx context.Context, e *Env) error {
	if st, ok := t.joined(ctx, e); ok {
		if authKey(e) != "" {
			e.note(fmt.Sprintf("Tailscale: already on the tailnet as %s, so the auth key typed wasn't used (to join again: sudo tailscale logout, then sudo yardo).",
				strings.TrimSuffix(st.Self.DNSName, ".")))
		}
		if t.prefsOK(ctx, e) {
			return nil
		}
		_, err := e.run(ctx, Cmd{Name: "tailscale", Args: append([]string{"set"}, tsFlags(e)...)})
		return err
	}
	if authKey(e) == "" {
		return nil
	}
	// From a file: on the command line, ps would show the key.
	f, err := os.CreateTemp("", "tailscale-key.")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, werr := f.WriteString(authKey(e))
	if err := errors.Join(werr, f.Close()); err != nil {
		return err
	}
	if _, err := e.run(ctx, Cmd{Name: "tailscale", Args: append([]string{"up", "--auth-key=file:" + f.Name()}, tsFlags(e)...)}); err != nil {
		return err
	}
	return e.record("tailscale", "")
}

// --- go:netbird_up -----------------------------------------------------------

// netbirdUpStep joins NetBird with the setup key typed this run, on NetBird's
// cloud or the self-hosted management URL typed (none: the user runs `sudo
// netbird up` later).
type netbirdUpStep struct{}

func setupKey(e *Env) (key, url string) {
	if e.Secrets == nil {
		return "", ""
	}
	return e.Secrets.NBSetupKey, e.Secrets.NBManagementURL
}

func (netbirdUpStep) Plan(e *Env) []Action {
	if k, _ := setupKey(e); k == "" {
		return nil
	}
	return []Action{{"run", "netbird up --setup-key-file … (the key typed)"}}
}

func (netbirdUpStep) Check(_ context.Context, e *Env) (bool, error) {
	k, _ := setupKey(e)
	return k == "", nil
}

func (netbirdUpStep) Apply(ctx context.Context, e *Env) error {
	key, url := setupKey(e)
	if key == "" {
		return nil
	}
	// From a file: on the command line, ps would show the key.
	f, err := os.CreateTemp("", "netbird-key.")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, werr := f.WriteString(key)
	if err := errors.Join(werr, f.Close()); err != nil {
		return err
	}
	args := []string{"up", "--setup-key-file", f.Name()}
	if url != "" {
		args = append(args, "--management-url", url)
	}
	_, err = e.run(ctx, Cmd{Name: "netbird", Args: args})
	return err
}

// --- go:sshharden ------------------------------------------------------------

// sshHardenStep turns off password and root login, skipping (with a note)
// while the user has no SSH key (they'd be locked out), and rolling back a
// config sshd rejects.
type sshHardenStep struct{}

const sshdDropin = "/etc/ssh/sshd_config.d/10-yardo.conf"

var keyLine = regexp.MustCompile(`(?m)^(ssh-|ecdsa-|sk-)`)

func (sshHardenStep) Plan(*Env) []Action {
	return []Action{{"file", sshdDropin}, {"unit", "try-reload-or-restart ssh"}}
}

func (sshHardenStep) want(e *Env) ([]byte, error) { return fs.ReadFile(e.Files, "sshd-yardo.conf") }

func (s sshHardenStep) Check(_ context.Context, e *Env) (bool, error) {
	want, err := s.want(e)
	if err != nil {
		return false, err
	}
	return same(e.Path(sshdDropin), wantFile{b: want, mode: 0o644}), nil
}

func (s sshHardenStep) Apply(ctx context.Context, e *Env) error {
	keys := e.Path("~/.ssh/authorized_keys")
	b, _ := os.ReadFile(keys)
	if !keyLine.Match(b) {
		e.note(fmt.Sprintf("SSH still takes passwords: %s has no SSH key, and keys-only login would lock %s out. Add one, then run 'sudo yardo --yes'.", keys, e.User))
		return nil
	}
	want, err := s.want(e)
	if err != nil {
		return err
	}
	p := e.Path(sshdDropin)
	if err := writeFile(p, want, 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(e.Path("/run/sshd"), 0o755); err != nil {
		return err
	}
	if _, err := e.run(ctx, Cmd{Name: "sshd", Args: []string{"-t"}}); err != nil {
		_ = os.Remove(p)
		return fmt.Errorf("sshd rejected the config; nothing changed: %w", err)
	}
	_, err = e.run(ctx, Cmd{Name: "systemctl", Args: []string{"try-reload-or-restart", "ssh"}})
	return err
}

// --- go:fail2ban -------------------------------------------------------------

// fail2banStep bans SSH brute force on a machine with a public IPv4 (one
// behind NAT or on a LAN isn't brute-forced from the internet): the
// package, yardo's sshd jail (Tailscale's addresses are never banned) and
// the service. Without one it does nothing.
type fail2banStep struct{}

var fail2banPkgs = []string{"fail2ban", "python3-systemd"}

// interfaceAddrs is net.InterfaceAddrs (tests replace it).
var interfaceAddrs = net.InterfaceAddrs

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// publicIPv4 is the first public IPv4 address on the machine's
// interfaces, "" when none: not private, not Tailscale's or carrier-grade
// NAT's 100.64.0.0/10, not loopback or link-local. A cloud VM behind 1:1
// NAT (an elastic IP) only sees its private address, so it has none.
func publicIPv4() string {
	addrs, err := interfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		p, err := netip.ParsePrefix(a.String())
		if err != nil {
			continue
		}
		if ip := p.Addr(); ip.Is4() && ip.IsGlobalUnicast() && !ip.IsPrivate() && !cgnat.Contains(ip) {
			return ip.String()
		}
	}
	return ""
}

func (fail2banStep) inner() []Step {
	return mustSteps("fail2ban",
		catalog.Step{Type: "file", Src: "fail2ban-yardo.local", To: "/etc/fail2ban/jail.d/yardo.local"},
		catalog.Step{Type: "systemctl", Unit: "fail2ban"},
		catalog.Step{Type: "systemctl", Unit: "fail2ban", Action: "restart"})
}

func (f fail2banStep) Plan(e *Env) []Action {
	ip := publicIPv4()
	if ip == "" {
		return []Action{{"skip", "fail2ban: no public IPv4 on this machine"}}
	}
	out := []Action{{"apt", strings.Join(fail2banPkgs, " ") + " (public IPv4 " + ip + ")"}}
	for _, st := range f.inner() {
		out = append(out, st.Plan(e)...)
	}
	return out
}

func (f fail2banStep) Check(ctx context.Context, e *Env) (bool, error) {
	if publicIPv4() == "" {
		return true, nil
	}
	if missing, err := MissingPackages(ctx, e, fail2banPkgs); err != nil || len(missing) > 0 {
		return false, err
	}
	return checkAll(ctx, e, f.inner())
}

func (f fail2banStep) Apply(ctx context.Context, e *Env) error {
	if publicIPv4() == "" {
		return nil
	}
	missing, err := MissingPackages(ctx, e, fail2banPkgs)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		if err := AptInstall(ctx, e, missing); err != nil {
			return err
		}
	}
	return applyAll(ctx, e, f.inner(), len(missing) > 0)
}
