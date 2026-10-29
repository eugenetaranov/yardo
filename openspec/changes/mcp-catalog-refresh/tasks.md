## 1. Pins

- [ ] 1.1 `github.spec`: `ghcr.io/github/github-mcp-server:v2.0.3`
- [ ] 1.2 `figma.spec`, `mysql.spec`: `node:24.22.2-slim`
- [ ] 1.3 `sequential-thinking`: `node:24.22.2-slim` + `npx -y @modelcontextprotocol/server-sequential-thinking@2.0.0` (vendored: change it in tack-roles too, or move it to yardo's own catalog)
- [ ] 1.4 Spec test: no `:latest` or untagged `IMAGE`; `npx -y`/`uvx` packages carry an exact version (both catalogs)

## 2. Read-only by default

- [ ] 2.1 `postgres.spec`: `IMAGE_ARGS="--access-mode=restricted"`; skeleton line on `unrestricted`
- [ ] 2.2 `opensearch.spec`: `OPENSEARCH_SETTINGS_ALLOW_WRITE=false` (skeleton); for machines set up before, set it in `image_args` unless the env says `true`
- [ ] 2.3 `slack.spec`: read-tool allowlist (`--enabled-tools` through `image_args`, from `SLACK_MCP_ENABLED_TOOLS` or the default list); posting comment rewritten
- [ ] 2.4 Check every description against what the server allows

## 3. Fixes

- [ ] 3.1 `aws.spec`: `--user 0:0` in `DOCKER_ARGS`; test with a profile in `~/.mcp/aws/config`
- [ ] 3.2 `grafana.spec`: usage reporting off
- [ ] 3.3 Skeletons: figma token URL; jenkins `/me/security` and 0.178+; opensearch port, session token, Serverless, no profile; jira PAT, `/wiki`, `TOOLSETS`
- [ ] 3.4 Optional lines: `#GITHUB_LOCKDOWN_MODE=1`, `#AZURE_TOKEN_CREDENTIALS=prod`, `#ENABLE_PII_REDACTION=true`

## 4. Keeping it current

- [ ] 4.1 `scripts/mcp-audit` (or a Taskfile task): print each spec's pin next to its registry's latest (GitHub releases, ghcr, Docker Hub, MCR, ECR, npm, PyPI)
- [ ] 4.2 Note the audit in the release checklist
- [ ] 4.3 Send the vendored spec changes to tack-roles

## 5. Verify

- [ ] 5.1 Register each changed server on a test VM; `sudo yardo mcp status` all ok
- [ ] 5.2 Release notes: what changed for servers set up before (skeleton lines reach new env files only)
