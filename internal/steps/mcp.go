package steps

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/mcp"
)

// MCPJungle runs per user, as mcpjungle@<user> on the user's port (from
// system.json). Its server catalog and mcp-sync are shared (the same
// places as tack-roles' ai-tools role).
const (
	mcpCatalog = "/usr/local/share/mcp-catalog"
	mcpSync    = "/usr/local/bin/mcp-sync"
	mcpUnit    = "/etc/systemd/system/mcpjungle@.service"
	mcpEnvDir  = "/etc/mcpjungle"
	// oldMCPUnit is Phase 1's single gateway, replaced by its user's instance.
	oldMCPUnit = "/etc/systemd/system/mcpjungle.service"
)

func init() {
	RegisterGo("mcp_files", func(catalog.Step) Step { return mcpFilesStep{} })
	RegisterGo("mcpjungle_unit", func(catalog.Step) Step { return mcpUnitStep{} })
	RegisterGo("mcp_wait", func(catalog.Step) Step { return mcpWaitStep{} })
	RegisterGo("mcp_auth", func(catalog.Step) Step { return mcpAuthStep{} })
	RegisterGo("mcp_firewall", func(catalog.Step) Step { return mcpFirewallStep{} })
	RegisterGo("docker_ports", func(catalog.Step) Step { return dockerPortsStep{} })
	RegisterGo("mcp_servers", func(catalog.Step) Step { return mcpServersStep{} })
	RegisterGo("mcp_wire", func(catalog.Step) Step { return mcpWireStep{} })
	RegisterGo("mcp_sync", func(catalog.Step) Step { return mcpSyncStep{} })
	RegisterGo("mcp_user", func(catalog.Step) Step { return mcpUserStep{} })
}

// MCPUser is the system user the user's gateway runs as. It owns
// ~/.mcp, so the login user (and agents running as them) can't read the
// MCP tokens without sudo.
func MCPUser(e *Env) string { return "mcp-" + e.User }

// mcpAsService says whether ~/.mcp is reached as the service user (as
// root, on a VM) or directly (tests, run as a plain user).
var mcpAsService = func(e *Env) bool { return e.Chown }

