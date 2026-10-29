## Context

The research behind this (2026-10-29, from each agent's docs and source):

| Agent | Reads skills from | Adds an attribution trailer | Switch |
|---|---|---|---|
| Claude Code | `~/.claude/skills` (not `~/.agents`) | yes, commit and PR | `attribution.commit`/`.pr` = "" in settings.json |
| Codex | `~/.agents/skills` (`~/.codex/skills` deprecated) | yes, when the ChatGPT account says so | account setting only; it overrides instructions |
| Gemini CLI | `~/.gemini/skills`, `~/.agents/skills` | no | — |
| OpenCode | `~/.config/opencode/skills`, `~/.claude/skills`, `~/.agents/skills` | no | — |
| Goose | `~/.agents/skills`, `~/.claude/skills` | no | — |
| pi / omp | `~/.agents/skills` (omp also `~/.omp/agent/skills`) | no | — |

## Decisions

- **Two folders, copies.** `~/.agents/skills` covers all but Claude Code;
  `~/.claude/skills` covers it (and OpenCode, Goose read it too: they drop
  a duplicate). Copies, because it's not confirmed that Claude Code follows
  a linked skill folder.
- **Portable frontmatter only**: `name`, `description`, `license`,
  `compatibility`, `metadata`. Claude-only fields break the others.
- **Always-on rules stay in the notes, not in a skill.** A skill loads only
  when its description matches the task; "no co-author line" must hold for
  every commit, so it's in the settings, the hook and the notes, and the
  skill repeats it.
- **The hook is config-based** (`hook.<name>.event commit-msg`, Git 2.54+):
  it runs beside a repo's own hooks. A global `core.hooksPath` would turn
  them off, husky's local one would turn ours off, and pre-commit refuses
  to install under it. Older git on Ubuntu is upgraded from the git-core
  PPA (the Ubuntu Git Maintainers'; key fingerprint pinned); elsewhere
  yardo sets no hook and says so. It
  strips only lines matching known agent identities, and changes nothing
  else (idempotent). It can't reach commits made through an API or PR
  bodies; the settings and the skill cover those.
- **Vendored, pinned, reviewed.** A lock entry per skill: repo, path, full
  commit, SPDX licence, tree sha256. `scripts/update-skills <name> <sha>`
  shows the diff, checks the licence and size, and runs a skill linter;
  the result is a normal commit. Nothing is fetched on the VM.
- **yardo owns only what it marks.** Each folder it writes has a
  `.yardo-skill` file with the lock hash. On a run, a folder that still
  matches is updated; one the user changed is left alone and noted; an
  unticked skill's folder goes only if it still matches.

## Risks

- Codex users signed in with ChatGPT keep attribution in PRs made by Codex
  itself until they turn it off in their account; yardo's note says where.
- Upstream skill quality drifts: updates are deliberate, never automatic.
