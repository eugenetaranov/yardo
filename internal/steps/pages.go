package steps

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// ~/pages over HTTP, by Caddy: what agents make to be looked at (archify's diagrams,
// reports), served to the user's laptop as the MCP gateway is: this
// machine only (an SSH tunnel), the VPNs, the internet at its domain name
// (under /pages/, with a password) or everywhere.

const (
	pagesUnit   = "/etc/systemd/system/yardo-pages@.service"
	pagesEnvDir = "/etc/yardo/pages"
	// PagesConf tells pages-url how the laptop reaches ~/pages.
	PagesConf = "~/.config/yardo/pages"
	// PagesPassword is the login for ~/pages on the internet (user "pages").
	PagesPassword = "~/.config/yardo/pages-password"
	pagesURLBin   = "/usr/local/bin/pages-url"
)

func init() {
	RegisterGo("pages_unit", func(catalog.Step) Step { return pagesUnitStep{} })
	RegisterGo("pages_firewall", func(catalog.Step) Step { return pagesFirewallStep{} })
	RegisterGo("pages_public", func(catalog.Step) Step { return pagesPublicStep{} })
}

// PagesPort is the user's port for ~/pages: their gateway port's, plus 1000
// (9080 for the first user).
func PagesPort(e *Env) int { return MCPPort(e) + 1000 }

func pagesInstance(e *Env) string { return "yardo-pages@" + e.User }

// pagesTemplate serves each user's ~/pages, as them, read only.
const pagesTemplate = `[Unit]
Description=yardo: %i's ~/pages over HTTP
After=network.target

[Service]
Type=simple
User=%i
EnvironmentFile=/etc/yardo/pages/%i.env
# PAGES_HOST is 127.0.0.1 (this machine, or behind the site Caddy serves at
# the gateway's domain) or 0.0.0.0 (the VPNs only, kept so by yardo's
# firewall, or everywhere). --browse lists the folder at /.
ExecStart=/usr/bin/caddy file-server --root ${PAGES_DIR} --listen ${PAGES_HOST}:${PAGES_PORT} --browse
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`

// pagesURLText is pages-url: the link to a page in ~/pages, as the laptop
// reaches it.
const pagesURLText = `#!/bin/sh
# yardo: the link to a page in ~/pages, as your laptop reaches it:
#   pages-url diagram.html   (or ~/pages/diagram.html, or nothing for the list)
# The address is the one your SSH session came in at ($SSH_CONNECTION).
# yardo rewrites this file.
conf="$HOME/.config/yardo/pages"
if [ ! -r "$conf" ]; then
  echo "pages-url: ~/pages isn't served here (sudo yardo, Agents: Web pages)" >&2
  exit 1
fi
. "$conf"
f="${1:-}"
case "$f" in
  "$HOME/pages/"*) f="${f#"$HOME/pages/"}" ;;
  "~/pages/"*) f="${f#"~/pages/"}" ;;
  pages/*) f="${f#pages/}" ;;
  /*) echo "pages-url: $f isn't in ~/pages: copy it there first" >&2; exit 1 ;;
esac
host=$(printf '%s\n' "${SSH_CONNECTION:-}" | awk '{print $3}')
[ -n "$host" ] || host=$(hostname -I 2>/dev/null | awk '{print $1}')
case "$host" in *:*) host="[$host]" ;; esac
case "$mode" in
  public) echo "https://$domain/pages/$f"
    echo "(user pages, password: cat ~/.config/yardo/pages-password)" >&2 ;;
  127.0.0.1) echo "http://127.0.0.1:$port/$f"
    echo "(this machine only: on your laptop first  ssh -N -L $port:127.0.0.1:$port $(hostname -s))" >&2 ;;
  *) echo "http://$host:$port/$f" ;;
esac
`

const pagesReadme = `Pages in this folder open in your browser: pages-url NAME.html prints the
link (yardo serves this folder; agents save diagrams and reports here).
`

// --- go:pages_unit -----------------------------------------------------------

type pagesUnitStep struct{}

func (pagesUnitStep) envFile(e *Env) wantFile {
	return wantFile{filepath.Join(pagesEnvDir, e.User+".env"),
		[]byte(fmt.Sprintf("PAGES_HOST=%s\nPAGES_PORT=%d\nPAGES_DIR=%s\n", bindHost(e), PagesPort(e), filepath.Join(e.Home, "pages"))), 0o644}
}

func pagesConfText(e *Env) []byte {
	domain := ""
	if e.Answers != nil {
		domain = e.Answers.MCPDomain
	}
	return []byte(fmt.Sprintf("# yardo writes this: how your laptop reaches ~/pages (pages-url reads it).\nmode=%s\nport=%d\ndomain=%s\n", listen(e), PagesPort(e), domain))
}

