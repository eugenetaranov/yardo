package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/eugenetaranov/yardo/internal/mcp"
	"github.com/eugenetaranov/yardo/internal/steps"
	"github.com/eugenetaranov/yardo/internal/ui"
)

// The AWS MCP server signs with a profile of the AWS CLI's: mcp-sync
// mounts ~/.aws (aws.spec's MOUNT_AWS), and the server's env names the
// profile. Adding or editing it picks one, checks it (an expired SSO login
// renewed there and then), and writes it to the env.

// awsProfile is a profile in ~/.aws: how it gets its credentials (sso,
// role, keys) and its region.
type awsProfile struct{ name, kind, region string }

// awsOwnKeys is the choice of keys of the server's own, in its env.
const awsOwnKeys = "Keys of its own: open its settings"

// awsProfiles are the profiles in home's ~/.aws (config, then
// credentials), default first. Those the server can't use are left out:
// credential_process (its program isn't in the container), a web identity
// token file (nor is that), and ones with no credentials at all.
func awsProfiles(home string) []awsProfile {
	type section map[string]string
	var order []string
	all := map[string]section{}
	read := func(file string, config bool) {
		f, err := os.Open(filepath.Join(home, ".aws", file))
		if err != nil {
			return
		}
		defer f.Close()
		var cur section
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			l := strings.TrimSpace(sc.Text())
			if l == "" || l[0] == '#' || l[0] == ';' {
				continue
			}
			if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
				name := strings.TrimSpace(l[1 : len(l)-1])
				cur = nil
				if config {
					if rest, ok := strings.CutPrefix(name, "profile "); ok {
						name = strings.TrimSpace(rest)
					} else if name != "default" {
						continue // sso-session, services
					}
				}
				if all[name] == nil {
					all[name] = section{}
					order = append(order, name)
				}
				cur = all[name]
				continue
			}
			if k, v, ok := strings.Cut(l, "="); ok && cur != nil {
				cur[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
		}
	}
	read("config", true)
	read("credentials", false)
	var out []awsProfile
	for _, n := range order {
		s := all[n]
		p := awsProfile{name: n, region: s["region"]}
		switch {
		case s["credential_process"] != "", s["web_identity_token_file"] != "":
			continue
		case s["role_arn"] != "":
			p.kind = "role"
		case s["sso_start_url"] != "", s["sso_session"] != "":
			p.kind = "SSO"
		case s["aws_access_key_id"] != "":
			p.kind = "keys"
		default:
			continue
		}
		if src := all[s["source_profile"]]; p.region == "" && src != nil {
			p.region = src["region"]
		}
		out = append(out, p)
	}
	slices.SortStableFunc(out, func(a, b awsProfile) int {
		switch {
		case a.name == "default" && b.name != "default":
			return -1
		case b.name == "default" && a.name != "default":
			return 1
		}
		return 0
	})
	return out
}

func (p awsProfile) label() string {
	l := p.name + "  ·  " + p.kind
	if p.region != "" {
		l += "  ·  " + p.region
	}
	return l
}

// adminRole: an ARN of a role (or root) that can do anything; the agent
// could too, read-only or not (that only hides write tools).
var adminRole = regexp.MustCompile(`(?i)(:root$|AdministratorAccess|/[^/]*admin[^/]*/)`)

// asUser is a command as user, from their home, mise's shims on PATH
// (the AWS CLI is often mise's).
func asUser(ctx context.Context, user string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "runuser", append([]string{"-u", user, "--", "sh", "-c", steps.UserShell, "_"}, args...)...)
}

// awsWho is who profile signs in as (its ARN), or why it can't.
func awsWho(ctx context.Context, user, profile string) (string, error) {
	out, err := asUser(ctx, user, "aws", "sts", "get-caller-identity", "--profile", profile, "--query", "Arn", "--output", "text").CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		lines := strings.Split(s, "\n")
		return "", fmt.Errorf("%s", strings.TrimSpace(lines[len(lines)-1]))
	}
	return s, nil
}

// awsChoice is how the AWS server signs in and what it may do: its
// profile and region, and apis: AWS's API tool shown (run_script; else
// read-only, docs and guides only).
type awsChoice struct {
	profile, region string
	apis            bool
}

// env are the choice's lines in the server's env.
func (c awsChoice) env() [][2]string {
	ro := "true"
	if c.apis {
		ro = "false"
	}
	return [][2]string{{"AWS_PROFILE", c.profile}, {"AWS_REGION", c.region}, {"READ_OPERATIONS_ONLY", ro}}
}

// says is the choice, in a line.
func (c awsChoice) says(name string) string {
	what := "AWS's docs and guides only (read-only)"
	if c.apis {
		what = "AWS APIs in your account, as the profile's role allows"
	}
	return fmt.Sprintf("%s signs with the AWS profile %s, in %s: %s. (Later: sudo yardo mcp edit %s)", name, c.profile, c.region, what, name)
}

// awsMode is what an AWS server registered read-only (or not) can do, for
// its screen.
func awsMode(readOnly bool) string {
	if readOnly {
		return "Read-only: AWS's docs and guides only, not your account (its API tool is hidden). To let the agent call AWS APIs: Edit its settings, and say yes."
	}
	return "Calls AWS APIs in your account: whatever its profile's role allows, the agent can do."
}

