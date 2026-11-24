## Why

Before yardo can check skills and MCP servers from elsewhere, the way it
installs its own has holes an add-on could use. MCP specs are shell, and
yardo runs them as root just to read them. A spec's Docker options reach
`docker run` unchecked, so one line can mount the whole machine. Skill
files are written as root through whatever links the user's home holds.
Archify is fetched at its latest version every time, with nothing
checked. Agents are kept from reading `~/.ssh`, but not `~/.aws` or
`~/.kube`. This change closes those holes first. It is phase 1 of 5;
the others add a skills switch, a holding area with checks, AI review,
and updates.

## What Changes

- **Specs aren't run as root.** Reading an MCP spec's fields runs as an
  unprivileged user with an empty environment, not as root. This covers
  the TUI, apply, and team specs a profile ships.
- **Dangerous Docker options are refused.** A server whose spec asks for
  `--privileged`, host network, PID or IPC, extra capabilities, devices,
  published ports, or a mount of `/`, the Docker socket or a home outside
  its own `~/.mcp/<name>` isn't registered. Its status says why. yardo's
  catalog specs already pass.
- **Skill writes don't follow links.** If `~/.agents/skills`,
  `~/.claude/skills` or anything under a skill folder is a symbolic link,
  yardo writes nothing through it and says so.
- **Skill names are checked**: lowercase letters, digits and hyphens, at
  most 64 characters, as agentskills.io requires.
- **Archify is pinned.** It comes from its repository at a fixed commit
  with a checked hash, not `npx skills@latest`. No **BREAKING** change for
  users: the same folders get the same skill.
- **Agents can't read more credentials.** Claude Code's deny rules add
  `~/.aws`, `~/.kube`, `~/.config/gh`, `~/.docker/config.json` and
  `~/.git-credentials`.
- **The Docker group is called what it is.** Docker's screen says that,
  in the docker group, anything run as you (agents and their skills
  included) can act as root.

Not in this change: checking add-ons from elsewhere (phases 3–4),
limiting what a running MCP container can do (phase 5), rootless Docker.

## Capabilities

### New Capabilities
- `yardo-addon-safety`: how yardo keeps skills and MCP servers from
  reaching beyond what they're for. This change adds reading specs without
  root, refusing dangerous Docker options and naming the Docker group's
  risk. Later phases add the holding area, checks, review and approvals.

### Modified Capabilities
- `yardo-agent-skills`: skill writes never follow links; names are
  checked; Archify is pinned.
- `yardo-catalog-additions`: Unattended agent settings deny reads of more
  credential files.

## Impact

- `internal/mcp/mcp.go` (`Load`): run the bash reader as `nobody` with
  `env -i`; callers in `cmd/yardo/status.go` and `internal/steps/mcp.go`
  are unchanged.
- `internal/assets/mcp-sync` (`render`): check `DOCKER_ARGS` and
  `IMAGE_ARGS` before registering; a `refused` status state.
- `internal/steps/skills.go`, `internal/steps/decl.go` (`writeFile`):
  write under an `os.Root` for each skills folder; name check.
- `internal/catalog/data/80-ai.toml` (archify) and
  `internal/assets/skills.lock.toml`: a pinned entry fetched at install.
- `internal/steps/agentsafe.go` (`ClaudeDeny`, `Plan` text).
- `internal/catalog/data/10-system.toml` (docker item text).
- README: the Docker group note and the Archify source.
