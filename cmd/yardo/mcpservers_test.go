package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/mcp"
	"github.com/eugenetaranov/yardo/internal/ui"
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
	want := []foundServer{{"jira", false, false, false}, {"opensearch:prod", true, false, true}, {"jira:work", false, false, false},
		{"aws-api", true, false, false}, {"aws-api:eu", false, false, false}, {"mine", true, true, true}, {"jira", true, false, true}}
	if got := parseFound(out, specs); !reflect.DeepEqual(got, want) {
		t.Errorf("parseFound =\n%v\nwant\n%v", got, want)
	}
}

// The menu's rows: yardo's servers with their state, then the ones found
// in ~/.mcp (once each), off or on by hand.
func TestMCPRows(t *testing.T) {
	specs := map[string]*mcp.Spec{"github": {Description: "GitHub"}, "jira": {Description: "Jira"}, "opensearch": {}}
	entries := []string{"github", "jira", "jira:work", "opensearch", "gone", "docker"}
	found := []foundServer{{"github", true, false, false}, {"jira:work", true, false, true}, {"jira", false, false, false},
		{"opensearch", false, false, false}, {"grafana:test", false, false, false}, {"opensearch:old", false, false, false},
		{"mine", true, true, true}, {"docker2", true, false, false}}
	specs["docker"], specs["docker2"] = &mcp.Spec{}, &mcp.Spec{}
	inputs := []string{"~/.mcp/jira/env: JIRA_URL", "~/.mcp/jira/env: JIRA_API_TOKEN", "~/.config/yardo/agent.env: OPENAI_API_KEY"}
	got := mcpRows(entries, found, inputs, specs, true, nil)
	// docker isn't there yet (new); docker2 is registered by hand with no
	// spec of its own, which the next run would drop: off.
	want := map[string]string{"github": "ok", "jira": "secret", "jira-work": "ok", "opensearch": "failed", "gone": "nospec",
		"docker": "new", "opensearch-old": "off", "mine": "byhand", "docker2": "off"}
	var names []string
	for _, r := range got {
		names = append(names, r.Name)
		if w, ok := want[r.Name]; !ok || r.State != w {
			t.Errorf("%s: state %q, want %q", r.Name, r.State, w)
		}
	}
	// grafana:test has no spec here, nor one of its own: not a server.
	if strings.Join(names, " ") != "github jira jira-work opensearch gone docker opensearch-old mine docker2" {
		t.Errorf("rows = %v", names)
	}
	if got[1].About != "Jira" || strings.Join(got[1].Keys, " ") != "JIRA_URL JIRA_API_TOKEN" || !got[1].Yardo {
		t.Errorf("jira = %+v", got[1])
	}
	if mine := got[7]; mine.Spec != "" || mine.Yardo {
		t.Errorf("mine = %+v", mine)
	}
	specs["aws-api"] = &mcp.Spec{}
	if r := mcpRows([]string{"aws-api"}, []foundServer{{"aws-api", true, false, false}}, nil, specs, true, nil); r[0].State != "retired" || r[0].Instead != "aws" {
		t.Errorf("retired: %+v", r[0])
	}
	if r := mcpRows([]string{"github"}, []foundServer{{"github", false, false, false}}, nil, specs, false, nil); r[0].State != "unchecked" {
		t.Errorf("unchecked: %+v", r[0])
	}
}

