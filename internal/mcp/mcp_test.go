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