func (u pagesUnitStep) files(e *Env) []wantFile {
	return []wantFile{{pagesUnit, []byte(pagesTemplate), 0o644}, u.envFile(e), {pagesURLBin, []byte(pagesURLText), 0o755}}
}

func (u pagesUnitStep) Plan(e *Env) []Action {
	return []Action{{"apt", "caddy (its own repository; its port-80 server stays off)"}, {"dir", "~/pages"}, {"file", pagesURLBin}, {"file", PagesConf},
		{"unit", fmt.Sprintf("enable-now %s on port %d (%s)", pagesInstance(e), PagesPort(e), pagesUnit)}}
}

func (u pagesUnitStep) Check(ctx context.Context, e *Env) (bool, error) {
	if _, err := os.Stat(e.Path("/usr/bin/caddy")); err != nil {
		return false, nil
	}
	for _, w := range u.files(e) {
		if !same(e.Path(w.to), w) {
			return false, nil
		}
	}
	if fi, err := os.Stat(e.Path("~/pages")); err != nil || !fi.IsDir() {
		return false, nil
	}
	if !same(e.Path(PagesConf), wantFile{b: pagesConfText(e), mode: 0o644}) {
		return false, nil
	}
	return systemctlStep{catalog.Step{Unit: pagesInstance(e)}}.Check(ctx, e)
}

// Down: the user's pages server is enabled but not running.
func (pagesUnitStep) Down(ctx context.Context, e *Env) (bool, string) {
	return unitDown(ctx, e, pagesInstance(e)), pagesInstance(e)
}

func (u pagesUnitStep) Apply(ctx context.Context, e *Env) error {
	fresh, err := installCaddy(ctx, e)
	if err != nil {
		return err
	}
	// Caddy's package starts its own server, on port 80: not wanted for
	// ~/pages. It's started again when the gateway goes on the internet.
	if fresh && !public(e) {
		if _, err := e.run(ctx, Cmd{Name: "systemctl", Args: []string{"disable", "--now", "caddy"}}); err != nil {
			return err
		}
	}
	dir := e.Path("~/pages")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dir, "README.txt"), []byte(pagesReadme), 0o644); err != nil {
			return err
		}
		for _, p := range []string{dir, filepath.Join(dir, "README.txt")} {
			if err := e.chown(p, e.UID, e.GID); err != nil {
				return err
			}
		}
	}
	conf := e.Path(PagesConf)
	if err := mkdirFor(e, conf, catalog.Step{To: PagesConf}); err != nil {
		return err
	}
	if err := writeFile(conf, pagesConfText(e), 0o644); err != nil {
		return err
	}
	if err := e.chown(conf, e.UID, e.GID); err != nil {
		return err
	}
	changed := false
	for _, w := range u.files(e) {
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
	inst := pagesInstance(e)
	cmds := [][]string{{"enable", "--now", inst}}
	if changed {
		cmds = [][]string{{"daemon-reload"}, {"enable", inst}, {"restart", inst}}
	}
	for _, args := range cmds {
		if _, err := e.run(ctx, Cmd{Name: "systemctl", Args: args}); err != nil {
			return err
		}
	}
	return nil
}

// PlanUndo stops the server and takes its files away; ~/pages is the
// user's, and stays.
func (pagesUnitStep) PlanUndo(_ context.Context, e *Env, p *RemovePlan) error {
	inst := pagesInstance(e)
	p.off(fmt.Sprintf("the %s service", inst), func(ctx context.Context) error {
		_, _ = e.run(ctx, Cmd{Name: "systemctl", Args: []string{"disable", "--now", inst}})
		for _, f := range []string{filepath.Join(pagesEnvDir, e.User+".env"), PagesConf} {
			if err := rmIfThere(e.Path(f)); err != nil {
				return err
			}
		}
		return nil
	})
	return nil
}

// --- go:pages_firewall: VPN only ---------------------------------------------

// pagesTable is the user's pages port, let in only from this machine and
// the VPN interfaces when the access is "vpn" (no table otherwise).
func pagesTable(e *Env) fwTable {
	t := fwTable{name: "yardo_pages_" + nftName(e.User)}
	if listen(e) == "vpn" {
		t.chains = fmt.Sprintf(`	chain input {
		type filter hook input priority filter - 1; policy accept;
		tcp dport %[1]d iifname "lo" accept
		tcp dport %[1]d iifname %[2]s accept
		tcp dport %[1]d drop
	}
`, PagesPort(e), nftSet(VPNInterfaces))
	}
	return t
}

type pagesFirewallStep struct{}

func (pagesFirewallStep) Plan(e *Env) []Action {
	t := pagesTable(e)
	if t.chains == "" {
		return nil
	}
	return []Action{{"run", fmt.Sprintf("nft: port %d only from this machine and %s (%s)", PagesPort(e), strings.Join(VPNInterfaces, ", "), t.file())}}
}

