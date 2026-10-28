package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/mcp"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// foundServer is a server in ~/.mcp: entry is how the answers name it
// (jira, opensearch:prod), registered whether the gateway has it now.
type foundServer struct {
	entry      string
	registered bool
	own        bool // its own spec, not the catalog's
	linked     bool // a spec of its own or a link: mcp-sync keeps it on
}

// mcpOwner is who owns ~/.mcp: the gateway's service user, or the user
// before there is one.
func mcpOwner(login string) string {
	if _, err := user.Lookup("mcp-" + login); err == nil {
		return "mcp-" + login
	}
	return login
}

// findMCP lists the servers in login's ~/.mcp, as its owner (a symlink
// planted there then can't show root's files): one line per directory
// with an env, "<name>\t<spec link or own>\t<registered 0|1>".
func findMCP(ctx context.Context, login string, specs map[string]*mcp.Spec) []foundServer {
	script := `cd -- "$1" 2>/dev/null || exit 0
for d in */; do
  d=${d%/}; [ -f "$d/env" ] || continue
  t=; if [ -L "$d/spec" ]; then t=$(readlink -- "$d/spec"); elif [ -f "$d/spec" ]; then t=own; fi
  r=0; [ -f "$d/.registered.json" ] && r=1
  printf '%s\t%s\t%s\n' "$d" "$t" "$r"
done`
	out, err := exec.CommandContext(ctx, "runuser", "-u", mcpOwner(login), "--", "sh", "-c", script, "_", filepath.Join(homeOf(login), ".mcp")).Output()
	if err != nil {
		return nil
	}
	return parseFound(string(out), specs)
}

// parseFound reads findMCP's lines. A directory is a catalog server
// (jira), another instance of one (opensearch-prod, its spec a link to
// opensearch.spec; or once dropped, with no link, named <spec>-<label>),
// or a server of its own (its own spec). Anything else isn't a server.
func parseFound(out string, specs map[string]*mcp.Spec) []foundServer {
	var found []foundServer
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 3 || !mcp.ValidName(f[0]) {
			continue
		}
		name, link, reg := f[0], f[1], f[2] == "1"
		entry := ""
		switch {
		case link == "own":
			entry = name
		case link != "" && strings.HasSuffix(link, ".spec"):
			spec := strings.TrimSuffix(filepath.Base(link), ".spec")
			if label, ok := strings.CutPrefix(name, spec+"-"); ok && specs[spec] != nil && mcp.ValidLabel(label) {
				entry = spec + ":" + label
			} else if name == spec && specs[spec] != nil {
				entry = name // renamed back from another instance
			}
		case specs[name] != nil:
			entry = name
		default:
			// The longest spec it's another instance of.
			best := ""
			for spec := range specs {
				if label, ok := strings.CutPrefix(name, spec+"-"); ok && mcp.ValidLabel(label) && len(spec) > len(best) {
					best = spec
				}
			}
			if best != "" {
				entry = best + ":" + strings.TrimPrefix(name, best+"-")
			}
		}
		if entry != "" {
			found = append(found, foundServer{entry, reg, link == "own", link != ""})
		}
	}
	return found
}

// mcpSyncAs runs mcp-sync args as ~/.mcp's owner, on the user's gateway.
func (k *asker) mcpSyncAs(ctx context.Context, args ...string) error {
	if k.mcpSync != nil {
		return k.mcpSync(args...)
	}
	port := k.a.MCPPort
	if port == 0 && k.port != nil {
		port = k.port()
	}
	// This version's mcp-sync (unpacked as root at start): the installed
	// one is replaced only when the run applies, after these questions.
	sync := filepath.Join(k.share, "mcp-sync")
	if _, err := os.Stat(sync); k.share == "" || err != nil {
		sync = "/usr/local/bin/mcp-sync"
	}
	c := exec.CommandContext(ctx, "runuser", append([]string{"-u", mcpOwner(k.a.User), "--", "env",
		"MCP_DIR=" + filepath.Join(homeOf(k.a.User), ".mcp"), fmt.Sprintf("MCPJUNGLE_REGISTRY=http://127.0.0.1:%d", port),
		sync}, args...)...)
	out, err := c.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil && s != "" {
		lines := strings.Split(s, "\n")
		return fmt.Errorf("%s", strings.TrimPrefix(lines[len(lines)-1], "mcp-sync: "))
	}
	if s != "" {
		fmt.Fprintln(k.out, s)
	}
	return err
}

