## Purpose

The tabs, items, labels and install sources yardo's picker
offers, and how the MCPJungle gateway is wired into the installed
coding agents.

## Requirements

### Requirement: Tabs
The catalog's tabs (`internal/catalog/data/00-tabs.toml`) SHALL be, in order:

| id | title | group | phase |
|---|---|---|---|
| lang | Languages | Dev | install |
| dev | Dev tools | Dev | install |
| k8s | Containers & Kubernetes | DevOps | install |
| iac | IaC | DevOps | install |
| cloud | Cloud | DevOps | install |
| ai | Coding agents | AI | install |
| aiext | Add-ons & models | AI | install |
| shell | Shell & CLI | | install |
| system | System | System | install |
| net | Connectivity | System | install |
| sysconf | Who can connect | | configure |
| git | Git & SSH | | configure |
| agents | Agents | | configure |
| logins | VPN logins | | configure |

Tabs of one group SHALL show as one picker tab named after the group, each tab a section under its own heading, so the install tabs read **Dev · DevOps · AI · Shell & CLI · System**. The configure tabs SHALL show as sections of **This VM**, followed by yardo's own sections: "Who can connect" (who can reach the MCP gateway, containers' published ports, the Docker API), "Look" and "Status & updates". The sysconf tab's only visible items are `docker-ports` and `docker-api`; its other items are `always` (the baseline every machine gets). Moving an existing item to a new tab SHALL keep its id.

#### Scenario: Tab order
- **WHEN** yardo opens the menu
- **THEN** the tab bar reads Dev, DevOps, AI, Shell & CLI, System, This VM

#### Scenario: Grouped sections
- **WHEN** the user opens the AI tab
- **THEN** it shows a "Coding agents" section and an "Add-ons & models" section

#### Scenario: Moved item keeps answers
- **WHEN** saved answers contain `backup` and `ollama`
- **THEN** both are ticked, under Dev tools and Add-ons & models respectively

### Requirement: Catalog items
The catalog SHALL offer the items below, besides the hidden helpers other items pull in through `needs` and the `always` sysconf items. Items on the System and Connectivity tabs, and those with `scope = "system"`, are machine-wide; the rest are per user.

| tab | items |
|---|---|
| Languages | go, python, node, java, rust |
| Dev tools | devcli, dbclients, jiractl, backup (restic + rclone) |
| Containers & Kubernetes | docker, podman, k8s, k8stools, k3s |
| IaC | terraform, opentofu, packer, ansible, tack, sops-install, vault |
| Cloud | aws, gcloud, azure, doctl, hcloud, cloudflared |
| Coding agents | claude, codex, gemini, opencode, aider, goose, omp |
| Add-ons & models | openspec, ecc, mcpjungle-install, ollama |
| Shell & CLI | zsh, neovim, tmux, zellij, mosh, helpers |
| System | etckeeper, essentials, debug, netfs |
| Connectivity | tailscale-install, netbird-install, openvpn-client, openvpn-server, wireguard-client, wireguard-server |
| Who can connect | docker-ports, docker-api; always: devlimits, zram, journald, tzlocale, autoupdates, autoupdate-tools, sshharden, fail2ban |
| Git & SSH | gitid, github, gitlab, bitbucket, gitsign, dotfiles |
| Agents | agentcfg, agentskills, communityskills, agentkeys, mcpjungle |
| VPN logins | tailscale, netbird |

#### Scenario: Every item resolves
- **WHEN** a test VM runs `yardo --yes` with every catalog id picked
- **THEN** every step succeeds, or any failure is recorded in `FAILED` with no effect on unrelated items

#### Scenario: Dependencies pulled in
- **WHEN** only `ecc` is picked
- **THEN** the plan also contains `mise`, `node` and `claude`, ordered before `ecc`

### Requirement: Language toolchains with their usual companions
The language rows SHALL install:
- **go:** `go@latest golangci-lint@latest`, plus gopls, delve and govulncheck through the `go:` backend
- **python:** `python@3.13 uv@latest ruff@latest`
- **rust:** `rust@stable` and `cargo-binstall@latest`, followed by `rustup component add rust-analyzer` (fn)
- **java:** `java@temurin-25 maven@latest gradle@latest kotlin@latest`

The `k8s` row SHALL install `helm@3` instead of `helm@latest`.

#### Scenario: Go editor support
- **WHEN** `go` is installed
- **THEN** `gopls version` and `dlv version` succeed in a new shell

