package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/ui"
)

// Finding EKS clusters for the Kubernetes MCP server's picker: in each AWS
// CLI profile picked, its own region, aws eks list-clusters; for each
// cluster picked, aws eks update-kubeconfig (as the user: their files),
// a kubeconfig of its own whose token comes from that profile.

// eksCluster is an EKS cluster found: its name and region, and the
// profile it was found with.
type eksCluster struct{ name, region, profile string }

func (c eksCluster) label() string {
	return c.name + "  ·  " + c.region + "  ·  profile " + c.profile
}

// eksARN is an EKS context's region and cluster
// (arn:aws:eks:<region>:<account>:cluster/<name>); ok false for another.
func eksARN(context string) (region, name string, ok bool) {
	f := strings.Split(context, ":")
	if len(f) != 6 || f[2] != "eks" {
		return "", "", false
	}
	name, ok = strings.CutPrefix(f[5], "cluster/")
	return f[3], name, ok
}

// eksKnown says whether cs has a context for cluster c.
func eksKnown(cs []kubeContext, c eksCluster) bool {
	return slices.ContainsFunc(cs, func(x kubeContext) bool {
		r, n, ok := eksARN(x.name)
		return ok && r == c.region && n == c.name
	})
}

// eksKubeDir is where the kubeconfigs made go: ~/.kube/config/ when
// that's a folder (a file a cluster), else ~/.kube/eks/.
func eksKubeDir(home string) string {
	if fi, err := os.Stat(filepath.Join(home, ".kube", "config")); err == nil && fi.IsDir() {
		return filepath.Join(home, ".kube", "config")
	}
	return filepath.Join(home, ".kube", "eks")
}

// eksKubeFile is cluster c's kubeconfig in dir: its name, else (taken)
// with its region, then its profile too.
func eksKubeFile(dir string, c eksCluster) string {
	for _, n := range []string{c.name, c.name + "-" + c.region, c.name + "-" + c.region + "-" + c.profile} {
		if _, err := os.Lstat(filepath.Join(dir, n)); err != nil {
			return filepath.Join(dir, n)
		}
	}
	return filepath.Join(dir, c.name+"-"+c.region+"-"+c.profile)
}

// parseClusters are the names in aws eks list-clusters' JSON.
func parseClusters(b []byte) ([]string, error) {
	var v struct{ Clusters []string }
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return v.Clusters, nil
}

// lastLine is a command's output's last line: what went wrong.
func lastLine(b []byte) string {
	l := strings.Split(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(l[len(l)-1])
}

// findEKS finds the EKS clusters in the AWS CLI profiles picked (each in
// its own region), and makes a kubeconfig for each cluster picked of
// those that have none yet.
func (k *asker) findEKS(ctx context.Context, home string) error {
	ps := awsProfiles(home)
	opts := make([]string, len(ps))
	for i, p := range ps {
		opts[i] = p.label()
	}
	picked, err := ui.ChooseMany("The AWS CLI profiles to look in (each in its own region)", opts, nil)
	if err != nil {
		return err
	}
	cs := kubeContexts(home)
	var found, known []eksCluster
	for _, p := range ps {
		if !slices.Contains(picked, p.label()) {
			continue
		}
		if p.region == "" {
			fmt.Fprintf(k.out, "%s: no region in ~/.aws/config, skipped (aws configure set region <it> --profile %s).\n", p.name, p.name)
			continue
		}
		who, err := k.awsSignIn(ctx, p)
		if err != nil {
			return err
		}
		if who == "" {
			continue
		}
		fmt.Fprintf(k.out, "Looking in %s (%s)…\n", p.name, p.region)
		out, err := asUser(ctx, k.a.User, "aws", "eks", "list-clusters", "--profile", p.name, "--region", p.region, "--output", "json").CombinedOutput()
		if err != nil {
			fmt.Fprintf(k.out, "%s: couldn't list its EKS clusters: %s\n", p.name, lastLine(out))
			continue
		}
		names, err := parseClusters(out)
		if err != nil {
			fmt.Fprintf(k.out, "%s: aws eks list-clusters said something else: %v\n", p.name, err)
			continue
		}
		for _, n := range names {
			c := eksCluster{n, p.region, p.name}
			if slices.ContainsFunc(append(slices.Clone(found), known...), func(x eksCluster) bool { return x.name == n && x.region == p.region }) {
				continue // the same cluster, through another profile
			}
			if eksKnown(cs, c) {
				known = append(known, c)
			} else {
				found = append(found, c)
			}
		}
	}
	for _, c := range known {
		fmt.Fprintf(k.out, "%s (%s): already in ~/.kube\n", c.name, c.region)
	}
	if len(found) == 0 {
		fmt.Fprintln(k.out, "No EKS clusters to add.")
		return nil
	}
	opts = opts[:0]
	for _, c := range found {
		opts = append(opts, c.label())
	}
	picked, err = ui.ChooseMany("The EKS clusters to make a kubeconfig for", opts, opts)
	if err != nil {
		return err
	}
	dir := eksKubeDir(home)
	if len(picked) > 0 {
		if out, err := asUser(ctx, k.a.User, "mkdir", "-p", "-m", "700", dir).CombinedOutput(); err != nil {
			return fmt.Errorf("making %s: %w: %s", dir, err, out)
		}
	}
	kubectl := asUser(ctx, k.a.User, "sh", "-c", "command -v kubectl").Run() == nil
	for _, c := range found {
		if !slices.Contains(picked, c.label()) {
			continue
		}
		f := eksKubeFile(dir, c)
		shown := "~/" + strings.TrimPrefix(f, home+"/")
		out, err := asUser(ctx, k.a.User, "aws", "eks", "update-kubeconfig", "--name", c.name, "--region", c.region, "--profile", c.profile, "--kubeconfig", f).CombinedOutput()
		if err != nil {
			fmt.Fprintf(k.out, "%s: couldn't make its kubeconfig: %s\n", c.name, lastLine(out))
			continue
		}
		fmt.Fprintf(k.out, "%s: %s\n", c.name, shown)
		if !kubectl {
			continue
		}
		// Reaching it: an EKS cluster also needs the role let in (an
		// access entry, or aws-auth).
		if out, err := asUser(ctx, k.a.User, "kubectl", "--kubeconfig", f, "get", "namespaces", "-o", "name", "--request-timeout=15s").CombinedOutput(); err != nil {
			fmt.Fprintf(k.out, "  but it can't reach it yet: %s (its role needs an access entry in the cluster)\n", lastLine(out))
		} else {
			fmt.Fprintln(k.out, "  reached: it lists the namespaces")
		}
	}
	return nil
}
