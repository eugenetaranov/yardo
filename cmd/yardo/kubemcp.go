package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/ui"
	"gopkg.in/yaml.v3"
)

// The Kubernetes MCP server reads a kubeconfig of the user's: mcp-sync
// mounts ~/.kube (and ~/.aws, for EKS's aws eks get-token), and the
// server's env names the file and the context. Adding or editing it picks
// a context from the kubeconfigs under ~/.kube, checks an EKS one's AWS
// profile (an expired SSO login renewed there and then), and writes it.

// kubeContext is a context in a kubeconfig under ~/.kube: its file (from
// ~/.kube), its name, whether it's the file's current one, and the AWS
// profile its token comes from (an EKS cluster's; "" none or the default).
type kubeContext struct {
	file, name string
	current    bool
	awsProfile string
}

// kubeOwnFile is the choice of a kubeconfig the list doesn't have;
// kubeFindEKS, of making kubeconfigs for EKS clusters found.
const (
	kubeOwnFile = "Another kubeconfig: open its settings"
	kubeFindEKS = "Find EKS clusters in your AWS accounts…"
)

// kubeconfig is what's read of one.
type kubeconfig struct {
	Current  string `yaml:"current-context"`
	Contexts []struct {
		Name    string
		Context struct{ User string }
	}
	Users []struct {
		Name string
		User struct {
			Exec *struct {
				Command string
				Args    []string
				Env     []struct{ Name, Value string }
			}
		}
	}
}

// kubeContexts are the contexts in the kubeconfigs under home's ~/.kube
// (not its caches), by file, each file's current one first.
func kubeContexts(home string) []kubeContext {
	root := filepath.Join(home, ".kube")
	var out []kubeContext
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); p != root && (n == "cache" || n == "http-cache" || strings.HasPrefix(n, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if fi, err := d.Info(); err != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		var kc kubeconfig
		if yaml.Unmarshal(b, &kc) != nil || len(kc.Contexts) == 0 {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		var cs []kubeContext
		for _, c := range kc.Contexts {
			if c.Name == "" {
				continue
			}
			kctx := kubeContext{file: filepath.ToSlash(rel), name: c.Name, current: c.Name == kc.Current}
			for _, u := range kc.Users {
				if u.Name == c.Context.User && u.User.Exec != nil {
					kctx.awsProfile = execAWSProfile(u.User.Exec.Command, u.User.Exec.Args, u.User.Exec.Env)
				}
			}
			cs = append(cs, kctx)
		}
		slices.SortStableFunc(cs, func(a, b kubeContext) int {
			switch {
			case a.current && !b.current:
				return -1
			case b.current && !a.current:
				return 1
			}
			return 0
		})
		out = append(out, cs...)
		return nil
	})
	return out
}

// execAWSProfile is the AWS profile an exec credential plugin (aws eks
// get-token, aws-iam-authenticator) signs with: --profile's, else
// AWS_PROFILE's; "" for another plugin, or the default profile.
func execAWSProfile(command string, args []string, env []struct{ Name, Value string }) string {
	switch path.Base(command) {
	case "aws", "aws-iam-authenticator":
	default:
		return ""
	}
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, "--profile="); ok {
			return v
		}
		if a == "--profile" && i+1 < len(args) {
			return args[i+1]
		}
	}
	for _, e := range env {
		if e.Name == "AWS_PROFILE" {
			return e.Value
		}
	}
	return ""
}

// short is a context's name in a line: an EKS ARN's cluster, with its
// region.
func (c kubeContext) short() string {
	if region, n, ok := eksARN(c.name); ok {
		return fmt.Sprintf("%s (EKS, %s)", n, region)
	}
	return c.name
}

func (c kubeContext) label() string {
	l := fmt.Sprintf("%s  ·  ~/.kube/%s", c.short(), c.file)
	if c.awsProfile != "" {
		l += "  ·  AWS profile " + c.awsProfile
	}
	return l
}

// kubeChoice is the context the Kubernetes server uses, and writes: its
// write tools shown (apply, scale, delete; else read-only tools only).
type kubeChoice struct {
	file, context string
	writes        bool
}

// env are the choice's lines in the server's env.
func (c kubeChoice) env() [][2]string {
	ro := "true"
	if c.writes {
		ro = "false"
	}
	return [][2]string{{"KUBECONFIG", "~/.kube/" + c.file}, {"K8S_CONTEXT", c.context}, {"ALLOW_ONLY_READONLY_TOOLS", ro}}
}

// says is the choice, in a line.
func (c kubeChoice) says(name string) string {
	what := "read-only tools only"
	if c.writes {
		what = "reads and writes (apply, scale, delete), as its credentials allow"
	}
	return fmt.Sprintf("%s uses the context %s in ~/.kube/%s: %s. (Later: sudo yardo mcp edit %s)", name, c.context, c.file, what, name)
}

// askKube picks the context the Kubernetes server uses, from the
// kubeconfigs under ~/.kube, and whether the agent may change things; ok
// is false for another kubeconfig (or none to pick from): its settings
// open instead.
func (k *asker) askKube(ctx context.Context, name string) (kubeChoice, bool, error) {
	if k.kubePick != nil {
		return k.kubePick(name)
	}
	home := homeOf(k.a.User)
	cs := kubeContexts(home)
	canFind := len(awsProfiles(home)) > 0
	if len(cs) == 0 && canFind {
		fmt.Fprintf(k.out, "%s: no kubeconfigs under ~/.kube yet.\n", name)
		if err := k.findEKS(ctx, home); err != nil {
			return kubeChoice{}, false, err
		}
		cs = kubeContexts(home)
	}
	if len(cs) == 0 {
		fmt.Fprintf(k.out, "%s: no kubeconfigs under ~/.kube (aws eks update-kubeconfig makes one for EKS); its settings open instead.\n", name)
		return kubeChoice{}, false, nil
	}
	var c kubeContext
	for {
		opts := make([]string, 0, len(cs)+2)
		for _, c := range cs {
			opts = append(opts, c.label())
		}
		if canFind {
			opts = append(opts, kubeFindEKS)
		}
		opts = append(opts, kubeOwnFile)
		pick, err := ui.Choose(name+": the Kubernetes context it uses (from ~/.kube)", opts, opts[0])
		if err != nil || pick == kubeOwnFile {
			return kubeChoice{}, false, err
		}
		if pick == kubeFindEKS {
			if err := k.findEKS(ctx, home); err != nil {
				return kubeChoice{}, false, err
			}
			cs = kubeContexts(home)
			continue
		}
		c = cs[slices.Index(opts, pick)]
		break
	}
	if c.awsProfile != "" {
		p := awsProfile{name: c.awsProfile}
		for _, q := range awsProfiles(home) {
			if q.name == c.awsProfile {
				p = q
			}
		}
		if _, err := k.awsSignIn(ctx, p); err != nil {
			return kubeChoice{}, false, err
		}
	}
	writes, err := ui.Confirm("Let the agent change things in this cluster?",
		"Yes: apply, scale and delete too, as the kubeconfig's credentials allow. No: read-only tools only.", false)
	if err != nil {
		return kubeChoice{}, false, err
	}
	return kubeChoice{file: c.file, context: c.name, writes: writes}, true, nil
}