#### Scenario: Rust analyzer
- **WHEN** `rust` is installed
- **THEN** `rust-analyzer --version` succeeds

### Requirement: Self-explanatory labels
Every label SHALL say what the item is, without the user needing to know the tool's name. Items that download more than about 500 MB or build from source SHALL say "large" or give a time estimate. Among them:
- `omp` → "omp (oh-my-pi coding agent)"
- `docker-ports` → "Limit containers' published ports to this machine or your VPNs (asks)"
- `mcpjungle-install` → "MCPJungle MCP gateway (runs in Docker; its servers: MCP servers, on the menu)"
- `agentcfg` → "Agent notes and safety settings for this VM", with the details in its source
- `tailscale-install` and `netbird-install` → the VPN's name, then "(log in under This VM → VPN logins)"
- `azure` and `gcloud` → "(large)" appended

#### Scenario: Large items marked
- **WHEN** the user views the Cloud section
- **THEN** the Azure CLI and Google Cloud CLI labels include "(large)"

### Requirement: MCP gateway wired into picked agents
When mcpjungle is installed and the user agrees to wiring (`A_WIRE_AGENTS=yes`), yardo SHALL register the mcpjungle endpoint with each picked agent that has a supported mechanism:
- Claude Code: `claude mcp add --scope user`
- Codex: its MCP config in `~/.codex/config.toml`
- Gemini CLI: `gemini mcp add --transport http`

Each registration SHALL be skipped when that agent already has an `mcpjungle` entry.

#### Scenario: Codex and Claude
- **WHEN** `claude`, `codex` and `mcpjungle` are installed with wiring on
- **THEN** both `claude mcp get mcpjungle` and Codex's config show the mcpjungle URL

#### Scenario: Re-run
- **WHEN** yardo runs again with the same picks
- **THEN** no duplicate mcpjungle entries are created

### Requirement: Credential items
The catalog SHALL offer these items. Each one follows the ask-or-skip contract of the `yardo-credentials` capability.

| id | tab | label | what it manages | needs |
|---|---|---|---|---|
| agentkeys | Agents | Agent API keys (Anthropic, OpenAI, Gemini) | `~/.config/yardo/agent.env` | |
| github | Git & SSH | GitHub: gh login + SSH key for git, act (run Actions locally) | gh login, SSH key on GitHub | |
| gitsign | Git & SSH | Sign commits with SSH | git signing config + signing key on GitHub | github |
| tailscale | VPN logins | Tailscale login (auth key; or 'sudo tailscale up' later) | `tailscale up --auth-key` | tailscale-install |
| netbird | VPN logins | NetBird login (setup key, NetBird cloud or your own; or 'sudo netbird up' later) | `netbird up --setup-key-file` | netbird-install |

#### Scenario: Items present
- **WHEN** the menu opens
- **THEN** each item above is on its tab, with its label

#### Scenario: Needs pulled in
- **WHEN** only `gitsign` is picked
- **THEN** the plan also includes `github` and its needs

### Requirement: yardo's own MCP server specs
yardo SHALL ship its own MCP server specs in `internal/assets/conf/mcp-catalog/`, installed into the same catalog directory as the specs that began as tack-roles copies (`internal/assets/mcp-catalog/`). A spec name SHALL NOT exist in both directories; a test SHALL enforce this.

yardo's own specs SHALL include:
- `github` (docker, `GITHUB_PERSONAL_ACCESS_TOKEN`)
- `postgres` (docker with host networking, `DATABASE_URI`)
- `playwright` (docker, no credentials)
- `linear` (http, bearer `LINEAR_API_KEY`)

Each spec SHALL name its image by a tag, not a digest. Each skeleton SHALL say where to create its credential.

#### Scenario: Offered in the MCP picker
- **WHEN** the user picks `mcpjungle`
- **THEN** the MCP server list includes github, postgres, playwright and linear next to the tack-roles servers

#### Scenario: Name clash caught
- **WHEN** a sync from tack-roles adds `github.spec` to `assets/mcp-catalog`
- **THEN** `go test ./internal/bootstrap/...` fails, naming `github`

### Requirement: MCP servers read-only by default
Where a server can be limited to reading, its spec SHALL do so by default: a flag in `IMAGE_ARGS` or `image_args`, or an active setting in the skeleton, with a comment saying how to allow writing. A server's description SHALL NOT claim read-only access it doesn't have.

#### Scenario: Postgres restricted
- **WHEN** the postgres server registers
- **THEN** its command includes `--access-mode=restricted`

