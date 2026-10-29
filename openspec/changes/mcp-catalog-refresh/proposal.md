## Why

AWS retired the AWS API MCP server, and yardo kept shipping it until a user
noticed (v0.7.7 moved to AWS's managed server). An audit of the other 18
servers on 2026-10-29 found nothing else retired, but it found stale pins,
an unpinned image, and four servers that let agents do more than their
specs say:

- `postgres` runs in `unrestricted` mode: agents can write with any SQL.
- `opensearch` lets agents write (`OPENSEARCH_SETTINGS_ALLOW_WRITE`
  defaults to `true`) while it's described as read-only.
- `slack` turns posting off, but every tool without its own switch is
  still on: joining and leaving channels, marking them read, editing user
  groups and saved items.
- `aws`: the proxy image runs as the user `app`, which can't read
  `~/.mcp/aws` (mode 700), so the `AWS_PROFILE` + files option fails.

## What Changes

- **Pins brought up to date** (each checked against its registry):
  - `github`: `v2.0.2` → `v2.0.3`, which includes Go 1.27.3's CA fix.
  - `node:24.19.0-slim` → `node:24.22.2-slim` (figma, mysql;
    24.22.1 was a security release).
  - `sequential-thinking`: `mcp/sequentialthinking:latest` (untagged; the
    image is from May 2025) → `node:24.22.2-slim` running
    `npx -y @modelcontextprotocol/server-sequential-thinking@2.0.0`.
  - Everything else is already at its latest release: aws (proxy 1.7.0),
    azure 3.0.0, grafana 2.1.1, kubernetes v4.2.1, docker 0.3.0, figma
    0.13.2, mysql 2.0.9, postgres 0.3.0, opensearch 0.12.0, playwright
    v0.0.83, slack v1.3.0, jira 0.23.1, uv 0.13.6.
- **Read-only by default, everywhere the server allows it:**
  - `postgres`: `--access-mode=restricted` (read-only transactions; the
    explain, health and index tools stay). The skeleton says how to let it
    write.
  - `opensearch`: `OPENSEARCH_SETTINGS_ALLOW_WRITE=false` in the skeleton.
  - `slack`: an allowlist of read tools, `SLACK_MCP_ENABLED_TOOLS`; posting
    is added to that list (`SLACK_MCP_ADD_MESSAGE_TOOL` alone no longer
    turns it on once the list is set).
- **Fixes:**
  - `aws`: `--user 0:0` (as `kubernetes` already has), so the
    `config`/`credentials` files in `~/.mcp/aws` can be read.
  - `grafana`: usage reporting off (`GRAFANA_USAGE_STATS=disabled`; it's on
    by default), as `azure` already does.
  - Skeletons:
    - **figma:** token page moved (`developers.figma.com/docs/rest-api/personal-access-tokens/`).
    - **jenkins:** token is under `/me/security`; plugin 0.178+ (two advisories before it).
    - **opensearch:** an explicit port in the URL (since 0.10 a missing one means 443/80); `AWS_SESSION_TOKEN` and `AWS_OPENSEARCH_SERVERLESS`; "or a profile" removed (no `~/.aws` in the container).
    - **jira:** Server/Data Center's `JIRA_PERSONAL_TOKEN`, Cloud's `/wiki` for Confluence, `TOOLSETS=default`.
  - Optional lines where upstream added a safety switch:
    - `GITHUB_LOCKDOWN_MODE=1`
    - `AZURE_TOKEN_CREDENTIALS=prod`
    - `ENABLE_PII_REDACTION` (mysql)
- **Kept, with the reason written in the spec**:
  - **figma:** the official remote server takes OAuth only, from approved clients.
  - **slack:** the official server needs a confidential OAuth app.
  - **jira:** Atlassian's Rovo server is Cloud-only, needs an admin to turn on and uses credits.
  - **mysql, postgres:** Google's MCP Toolbox would be a bigger change, and nothing forces it.
  - **kubernetes:** `containers/kubernetes-mcp-server` is still pre-1.0.
- **Not in this change** (possible later specs): GitHub's hosted server,
  Atlassian Rovo, OpenSearch's in-cluster server (3.3+), Slack's official
  server, a Context7 API key (needs mcp-sync to skip an empty bearer token),
  Node 26 LTS.

## Capabilities

### Modified Capabilities
- `yardo-catalog`: the MCP servers yardo ships stay pinned, current and
  read-only by default, and retired ones are handled.

## Impact

- `internal/assets/conf/mcp-catalog/*.spec`: aws, figma, github, grafana,
  mysql, opensearch, postgres, slack, azure.
- `internal/assets/mcp-catalog/` (vendored from tack-roles): jenkins, jira,
  sequential-thinking. These edits go to tack-roles too, or `assets:sync`
  undoes them.
- Machines set up before: a changed spec re-registers on the next run
  (mcp-sync compares configs). Skeleton changes reach only new env files,
  so the read-only switches that live in a skeleton (opensearch, slack) are
  set in `image_args`, or as flags, where the server allows that, and
  otherwise noted in the release notes.
