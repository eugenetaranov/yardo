package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
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
	a := *k.a
	if a.MCPPort == 0 && k.port != nil {
		a.MCPPort = k.port()
	}
	c := mcpSyncCmd(ctx, k.share, &a, args...)
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

// retiredMCP are catalog servers their makers retired (yardo ships them
// no more: steps.RetiredSpecs), and what replaces each: one set up before
// says so on the menu, to delete.
var retiredMCP = map[string]string{"aws-api": "aws"}

// mcpRows are the menu's MCP servers: yardo's (entries), then the others
// found in ~/.mcp, each with its state. inputs are the MCP servers' empty
// values ("~/.mcp/<name>/env: KEY"); checked: the gateway's state is known.
// st is mcp-sync status's say, when it could (it wins over the rest).
func mcpRows(entries []string, found []foundServer, inputs []string, specs map[string]*mcp.Spec, checked bool, st map[string]syncState) []ui.MCPServer {
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
		if s, ok := st[name]; ok && yardo {
			switch s.state {
			case "ok":
				r.State = "ok"
			case "waiting":
				r.State, r.Keys = "secret", strings.Fields(s.detail)
			case "failed":
				r.State, r.Detail = "failed", s.detail
			case "changed", "unregistered":
				r.State = "pending"
			case "noenv":
				r.State = "new"
			}
		}
		return r, true
	}
	var out []ui.MCPServer
	for _, e := range entries {
		if r, ok := row(e, true); ok {
			if instead := retiredMCP[entrySpec(r)]; instead != "" {
				r.State, r.Instead = "retired", instead
			}
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

// entrySpec is r's spec, even when it's gone from the catalog.
func entrySpec(r ui.MCPServer) string {
	s, _, _ := mcp.Instance(r.Entry)
	return s
}

// mcpOutcome is what an MCP server action leaves: apply the MCP servers
// again (edit: the env files to open in the editor after), or stay on the
// menu with toast.
type mcpOutcome struct {
	apply bool
	edit  []string
	toast string
	sync  string // register now (mcp-sync), and say how name went
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
		yardo := slices.Contains(k.a.MCPServers, act.Entry)
		adopt()
		edited, err := k.editNow(ctx, name)
		if err != nil {
			k.a.MCPServers = before
			return mcpOutcome{}, err
		}
		if edited && yardo {
			return mcpOutcome{sync: name}, nil // registered now, said on the menu
		}
		out := mcpOutcome{apply: true}
		if !edited {
			out.edit = []string{name} // no env yet: the run makes it from the spec
		}
		return out, nil
	case "retry":
		return mcpOutcome{sync: name}, nil
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

// syncState is a server's state as mcp-sync status says it: ok, waiting
// (detail: the keys), changed, unregistered, failed (detail: why),
// nospec, noenv.
type syncState struct{ state, detail string }

// mcpSyncCmd is this version's mcp-sync (unpacked at start; else the
// installed one) with args, as login's ~/.mcp owner on its gateway.
func mcpSyncCmd(ctx context.Context, share string, a *answers.Answers, args ...string) *exec.Cmd {
	sync := filepath.Join(share, "mcp-sync")
	if _, err := os.Stat(sync); share == "" || err != nil {
		sync = "/usr/local/bin/mcp-sync"
	}
	port := a.MCPPort
	if port == 0 {
		port = answers.FirstMCPPort
	}
	return exec.CommandContext(ctx, "runuser", append([]string{"-u", mcpOwner(a.User), "--", "env",
		"MCP_DIR=" + filepath.Join(homeOf(a.User), ".mcp"), fmt.Sprintf("MCPJUNGLE_REGISTRY=http://127.0.0.1:%d", port),
		"MCP_SYNC_QUIET=1", sync}, args...)...)
}

// mcpStatus is each server's state (mcp-sync status), and whether the
// gateway can't be reached; nil when mcp-sync can't say.
func (o *options) mcpStatus(ctx context.Context, a *answers.Answers) (map[string]syncState, bool) {
	out, err := mcpSyncCmd(ctx, o.share(), a, "status").Output()
	if err != nil {
		return nil, false
	}
	return parseStatus(string(out))
}

func parseStatus(out string) (map[string]syncState, bool) {
	st := map[string]syncState{}
	down := false
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 3)
		if len(f) < 2 {
			continue
		}
		if f[0] == "-" && f[1] == "down" {
			down = true
			continue
		}
		s := syncState{state: f[1]}
		if len(f) == 3 {
			s.detail = f[2]
		}
		st[f[0]] = s
	}
	return st, down
}

// mcpSource is where a server runs from: its image or URL, and for an
// image pulled here, its version (the image's label, else its id) and
// when it was built.
type mcpSource struct {
	from, version, id string
	built             time.Time
	here              bool // the image is pulled here
}

// mcpSources is where each server named runs from (mcp-sync source); nil
// when mcp-sync can't say.
func (o *options) mcpSources(ctx context.Context, a *answers.Answers, names []string) map[string]mcpSource {
	out, err := mcpSyncCmd(ctx, o.share(), a, append([]string{"source"}, names...)...).Output()
	if err != nil {
		return nil
	}
	return parseSources(string(out))
}

func parseSources(out string) map[string]mcpSource {
	src := map[string]mcpSource{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, "\t")
		if len(f) < 2 || f[1] == "" {
			continue
		}
		s := mcpSource{from: f[1]}
		if len(f) == 5 {
			s.here, s.version, s.id = true, f[2], strings.TrimPrefix(f[3], "sha256:")
			if len(s.id) > 12 {
				s.id = s.id[:12]
			}
			s.built, _ = time.Parse(time.RFC3339Nano, f[4])
		}
		src[f[0]] = s
	}
	return src
}

