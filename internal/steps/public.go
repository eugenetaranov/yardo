package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// The gateway on the internet: Caddy (from its own apt repository) answers
// at the user's domain name with an automatic HTTPS certificate and passes
// requests to the gateway on loopback, which needs a token. fail2ban bans
// addresses that keep failing it.
const (
	caddyfile     = "/etc/caddy/Caddyfile"
	caddySites    = "/etc/caddy/yardo"
	caddyImport   = "import /etc/caddy/yardo/*.caddy"
	f2bFilter     = "/etc/fail2ban/filter.d/yardo-mcp.conf"
	f2bJail       = "/etc/fail2ban/jail.d/yardo-mcp.conf"
	caddyLogDir   = "/var/log/caddy"
	caddyRepoKey  = "https://dl.cloudsmith.io/public/caddy/stable/gpg.key"
	caddyRepoFpr  = "65760C51EDEA2017CEA2CA15155B6D79CA56EA34"
	caddyRepoBody = `Types: deb
URIs: https://dl.cloudsmith.io/public/caddy/stable/deb/debian
Suites: any-version
Components: main
Architectures: {{arch}}
Signed-By: {{keyring}}
`
)

const f2bFilterText = `# yardo: requests to the MCP gateway (behind Caddy) without a valid token.
[Definition]
failregex = ^.*"remote_ip":"<HOST>".*"status":(?:401|403)\b
ignoreregex =
datepattern = "ts":{EPOCH}
`

const f2bJailText = `# yardo: ban addresses that keep failing the MCP gateway's token.
[yardo-mcp]
enabled = true
port = http,https
filter = yardo-mcp
logpath = /var/log/caddy/yardo-mcp-*.log
backend = auto
maxretry = 10
findtime = 10m
bantime = 1h
`

// domainRe is a domain name Caddy can get a certificate for: letters,
// digits, dots and dashes, with a dot (it goes into Caddy's config).
var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

// ValidDomain reports whether d can be the gateway's domain name.
func ValidDomain(d string) bool { return len(d) <= 253 && domainRe.MatchString(d) }

// mcpSite is the user's Caddy site file.
func mcpSite(e *Env) string { return filepath.Join(caddySites, "mcp-"+e.User+".caddy") }

func mcpLog(e *Env) string { return filepath.Join(caddyLogDir, "yardo-mcp-"+e.User+".log") }

func siteText(e *Env) []byte {
	return []byte(fmt.Sprintf(`# yardo: %s's MCP gateway, at its domain name (yardo rewrites this file).
%s {
	reverse_proxy 127.0.0.1:%d
	log {
		output file %s
		format json
	}
}
`, e.User, e.Answers.MCPDomain, MCPPort(e), mcpLog(e)))
}

type mcpPublicStep struct{}

func public(e *Env) bool { return listen(e) == "public" }

func (mcpPublicStep) Plan(e *Env) []Action {
	if !public(e) {
		return nil
	}
	return []Action{{"run", fmt.Sprintf("caddy: https://%s → 127.0.0.1:%d (%s); fail2ban jail yardo-mcp", e.Answers.MCPDomain, MCPPort(e), mcpSite(e))}}
}

func (s mcpPublicStep) Check(ctx context.Context, e *Env) (bool, error) {
	if !public(e) {
		_, err := os.Stat(e.Path(mcpSite(e)))
		return os.IsNotExist(err), nil
	}
	if !ValidDomain(e.Answers.MCPDomain) {
		return false, nil
	}
	imp, _ := os.ReadFile(e.Path(caddyfile))
	if !same(e.Path(mcpSite(e)), wantFile{b: siteText(e), mode: 0o644}) || !hasLine(imp, caddyImport) {
		return false, nil
	}
	if _, err := os.Stat(e.Path("/etc/fail2ban")); err == nil &&
		(!same(e.Path(f2bFilter), wantFile{b: []byte(f2bFilterText), mode: 0o644}) || !same(e.Path(f2bJail), wantFile{b: []byte(f2bJailText), mode: 0o644})) {
		return false, nil
	}
	return systemctlStep{catalog.Step{Unit: "caddy"}}.Check(ctx, e)
}