// mcpRows are the menu's MCP servers: yardo's (entries), then the others
// found in ~/.mcp, each with its state. inputs are the MCP servers' empty
// values ("~/.mcp/<name>/env: KEY"); checked: the gateway's state is known.
func mcpRows(entries []string, found []foundServer, inputs []string, specs map[string]*mcp.Spec, checked bool) []ui.MCPServer {
	byEntry := map[string]foundServer{}
	for _, f := range found {
		byEntry[f.entry] = f
	}
	missing := map[string][]string{}
	for _, in := range inputs {
		path, key, ok := strings.Cut(in, ": ")
		if rest, isMCP := strings.CutPrefix(path, "~/.mcp/"); ok && isMCP {
			n := strings.SplitN(rest, "/", 2)[0]
			missing[n] = append(missing[n], key)
		}
	}
	row := func(entry string, yardo bool) (ui.MCPServer, bool) {
		spec, name, err := mcp.Instance(entry)
		if err != nil {
			return ui.MCPServer{}, false
		}
		f, there := byEntry[entry]
		r := ui.MCPServer{Entry: entry, Name: name, Spec: spec, Yardo: yardo, Keys: missing[name]}
		if f.own {
			r.Spec = "" // its own, even named after a catalog one
		} else if s := specs[spec]; s != nil {
			r.About = s.Description
		}
		switch {
		case specs[spec] == nil && !f.own && !yardo:
			return ui.MCPServer{}, false // found, with nothing to run it by
		case specs[spec] == nil && !f.own:
			r.State = "nospec"
		case !yardo && f.registered && f.linked:
			r.State = "byhand"
		case !yardo:
			r.State = "off"
		case len(r.Keys) > 0:
			r.State = "secret"
		case !there:
			r.State = "new" // not set up yet: the next run makes it
		case !checked:
			r.State = "unchecked"
		case f.registered:
			r.State = "ok"
		default:
			r.State = "failed"
		}
		return r, true
	}
	var out []ui.MCPServer
	for _, e := range entries {
		if r, ok := row(e, true); ok {
			out = append(out, r)
		}
	}
	for _, f := range found {
		if !slices.Contains(entries, f.entry) && !slices.ContainsFunc(out, func(r ui.MCPServer) bool { return r.Entry == f.entry }) {
			if r, ok := row(f.entry, false); ok {
				out = append(out, r)
			}
		}
	}
	return out
}

// mcpOutcome is what an MCP server action leaves: apply the MCP servers
// again (edit: the env files to open in the editor after), or stay on the
// menu with toast.
type mcpOutcome struct {
	apply bool
	edit  []string
	toast string
}

// mcpDo does act to an MCP server, changing the answers' servers; on an
// error they're as they were.
func (k *asker) mcpDo(ctx context.Context, act ui.MCPAct, specs map[string]*mcp.Spec) (mcpOutcome, error) {
	spec, name, err := mcp.Instance(act.Entry)
	if err != nil {
		return mcpOutcome{}, err
	}
	before := slices.Clone(k.a.MCPServers)
	adopt := func() {
		if !slices.Contains(k.a.MCPServers, act.Entry) {
			k.a.MCPServers = append(k.a.MCPServers, act.Entry)
		}
	}
	switch act.Do {
	case "add":
		adopt()
		out := mcpOutcome{apply: true}
		if s := specs[spec]; s != nil && len(s.Keys)+len(s.Optional) > 0 {
			out.edit = []string{name} // once the run has made its env
		}
		return out, nil
	case "on":
		adopt()
		return mcpOutcome{apply: true}, nil
	case "edit":
		adopt()
		edited, err := k.editNow(ctx, name)
		if err != nil {
			k.a.MCPServers = before
			return mcpOutcome{}, err
		}
		out := mcpOutcome{apply: true}
		if !edited {
			out.edit = []string{name} // no env yet: the run makes it from the spec
		}
		return out, nil
	case "rename":
		_, to, err := mcp.Instance(act.To)
		if err != nil {
			return mcpOutcome{}, err
		}
		if err := k.mcpSyncAs(ctx, "rename", name, to); err != nil {
			return mcpOutcome{}, fmt.Errorf("couldn't rename %s: %w", name, err)
		}
		i := slices.Index(k.a.MCPServers, act.Entry)
		if i < 0 {
			// Not yardo's: renamed, and as it was (off stays off).
			return mcpOutcome{toast: name + " is " + to + " now."}, nil
		}
		k.a.MCPServers[i] = act.To
		return mcpOutcome{apply: true}, nil
	case "delete":
		if err := k.mcpSyncAs(ctx, "remove", name); err != nil {
			return mcpOutcome{}, fmt.Errorf("couldn't delete %s: %w", name, err)
		}
		k.a.MCPServers = slices.DeleteFunc(k.a.MCPServers, func(e string) bool { return e == act.Entry })
		return mcpOutcome{toast: name + " deleted."}, nil
	}
	return mcpOutcome{}, fmt.Errorf("nothing to do for %q", act.Do)
}

// editNow opens name's env in the editor, if there's one yet (else the
// run makes it from the spec, and it opens then: false).
func (k *asker) editNow(ctx context.Context, name string) (bool, error) {
	if k.edit != nil {
		return k.edit(name)
	}
	env := filepath.Join(homeOf(k.a.User), ".mcp", name, "env")
	if exec.CommandContext(ctx, "runuser", "-u", mcpOwner(k.a.User), "--", "test", "-f", env).Run() != nil {
		return false, nil
	}
	return true, editMCP(ctx, k.a.User, name)
}