// cached says which of a server's image is pulled here: its version (else
// its id) and when it was built; "" for a URL, "not pulled yet" for an
// image that isn't.
func (s mcpSource) cached() string {
	if !s.here {
		if strings.Contains(s.from, "://") {
			return ""
		}
		return "not pulled yet"
	}
	v := s.version
	if v == "" {
		v = s.id
	}
	if !s.built.IsZero() {
		v += ", built " + s.built.Format("2006-01-02")
	}
	return "cached: " + v
}

// says is a server's source in a line: from where, and what's cached.
func (s mcpSource) says() string {
	if c := s.cached(); c != "" {
		return s.from + " (" + c + ")"
	}
	return s.from
}

// mcpSyncNow registers yardo's servers (those changed or not registered
// yet) now, as the mcp_sync step does; mcp-sync's lines go to w.
func (o *options) mcpSyncNow(ctx context.Context, a *answers.Answers, w io.Writer) error {
	c := mcpSyncCmd(ctx, o.share(), a, mcpNames(a.MCPServers)...)
	c.Stdout, c.Stderr = w, w
	return c.Run()
}

// mcpSays is what a server's state reads as, in a line: what happened
// after it was synced.
func mcpSays(name string, s syncState, ok bool) string {
	switch {
	case !ok:
		return name + ": yardo can't tell how it is (sudo yardo mcp status)."
	case s.state == "ok":
		return "✓ " + name + ": registered; agents see its tools as " + name + "__<tool>."
	case s.state == "waiting":
		return "○ " + name + ": not registered until " + s.detail + " is filled in (sudo yardo mcp edit " + name + ")."
	case s.state == "failed":
		return "✗ " + name + " didn't register: " + s.detail
	case s.state == "nospec":
		return "✗ " + name + ": no longer in the catalog."
	}
	return "✗ " + name + ": not registered yet (sudo yardo mcp sync)."
}

// mcpPending says whether any server would register if synced now: its
// settings changed, or it never registered (one that failed is tried
// again only once its settings change).
func mcpPending(st map[string]syncState) []string {
	var out []string
	for n, s := range st {
		if s.state == "changed" || s.state == "unregistered" {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// mcpCmd runs `yardo mcp edit <name>`, `sync` and `status` (the bare
// `yardo mcp` opens the menu on its MCP servers).
func (o *options) mcpCmd(ctx context.Context, stdout io.Writer, st *answers.Store, args []string) error {
	u, err := o.userFor(st)
	if err != nil {
		return err
	}
	a, _, err := st.Load(u)
	if err != nil {
		return err
	}
	a.User = u
	if os.Geteuid() != 0 {
		return errors.New("run it with sudo: sudo yardo mcp " + strings.Join(args, " "))
	}
	show := func(names []string) {
		s, down := o.mcpStatus(ctx, a)
		if down {
			fmt.Fprintln(stdout, "✗ The MCP gateway isn't answering: sudo yardo, then Fix it.")
		}
		src := o.mcpSources(ctx, a, names)
		for _, n := range names {
			v, ok := s[n]
			fmt.Fprintln(stdout, mcpSays(n, v, ok))
			if f, ok := src[n]; ok {
				fmt.Fprintln(stdout, "    from "+f.says())
			}
		}
	}
	switch args[0] {
	case "edit":
		if len(args) != 2 || !slices.Contains(mcpNames(a.MCPServers), args[1]) {
			return fmt.Errorf("usage: sudo yardo mcp edit <server>; the servers: %s", strings.Join(mcpNames(a.MCPServers), ", "))
		}
		if err := editMCP(ctx, a.User, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Registering %s…\n", args[1])
		_ = o.mcpSyncNow(ctx, a, io.Discard)
		show([]string{args[1]})
	case "sync":
		err := o.mcpSyncNow(ctx, a, stdout)
		show(mcpNames(a.MCPServers))
		if err != nil {
			return errors.New("some MCP servers didn't register (above)")
		}
	case "status":
		show(mcpNames(a.MCPServers))
	default:
		return fmt.Errorf("unknown: yardo mcp %s (yardo mcp, mcp edit <server>, mcp sync, mcp status)", args[0])
	}
	return nil
}
