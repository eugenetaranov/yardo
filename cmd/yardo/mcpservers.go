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

// The server list's rows that aren't servers.
const (
	addInstance = "+instance"
	manageRow   = "+manage"
)

// foundServer is a server in ~/.mcp: entry is how the answers name it
// (jira, opensearch:prod), registered whether the gateway has it now.
type foundServer struct {
	entry      string
	registered bool
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
			found = append(found, foundServer{entry, reg})
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
	if s := strings.TrimSpace(string(out)); s != "" {
		fmt.Fprintln(k.out, s)
	}
	return err
}

// askManage renames or deletes servers: the picked ones and others in
// ~/.mcp (found, dropped). It returns the picked servers as they are
// after, and the names it renamed to (their settings are filled in).
func (k *asker) askManage(ctx context.Context, servers, others []string, specs map[string]*mcp.Spec) ([]string, []string, error) {
	var renamed []string
	for {
		cands := slices.Concat(servers, slices.DeleteFunc(slices.Clone(others), func(e string) bool { return slices.Contains(servers, e) }))
		var names []string
		byName := map[string]string{}
		for _, e := range cands {
			if _, n, err := mcp.Instance(e); err == nil {
				names = append(names, n)
				byName[n] = e
			}
		}
		if len(names) == 0 {
			fmt.Fprintln(k.out, "No MCP servers to rename or delete.")
			return servers, renamed, nil
		}
		slices.Sort(names)
		pick, err := ui.Choose("Rename or delete which server?", names, names[0])
		if err != nil {
			return servers, renamed, err
		}
		entry := byName[pick]
		const ren, del = "Rename it", "Delete it, with its settings (~/.mcp/"
		act, err := ui.Choose(pick, []string{ren, del + pick + ")", "Neither"}, ren)
		if err != nil {
			return servers, renamed, err
		}
		switch {
		case act == ren:
			to, err := k.askRename(entry, names, specs)
			if err != nil {
				return servers, renamed, err
			}
			_, newName, _ := mcp.Instance(to)
			if err := k.mcpSyncAs(ctx, "rename", pick, newName); err != nil {
				fmt.Fprintf(k.out, "Couldn't rename %s: %v\n", pick, err)
				break
			}
			if i := slices.Index(servers, entry); i >= 0 {
				servers[i] = to
			} else {
				servers = append(servers, to) // a found one, picked now
			}
			others = slices.DeleteFunc(others, func(e string) bool { return e == entry })
			renamed = append(renamed, newName)
		case strings.HasPrefix(act, del):
			yes, err := ui.Confirm("Delete "+pick+"?", "It goes from the gateway, and ~/.mcp/"+pick+" (its settings) is deleted: this can't be undone.", false)
			if err != nil {
				return servers, renamed, err
			}
			if !yes {
				break
			}
			if err := k.mcpSyncAs(ctx, "remove", pick); err != nil {
				fmt.Fprintf(k.out, "Couldn't delete %s: %v\n", pick, err)
				break
			}
			servers = slices.DeleteFunc(servers, func(e string) bool { return e == entry })
			others = slices.DeleteFunc(others, func(e string) bool { return e == entry })
		}
		more, err := ui.Confirm("Rename or delete another?", "", false)
		if err != nil || !more {
			return servers, renamed, err
		}
	}
}

// askRename asks entry's new name: another instance's (spec:label) for a
// catalog server, any name for one with its own spec.
func (k *asker) askRename(entry string, taken []string, specs map[string]*mcp.Spec) (string, error) {
	spec, name, _ := mcp.Instance(entry)
	for {
		if specs[spec] == nil {
			to, err := ui.Input("Rename "+name+" to", "Letters, digits, '.', '_' and '-'.", name)
			if err != nil {
				return "", err
			}
			to = strings.TrimSpace(to)
			switch {
			case !mcp.ValidName(to) || strings.Contains(to, ":"):
				fmt.Fprintf(k.out, "%q isn't a server name.\n", to)
			case slices.Contains(taken, to) || specs[to] != nil:
				fmt.Fprintf(k.out, "There's a %s already.\n", to)
			default:
				return to, nil
			}
			continue
		}
		label, err := ui.Input("Rename "+name+" to "+spec+"-<name>", "Lowercase letters, digits and '-', such as prod or work.", "")
		if err != nil {
			return "", err
		}
		to, err := instanceEntry(spec, strings.TrimSpace(label), specs)
		if err == nil && slices.Contains(taken, spec+"-"+label) {
			err = fmt.Errorf("there's a %s-%s already", spec, label)
		}
		if err != nil {
			fmt.Fprintln(k.out, err)
			continue
		}
		return to, nil
	}
}

// instanceEntry is another instance of spec named label (spec:label), or
// why it can't be.
func instanceEntry(spec, label string, specs map[string]*mcp.Spec) (string, error) {
	entry := spec + ":" + label
	_, name, err := mcp.Instance(entry)
	switch {
	case err != nil:
		return "", err
	case label == spec:
		return "", fmt.Errorf("that's %s itself: tick %s in the list instead", spec, spec)
	case specs[name] != nil:
		return "", fmt.Errorf("%s is a server of its own; name it otherwise", name)
	}
	return entry, nil
}