// askAWS picks the profile the AWS server signs with, its region, and
// whether the agent may call AWS APIs; ok is false for keys of its own
// (or no profiles to pick from): its settings open instead.
func (k *asker) askAWS(ctx context.Context, name string) (awsChoice, bool, error) {
	if k.awsPick != nil {
		return k.awsPick(name)
	}
	ps := awsProfiles(homeOf(k.a.User))
	if len(ps) == 0 {
		fmt.Fprintf(k.out, "%s: no AWS CLI profiles in ~/.aws (aws configure sso makes one); its settings open instead.\n", name)
		return awsChoice{}, false, nil
	}
	opts := make([]string, 0, len(ps)+1)
	for _, p := range ps {
		opts = append(opts, p.label())
	}
	opts = append(opts, awsOwnKeys)
	pick, err := ui.Choose(name+": the AWS CLI profile it signs with (from ~/.aws)", opts, opts[0])
	if err != nil || pick == awsOwnKeys {
		return awsChoice{}, false, err
	}
	p := ps[slices.Index(opts, pick)]
	who, err := k.awsSignIn(ctx, p)
	if err != nil {
		return awsChoice{}, false, err
	}
	role := "A read-only role (ReadOnlyAccess) keeps it to reading."
	if adminRole.MatchString(who) {
		role = "This one is an admin role: it can change or delete anything you can."
	}
	apis, err := ui.Confirm("Let the agent call AWS APIs in this account?",
		"Yes: it does what the role allows. "+role+" No: read-only, AWS's docs and guides only.", true)
	if err != nil {
		return awsChoice{}, false, err
	}
	c := awsChoice{profile: p.name, region: p.region, apis: apis}
	if c.region == "" {
		c.region = "us-east-1"
	}
	return c, true, nil
}

// awsSignIn is who profile p signs in as ("" when it doesn't, said with
// why), an SSO login that's missing or run out offered there and then;
// err is only the prompt's.
func (k *asker) awsSignIn(ctx context.Context, p awsProfile) (string, error) {
	who, err := awsWho(ctx, k.a.User, p.name)
	if err != nil && p.kind == "SSO" {
		fmt.Fprintf(k.out, "%s: %v\n", p.name, err)
		login, cerr := ui.Confirm("Log in to AWS now?", "aws sso login --profile "+p.name+" --use-device-code: open the link it shows, on any device.", true)
		if cerr != nil {
			return "", cerr
		}
		if login {
			c := asUser(ctx, k.a.User, "aws", "sso", "login", "--profile", p.name, "--use-device-code")
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			_ = c.Run()
			who, err = awsWho(ctx, k.a.User, p.name)
		}
	}
	if err != nil {
		fmt.Fprintf(k.out, "%s doesn't sign in yet (%v): the server won't register until it does. Then: Try again, on its screen.\n", p.name, err)
		return "", nil
	}
	fmt.Fprintf(k.out, "%s is %s\n", p.name, who)
	return who, nil
}

// askPick picks a server's settings when its spec has a picker (AWS: a
// profile; Kubernetes: a context): its env's lines, and a line saying
// them; ok false: its settings open instead.
func (k *asker) askPick(ctx context.Context, spec, name string) (env [][2]string, says string, ok bool, err error) {
	switch spec {
	case "aws":
		c, ok, err := k.askAWS(ctx, name)
		return c.env(), c.says(name), ok, err
	case "kubernetes":
		c, ok, err := k.askKube(ctx, name)
		return c.env(), c.says(name), ok, err
	}
	return nil, "", false, nil
}

// setMCPEnv sets keys in server name's env, as ~/.mcp's owner.
func setMCPEnv(ctx context.Context, user, name string, kv [][2]string) error {
	env := filepath.Join(homeOf(user), ".mcp", name, "env")
	b, err := exec.CommandContext(ctx, "runuser", "-u", mcpOwner(user), "--", "cat", env).Output()
	if err != nil {
		return fmt.Errorf("reading ~/.mcp/%s/env: %w", name, err)
	}
	for _, p := range kv {
		b = steps.EnvSet(b, p[0], p[1])
	}
	c := exec.CommandContext(ctx, "runuser", "-u", mcpOwner(user), "--", "sh", "-c", `cat > "$1"`, "_", env)
	c.Stdin = bytes.NewReader(b)
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("writing ~/.mcp/%s/env: %w: %s", name, err, out)
	}
	return nil
}

// mcpSpecOf is the spec of the server named name among entries.
func mcpSpecOf(entries []string, name string) string {
	for _, e := range entries {
		if spec, n, err := mcp.Instance(e); err == nil && n == name {
			return spec
		}
	}
	return ""
}

// awsLoginNeeded: a failed AWS server's reason is its login, missing or
// run out. With no SSO login the proxy answers the gateway's first
// request with only "Invalid request parameters" (its log has
// SSOTokenLoadError).
var awsLoginNeeded = regexp.MustCompile(`(?i)no aws credentials|invalid request parameters|token (has|is) expired|expiredtoken|unauthorizedssotoken|ssotokenloaderror|sso.*(expired|invalid)`)

// awsHint is a failed server's reason, with what to do when it's an AWS
// server's login.
func awsHint(spec, detail string) string {
	if spec != "aws" || !awsLoginNeeded.MatchString(detail) {
		return detail
	}
	return "no AWS login: run aws sso login (--profile <its profile>) here, then Try again. (" + detail + ")"
}
