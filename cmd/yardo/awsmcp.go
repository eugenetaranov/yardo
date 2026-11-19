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

// askAWS picks the profile the AWS server signs with, and its region;
// ok is false for keys of its own (or no profiles to pick from): its
// settings open instead.
func (k *asker) askAWS(ctx context.Context, name string) (profile, region string, ok bool, err error) {
	if k.awsPick != nil {
		return k.awsPick(name)
	}
	ps := awsProfiles(homeOf(k.a.User))
	if len(ps) == 0 {
		fmt.Fprintf(k.out, "%s: no AWS CLI profiles in ~/.aws (aws configure sso makes one); its settings open instead.\n", name)
		return "", "", false, nil
	}
	opts := make([]string, 0, len(ps)+1)
	for _, p := range ps {
		opts = append(opts, p.label())
	}
	opts = append(opts, awsOwnKeys)
	for {
		pick, err := ui.Choose(name+": the AWS CLI profile it signs with (from ~/.aws)", opts, opts[0])
		if err != nil || pick == awsOwnKeys {
			return "", "", false, err
		}
		p := ps[slices.Index(opts, pick)]
		who, err := awsWho(ctx, k.a.User, p.name)
		if err != nil && p.kind == "SSO" {
			fmt.Fprintf(k.out, "%s: %v\n", p.name, err)
			login, cerr := ui.Confirm("Log in to AWS now?", "aws sso login --profile "+p.name+" --use-device-code: open the link it shows, on any device.", true)
			if cerr != nil {
				return "", "", false, cerr
			}
			if login {
				c := asUser(ctx, k.a.User, "aws", "sso", "login", "--profile", p.name, "--use-device-code")
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				_ = c.Run()
				who, err = awsWho(ctx, k.a.User, p.name)
			}
		}
		switch {
		case err != nil:
			fmt.Fprintf(k.out, "%s doesn't sign in yet (%v): the server won't register until it does. Then: Try again, on its screen.\n", p.name, err)
		case adminRole.MatchString(who):
			fmt.Fprintf(k.out, "%s is %s\n", p.name, who)
			use, cerr := ui.Confirm("Give the agent an admin role?", "It can then do anything the role can: read-only only hides the write tools. A read-only role or permission set (ReadOnlyAccess) is safer.", false)
			if cerr != nil {
				return "", "", false, cerr
			}
			if !use {
				continue // another profile
			}
		default:
			fmt.Fprintf(k.out, "%s is %s\n", p.name, who)
		}
		region = p.region
		if region == "" {
			region = "us-east-1"
		}
		return p.name, region, true, nil
	}
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
