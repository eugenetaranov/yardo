## Why

MCP servers can be turned off without losing them. Skills can't. Every
skill in `~/.claude/skills` or `~/.agents/skills` is in every agent's
context, all the time, and the only way to stop one is to delete it.
Phase 2 of 5 adds the same switch for skills. Phases 3–5 need it to be
there, because an add-on that is waiting to be checked, or that failed a
re-check, is turned off.

## What Changes

- **A Skills screen** next to MCP servers, reached from the home's Go to.
  It lists every skill in `~/.claude/skills` and `~/.agents/skills`:
  - yardo's own;
  - those installed by other tools;
  - ones the user made.
  
  Each row says where the skill is from and whether it is on.
- **On and off**, as on MCP servers:
  - space marks a skill on or off;
  - an **All skills** row marks them all;
  - `a` applies, and leaving with unsaved changes asks once;
  - a skill's own screen offers **Turn it off** or **Turn it on**.
- **Off means the agents can't see it.** The skill's folders are moved out
  of both skills folders into `~/.local/share/yardo/skills-off/`, and
  moved back when it is turned on, unchanged.
- **The choice persists** through yardo runs, profiles and yardo updates.
  A skill yardo ships that is off stays off, and yardo updates it where it
  is kept.
- **From the command line**: `sudo yardo skills`, `sudo yardo skills off
  [skill …]`, `sudo yardo skills on [skill …]`. With no skill named, they
  act on all skills.

Not in this change: adding skills from elsewhere (phase 3), Claude Code
plugin skills (they have Claude's own switch), project skills in a repo's
`.claude/skills`.

## Capabilities

### New Capabilities
- `yardo-skills-toggle`: turning skills on and off, each or all at once,
  from the TUI and the command line, keeping them unchanged while off.

### Modified Capabilities
- `yardo-tui-screens`: Skills is one more view of the one program, and the
  home's Go to lists it.

## Impact

- New `cmd/yardo/skills.go` (screen, CLI), modelled on
  `cmd/yardo/mcpservers.go`.
- New `internal/steps/skillsoff.go`: list, move off and on, the off list.
- `internal/steps/skills.go`: write to the held copy for a skill that's
  off.
- `cmd/yardo/dashboard.go` (Go to), `cmd/yardo/main.go` (subcommand),
  `cmd/yardo/help.go`.
- README: Skills section.
