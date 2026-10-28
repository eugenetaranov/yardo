package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// The Docker API over TLS, for a laptop on the local network or a VPN:
// port 2376, client certificates only (yardo-docker makes them, from a CA
// of its own), and a firewall table that lets the port in from where the
// setting says only. Docker gets it from a systemd drop-in, with
// live-restore (the flag, unless daemon.json sets it: dockerd refuses a
// setting given both ways), so restarting it leaves the containers running.
const (
	dockerAPIHelper = "/usr/local/sbin/yardo-docker"
	dockerAPIDropin = "/etc/systemd/system/docker.service.d/yardo-api.conf"
	dockerAPIPort   = 2376
)

const dockerDaemonJSON = "/etc/docker/daemon.json"

// dockerAPIDropinText is the drop-in for e: dockerd's flags, less those
// daemon.json already sets (dockerd won't start with a setting given both
// as a flag and in the file).
func dockerAPIDropinText(e *Env) (string, error) {
	conf := map[string]any{}
	if b, err := os.ReadFile(e.Path(dockerDaemonJSON)); err == nil {
		if err := json.Unmarshal(b, &conf); err != nil {
			return "", fmt.Errorf("%s: %w", dockerDaemonJSON, err)
		}
	}
	for _, k := range []string{"hosts", "tls", "tlsverify", "tlscacert", "tlscert", "tlskey"} {
		if _, ok := conf[k]; ok {
			return "", fmt.Errorf("%s sets %q, which the Docker API over TLS sets itself: take it out of that file, or turn the Docker API off", dockerDaemonJSON, k)
		}
	}
	live := " --live-restore"
	if _, ok := conf["live-restore"]; ok {
		live = ""
	}
	return `# yardo: the Docker API over TLS on port 2376 (client certificates only),
# besides the socket. live-restore keeps containers running when Docker
# restarts. yardo rewrites this file.
[Service]
ExecStart=
ExecStart=/usr/bin/dockerd -H fd:// --containerd=/run/containerd/containerd.sock` + live + ` -H tcp://0.0.0.0:2376 --tlsverify --tlscacert=/etc/docker/tls/ca.pem --tlscert=/etc/docker/tls/server/cert.pem --tlskey=/etc/docker/tls/server/key.pem
`, nil
}

// LANNets are the local networks "lan" lets in: the private ranges, and
// Tailscale's.
var LANNets = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10"}

// DockerAPI is who can reach the Docker API over TLS: "" (nobody), "vpn"
// or "lan".
func DockerAPI(e *Env) string {
	if e.Answers == nil {
		return ""
	}
	switch e.Answers.DockerAPI {
	case "vpn", "lan":
		return e.Answers.DockerAPI
	}
	return ""
}

// dockerAPITable lets port 2376 in from this machine and the VPNs (and, for
// "lan", the local networks) only.
func dockerAPITable(e *Env) fwTable {
	t := fwTable{name: "yardo_docker_api"}
	mode := DockerAPI(e)
	if mode == "" {
		return t
	}
	lan := ""
	if mode == "lan" {
		lan = fmt.Sprintf("\t\ttcp dport %d ip saddr { %s } accept\n", dockerAPIPort, strings.Join(LANNets, ", "))
	}
	t.chains = fmt.Sprintf(`	chain input {
		type filter hook input priority filter - 1; policy accept;
		tcp dport %[1]d iifname "lo" accept
		tcp dport %[1]d iifname %[2]s accept
%[3]s		tcp dport %[1]d drop
	}
`, dockerAPIPort, nftSet(VPNInterfaces), lan)
	return t
}

type dockerAPIStep struct{}

func (dockerAPIStep) Plan(e *Env) []Action {
	if DockerAPI(e) == "" {
		return nil
	}
	who := "this machine and the VPNs"
	if DockerAPI(e) == "lan" {
		who = "the local network and the VPNs"
	}
	return []Action{{"run", fmt.Sprintf("Docker API on port %d over TLS, client certificates (yardo-docker), from %s", dockerAPIPort, who)}}
}

