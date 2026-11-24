package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Orphaned MCP containers: the gateway runs each Docker server as
// `docker run -i --rm -v ~/.mcp/<name>:/mcp:ro …`, one a session. When it
// drops a session (it restarts, a server re-registers) the docker client
// goes but its container keeps running. A container is the gateway's
// while a `docker run` of its server started when it was made; one no
// such client accounts for is orphaned.

// mcpContainer is a running container of an MCP server: its id, the
// server's name, and when it was made.
type mcpContainer struct {
	id, name string
	created  time.Time
}

// mcpClient is a running `docker run` of an MCP server, and when it
// started.
type mcpClient struct {
	name    string
	started time.Time
}

const (
	// orphanGrace: a container younger is never orphaned (its client may
	// just be starting).
	orphanGrace = 2 * time.Minute
	// A client's container is made from a little before it started (the
	// clock's seconds) to a while after (an image pulled first).
	madeBefore, madeAfter = 5 * time.Second, 5 * time.Minute
)

// orphaned are the containers cs no client in ps accounts for, oldest
// first. Each client takes the earliest container made in its window.
func orphaned(cs []mcpContainer, ps []mcpClient, now time.Time) []mcpContainer {
	cs = slices.Clone(cs)
	slices.SortFunc(cs, func(a, b mcpContainer) int { return a.created.Compare(b.created) })
	ps = slices.Clone(ps)
	slices.SortFunc(ps, func(a, b mcpClient) int { return a.started.Compare(b.started) })
	taken := make([]bool, len(cs))
	for _, p := range ps {
		for n, c := range cs {
			if !taken[n] && c.name == p.name && !c.created.Before(p.started.Add(-madeBefore)) && !c.created.After(p.started.Add(madeAfter)) {
				taken[n] = true
				break
			}
		}
	}
	var out []mcpContainer
	for n, c := range cs {
		if !taken[n] && now.Sub(c.created) >= orphanGrace {
			out = append(out, c)
		}
	}
	return out
}

// parseMCPContainers reads docker inspect's lines (id, created, then the
// /mcp mount's source): the containers of servers in dir.
func parseMCPContainers(out, dir string) []mcpContainer {
	var cs []mcpContainer
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) != 3 {
			continue
		}
		name, ok := mcpDirName(f[2], dir)
		t, err := time.Parse(time.RFC3339Nano, f[1])
		if ok && err == nil {
			cs = append(cs, mcpContainer{id: f[0], name: name, created: t})
		}
	}
	return cs
}

// parseMCPClients reads ps' lines (seconds since it started, then the
// command): the `docker run`s of servers in dir.
func parseMCPClients(out, dir string, now time.Time) []mcpClient {
	var ps []mcpClient
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 3 || filepath.Base(f[1]) != "docker" || f[2] != "run" {
			continue
		}
		secs, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		for _, a := range f[3:] {
			src, ok := strings.CutSuffix(a, ":/mcp:ro")
			if !ok {
				continue
			}
			if name, ok := mcpDirName(src, dir); ok {
				ps = append(ps, mcpClient{name: name, started: now.Add(-time.Duration(secs) * time.Second)})
			}
			break
		}
	}
	return ps
}

// mcpDirName is the server whose directory src is: one right in dir.
func mcpDirName(src, dir string) (string, bool) {
	name, ok := strings.CutPrefix(src, dir+"/")
	return name, ok && name != "" && !strings.Contains(name, "/")
}

// mcpOrphans are user's orphaned MCP containers; none when docker or ps
// can't say (never a guess).
func mcpOrphans(ctx context.Context, user string) []mcpContainer {
	dir := filepath.Join(homeOf(user), ".mcp")
	ids, ok := quick(ctx, "docker", "ps", "-q", "--no-trunc")
	if !ok || strings.TrimSpace(ids) == "" {
		return nil
	}
	out, ok := quick(ctx, "docker", append([]string{"inspect", "--format",
		`{{.Id}} {{.Created}}{{range .Mounts}}{{if eq .Destination "/mcp"}} {{.Source}}{{end}}{{end}}`}, strings.Fields(ids)...)...)
	if !ok {
		return nil
	}
	cs := parseMCPContainers(out, dir)
	if len(cs) == 0 {
		return nil
	}
	now := time.Now()
	ps, ok := quick(ctx, "ps", "-eo", "etimes=,args=")
	if !ok {
		return nil
	}
	return orphaned(cs, parseMCPClients(ps, dir, now), now)
}

// orphanNames are the servers cs are of, a name for each container.
func orphanNames(cs []mcpContainer) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.name)
	}
	return out
}

// cleanOrphans removes user's orphaned MCP containers, found again now:
// how many it removed.
func cleanOrphans(ctx context.Context, user string) (int, error) {
	cs := mcpOrphans(ctx, user)
	if len(cs) == 0 {
		return 0, nil
	}
	args := []string{"rm", "-f"}
	for _, c := range cs {
		args = append(args, c.id)
	}
	if out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("docker rm: %s", strings.TrimSpace(string(out)))
	}
	return len(cs), nil
}

// orphansSay is what cleaning up n orphaned containers did.
func orphansSay(n int) string {
	if n == 0 {
		return "✓ No orphaned MCP containers: nothing to clean up."
	}
	return "✓ " + orphanCount(n) + " removed."
}

// cleanOrphans removes the orphaned MCP containers of k's user.
func (k *asker) cleanOrphans(ctx context.Context) (int, error) {
	if k.cleanup != nil {
		return k.cleanup()
	}
	return cleanOrphans(ctx, k.a.User)
}

// orphanCount is n orphaned containers, in words.
func orphanCount(n int) string {
	if n == 1 {
		return "1 orphaned MCP container"
	}
	return fmt.Sprintf("%d orphaned MCP containers", n)
}