// mcpRead reads ~/.mcp/rel as the service user; nil and no error when it
// doesn't exist. As that user, a symlink planted by the login user leads
// nowhere it couldn't already read.
func mcpRead(ctx context.Context, e *Env, rel string) ([]byte, error) {
	p := filepath.Join(e.Home, ".mcp", rel)
	if !mcpAsService(e) {
		b, err := os.ReadFile(e.Path(p))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return b, err
	}
	// "x" marks a file that is there (an empty one included).
	out, err := e.Exec.Run(ctx, Cmd{Name: "sh", Args: []string{"-c", `[ -e "$1" ] || exit 0; printf x; cat -- "$1"`, "_", p}, As: MCPUser(e)})
	if err != nil {
		return nil, fmt.Errorf("read %s as %s: %w", p, MCPUser(e), err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out[1:], nil
}

// mcpWrite writes ~/.mcp/rel (0600, its directory 0700) as the service user.
func mcpWrite(ctx context.Context, e *Env, rel string, b []byte) error {
	p := filepath.Join(e.Home, ".mcp", rel)
	if !mcpAsService(e) {
		dir := filepath.Dir(e.Path(p))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := errors.Join(os.Chmod(dir, 0o700), e.chown(dir, e.UID, e.GID)); err != nil {
			return err
		}
		if err := writeFile(e.Path(p), b, 0o600); err != nil {
			return err
		}
		return e.chown(e.Path(p), e.UID, e.GID)
	}
	_, err := e.run(ctx, Cmd{Name: "sh", Args: []string{"-c",
		`umask 077 && mkdir -p -- "$(dirname -- "$1")" && cat > "$1.tmp" && chmod 600 "$1.tmp" && mv -f -- "$1.tmp" "$1"`, "_", p},
		Stdin: b, As: MCPUser(e)})
	return err
}

// --- go:mcp_user: the gateway's service user, owning ~/.mcp -----------------

type mcpUserStep struct{}

func (mcpUserStep) Plan(e *Env) []Action {
	return []Action{{"run", fmt.Sprintf("system user %s (docker group) owns ~/.mcp; it may pass through %s (ACL)", MCPUser(e), e.Home)}}
}

func (mcpUserStep) Check(ctx context.Context, e *Env) (bool, error) {
	svc := MCPUser(e)
	groups, err := e.Exec.Run(ctx, Cmd{Name: "id", Args: []string{"-nG", svc}})
	if err != nil || !slices.Contains(strings.Fields(string(groups)), "docker") {
		return false, nil
	}
	owner, err := e.Exec.Run(ctx, Cmd{Name: "stat", Args: []string{"-c", "%U %a", filepath.Join(e.Home, ".mcp")}})
	if err != nil || strings.TrimSpace(string(owner)) != svc+" 700" {
		return false, nil
	}
	acl, err := e.Exec.Run(ctx, Cmd{Name: "getfacl", Args: []string{"-cp", e.Home}})
	return err == nil && hasLine(acl, "user:"+svc+":--x"), nil
}

func (mcpUserStep) Apply(ctx context.Context, e *Env) error {
	svc := MCPUser(e)
	if len(svc) > 32 {
		return fmt.Errorf("%s: user names are at most 32 characters; the gateway needs a shorter login name", svc)
	}
	dir := filepath.Join(e.Home, ".mcp")
	if _, err := e.Exec.Run(ctx, Cmd{Name: "id", Args: []string{"-u", svc}}); err != nil {
		if _, err := e.run(ctx, Cmd{Name: "useradd", Args: []string{"--system", "--user-group", "--no-create-home",
			"--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", "--comment", "MCPJungle gateway for " + e.User, svc}}); err != nil {
			return err
		}
	}
	for _, args := range [][]string{
		{"usermod", "-aG", "docker", svc},
		// The login user's own ~/.mcp from before: the service user's now
		// (chown -R doesn't follow symlinks).
		{"install", "-d", "-m", "0700", "-o", svc, "-g", svc, dir},
		{"chown", "-R", svc + ":" + svc, dir},
		{"setfacl", "-m", "u:" + svc + ":x", e.Home},
	} {
		if _, err := e.run(ctx, Cmd{Name: args[0], Args: args[1:]}); err != nil {
			return err
		}
	}
	return nil
}

// MCPPort is the user's gateway port.
func MCPPort(e *Env) int {
	if e.Answers != nil && e.Answers.MCPPort > 0 {
		return e.Answers.MCPPort
	}
	return answers.FirstMCPPort
}

// mcpURL is where this machine reaches the gateway: loopback, which every
// access setting lets in.
func mcpURL(e *Env) string {
	return fmt.Sprintf("http://127.0.0.1:%d", MCPPort(e))
}

// mcpInstance is the user's gateway unit.
func mcpInstance(e *Env) string { return "mcpjungle@" + e.User }

// --- go:mcp_files: the server catalog and mcp-sync -------------------------

type mcpFilesStep struct{}

type wantFile struct {
	to   string
	b    []byte
	mode fs.FileMode
}

// files are the specs (yardo's own win over tack-roles' of the same name)
// and mcp-sync.
func (mcpFilesStep) files(e *Env) ([]wantFile, error) {
	specs := map[string][]byte{}
	for _, dir := range []string{filepath.Join(e.Share, "mcp-catalog"), filepath.Join(e.Share, "conf", "mcp-catalog")} {
		paths, _ := filepath.Glob(filepath.Join(dir, "*.spec"))
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			specs[filepath.Base(p)] = b
		}
	}
	// A team's specs (listed in .team-files by whoever shipped them) win
	// over ours of the same name.
	team, _ := os.ReadFile(e.Path(filepath.Join(mcpCatalog, ".team-files")))
	var out []wantFile
	for name, b := range specs {
		if !hasLine(team, name) {
			out = append(out, wantFile{filepath.Join(mcpCatalog, name), b, 0o644})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].to < out[j].to })
	b, err := os.ReadFile(filepath.Join(e.Share, "mcp-sync"))
	if err != nil {
		return nil, err
	}
	return append(out, wantFile{mcpSync, b, 0o755}), nil
}

func (mcpFilesStep) Plan(*Env) []Action {
	return []Action{{"file", mcpCatalog + "/*.spec"}, {"file", mcpSync}}
}

func same(p string, w wantFile) bool {
	cur, err := os.ReadFile(p)
	fi, serr := os.Stat(p)
	return err == nil && serr == nil && bytes.Equal(cur, w.b) && fi.Mode().Perm() == w.mode
}

