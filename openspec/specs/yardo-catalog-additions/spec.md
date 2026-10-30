# yardo-catalog-additions Specification

## Purpose
Items added to the catalog after the Go rewrite: dotfiles, agent safety settings, the MCP gateway's reach and wiring, and smaller additions.

## Requirements

### Requirement: Dotfiles repo
A `dotfiles` item on the Git & SSH tab (under This VM) SHALL ask for a repo URL, which is saved in answers. It SHALL clone the repo to `~/.dotfiles`, or pull if the clone is already there. It SHALL then run the first of `install.sh`, `bootstrap.sh` or `setup.sh` it finds, or otherwise `stow` every top-level directory. It SHALL run after every other user item, so the files it writes win.

#### Scenario: Install script
- **WHEN** the repo has `install.sh`
- **THEN** yardo runs it as the user, from `~/.dotfiles`, as the last user step

### Requirement: Unattended agent settings
The `agentcfg` item on the Agents tab (under This VM), labelled "Agent notes and safety settings for this VM", SHALL write agent notes (`~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, only if absent) and safety settings for each installed agent, merging them into existing files rather than replacing them:
- For Claude Code, permission rules in `~/.claude/settings.json` that deny `git push --force`, reads of `~/.mcp/**`, `~/.ssh/**` and `~/.config/yardo/agent.env`, and `sudo`.
- For Codex, `approval_policy` and `sandbox_mode` in `~/.codex/config.toml`.
- A "Pushing" section in the agent notes, asking agents to push only to non-default branches.

#### Scenario: Existing settings kept
- **WHEN** `~/.claude/settings.json` already has an `env` block
- **THEN** after the run it still has that block and also the deny rules

### Requirement: Gateway wired into more agents
Agent wiring SHALL also register the MCPJungle gateway with OpenCode (`mcp` in `~/.config/opencode/opencode.json`) and Goose (`extensions` in `~/.config/goose/config.yaml`), when they are installed. It SHALL do this once, and SHALL keep the other settings in those files.

#### Scenario: OpenCode
- **WHEN** `opencode` and `mcpjungle` are picked with wiring on
- **THEN** `opencode.json` has an `mcpjungle` remote server at the gateway URL

### Requirement: Gateway listen options
The `mcpjungle-install` item is on the Add-ons & models section of the AI tab; who can reach the gateway SHALL be a setting in This VM → Who can connect, offering:
- this machine only
- this machine and your VPNs (with a token)
- the internet, at a domain name (HTTPS, with a token)
- everywhere, plain HTTP (with a token)

For the options with a token, yardo SHALL generate the token, store it at 0600 in the gateway's env, and wire agents with it. It SHALL NOT offer any interface beyond this machine without auth.

#### Scenario: VPN only
- **WHEN** the VPN option is chosen
- **THEN** the gateway is reachable from this machine and over the VPNs, and agents are wired with its token

### Requirement: Smaller additions
The catalog SHALL add:
- `kubelogin` (mise) to the `k8stools` item.
- An `autoupdate-tools` item (always on, per user, listed with the sysconf items in `10-system.toml`) that enables a weekly timer running `mise up` and `krew upgrade` as the user.
- A closing note when the user was added to the `docker` group, saying that VS Code and Cursor remote servers must be restarted ("Kill VS Code Server on Host") to pick it up.

#### Scenario: Docker group note
- **WHEN** a run adds the user to the `docker` group
- **THEN** the closing output names the remote-editor restart
