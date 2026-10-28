package mcp

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestLoadShippedSpecs(t *testing.T) {
	dirs := []string{"../assets/mcp-catalog", "../assets/conf/mcp-catalog"}
	names := Names(dirs)
	for _, want := range []string{"github", "jira", "linear", "context7"} {
		found := false
		for _, n := range names {
			found = found || n == want
		}
		if !found {
			t.Errorf("no %s in %v", want, names)
		}
	}
	jira, err := Load(context.Background(), dirs, "jira")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jira.Keys, []string{"JIRA_URL", "JIRA_USERNAME", "JIRA_API_TOKEN"}) || len(jira.Optional) == 0 || jira.Description == "" {
		t.Errorf("jira = %+v", jira)
	}
	if !IsSecret("JIRA_API_TOKEN") || IsSecret("JIRA_URL") || !IsSecret("LINEAR_API_KEY") {
		t.Error("IsSecret")
	}
}

// GitHub's token comes with how to make it, and the host (always
// github.com but for Enterprise) isn't an optional setting to pick.
func TestGitHubSpecHelp(t *testing.T) {
	gh, err := Load(context.Background(), []string{"../assets/conf/mcp-catalog"}, "github")
	if err != nil {
		t.Fatal(err)
	}
	help := gh.Help["GITHUB_PERSONAL_ACCESS_TOKEN"]
	for _, want := range []string{"fine-grained", "personal-access-tokens/new", "Contents: Read and write", "Pull requests: Read and write", "classic"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q:\n%s", want, help)
		}
	}
	if slices.Contains(gh.Optional, "GITHUB_HOST") || !slices.Contains(gh.Optional, "GITHUB_TOOLSETS") {
		t.Errorf("optional = %v", gh.Optional)
	}
	if !reflect.DeepEqual(gh.Keys, []string{"GITHUB_PERSONAL_ACCESS_TOKEN"}) {
		t.Errorf("keys = %v", gh.Keys)
	}
}

func TestInstance(t *testing.T) {
	for entry, want := range map[string][2]string{
		"grafana": {"grafana", "grafana"}, "grafana:prod": {"grafana", "grafana-prod"},
		"aws-api:eu-1": {"aws-api", "aws-api-eu-1"}, "opensearch:logs": {"opensearch", "opensearch-logs"},
	} {
		spec, name, err := Instance(entry)
		if err != nil || spec != want[0] || name != want[1] {
			t.Errorf("Instance(%q) = %q, %q, %v", entry, spec, name, err)
		}
	}
	for _, bad := range []string{"", ":prod", "grafana:", "grafana:Prod", "grafana:a b", "grafana:-x", "grafana:a--b", "../x", "grafana:../x", "a__b"} {
		if _, _, err := Instance(bad); err == nil {
			t.Errorf("Instance(%q): want an error", bad)
		}
	}
}

func TestEntryFor(t *testing.T) {
	isSpec := func(n string) bool { return n == "jira" || n == "aws" || n == "aws-api" }
	for _, c := range []struct {
		spec, typed string
		taken       []string
		want        string
	}{
		{"jira", "jira", nil, "jira"},
		{"jira", " work ", []string{"jira"}, "jira:work"},
		{"jira", "jira-work", []string{"jira"}, "jira:work"},
	} {
		if got, err := EntryFor(c.spec, c.typed, isSpec, c.taken); err != nil || got != c.want {
			t.Errorf("EntryFor(%q, %q) = %q, %v; want %q", c.spec, c.typed, got, err, c.want)
		}
	}
	for _, c := range []struct {
		spec, typed string
		taken       []string
		err         string
	}{
		{"jira", "", nil, "give it a name"},
		{"jira", "jira", []string{"jira"}, "there's a jira already"},
		{"jira", "jira-jira", nil, "jira itself"},
		{"jira", "Work", nil, "lowercase"},
		{"aws", "api", nil, "aws-api is a server of its own"},
		{"jira", "work", []string{"jira-work"}, "there's a jira-work already"},
	} {
		if _, err := EntryFor(c.spec, c.typed, isSpec, c.taken); err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("EntryFor(%q, %q): err = %v, want %q", c.spec, c.typed, err, c.err)
		}
	}
}

func TestRenameEntry(t *testing.T) {
	isSpec := func(n string) bool { return n == "jira" }
	taken := []string{"jira", "jira-work", "mine"}
	if got, err := RenameEntry("jira:work", "prod", isSpec, taken); err != nil || got != "jira:prod" {
		t.Errorf("jira-work → prod = %q, %v", got, err)
	}
	if got, err := RenameEntry("jira:work", "jira", isSpec, []string{"jira-work"}); err != nil || got != "jira" {
		t.Errorf("jira-work → jira = %q, %v", got, err)
	}
	if got, err := RenameEntry("mine", "Ours.v2", isSpec, taken); err != nil || got != "Ours.v2" {
		t.Errorf("own spec → Ours.v2 = %q, %v", got, err)
	}
	for _, c := range [][2]string{{"jira:work", "jira-work"}, {"jira:work", "jira"}, {"mine", "jira"}, {"mine", "a:b"}, {"mine", ""}} {
		if _, err := RenameEntry(c[0], c[1], isSpec, taken); err == nil {
			t.Errorf("RenameEntry(%q, %q): want an error", c[0], c[1])
		}
	}
}
