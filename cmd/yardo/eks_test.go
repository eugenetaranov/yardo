package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A cluster found is known when a context's EKS ARN names it (its region
// and name); list-clusters' JSON is read for the names.
func TestEKSKnown(t *testing.T) {
	cs := []kubeContext{{name: "arn:aws:eks:us-east-1:111111111111:cluster/dev1"}, {name: "kind-a"}}
	for c, want := range map[eksCluster]bool{
		{"dev1", "us-east-1", "dev"}: true,
		{"dev1", "eu-west-1", "dev"}: false,
		{"prod", "us-east-1", "dev"}: false,
	} {
		if eksKnown(cs, c) != want {
			t.Errorf("%+v: known %v", c, !want)
		}
	}
	if _, _, ok := eksARN("arn:aws:iam::1:role/x"); ok {
		t.Error("an IAM ARN read as EKS")
	}
	names, err := parseClusters([]byte(`{"clusters": ["dev1", "prod"]}`))
	if err != nil || len(names) != 2 || names[1] != "prod" {
		t.Errorf("clusters %v %v", names, err)
	}
}

// The kubeconfigs made go in ~/.kube/config/ when it's a folder, else
// ~/.kube/eks/; a file taken gets the cluster's region, then its profile.
func TestEKSKubeFile(t *testing.T) {
	home := t.TempDir()
	if d := eksKubeDir(home); d != filepath.Join(home, ".kube", "eks") {
		t.Errorf("no ~/.kube: %s", d)
	}
	dir := filepath.Join(home, ".kube", "config")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if d := eksKubeDir(home); d != dir {
		t.Errorf("~/.kube/config a folder: %s", d)
	}
	c := eksCluster{"dev1", "us-east-1", "dev"}
	for _, want := range []string{"dev1", "dev1-us-east-1", "dev1-us-east-1-dev"} {
		f := eksKubeFile(dir, c)
		if f != filepath.Join(dir, want) {
			t.Fatalf("file %s, want %s", f, want)
		}
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
