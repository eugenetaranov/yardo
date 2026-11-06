package steps

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// Kept is what removing an item leaves in place: the packages, apt repos
// and mise tools something still set up uses (another item, the
// baseline). See KeptBy.
type Kept struct{ Packages, Repos, Tools map[string]bool }

// basePackages are never removed with an item: the machine's own, which
// some items list only to be sure they're there.
var basePackages = []string{"ca-certificates", "curl", "wget", "gnupg", "git", "jq", "tar", "acl", "unzip", "xz-utils",
	"sudo", "openssh-server", "openssh-client", "apt-transport-https", "lsb-release", "software-properties-common", "python3"}

// KeptBy is what items (the ones staying) use, and the base packages.
func KeptBy(e *Env, items []*catalog.Item) Kept {
	k := Kept{Packages: map[string]bool{}, Repos: map[string]bool{}, Tools: map[string]bool{}}
	for _, p := range basePackages {
		k.Packages[p] = true
	}
	for _, it := range items {
		for _, s := range it.Steps {
			switch s.Type {
			case "apt":
				for _, p := range (aptStep{s}).Packages(e) {
					k.Packages[p] = true
				}
			case "apt_repo":
				k.Repos[s.Name] = true
			case "mise":
				for _, t := range s.Tools {
					k.Tools[MiseName(t)] = true
				}
			}
		}
	}
	return k
}

// Remove uninstalls it: its Remove script first, then its packages, apt
// repos and mise tools, but the ones in keep. It returns what it took
// off, for the log.
func Remove(ctx context.Context, e *Env, it *catalog.Item, keep Kept) ([]string, error) {
	if !it.Removable() {
		return nil, fmt.Errorf("%s can't be removed by yardo", it.ID)
	}
	var did []string
	if it.Remove != "" {
		script := strings.NewReplacer("{{user}}", e.User, "{{group}}", e.Group, "{{home}}", e.Home, "{{arch}}", e.Arch).Replace(it.Remove)
		if _, err := e.run(ctx, Cmd{Name: "bash", Args: []string{"-c", script}, Env: AptEnv}); err != nil {
			return did, fmt.Errorf("removing %s: %w", it.ID, err)
		}
		did = append(did, "its own removal")
	}
	var pkgs, tools []string
	for _, s := range it.Steps {
		switch s.Type {
		case "apt":
			for _, p := range (aptStep{s}).Packages(e) {
				if !keep.Packages[p] && !slices.Contains(pkgs, p) {
					pkgs = append(pkgs, p)
				}
			}
		case "mise":
			for _, t := range s.Tools {
				if n := MiseName(t); !keep.Tools[n] && !slices.Contains(tools, n) {
					tools = append(tools, n)
				}
			}
		}
	}
	if len(pkgs) > 0 {
		// Only the ones installed: purging one that isn't fails.
		var there []string
		for _, p := range pkgs {
			if _, err := e.Exec.Run(ctx, Cmd{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}", p}}); err == nil {
				there = append(there, p)
			}
		}
		if len(there) > 0 {
			if _, err := e.run(ctx, Cmd{Name: "apt-get", Args: append([]string{"purge", "-y", "-q"}, there...), Env: AptEnv}); err != nil {
				return did, fmt.Errorf("removing %s's packages: %w", it.ID, err)
			}
			did = append(did, there...)
		}
	}
	for _, s := range it.Steps {
		if s.Type == "apt_repo" && !keep.Repos[s.Name] {
			r := aptRepoStep{s}
			for _, f := range []string{r.listPath(), r.keyPath()} {
				if err := os.Remove(e.Path(f)); err == nil {
					did = append(did, f)
				}
			}
		}
	}
	for _, t := range tools {
		// mise unuse takes it off the global config and uninstalls it; an
		// older mise has use --remove instead.
		if _, err := e.run(ctx, Cmd{Name: "mise", Args: []string{"unuse", "-g", t}, AsUser: true}); err != nil {
			if _, err := e.run(ctx, Cmd{Name: "mise", Args: []string{"use", "-g", "--remove", t}, AsUser: true}); err != nil {
				return did, fmt.Errorf("removing %s's %s (mise): %w", it.ID, t, err)
			}
		}
		did = append(did, t)
	}
	return did, nil
}
