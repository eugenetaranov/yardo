package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// A container a `docker run` of its server started just before accounts
// for is the gateway's; the rest, past the grace, are orphaned. One
// client takes one container, the earliest in its window.
func TestOrphaned(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	cs := []mcpContainer{
		{"live", "jira", ago(time.Hour - 2*time.Second)},
		{"lost", "jira", ago(3 * time.Hour)},
		{"twin", "jira", ago(time.Hour - 3*time.Second)}, // a second container, one client
		{"other", "aws", ago(time.Hour)},                 // jira's client is no aws's
		{"new", "aws", ago(30 * time.Second)},            // too young to say
		{"slow", "k8s", ago(time.Hour - 4*time.Minute)},  // made after its image was pulled
	}
	ps := []mcpClient{{"jira", ago(time.Hour)}, {"k8s", ago(time.Hour)}}
	var ids []string
	for _, c := range orphaned(cs, ps, now) {
		ids = append(ids, c.id)
	}
	if want := []string{"lost", "other", "twin"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("orphaned = %v, want %v", ids, want)
	}
	if got := orphaned(cs[:1], ps, now); got != nil {
		t.Errorf("its client's container: %v", got)
	}
}

func TestParseMCPContainers(t *testing.T) {
	out := "id1 2026-11-24T10:00:00.123456789Z /home/ada/.mcp/jira\n" +
		"id2 2026-11-24T10:00:00Z\n" + // no /mcp mount
		"id3 2026-11-24T10:00:00Z /home/bob/.mcp/jira\n" + // another user's
		"id4 2026-11-24T10:00:00Z /home/ada/.mcp/a/b\n" +
		"id5 nope /home/ada/.mcp/aws\n"
	cs := parseMCPContainers(out, "/home/ada/.mcp")
	if len(cs) != 1 || cs[0].id != "id1" || cs[0].name != "jira" || cs[0].created.Nanosecond() != 123456789 {
		t.Errorf("containers = %+v", cs)
	}
}

func TestParseMCPClients(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	out := `   3600 docker run -i --rm --env-file /home/ada/.mcp/jira/env -v /home/ada/.mcp/jira:/mcp:ro img/jira:1
     60 /usr/bin/docker run -i --rm -v /home/ada/.mcp/aws:/mcp:ro -v /home/ada/.aws:/aws/.aws:ro img/aws
     10 docker ps
     20 vim /home/ada/.mcp/jira:/mcp:ro
     30 docker run -v /home/bob/.mcp/jira:/mcp:ro img
`
	ps := parseMCPClients(out, "/home/ada/.mcp", now)
	want := []mcpClient{{"jira", now.Add(-time.Hour)}, {"aws", now.Add(-time.Minute)}}
	if !reflect.DeepEqual(ps, want) {
		t.Errorf("clients = %+v, want %+v", ps, want)
	}
}

func TestMCPDoCleanup(t *testing.T) {
	n, err := 2, error(nil)
	k := &asker{a: &answers.Answers{User: "ada"}, cleanup: func() (int, error) { return n, err }}
	out, e := k.mcpDo(context.Background(), ui.MCPAct{Do: "cleanup"}, nil)
	if e != nil || out.toast != "✓ 2 orphaned MCP containers removed." || out.apply {
		t.Errorf("cleanup: %+v, %v", out, e)
	}
	err = errors.New("docker rm: no daemon")
	if _, e := k.mcpDo(context.Background(), ui.MCPAct{Do: "cleanup"}, nil); e == nil || !strings.Contains(e.Error(), "no daemon") {
		t.Errorf("cleanup failing: %v", e)
	}
}