func (m mcpFilesStep) Check(_ context.Context, e *Env) (bool, error) {
	files, err := m.files(e)
	if err != nil {
		return false, err
	}
	for _, f := range files {
		if !same(e.Path(f.to), f) {
			return false, nil
		}
	}
	return true, nil
}

func (m mcpFilesStep) Apply(_ context.Context, e *Env) error {
	files, err := m.files(e)
	if err != nil {
		return err
	}
	for _, f := range files {
		if p := e.Path(f.to); !same(p, f) {
			if err := writeFile(p, f.b, f.mode); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- go:mcpjungle_unit -------------------------------------------------------

type mcpUnitStep struct{}

func (mcpUnitStep) Plan(e *Env) []Action {
	return []Action{{"unit", fmt.Sprintf("enable-now %s on port %d (%s)", mcpInstance(e), MCPPort(e), mcpUnit)}}
}

// listen is the gateway's access: "127.0.0.1" (this machine only), "vpn"
// (this machine and the VPN interfaces: yardo's firewall keeps the rest
// out) or "0.0.0.0" (everywhere). "tailscale", from before, is "vpn".
func listen(e *Env) string {
	if e.Answers == nil {
		return "127.0.0.1"
	}
	switch e.Answers.MCPListen {
	case "vpn", "tailscale":
		return "vpn"
	case "0.0.0.0":
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

// bindHost is the address the gateway binds: loopback, or every interface
// (for "vpn", the firewall decides who gets in: an interface that comes up
// later, or a VPN address that changes, needs nothing redone).
func bindHost(e *Env) string {
	if listen(e) == "127.0.0.1" {
		return "127.0.0.1"
	}
	return "0.0.0.0"
}

// Enterprise reports whether the gateway requires tokens: whenever it
// listens beyond this machine. In development mode (this machine only)
// anyone who reaches the port may use every server and register new ones.
func Enterprise(e *Env) bool { return listen(e) != "127.0.0.1" }

// mcpMode is the gateway's mode, as MCPJungle's SERVER_MODE names it.
func mcpMode(e *Env) string {
	if Enterprise(e) {
		return "enterprise"
	}
	return "development"
}

// mcpDB is the gateway's database: one per mode, since MCPJungle won't
// start a database in the other mode (mcp-sync registers the servers again
// in a new one).
func mcpDB(e *Env) string {
	if Enterprise(e) {
		return filepath.Join(e.Home, ".mcp", "mcpjungle-enterprise.db")
	}
	return filepath.Join(e.Home, ".mcp", "mcpjungle.db")
}

// mcpTemplate is the gateway's template unit: one instance per user, each
// with its host, port, mode and database in /etc/mcpjungle/<user>.env.
const mcpTemplate = `[Unit]
Description=MCPJungle MCP gateway for %i
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
# The service user owns ~<user>/.mcp: the login user can't read the tokens.
User=mcp-%i
SupplementaryGroups=docker
EnvironmentFile=/etc/mcpjungle/%i.env
# Docker-based servers can take a while to start the first time.
Environment=MCP_SERVER_INIT_REQ_TIMEOUT_SEC=120
# Enterprise mode turns telemetry on unless told not to.
Environment=OTEL_ENABLED=false
# MCP_HOST is 127.0.0.1 (this machine; SERVER_MODE development) or 0.0.0.0
# (VPN only, kept so by yardo's firewall, or everywhere; enterprise: every
# client needs a token).
ExecStart=/usr/local/bin/mcpjungle start --host=${MCP_HOST} --port ${MCP_PORT} --sqlite-db-path ${MCP_DB}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`

func (mcpUnitStep) envFile(e *Env) wantFile {
	host := bindHost(e)
	return wantFile{filepath.Join(mcpEnvDir, e.User+".env"),
		[]byte(fmt.Sprintf("MCP_HOST=%s\nMCP_PORT=%d\nMCP_DIR=%s\nMCP_DB=%s\nSERVER_MODE=%s\n",
			host, MCPPort(e), filepath.Join(e.Home, ".mcp"), mcpDB(e), mcpMode(e))), 0o644}
}

// oldUnit reports whether Phase 1's single gateway runs as this user.
func oldUnit(e *Env) bool {
	b, err := os.ReadFile(e.Path(oldMCPUnit))
	return err == nil && hasLine(b, "User="+e.User)
}

func (u mcpUnitStep) Check(ctx context.Context, e *Env) (bool, error) {
	env := u.envFile(e)
	if !same(e.Path(mcpUnit), wantFile{b: []byte(mcpTemplate), mode: 0o644}) || !same(e.Path(env.to), env) || oldUnit(e) {
		return false, nil
	}
	return systemctlStep{catalog.Step{Unit: mcpInstance(e)}}.Check(ctx, e)
}

// Down: the user's gateway is enabled but not running.
func (mcpUnitStep) Down(ctx context.Context, e *Env) (bool, string) {
	return unitDown(ctx, e, mcpInstance(e)), mcpInstance(e)
}

func (u mcpUnitStep) Apply(ctx context.Context, e *Env) error {
	var cmds [][]string
	if oldUnit(e) {
		// Phase 1's gateway: same user, same database; the instance replaces it.
		if _, err := e.run(ctx, Cmd{Name: "systemctl", Args: []string{"disable", "--now", "mcpjungle"}}); err != nil {
			return err
		}
		if err := os.Remove(e.Path(oldMCPUnit)); err != nil {
			return err
		}
		cmds = append(cmds, []string{"daemon-reload"})
	}
	changed := false
	for _, w := range []wantFile{{mcpUnit, []byte(mcpTemplate), 0o644}, u.envFile(e)} {
		if p := e.Path(w.to); !same(p, w) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := writeFile(p, w.b, w.mode); err != nil {
				return err
			}
			changed = true
		}
	}
	inst := mcpInstance(e)
	if changed {
		if len(cmds) == 0 {
			cmds = append(cmds, []string{"daemon-reload"})
		}
		cmds = append(cmds, []string{"enable", inst}, []string{"restart", inst})
	} else {
		cmds = append(cmds, []string{"enable", "--now", inst})
	}
	for _, args := range cmds {
		if _, err := e.run(ctx, Cmd{Name: "systemctl", Args: args}); err != nil {
			return err
		}
	}
	return nil
}

// --- go:mcp_wait: the gateway answers ----------------------------------------

type mcpWaitStep struct{}

// mcpUp reports whether the gateway at url answers (tests replace it).
var mcpUp = func(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// mcpWaitTries is how many seconds to wait for the gateway.
var mcpWaitTries = 60

func (mcpWaitStep) Plan(e *Env) []Action { return []Action{{"wait", mcpURL(e)}} }

func (mcpWaitStep) Check(ctx context.Context, e *Env) (bool, error) {
	return mcpUp(ctx, mcpURL(e)), nil
}

func (mcpWaitStep) Apply(ctx context.Context, e *Env) error {
	for i := 0; i < mcpWaitTries; i++ {
		if mcpUp(ctx, mcpURL(e)) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	e.note(fmt.Sprintf("mcpjungle is not answering on port %d after %ds; see: journalctl -u %s", MCPPort(e), mcpWaitTries, mcpInstance(e)))
	return nil
}

// randomToken is a client token: 32 random bytes, hex.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- go:mcp_auth: the tokens, in enterprise mode -------------------------------

// mcpAuthStep initializes an enterprise-mode gateway (its admin token, in
// ~/.mcp/.mcpjungle.conf, for mcp-sync) and creates the client the VM's own
// agents use (allowed every server; its token in ~/.mcp/agent-token). Both
// stay with the service user; the token reaches the agents' configs.
type mcpAuthStep struct{}

const (
	mcpAdminConf  = ".mcpjungle.conf"
	mcpAgentToken = "agent-token"
	mcpAgentName  = "yardo-agents"
)

func (mcpAuthStep) Plan(e *Env) []Action {
	if !Enterprise(e) {
		return nil
	}
	return []Action{{"run", "mcpjungle init-server; create mcp-client " + mcpAgentName + " (token in ~/.mcp/" + mcpAgentToken + ")"}}
}

// mj runs the mcpjungle CLI against the user's gateway, as the service
// user, with ~/.mcp as its home (where the admin token is).
func mj(ctx context.Context, e *Env, args ...string) ([]byte, error) {
	a := append([]string{"HOME=" + filepath.Join(e.Home, ".mcp"), "mcpjungle", "--registry", mcpURL(e)}, args...)
	c := Cmd{Name: "env", Args: a, As: MCPUser(e)}
	if !mcpAsService(e) {
		c = Cmd{Name: "env", Args: a, AsUser: true}
	}
	return e.Exec.Run(ctx, c)
}

func (mcpAuthStep) Check(ctx context.Context, e *Env) (bool, error) {
	if !Enterprise(e) {
		return true, nil
	}
	conf, err := mcpRead(ctx, e, mcpAdminConf)
	if err != nil || !bytes.Contains(conf, []byte("access_token:")) {
		return false, nil
	}
	tok, err := mcpRead(ctx, e, mcpAgentToken)
	if err != nil || len(bytes.TrimSpace(tok)) == 0 {
		return false, nil
	}
	out, err := mj(ctx, e, "list", "mcp-clients")
	return err == nil && bytes.Contains(out, []byte(mcpAgentName)), nil
}

func (mcpAuthStep) Apply(ctx context.Context, e *Env) error {
	if !Enterprise(e) {
		return nil
	}
	conf, err := mcpRead(ctx, e, mcpAdminConf)
	if err != nil {
		return err
	}
	if !bytes.Contains(conf, []byte("access_token:")) {
		if out, err := mj(ctx, e, "init-server"); err != nil {
			return fmt.Errorf("mcpjungle init-server: %w: %s", err, out)
		}
	}
	tok, err := mcpRead(ctx, e, mcpAgentToken)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(tok)) == 0 {
		t, err := randomToken()
		if err != nil {
			return err
		}
		if err := mcpWrite(ctx, e, mcpAgentToken, []byte(t+"\n")); err != nil {
			return err
		}
	}
	if out, err := mj(ctx, e, "list", "mcp-clients"); err == nil && bytes.Contains(out, []byte(mcpAgentName)) {
		return nil
	}
	// From a file the CLI reads: on the command line, ps would show it.
	client := fmt.Sprintf(`{"name": %q, "description": "the coding agents on this machine (yardo)", "allowed_servers": ["*"], "access_token_ref": {"file": %q}}`+"\n",
		mcpAgentName, filepath.Join(e.Home, ".mcp", mcpAgentToken))
	if err := mcpWrite(ctx, e, "agent-client.json", []byte(client)); err != nil {
		return err
	}
	if out, err := mj(ctx, e, "create", "mcp-client", "--conf", filepath.Join(e.Home, ".mcp", "agent-client.json")); err != nil {
		return fmt.Errorf("mcpjungle create mcp-client: %w: %s", err, out)
	}
	return nil
}

// agentHeaders are the headers the agents send the gateway: its client
// token in enterprise mode, none in development mode.
func agentHeaders(ctx context.Context, e *Env) (map[string]string, error) {
	if !Enterprise(e) {
		return nil, nil
	}
	tok, err := mcpRead(ctx, e, mcpAgentToken)
	if err != nil {
		return nil, err
	}
	t := strings.TrimSpace(string(tok))
	if t == "" {
		return nil, errors.New("the gateway's agent token isn't there yet (~/.mcp/" + mcpAgentToken + ")")
	}
	return map[string]string{"Authorization": "Bearer " + t}, nil
}

// --- go:mcp_servers: each picked server's env file ---------------------------

type mcpServersStep struct{}

func servers(e *Env) []string {
	if e.Answers == nil {
		return nil
	}
	return e.Answers.MCPServers
}

func (mcpServersStep) Plan(e *Env) []Action {
	var out []Action
	for _, n := range servers(e) {
		out = append(out, Action{"file", "~/.mcp/" + n + "/env (0600)"})
	}
	return out
}

// typed are the keys typed for server n this run.
func typed(e *Env, n string) map[string]string {
	out := map[string]string{}
	if e.Secrets == nil {
		return out
	}
	for k, v := range e.Secrets.MCP {
		if srv, key, ok := strings.Cut(k, " "); ok && srv == n {
			out[key] = v
		}
	}
	return out
}

func (mcpServersStep) Check(ctx context.Context, e *Env) (bool, error) {
	for _, n := range servers(e) {
		b, err := mcpRead(ctx, e, n+"/env")
		if err != nil || b == nil {
			return false, nil
		}
		if !mcpAsService(e) {
			if fi, _ := os.Stat(e.Path(filepath.Join(e.Home, ".mcp", n, "env"))); fi.Mode().Perm() != 0o600 {
				return false, nil
			}
		}
		for k, v := range typed(e, n) {
			if EnvGet(b, k) != v {
				return false, nil
			}
		}
	}
	return true, nil
}

// Missing: the required keys of the picked servers left empty (the server
// isn't registered until they're filled in).
func (mcpServersStep) Missing(ctx context.Context, e *Env) []string {
	var out []string
	for _, n := range servers(e) {
		s, err := mcp.Load(ctx, mcp.Dirs(e.Share), n)
		if err != nil {
			continue
		}
		b, _ := mcpRead(ctx, e, n+"/env")
		for _, k := range s.Required {
			if EnvGet(b, k) == "" {
				out = append(out, "~/.mcp/"+n+"/env: "+k)
			}
		}
	}
	return out
}

func (mcpServersStep) Apply(ctx context.Context, e *Env) error {
	for _, n := range servers(e) {
		b, err := mcpRead(ctx, e, n+"/env")
		if err != nil {
			return err
		}
		if b == nil {
			spec, err := mcp.Load(ctx, mcp.Dirs(e.Share), n)
			if err != nil {
				return err
			}
			b = []byte(spec.Skeleton)
			if err := e.record("file", filepath.Join(e.Home, ".mcp", n, "env")); err != nil {
				return err
			}
		}
		keys := typed(e, n)
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			b = EnvSet(b, k, keys[k])
		}
		if err := mcpWrite(ctx, e, n+"/env", b); err != nil {
			return err
		}
	}
	return nil
}

// EnvGet returns the value of the last KEY= line in an env file.
func EnvGet(b []byte, key string) string {
	v := ""
	for _, l := range strings.Split(string(b), "\n") {
		if x, ok := strings.CutPrefix(l, key+"="); ok {
			v = x
		}
	}
	return v
}

// EnvSet sets KEY in an env file: in place of its first KEY= line (or of
// a commented "#KEY=" one), else appended. As the bash installer's envset.
func EnvSet(b []byte, key, value string) []byte {
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(b) == 0 {
		lines = nil
	}
	set := key + "=" + value
	done := false
	for i, l := range lines {
		commented := strings.HasPrefix(l, "#") && strings.HasPrefix(strings.TrimLeft(l[1:], " "), key+"=")
		if strings.HasPrefix(l, key+"=") || commented {
			lines[i], done = set, true
			break
		}
	}
	if !done {
		lines = append(lines, set)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// --- go:mcp_wire: the gateway in each installed agent ------------------------

type mcpWireStep struct{}

func wire(e *Env) bool {
	return e.Answers != nil && e.Answers.WireAgents != nil && *e.Answers.WireAgents
}

func agentURL(e *Env) string {
	return fmt.Sprintf("http://localhost:%d/mcp", MCPPort(e))
}

func (mcpWireStep) Plan(e *Env) []Action {
	if !wire(e) {
		return nil
	}
	return []Action{{"run", "add the mcpjungle MCP server to Claude Code, Codex, Gemini CLI, OpenCode and Goose (those installed)"}}
}

// has reports whether the user can run tool (mise tools are only on
// their PATH).
func has(ctx context.Context, e *Env, tool string) bool {
	_, err := e.Exec.Run(ctx, Cmd{Name: "sh", Args: []string{"-c", "command -v " + tool}, AsUser: true})
	return err == nil
}

const codexSection = "[mcp_servers.mcpjungle]"

// todo lists the agents not wired to the user's gateway yet: not at all
// ("claude"), or to another port or token ("claude!", Claude Code only,
// which needs a remove first; the others are rewritten in place).
func (mcpWireStep) todo(ctx context.Context, e *Env, hdr map[string]string) []string {
	var out []string
	url := agentURL(e)
	if has(ctx, e, "claude") {
		got, err := e.Exec.Run(ctx, Cmd{Name: "claude", Args: []string{"mcp", "get", "mcpjungle"}, AsUser: true})
		switch {
		case err != nil:
			out = append(out, "claude")
		case !strings.Contains(string(got), url) || (hdr != nil) != strings.Contains(string(got), "Authorization"):
			out = append(out, "claude!")
		}
	}
	if has(ctx, e, "codex") {
		b, _ := os.ReadFile(e.Path("~/.codex/config.toml"))
		if !bytes.Equal(codexWired(b, url, hdr), b) {
			out = append(out, "codex")
		}
	}
	if has(ctx, e, "gemini") {
		h, _ := json.Marshal(hdr)
		if _, err := e.Exec.Run(ctx, Cmd{Name: "jq", Args: []string{"-e", "--arg", "u", url, "--argjson", "h", string(h),
			".mcpServers.mcpjungle.httpUrl == $u and (.mcpServers.mcpjungle.headers // null) == $h", e.Path("~/.gemini/settings.json")}}); err != nil {
			out = append(out, "gemini")
		}
	}
	if has(ctx, e, "opencode") {
		b, _ := readOrEmpty(e, openCodeConfig)
		if _, changed, err := openCodeWired(b, url, hdr); err != nil || changed {
			out = append(out, "opencode")
		}
	}
	if has(ctx, e, "goose") {
		b, _ := readOrEmpty(e, gooseConfig)
		if _, changed, err := gooseWired(b, url, hdr); err != nil || changed {
			out = append(out, "goose")
		}
	}
	return out
}

// codexWired returns Codex's config with the mcpjungle section pointing
// at url and sending hdr: its url and http_headers lines replaced (the
// headers line dropped without hdr), or the section appended.
func codexWired(b []byte, url string, hdr map[string]string) []byte {
	want := []string{fmt.Sprintf("url = %q", url)}
	if len(hdr) > 0 {
		keys := make([]string, 0, len(hdr))
		for k := range hdr {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var kv []string
		for _, k := range keys {
			kv = append(kv, fmt.Sprintf("%q = %q", k, hdr[k]))
		}
		want = append(want, "http_headers = { "+strings.Join(kv, ", ")+" }")
	}
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if l != codexSection {
			continue
		}
		// The section's own lines, without url and http_headers, then ours.
		j := i + 1
		var rest []string
		for ; j < len(lines) && !strings.HasPrefix(lines[j], "["); j++ {
			k := strings.TrimSpace(lines[j])
			if !strings.HasPrefix(k, "url") && !strings.HasPrefix(k, "http_headers") {
				rest = append(rest, lines[j])
			}
		}
		out := slices.Concat(lines[:i+1], want, rest, lines[j:])
		return []byte(strings.Join(out, "\n"))
	}
	return append(append([]byte{}, b...), fmt.Sprintf("\n%s\n%s\n", codexSection, strings.Join(want, "\n"))...)
}

func hasLine(b []byte, line string) bool {
	for _, l := range strings.Split(string(b), "\n") {
		if l == line {
			return true
		}
	}
	return false
}

func (w mcpWireStep) Check(ctx context.Context, e *Env) (bool, error) {
	if !wire(e) {
		return true, nil
	}
	hdr, err := agentHeaders(ctx, e)
	if err != nil {
		return false, nil
	}
	return len(w.todo(ctx, e, hdr)) == 0, nil
}

func (w mcpWireStep) Apply(ctx context.Context, e *Env) error {
	if !wire(e) {
		return nil
	}
	hdr, err := agentHeaders(ctx, e)
	if err != nil {
		return err
	}
	for _, agent := range w.todo(ctx, e, hdr) {
		var err error
		switch agent {
		case "claude!":
			if _, err = e.run(ctx, Cmd{Name: "claude", Args: []string{"mcp", "remove", "mcpjungle", "--scope", "user"}, AsUser: true}); err != nil {
				break
			}
			fallthrough
		case "claude":
			args := []string{"mcp", "add", "--transport", "http", "mcpjungle", agentURL(e), "--scope", "user"}
			for k, v := range hdr {
				args = append(args, "--header", k+": "+v)
			}
			_, err = e.run(ctx, Cmd{Name: "claude", Args: args, AsUser: true})
		case "codex":
			err = w.codex(e, hdr)
		case "gemini":
			err = w.gemini(ctx, e, hdr)
		case "opencode", "goose":
			p, wired := openCodeConfig, openCodeWired
			if agent == "goose" {
				p, wired = gooseConfig, gooseWired
			}
			var cur, b []byte
			if cur, err = readOrEmpty(e, p); err == nil {
				if b, _, err = wired(cur, agentURL(e), hdr); err == nil {
					err = writeAgentConfig(e, p, b)
				}
			}
		}
		if err != nil {
			return fmt.Errorf("%s: %w", agent, err)
		}
	}
	return nil
}

func (mcpWireStep) userDir(e *Env, d string) error {
	p := e.Path(d)
	if err := os.MkdirAll(p, 0o700); err != nil {
		return err
	}
	return e.chown(p, e.UID, e.GID)
}

func (w mcpWireStep) codex(e *Env, hdr map[string]string) error {
	if err := w.userDir(e, "~/.codex"); err != nil {
		return err
	}
	p := e.Path("~/.codex/config.toml")
	b, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(p); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := writeFile(p, codexWired(b, agentURL(e), hdr), mode); err != nil {
		return err
	}
	return e.chown(p, e.UID, e.GID)
}

func (w mcpWireStep) gemini(ctx context.Context, e *Env, hdr map[string]string) error {
	if err := w.userDir(e, "~/.gemini"); err != nil {
		return err
	}
	p := e.Path("~/.gemini/settings.json")
	b, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		b = []byte("{}")
	}
	// jq keeps the user's key order and formatting choices.
	h, err := json.Marshal(hdr)
	if err != nil {
		return err
	}
	out, err := e.Exec.Run(ctx, Cmd{Name: "jq", Args: []string{"--arg", "u", agentURL(e), "--argjson", "h", string(h),
		".mcpServers.mcpjungle = ({httpUrl: $u} + if $h then {headers: $h} else {} end)"}, Stdin: b})
	if err != nil {
		return fmt.Errorf("jq: %w: %s", err, out)
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(p); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := writeFile(p, out, mode); err != nil {
		return err
	}
	return e.chown(p, e.UID, e.GID)
}

// --- go:mcp_sync: register the picked servers with the gateway ---------------

// mcpSyncStep runs mcp-sync, which registers servers whose keys are filled
// in and re-registers those whose config changed: on every run.
type mcpSyncStep struct{}

func (mcpSyncStep) Always() bool { return true }

func (mcpSyncStep) Plan(e *Env) []Action {
	return []Action{{"run", strings.TrimSpace("mcp-sync " + strings.Join(servers(e), " "))}}
}

func (mcpSyncStep) Check(context.Context, *Env) (bool, error) { return true, nil }

func (mcpSyncStep) Apply(ctx context.Context, e *Env) error {
	args := append([]string{"MCP_DIR=" + filepath.Join(e.Home, ".mcp"), "MCP_CATALOG=" + mcpCatalog, "MCPJUNGLE_REGISTRY=" + mcpURL(e), mcpSync}, servers(e)...)
	c := Cmd{Name: "env", Args: args, As: MCPUser(e)}
	if !mcpAsService(e) {
		c = Cmd{Name: "env", Args: args, AsUser: true}
	}
	if _, err := e.run(ctx, c); err != nil {
		return fmt.Errorf("some MCP servers did not register (see the log); fill in the keys with 'mcp-sync edit <server>', then run mcp-sync: %w", err)
	}
	return nil
}

// --- go:mcp_firewall: VPN only ------------------------------------------------

// mcpFirewallStep keeps the gateway's port to this machine and the VPN
// interfaces when its access is "vpn", and takes the rule away otherwise.
type mcpFirewallStep struct{}

func (mcpFirewallStep) Plan(e *Env) []Action {
	t := mcpTable(e)
	if t.chains == "" {
		return nil
	}
	return []Action{{"run", fmt.Sprintf("nft: port %d only from this machine and %s (%s)", MCPPort(e), strings.Join(VPNInterfaces, ", "), t.file())}}
}

func (mcpFirewallStep) Check(ctx context.Context, e *Env) (bool, error) {
	return mcpTable(e).Check(ctx, e), nil
}

func (mcpFirewallStep) Apply(ctx context.Context, e *Env) error { return mcpTable(e).Apply(ctx, e) }

// --- go:docker_ports: where published ports can be reached from ---------------

type dockerPortsStep struct{}

func (dockerPortsStep) Plan(e *Env) []Action {
	t := dockerTable(e)
	if t.chains == "" {
		return nil
	}
	who := "this machine"
	if DockerPorts(e) == "vpn" {
		who = "this machine and " + strings.Join(VPNInterfaces, ", ")
	}
	return []Action{{"run", "nft: containers' published ports only from " + who + " (" + t.file() + ")"}}
}

func (dockerPortsStep) Check(ctx context.Context, e *Env) (bool, error) {
	return dockerTable(e).Check(ctx, e), nil
}

func (dockerPortsStep) Apply(ctx context.Context, e *Env) error { return dockerTable(e).Apply(ctx, e) }
