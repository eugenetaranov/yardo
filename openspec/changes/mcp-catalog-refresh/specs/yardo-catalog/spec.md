## ADDED Requirements

### Requirement: MCP servers stay current
Each MCP server spec SHALL pin its image (or package) to a released version, never `latest` or an untagged image. The catalog SHALL be audited against upstream before each minor release: each server's latest stable release, deprecation or archival, and whether an official replacement now exists. A server its maker retired SHALL be removed (see Retired MCP servers).

#### Scenario: Unpinned image refused
- **WHEN** a spec's `IMAGE` ends in `:latest` or has no tag
- **THEN** the spec test fails, naming the spec

#### Scenario: Package run through npx or uvx is pinned
- **WHEN** a spec runs a package with `npx -y` or `uvx` in `IMAGE_ARGS`
- **THEN** the package carries an exact version (`@2.0.0`), and the spec test fails otherwise

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
- **THEN** `GRAFANA_USAGE_STATS=disabled` or `AZURE_MCP_COLLECT_TELEMETRY=false` is set

### Requirement: Retired MCP servers
A catalog server its maker retired SHALL leave yardo's catalog, its installed spec SHALL be removed (a team's spec of that name stays), the catalog refresh from tack-roles SHALL drop it, and one already set up SHALL show on the MCP servers screen as retired, naming its replacement, with Delete.

#### Scenario: aws-api
- **WHEN** a machine set up before has an `aws-api` server
- **THEN** the next run removes `/usr/local/share/mcp-catalog/aws-api.spec`, and the MCP servers screen shows `aws-api` as "retired: add aws instead"
