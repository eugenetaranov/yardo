package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/mcp"
)

// What's in ~/.mcp, as the answers name it: catalog servers, other
// instances (linked, or dropped and unlinked), servers of their own.
func TestParseFound(t *testing.T) {
	specs := map[string]*mcp.Spec{"jira": {}, "opensearch": {}, "aws-api": {}, "aws": {}}
	out := strings.Join([]string{
		"jira\t\t0",
		"opensearch-prod\t/usr/local/share/mcp-catalog/opensearch.spec\t1",
		"jira-work\t\t0",             // dropped: its link went
		"aws-api\t\t1",               // a catalog name, not aws's api instance
		"aws-api-eu\t\t0",            // the longest spec: aws-api's eu
		"mine\town\t1",               // its own spec
		"jira\t/x/jira.spec\t1",      // renamed back from an instance
		"stray\t\t0",                 // no spec: not a server
		"bad name\t\t0", "../x\t\t0", // not names
		"",
	}, "\n")
	want := []foundServer{{"jira", false}, {"opensearch:prod", true}, {"jira:work", false}, {"aws-api", true},
		{"aws-api:eu", false}, {"mine", true}, {"jira", true}}
	if got := parseFound(out, specs); !reflect.DeepEqual(got, want) {
		t.Errorf("parseFound =\n%v\nwant\n%v", got, want)
	}
}

// Another instance's name: not its spec's own (jira:jira), nor another
// catalog server's.
func TestInstanceEntry(t *testing.T) {
	specs := map[string]*mcp.Spec{"jira": {}, "aws": {}, "aws-api": {}}
	if e, err := instanceEntry("jira", "work", specs); err != nil || e != "jira:work" {
		t.Errorf("jira work = %q, %v", e, err)
	}
	for _, c := range [][2]string{{"jira", "jira"}, {"aws", "api"}, {"jira", "Work"}, {"jira", ""}} {
		if _, err := instanceEntry(c[0], c[1], specs); err == nil {
			t.Errorf("instanceEntry(%q, %q): want an error", c[0], c[1])
		}
	}
	if _, err := instanceEntry("jira", "jira", specs); err == nil || !strings.Contains(err.Error(), "tick jira") {
		t.Errorf("jira:jira: err = %v", err)
	}
}