#### Scenario: OpenSearch writes off
- **WHEN** a new `~/.mcp/opensearch/env` is made from the skeleton
- **THEN** it has `OPENSEARCH_SETTINGS_ALLOW_WRITE=false`

#### Scenario: Slack offers only its read tools
- **WHEN** the slack server registers with the skeleton's settings
- **THEN** only the tools in `SLACK_MCP_ENABLED_TOOLS` are offered, and posting is not among them

### Requirement: MCP servers send no usage reports
Where a server reports usage by default and has a documented switch, its spec SHALL turn reporting off.

#### Scenario: Grafana and Azure quiet
- **WHEN** the grafana or azure server registers
- **THEN** grafana runs with `--usage-stats=disabled`, and azure's env has `AZURE_MCP_COLLECT_TELEMETRY=false`

### Requirement: Retired MCP servers
A catalog server its maker retired SHALL leave yardo's catalog, its installed spec SHALL be removed (a team's spec of that name stays), and one already set up SHALL show on the MCP servers screen as retired, naming its replacement, with Delete.

#### Scenario: aws-api
- **WHEN** a machine set up before has an `aws-api` server
- **THEN** the next run removes `/usr/local/share/mcp-catalog/aws-api.spec`, and the MCP servers screen shows `aws-api` as "retired: add aws instead"

### Requirement: MCP server specs pin nothing
No catalog MCP server spec SHALL pin a version. An image SHALL be named by a tag, such as `:latest` or a floating tag like `node:lts-slim`, and never by a digest. A package run with `npx -y` or `uvx` SHALL be named without a version. A server its maker retired SHALL be removed (see Retired MCP servers).

#### Scenario: A pinned spec refused
- **WHEN** a spec's `IMAGE` names a digest or has no tag, or a package run with `npx -y` or `uvx` carries a version
- **THEN** the spec test fails, naming the spec

### Requirement: MCP servers resolved on each run
mcp-sync SHALL resolve what each catalog server runs: the tag's current digest, pulled, and the package's newest version on npm or PyPI. It SHALL record both in the server's `.resolved` file and register the server with them. It SHALL resolve a server when first synced and on `mcp-sync update`, which every yardo run and `sudo yardo mcp update` run. Between updates a server SHALL run what was recorded. A failed lookup SHALL keep what was recorded. A server from elsewhere SHALL NOT be resolved.

#### Scenario: Resolved once, kept until update
- **WHEN** a catalog server is synced for the first time
- **THEN** its image runs by the digest its tag pointed to, and its package at its newest version, both recorded in `.resolved`
- **WHEN** upstream publishes a newer version and mcp-sync runs without `update`
- **THEN** the server still runs what was recorded
- **WHEN** a yardo run calls `mcp-sync update`
- **THEN** the newer version is recorded, the server is registered again with it, and mcp-sync says `<name>: now <package>, <image>`

#### Scenario: Registry unreachable
- **WHEN** `mcp-sync update` can't reach npm, PyPI or the image's registry
- **THEN** the server keeps what was recorded and stays registered

#### Scenario: What runs is shown
- **WHEN** the MCP servers screen or `mcp-sync source` lists a catalog server whose image is pulled
- **THEN** it shows the resolved version (the package's, else the image's label) and a short digest

### Requirement: Skills say their version
Every skill row SHALL show what is installed. A skill checked from elsewhere SHALL show its source, the ref it was asked for (or "default branch") and a 7-character commit. Any other skill SHALL show its SKILL.md's `version:`, or "yardo <version>" for one of yardo's, then a 7-character hash of its files. A skill with no version SHALL show "content <hash>".

#### Scenario: A checked skill
- **WHEN** `sudo yardo skills` lists a skill approved from `acme/skills@v1.2.0` at commit `e4bb5d9…`
- **THEN** its line reads `acme/skills · v1.2.0 · commit e4bb5d9`

#### Scenario: A skill of the user's own
- **WHEN** a skill not from yardo has `version: 1.4.0` in its front matter
- **THEN** its row reads `1.4.0 · <hash of its files, 7 characters>`

### Requirement: Servers from elsewhere say what was checked
A server added from elsewhere SHALL show its image tag and a 7-character digest. For a URL or a spec file, it SHALL show the 7-character hash of its approved spec.

#### Scenario: An image from elsewhere
- **WHEN** the MCP servers screen lists `mcp/time`, approved at `sha256:9c46a91…`
- **THEN** its row reads `mcp/time:latest · sha256:9c46a91`
