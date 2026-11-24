## MODIFIED Requirements

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

## REMOVED Requirements

### Requirement: MCP servers stay current
**Reason**: Pins went stale between releases. Servers now run what's newest, resolved on each run.
**Migration**: Replaced by "MCP servers resolved on each run". Specs drop their versions, and `task mcp-audit` is gone.

## ADDED Requirements

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