// Each action changes the answers' servers (or doesn't, when mcp-sync
// fails), and says whether to apply and what to open in the editor.
func TestMCPDo(t *testing.T) {
	specs := map[string]*mcp.Spec{"jira": {Keys: []string{"JIRA_URL"}}, "docker": {}}
	var ran [][]string
	var edited []string
	syncErr := error(nil)
	envThere := true
	k := &asker{a: &answers.Answers{User: "ada", MCPServers: []string{"jira", "jira:work"}},
		mcpSync: func(args ...string) error { ran = append(ran, args); return syncErr },
		edit:    func(n string) (bool, error) { edited = append(edited, n); return envThere, nil }}
	ctx := context.Background()
	do := func(act ui.MCPAct) mcpOutcome {
		t.Helper()
		out, err := k.mcpDo(ctx, act, specs)
		if err != nil {
			t.Fatalf("%+v: %v", act, err)
		}
		return out
	}
	if out := do(ui.MCPAct{Do: "add", Entry: "jira:prod"}); !out.apply || strings.Join(out.edit, " ") != "jira-prod" {
		t.Errorf("add: %+v", out)
	}
	if out := do(ui.MCPAct{Do: "add", Entry: "docker"}); !out.apply || out.edit != nil {
		t.Errorf("add docker (no keys): %+v", out)
	}
	if out := do(ui.MCPAct{Do: "rename", Entry: "jira:work", To: "jira:team"}); !out.apply ||
		!reflect.DeepEqual(ran[0], []string{"rename", "jira-work", "jira-team"}) {
		t.Errorf("rename: %+v, ran %v", out, ran)
	}
	if out := do(ui.MCPAct{Do: "delete", Entry: "jira:prod"}); out.apply || out.toast != "jira-prod deleted." ||
		!reflect.DeepEqual(ran[1], []string{"remove", "jira-prod"}) {
		t.Errorf("delete: %+v, ran %v", out, ran)
	}
	if out := do(ui.MCPAct{Do: "rename", Entry: "grafana:old", To: "grafana:new"}); out.apply || out.toast != "grafana-old is grafana-new now." {
		t.Errorf("rename a found one: %+v", out)
	}
	if got := strings.Join(k.a.MCPServers, " "); got != "jira jira:team docker" {
		t.Errorf("servers = %q", got)
	}
	if out := do(ui.MCPAct{Do: "edit", Entry: "jira"}); out.apply || out.sync != "jira" || strings.Join(edited, " ") != "jira" {
		t.Errorf("edit (registers now): %+v, edited %v", out, edited)
	}
	if out := do(ui.MCPAct{Do: "retry", Entry: "jira"}); out.sync != "jira" {
		t.Errorf("retry: %+v", out)
	}
	envThere = false
	if out := do(ui.MCPAct{Do: "on", Entry: "grafana:test"}); !out.apply {
		t.Errorf("on: %+v", out)
	}
	if out := do(ui.MCPAct{Do: "edit", Entry: "mine"}); strings.Join(out.edit, " ") != "mine" {
		t.Errorf("edit without an env: %+v", out)
	}
	if got := strings.Join(k.a.MCPServers, " "); got != "jira jira:team docker grafana:test mine" {
		t.Errorf("servers after on, edit = %q", got)
	}
	syncErr = errors.New("there's a jira-x already")
	before := strings.Join(k.a.MCPServers, " ")
	for _, act := range []ui.MCPAct{{Do: "rename", Entry: "jira:team", To: "jira:x"}, {Do: "delete", Entry: "docker"}} {
		if _, err := k.mcpDo(ctx, act, specs); err == nil || !strings.Contains(err.Error(), "jira-x already") {
			t.Errorf("%+v: err = %v", act, err)
		}
	}
	if got := strings.Join(k.a.MCPServers, " "); got != before {
		t.Errorf("servers after failures = %q, want %q", got, before)
	}
}

// mcp-sync status's lines, the servers worth registering now, and how
// each reads after.
func TestMCPStatus(t *testing.T) {
	st, down := parseStatus("-\tdown\thttp://127.0.0.1:8080\njira\tok\t\naws\tfailed\tError: denied\ngh\twaiting\tTOKEN URL\nos\tchanged\t\nnew\tunregistered\t\nbad line\n")
	if !down || st["jira"].state != "ok" || st["aws"].detail != "Error: denied" || st["gh"].detail != "TOKEN URL" || len(st) != 5 {
		t.Errorf("parseStatus = %v, %v", st, down)
	}
	if got := strings.Join(mcpPending(st), " "); got != "new os" {
		t.Errorf("pending = %q", got)
	}
	for name, want := range map[string]string{"jira": "✓ jira: registered", "aws": "✗ aws didn't register: Error: denied",
		"gh": "not registered until TOKEN URL is filled in (sudo yardo mcp edit gh)"} {
		if got := mcpSays(name, st[name], true); !strings.Contains(got, want) {
			t.Errorf("%s: %q", name, got)
		}
	}
	// What mcp-sync says wins: failed with why, pending, waiting's keys.
	specs := map[string]*mcp.Spec{"aws": {}, "os": {}, "gh": {}}
	rows := mcpRows([]string{"aws", "os", "gh"}, []foundServer{{"aws", false, false, false}, {"os", true, false, false}, {"gh", false, false, false}}, nil, specs, true, st)
	if rows[0].State != "failed" || rows[0].Detail != "Error: denied" || rows[1].State != "pending" || rows[2].State != "secret" || strings.Join(rows[2].Keys, " ") != "TOKEN URL" {
		t.Errorf("rows = %+v", rows)
	}
}

// mcp-sync source's lines: an image pulled here (its label, else its id),
// one not pulled yet, and a URL.
func TestParseSources(t *testing.T) {
	src := parseSources("jira\tghcr.io/x/atlassian:0.23.1\t0.23.1\tsha256:0123456789abcdef\t2026-03-02T10:00:00.123Z\n" +
		"seq\tmcp/seq:latest\t\tsha256:abcdef0123456789\t2026-01-05T00:00:00Z\n" +
		"gh\tghcr.io/github/gh:1\n" +
		"c7\thttps://mcp.context7.com/mcp\n" +
		"bad line\nempty\t\n")
	for name, want := range map[string]string{
		"jira": "ghcr.io/x/atlassian:0.23.1 (cached: 0.23.1, built 2026-03-02)",
		"seq":  "mcp/seq:latest (cached: abcdef012345, built 2026-01-05)",
		"gh":   "ghcr.io/github/gh:1 (not pulled yet)",
		"c7":   "https://mcp.context7.com/mcp",
	} {
		if got := src[name].says(); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if len(src) != 4 {
		t.Errorf("parseSources = %v", src)
	}
}