func (s mcpPublicStep) Apply(ctx context.Context, e *Env) error {
	if !public(e) {
		// No longer public: the site goes; Caddy stays, for anything else.
		if err := os.Remove(e.Path(mcpSite(e))); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		_, err := e.run(ctx, Cmd{Name: "systemctl", Args: []string{"try-reload-or-restart", "caddy"}})
		return err
	}
	if !ValidDomain(e.Answers.MCPDomain) {
		return fmt.Errorf("the MCP gateway's domain name %q isn't one (run 'sudo yardo' and choose it again)", e.Answers.MCPDomain)
	}
	for _, st := range []catalog.Step{
		{Type: "apt_repo", Name: "caddy", KeyURL: caddyRepoKey, KeyFingerprint: caddyRepoFpr, Source: caddyRepoBody},
		{Type: "apt", Packages: []string{"caddy"}},
	} {
		x, err := New("mcpjungle-install", st)
		if err != nil {
			return err
		}
		if done, err := x.Check(ctx, e); err == nil && done {
			continue
		}
		if r, ok := x.(aptRepoStep); ok {
			if err := r.Apply(ctx, e); err != nil {
				return err
			}
			if _, err := e.run(ctx, Cmd{Name: "apt-get", Args: append(slices.Clone(AptOpts), "update"), Env: AptEnv}); err != nil {
				return err
			}
			continue
		}
		if err := x.Apply(ctx, e); err != nil {
			return err
		}
	}
	imp, err := os.ReadFile(e.Path(caddyfile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !hasLine(imp, caddyImport) {
		if len(imp) > 0 && imp[len(imp)-1] != '\n' {
			imp = append(imp, '\n')
		}
		if err := writeFile(e.Path(caddyfile), append(imp, []byte("\n# yardo's sites (the MCP gateway at its domain name).\n"+caddyImport+"\n")...), 0o644); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(e.Path(caddySites), 0o755); err != nil {
		return err
	}
	if err := writeFile(e.Path(mcpSite(e)), siteText(e), 0o644); err != nil {
		return err
	}
	// The log exists before fail2ban looks for it (a jail with no log file
	// stops fail2ban starting), and is Caddy's to write.
	if _, err := e.run(ctx, Cmd{Name: "install", Args: []string{"-d", "-o", "caddy", "-g", "caddy", e.Path(caddyLogDir)}}); err != nil {
		return err
	}
	if _, err := e.run(ctx, Cmd{Name: "sh", Args: []string{"-c", `[ -e "$1" ] || install -m 0640 -o caddy -g caddy /dev/null "$1"`, "_", e.Path(mcpLog(e))}}); err != nil {
		return err
	}
	for _, args := range [][]string{{"enable", "--now", "caddy"}, {"reload", "caddy"}} {
		if _, err := e.run(ctx, Cmd{Name: "systemctl", Args: args}); err != nil {
			return err
		}
	}
	if _, err := os.Stat(e.Path("/etc/fail2ban")); err != nil {
		e.note("The MCP gateway is on the internet without fail2ban: install it (it comes on machines with a public IPv4) to ban addresses that keep failing its token.")
		return nil
	}
	changed := false
	for _, w := range []wantFile{{f2bFilter, []byte(f2bFilterText), 0o644}, {f2bJail, []byte(f2bJailText), 0o644}} {
		if p := e.Path(w.to); !same(p, w) {
			if err := writeFile(p, w.b, w.mode); err != nil {
				return err
			}
			changed = true
		}
	}
	if changed {
		if _, err := e.run(ctx, Cmd{Name: "systemctl", Args: []string{"try-reload-or-restart", "fail2ban"}}); err != nil {
			return err
		}
	}
	e.note(fmt.Sprintf("The MCP gateway is at https://%s/mcp (clients need a token: mcp-sync client <name>). Point %s's DNS at this machine and open TCP 80 and 443 in the cloud firewall, for its certificate.", e.Answers.MCPDomain, e.Answers.MCPDomain))
	return nil
}