func (pagesFirewallStep) Check(ctx context.Context, e *Env) (bool, error) {
	return pagesTable(e).Check(ctx, e), nil
}

func (pagesFirewallStep) Apply(ctx context.Context, e *Env) error { return pagesTable(e).Apply(ctx, e) }

func (pagesFirewallStep) PlanUndo(_ context.Context, e *Env, p *RemovePlan) error {
	p.off("the firewall table "+pagesTable(e).name, func(ctx context.Context) error {
		return fwTable{name: pagesTable(e).name}.Apply(ctx, e)
	})
	return nil
}

// --- go:pages_public: on the internet, under the gateway's domain ------------

// pagesHash is the user's pages password, hashed for Caddy's basic_auth.
func pagesHash(e *Env) string { return filepath.Join(pagesEnvDir, e.User+".hash") }

// pagesSiteBlock is what the gateway's Caddy site gets for ~/pages, once
// its password is hashed: /pages/ with a login.
func pagesSiteBlock(e *Env) string {
	b, err := os.ReadFile(e.Path(pagesHash(e)))
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		return ""
	}
	return fmt.Sprintf(`	redir /pages /pages/
	handle_path /pages/* {
		basic_auth {
			pages %s
		}
		reverse_proxy 127.0.0.1:%d
	}
`, strings.TrimSpace(string(b)), PagesPort(e))
}

type pagesPublicStep struct{}

func (pagesPublicStep) Plan(e *Env) []Action {
	if !public(e) {
		return nil
	}
	return []Action{{"run", fmt.Sprintf("caddy: https://%s/pages/ → 127.0.0.1:%d, user pages, password in %s", e.Answers.MCPDomain, PagesPort(e), PagesPassword)}}
}

func (pagesPublicStep) Check(_ context.Context, e *Env) (bool, error) {
	if !public(e) {
		return true, nil
	}
	if _, err := os.Stat(e.Path(PagesPassword)); err != nil || pagesSiteBlock(e) == "" {
		return false, nil
	}
	return same(e.Path(mcpSite(e)), wantFile{b: siteText(e), mode: 0o644}), nil
}

func (pagesPublicStep) Apply(ctx context.Context, e *Env) error {
	if !public(e) {
		return nil
	}
	pw := e.Path(PagesPassword)
	b, err := os.ReadFile(pw)
	if err != nil {
		r := make([]byte, 18)
		if _, err := rand.Read(r); err != nil {
			return err
		}
		b = []byte(base64.RawURLEncoding.EncodeToString(r) + "\n")
		if err := mkdirFor(e, pw, catalog.Step{To: PagesPassword}); err != nil {
			return err
		}
		if err := writeFile(pw, b, 0o600); err != nil {
			return err
		}
		if err := e.chown(pw, e.UID, e.GID); err != nil {
			return err
		}
	}
	if pagesSiteBlock(e) == "" {
		out, err := e.run(ctx, Cmd{Name: "caddy", Args: []string{"hash-password", "--plaintext", strings.TrimSpace(string(b))}})
		if err != nil {
			return fmt.Errorf("hashing the pages password: %w", err)
		}
		if err := os.MkdirAll(e.Path(pagesEnvDir), 0o755); err != nil {
			return err
		}
		if err := writeFile(e.Path(pagesHash(e)), []byte(strings.TrimSpace(string(out))+"\n"), 0o600); err != nil {
			return err
		}
	}
	if p := e.Path(mcpSite(e)); !same(p, wantFile{b: siteText(e), mode: 0o644}) {
		if err := writeFile(p, siteText(e), 0o644); err != nil {
			return err
		}
		if _, err := e.run(ctx, Cmd{Name: "systemctl", Args: []string{"reload", "caddy"}}); err != nil {
			return err
		}
	}
	e.note(fmt.Sprintf("Your pages are at https://%s/pages/ (user pages, password: cat %s).", e.Answers.MCPDomain, PagesPassword))
	return nil
}

// PlanUndo takes /pages/ off the gateway's site; the password file stays
// with the pages.
func (pagesPublicStep) PlanUndo(_ context.Context, e *Env, p *RemovePlan) error {
	if _, err := os.Stat(e.Path(pagesHash(e))); err != nil {
		return nil
	}
	p.off("/pages/ on the gateway's domain", func(ctx context.Context) error {
		if err := rmIfThere(e.Path(pagesHash(e))); err != nil {
			return err
		}
		if !public(e) {
			return nil
		}
		if err := writeFile(e.Path(mcpSite(e)), siteText(e), 0o644); err != nil {
			return err
		}
		_, err := e.run(ctx, Cmd{Name: "systemctl", Args: []string{"try-reload-or-restart", "caddy"}})
		return err
	})
	return nil
}
