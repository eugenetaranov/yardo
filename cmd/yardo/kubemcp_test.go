package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/mcp"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// The contexts in the kubeconfigs under ~/.kube, a file or a folder of
// them: each file's current one first, an EKS one's AWS profile (from
// AWS_PROFILE or --profile); the caches and files that aren't
// kubeconfigs left out.
func TestKubeContexts(t *testing.T) {
	home := t.TempDir()
	eks := `apiVersion: v1
kind: Config
current-context: arn:aws:eks:us-east-1:111111111111:cluster/dev1-us-east-1
contexts:
- name: arn:aws:eks:us-east-1:111111111111:cluster/dev1-us-east-1
  context:
    cluster: dev1
    user: dev1
users:
- name: dev1
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws
      args: [--region, us-east-1, eks, get-token, --cluster-name, dev1-us-east-1, --output, json]
      env:
      - name: AWS_PROFILE
        value: dev
`
	plain := `current-context: kind-b
contexts:
- name: kind-a
  context: {user: a}
- name: kind-b
  context: {user: b}
users:
- name: a
  user:
    exec: {command: /usr/local/bin/aws, args: [eks, get-token, --profile, ops]}
- name: b
  user: {token: x}
`
	for f, b := range map[string]string{
		"config/dev1-us-east-1": eks,
		"other":                 plain,
		"cache/discovery/x":     plain,
		"kubectx":               "kind-a\n",
	} {
		p := filepath.Join(home, ".kube", f)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, c := range kubeContexts(home) {
		got = append(got, c.label())
	}
	want := []string{
		"dev1-us-east-1 (EKS, us-east-1)  ·  ~/.kube/config/dev1-us-east-1  ·  AWS profile dev",
		"kind-b  ·  ~/.kube/other",
		"kind-a  ·  ~/.kube/other  ·  AWS profile ops",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if cs := kubeContexts(t.TempDir()); len(cs) != 0 {
		t.Errorf("no ~/.kube: %v", cs)
	}
}

// Editing a Kubernetes server picks a context: written to its env (the
// file under ~/.kube, the context, read-only or not), and registered now;
// another kubeconfig opens the file instead.
func TestMCPDoKube(t *testing.T) {
	specs := map[string]*mcp.Spec{"kubernetes": {Keys: []string{"KUBECONFIG"}}}
	var set [][2]string
	var edited []string
	pick := true
	k := &asker{a: &answers.Answers{User: "ada", MCPServers: []string{"kubernetes:k8s-dev1"}},
		edit:     func(n string) (bool, error) { edited = append(edited, n); return true, nil },
		kubePick: func(string) (kubeChoice, bool, error) { return kubeChoice{"config/dev1", "arn:x", false}, pick, nil },
		mcpEnv:   func(n string, kv [][2]string) error { set = append(set, kv...); return nil }}
	out, err := k.mcpDo(context.Background(), ui.MCPAct{Do: "edit", Entry: "kubernetes:k8s-dev1"}, specs)
	if err != nil || out.sync != "kubernetes-k8s-dev1" || len(edited) != 0 {
		t.Errorf("picked: %+v %v, edited %v", out, err, edited)
	}
	if len(set) != 3 || set[0] != [2]string{"KUBECONFIG", "~/.kube/config/dev1"} || set[1] != [2]string{"K8S_CONTEXT", "arn:x"} || set[2] != [2]string{"ALLOW_ONLY_READONLY_TOOLS", "true"} {
		t.Errorf("set %v", set)
	}
	pick, set = false, nil
	if out, err := k.mcpDo(context.Background(), ui.MCPAct{Do: "edit", Entry: "kubernetes:k8s-dev1"}, specs); err != nil || out.sync != "kubernetes-k8s-dev1" || len(edited) != 1 || set != nil {
		t.Errorf("another kubeconfig: %+v %v, edited %v, set %v", out, err, edited, set)
	}
	c := kubeChoice{"config/dev1", "arn:x", true}
	if !strings.Contains(c.says("k"), "apply, scale, delete") || c.env()[2] != [2]string{"ALLOW_ONLY_READONLY_TOOLS", "false"} {
		t.Errorf("writes: %q %v", c.says("k"), c.env())
	}
}
