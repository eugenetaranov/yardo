## MODIFIED Requirements

### Requirement: Unattended agent settings
The `agentcfg` item on the Agents tab (under This VM), labelled "Agent notes and safety settings for this VM", SHALL write agent notes (`~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, only if absent) and safety settings for each installed agent, merging them into existing files rather than replacing them:
- For Claude Code, permission rules in `~/.claude/settings.json` that deny `git push --force`, `sudo`, and reads of `~/.mcp/**`, `~/.ssh/**`, `~/.config/yardo/agent.env`, `~/.aws/**`, `~/.kube/**`, `~/.config/gh/**`, `~/.docker/config.json` and `~/.git-credentials`.
- For Codex, `approval_policy` and `sandbox_mode` in `~/.codex/config.toml`.
- A "Pushing" section in the agent notes, asking agents to push only to non-default branches.

#### Scenario: Existing settings kept
- **WHEN** `~/.claude/settings.json` already has an `env` block
- **THEN** after the run it still has that block and also the deny rules

#### Scenario: Cloud credentials
- **WHEN** the item is applied for a user with Claude Code
- **THEN** `~/.claude/settings.json` denies `Read(~/.aws/**)` and `Read(~/.kube/**)`, along with the rules it had