func (s dockerAPIStep) helper(e *Env) (wantFile, error) {
	if e.Files == nil {
		return wantFile{}, errors.New("yardo-docker: no catalog files")
	}
	b, err := fs.ReadFile(e.Files, "yardo-docker")
	return wantFile{dockerAPIHelper, b, 0o755}, err
}

func (s dockerAPIStep) Check(ctx context.Context, e *Env) (bool, error) {
	if DockerAPI(e) == "" {
		_, err := os.Stat(e.Path(dockerAPIDropin))
		return os.IsNotExist(err) && dockerAPITable(e).Check(ctx, e), nil
	}
	h, err := s.helper(e)
	if err != nil {
		return false, err
	}
	text, err := dockerAPIDropinText(e)
	if err != nil {
		return false, nil
	}
	if !same(e.Path(h.to), h) || !same(e.Path(dockerAPIDropin), wantFile{b: []byte(text), mode: 0o644}) || !dockerAPITable(e).Check(ctx, e) {
		return false, nil
	}
	if _, err := e.Exec.Run(ctx, Cmd{Name: dockerAPIHelper, Args: []string{"check"}}); err != nil {
		return false, nil
	}
	return systemctlStep{catalog.Step{Unit: "docker"}}.Check(ctx, e)
}

func (s dockerAPIStep) Apply(ctx context.Context, e *Env) error {
	restart := false
	if DockerAPI(e) == "" {
		// Off: the socket (and SSH) only again. The certificates stay, so
		// turning it back on keeps the laptops' working.
		if err := os.Remove(e.Path(dockerAPIDropin)); err == nil {
			restart = true
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := dockerAPITable(e).Apply(ctx, e); err != nil {
			return err
		}
	} else {
		h, err := s.helper(e)
		if err != nil {
			return err
		}
		text, err := dockerAPIDropinText(e)
		if err != nil {
			return err
		}
		if !same(e.Path(h.to), h) {
			if err := writeFile(e.Path(h.to), h.b, h.mode); err != nil {
				return err
			}
		}
		// The firewall first: the port is never open to more than it should be.
		if err := dockerAPITable(e).Apply(ctx, e); err != nil {
			return err
		}
		if _, err := e.Exec.Run(ctx, Cmd{Name: dockerAPIHelper, Args: []string{"check"}}); err != nil {
			if _, err := e.run(ctx, Cmd{Name: dockerAPIHelper, Args: []string{"setup"}}); err != nil {
				return err
			}
			restart = true // a new server certificate
		}
		if p := e.Path(dockerAPIDropin); !same(p, wantFile{b: []byte(text), mode: 0o644}) {
			if err := os.MkdirAll(e.Path("/etc/systemd/system/docker.service.d"), 0o755); err != nil {
				return err
			}
			if err := writeFile(p, []byte(text), 0o644); err != nil {
				return err
			}
			restart = true
		}
	}
	if !restart {
		return nil
	}
	if err := restartDocker(ctx, e); err != nil {
		if DockerAPI(e) == "" {
			return err
		}
		// Docker won't start with the API: back to the socket only, so
		// it (and the containers) keep running.
		if rerr := os.Remove(e.Path(dockerAPIDropin)); rerr != nil && !os.IsNotExist(rerr) {
			return errors.Join(err, rerr)
		}
		if rerr := restartDocker(ctx, e); rerr != nil {
			return fmt.Errorf("docker didn't start with the API over TLS, nor without it (journalctl -xeu docker): %w", rerr)
		}
		return fmt.Errorf("docker didn't start with the API over TLS, so it's off again and Docker runs as before (journalctl -xeu docker says why): %w", err)
	}
	if DockerAPI(e) != "" {
		e.note("The Docker API is on port 2376 (TLS, client certificates). A laptop's certificate: sudo yardo-docker client <name> (it says how to use it); sudo yardo connect shows it all.")
	}
	return nil
}

// restartDocker reloads systemd and restarts Docker, clearing a failed
// start's rate limit first.
func restartDocker(ctx context.Context, e *Env) error {
	for _, args := range [][]string{{"daemon-reload"}, {"reset-failed", "docker"}, {"restart", "docker"}} {
		if _, err := e.run(ctx, Cmd{Name: "systemctl", Args: args}); err != nil && args[0] != "reset-failed" {
			return err
		}
	}
	return nil
}
