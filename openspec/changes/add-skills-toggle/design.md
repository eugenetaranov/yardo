## Context

- Skills live in `~/.claude/skills/<name>` (Claude Code) and
  `~/.agents/skills/<name>` (Codex, Gemini CLI, OpenCode, Goose, omp).
  yardo's skills step writes both, each with a `.yardo-skill` marker
  holding the content hash. It leaves a folder the user changed, or one
  without a marker.
- Agents load every folder there with a `SKILL.md`. None of them has a
  per-skill switch they all honour.
- The MCP switch (`cmd/yardo/mcpservers.go`, `mcp-sync off|on`) keeps its
  state in `~/.mcp/<name>/off` and `~/.mcp/off`.

## Goals / Non-Goals

**Goals:**
- One switch that works for every agent yardo installs.
- The skills step and the switch never fight: a yardo run doesn't turn a
  skill back on.

**Non-Goals:**
- Hiding a skill from one agent but not another.
- Stopping an agent from moving a skill back by hand. Agents can write the
  skills folders anyway. Phase 3 is where trust comes in.

## Decisions

- **Off = the folder moves.** It goes to
  `~/.local/share/yardo/skills-off/{claude,agents}/<name>` with
  `os.Rename`, which keeps modes, the marker and the inode, all on the same
  filesystem. Alternatives considered:
  - Claude Code's `Skill(<name>)` deny rule. Rejected: Claude only, and the
    skill text still sits in the folder.
  - Renaming to `<name>.off` in place. Rejected: some agents scan
    subfolders by `SKILL.md` alone.
- **State is where the folder is.** A held folder means that skill is off.
  `~/.local/share/yardo/skills-off/ALL` means all are off, and
  `~/.local/share/yardo/skills-off/own` lists the skills turned off by
  themselves, so "all on" restores each skill's own choice. Nothing goes
  in answers.json or profiles, as with MCP.
- **The skills step knows about held copies.** `write` resolves the target
  as the held path when the skill is held. Its ownership check (marker
  equals content) runs there as it would in place. When ALL is set, the
  step writes new skills straight into the held place.
- **Moves run as the user**, through `Cmd{AsUser}`-style file operations.
  The folders are the user's and stay the user's. Phase 1's `os.Root` rule
  applies: a link as a skill folder is not moved.
- **Screen** reuses the MCP servers list model: the All row, marks,
  dimming when all are off, `a` applies in the background, a confirm on
  leaving. The source column comes from the marker check: `yardo`,
  `yardo, changed`, or `not yardo's`.

## Risks / Trade-offs

- [Another tool reinstalls a skill that's held off] → that's a clash.
  Turning it on reports the folder in the way and changes nothing (spec).
  The list shows the new folder as on and not yardo's.
- [An agent session already running keeps a skill it loaded] → the screen
  says changes apply to new sessions, as MCP does.
- [`~/.local/share` on another filesystem than `~`] → rename fails; fall
  back to copy, compare hashes, then remove.

## Migration Plan

None. With nothing held, every skill is on, which is today's behaviour.
