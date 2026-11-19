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

// The profiles in ~/.aws: default first, each with how it signs in and
// its region (a role's from its source profile); those the server can't
// use, or with no credentials, left out.
func TestAWSProfiles(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".aws"), 0o700); err != nil {
		t.Fatal(err)
	}
	config := `[profile shared]
sso_start_url = https://x.awsapps.com/start
sso_region = us-east-1
sso_account_id = 111111111111
sso_role_name = ReadOnly
region = eu-west-1

[default]
sso_session = corp
sso_account_id = 222222222222
sso_role_name = ReadOnly
region = us-east-1

[sso-session corp]
sso_start_url = https://x.awsapps.com/start

[profile non-prod]
role_arn = arn:aws:iam::333333333333:role/ReadOnly
source_profile = shared

[profile vault]
credential_process = aws-vault export --format=json vault

[profile ci]
role_arn = arn:aws:iam::444444444444:role/ci
web_identity_token_file = /var/run/token

[profile bare]
region = us-west-2
`
	creds := "[iam-ro]\naws_access_key_id = AKIA\naws_secret_access_key = s\n"
	for f, b := range map[string]string{"config": config, "credentials": creds} {
		if err := os.WriteFile(filepath.Join(home, ".aws", f), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, p := range awsProfiles(home) {
		got = append(got, p.label())
	}
	want := []string{"default  ·  SSO  ·  us-east-1", "shared  ·  SSO  ·  eu-west-1", "non-prod  ·  role  ·  eu-west-1", "iam-ro  ·  keys"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("profiles:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if ps := awsProfiles(t.TempDir()); len(ps) != 0 {
		t.Errorf("no ~/.aws: %v", ps)
	}
}

// A login missing or run out says what to do; another failure is as it was.
func TestAWSHint(t *testing.T) {
	for _, d := range []string{
		"failed to register server: failed to initialize connection with MCP server: internal error: No AWS credentials available. Configure credentials or use --skip-auth to send unsigned requests.",
		"Error when retrieving token from sso: Token has expired and refresh failed",
		"failed to register server: failed to initialize connection with MCP server: invalid params: Invalid request parameters",
	} {
		if h := awsHint("aws", d); !strings.HasPrefix(h, "no AWS login: run aws sso login") || !strings.Contains(h, d) {
			t.Errorf("%q: %q", d, h)
		}
	}
	if d := "image not found"; awsHint("aws", d) != d {
		t.Errorf("other: %q", awsHint("aws", d))
	}
	if d := "invalid params: Invalid request parameters"; awsHint("jira", d) != d {
		t.Errorf("not AWS: %q", awsHint("jira", d))
	}
	for arn, admin := range map[string]bool{
		"arn:aws:sts::1:assumed-role/AWSReservedSSO_AdministratorAccess_fe0d/ada@x.com": true,
		"arn:aws:sts::1:assumed-role/platform-admin/ada":                                true,
		"arn:aws:iam::1:root": true,
		"arn:aws:sts::1:assumed-role/AWSReservedSSO_ReadOnlyAccess_ab12/admin@x.com": false,
		"arn:aws:iam::1:user/mcp-readonly":                                           false,
	} {
		if adminRole.MatchString(arn) != admin {
			t.Errorf("%s: admin %v", arn, !admin)
		}
	}
}

// Editing an AWS server picks a profile: written to its env (its region
// too), and registered now; keys of its own open the file instead.
func TestMCPDoAWS(t *testing.T) {
	specs := map[string]*mcp.Spec{"aws": {Keys: []string{"AWS_REGION"}}}
	var set [][2]string
	var edited []string
	pick := true
	k := &asker{a: &answers.Answers{User: "ada", MCPServers: []string{"aws:prod"}},
		edit:    func(n string) (bool, error) { edited = append(edited, n); return true, nil },
		awsPick: func(string) (awsChoice, bool, error) { return awsChoice{"shared", "eu-west-1", true}, pick, nil },
		mcpEnv:  func(n string, kv [][2]string) error { set = append(set, kv...); return nil }}
	out, err := k.mcpDo(context.Background(), ui.MCPAct{Do: "edit", Entry: "aws:prod"}, specs)
	if err != nil || out.sync != "aws-prod" || len(edited) != 0 {
		t.Errorf("picked: %+v %v, edited %v", out, err, edited)
	}
	if len(set) != 3 || set[0] != [2]string{"AWS_PROFILE", "shared"} || set[1] != [2]string{"AWS_REGION", "eu-west-1"} || set[2] != [2]string{"READ_OPERATIONS_ONLY", "false"} {
		t.Errorf("set %v", set)
	}
	pick, set = false, nil
	if out, err := k.mcpDo(context.Background(), ui.MCPAct{Do: "edit", Entry: "aws:prod"}, specs); err != nil || out.sync != "aws-prod" || len(edited) != 1 || set != nil {
		t.Errorf("own keys: %+v %v, edited %v, set %v", out, err, edited, set)
	}
	if mcpSpecOf(k.a.MCPServers, "aws-prod") != "aws" || mcpSpecOf(k.a.MCPServers, "nope") != "" {
		t.Error("mcpSpecOf")
	}
}

// An AWS server's screen says what it may do, as registered: read-only
// is docs and guides; otherwise the profile's role is the limit.
func TestAWSMode(t *testing.T) {
	specs := map[string]*mcp.Spec{"aws": {}, "jira": {}}
	found := parseFound("aws-prod\t/usr/local/share/mcp-catalog/aws.spec\t1\t1\naws\t/usr/local/share/mcp-catalog/aws.spec\t1\t0\njira\t\t1\t1\n", specs)
	rows := mcpRows([]string{"aws:prod", "aws", "jira"}, found, nil, specs, true, nil)
	if len(rows) != 3 || !strings.HasPrefix(rows[0].Note, "Read-only: AWS's docs and guides only") ||
		!strings.HasPrefix(rows[1].Note, "Calls AWS APIs in your account") || rows[2].Note != "" {
		t.Errorf("notes: %+v", rows)
	}
	// Before the read-only field: three, read as not read-only.
	if f := parseFound("jira\t\t1\n", specs); len(f) != 1 || f[0].readOnly {
		t.Errorf("three fields: %+v", f)
	}
	c := awsChoice{"shared", "eu-west-1", false}
	if !strings.Contains(c.says("aws-prod"), "docs and guides only") || c.env()[2] != [2]string{"READ_OPERATIONS_ONLY", "true"} {
		t.Errorf("read-only choice: %q %v", c.says("aws-prod"), c.env())
	}
}
