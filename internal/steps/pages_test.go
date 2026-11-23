package steps

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

func TestPagesUnit(t *testing.T) {
	active := false
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "systemctl" && (c.Args[0] == "is-enabled" || c.Args[0] == "is-active") && !active {
			return nil, errors.New("inactive")
		}
		if c.Name == "dpkg-query" {
			return []byte("install ok installed"), nil
		}
		return nil, nil
	}}
	e := mcpEnv(t, x)
	e.Answers.MCPListen, e.Answers.MCPPort = "vpn", 8081
	// Caddy is there already (no key to fetch in a test).
	repo := aptRepoStep{catalog.Step{Type: "apt_repo", Name: "caddy", Source: caddyRepoBody}}
	put(t, e.Path(repo.keyPath()), "key")
	put(t, e.Path(repo.listPath()), repo.source(e))
	put(t, e.Path("/usr/bin/caddy"), "")
	st := step(t, catalog.Step{Type: "go:pages_unit"})
	if check(t, e, st) {
		t.Fatal("Check = true before anything is written")
	}
	apply(t, e, st)
	if got := read(t, e.Path(pagesUnit)); !strings.Contains(got, "ExecStart=/usr/bin/caddy file-server --root ${PAGES_DIR} --listen ${PAGES_HOST}:${PAGES_PORT} --browse") {
		t.Errorf("unit:\n%s", got)
	}
	if got := strings.Join(x.commands(), "; "); strings.Contains(got, "disable --now caddy") {
		t.Errorf("stopped a Caddy it didn't install: %s", got)
	}
	if got := read(t, e.Path("/etc/yardo/pages/ada.env")); got != "PAGES_HOST=0.0.0.0\nPAGES_PORT=9081\nPAGES_DIR=/home/ada/pages\n" {
		t.Errorf("env = %q", got)
	}
	if got := read(t, e.Path(PagesConf)); !strings.Contains(got, "\nmode=vpn\nport=9081\n") {
		t.Errorf("pages conf = %q", got)
	}
	if fi, err := os.Stat(e.Path(pagesURLBin)); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("pages-url: %v", err)
	}
	if _, err := os.Stat(e.Path("~/pages/README.txt")); err != nil {
		t.Error("no ~/pages")
	}
	if got := strings.Join(x.commands(), "; "); !strings.Contains(got, "systemctl restart yardo-pages@ada") {
		t.Errorf("ran %s", got)
	}
	active = true
	if !check(t, e, st) {
		t.Error("Check = false once applied")
	}
	// This machine only: loopback, and the firewall table goes.
	e.Answers.MCPListen = ""
	if check(t, e, st) {
		t.Error("Check = true with the access changed")
	}
	if pagesTable(e).chains != "" {
		t.Error("a firewall table for this machine only")
	}
	e.Answers.MCPListen = "vpn"
	if tb := pagesTable(e); !strings.Contains(tb.chains, "tcp dport 9081 drop") {
		t.Errorf("vpn table:\n%s", tb.chains)
	}
}

func TestPagesPublic(t *testing.T) {
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "caddy" {
			return []byte("$2a$14$hash\n"), nil
		}
		return nil, nil
	}}
	e := mcpEnv(t, x)
	e.Answers.MCPListen, e.Answers.MCPDomain, e.Answers.MCPPort = "public", "mcp.example.com", 8081
	st := step(t, catalog.Step{Type: "go:pages_public"})
	if check(t, e, st) {
		t.Fatal("Check = true before the password")
	}
	apply(t, e, st)
	if pw := read(t, e.Path(PagesPassword)); len(strings.TrimSpace(pw)) < 20 {
		t.Errorf("password %q", pw)
	}
	site := read(t, e.Path(mcpSite(e)))
	if !strings.Contains(site, "handle_path /pages/* {\n\t\tbasic_auth {\n\t\t\tpages $2a$14$hash\n") || !strings.Contains(site, "reverse_proxy 127.0.0.1:9081") || !strings.Contains(site, "\treverse_proxy 127.0.0.1:8081\n") {
		t.Errorf("site:\n%s", site)
	}
	if !check(t, e, st) {
		t.Error("Check = false once applied")
	}
	// Not public: nothing to do.
	e.Answers.MCPListen = "vpn"
	if !check(t, e, st) {
		t.Error("Check = false when not public")
	}
}
