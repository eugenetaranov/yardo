## Why

yardo sets up coding agents (Claude Code, Codex, Gemini CLI, OpenCode,
Goose, omp) but leaves how they write to their defaults. Two habits keep
coming back:

- Commits get a `Co-Authored-By: Claude …` or `Co-authored-by: Codex …`
  line and PRs a "Generated with …" footer. A line in CLAUDE.md doesn't
  stop it: Claude Code's built-in git instructions compete with it, and
  Codex now tells itself to ignore any instruction that turns its
  attribution off.
- Jira tickets come out as walls of text, with headings, bold labels and
  boilerplate, instead of a few plain sentences.

Agents now share a skill format (agentskills.io: a folder with a
`SKILL.md`), and most read one folder, `~/.agents/skills`, so yardo can
ship a few skills once for all of them, with good community ones on offer.

## What Changes

- **No AI attribution, enforced where it can be:**
  - Claude Code: `"attribution": {"commit": "", "pr": ""}` merged into
    `~/.claude/settings.json` (the agentsafe step).
  - A git `commit-msg` hook that strips only agent trailers
    (`noreply@anthropic.com`, `noreply@openai.com`, Cursor's, "Generated
    with [Claude Code|Codex]"), never a person's `Co-authored-by`. It's
    set as a config-based hook (Git 2.54+), so repos' own hooks, husky and
    pre-commit keep working; on older git it's skipped, with a note.
  - Codex's attribution is a ChatGPT account setting: yardo says so, and
    the hook removes it locally.
  - The agent notes (CLAUDE.md, AGENTS.md) say it too.
- **yardo's own skills** (short, portable frontmatter, about 60 lines each):
  - `git-commits`: Conventional Commits, imperative, about 72 characters,
    the body says why, one change per commit, no agent trailer or footer.
  - `pr-descriptions`: a short note for the reviewer (what, why, how it
    was tested); no file-by-file lists, no agent footer.
  - `jira-tickets`: a title, 2–5 plain sentences, then up to 5 acceptance
    criteria; no headings, bold labels, emojis or "Overview/Background";
    written like a teammate. Comments and updates follow the same rules.
- **Community skills, opt-in**, vendored at a pinned commit with their
  licence:
  - obra/superpowers (MIT): systematic-debugging,
    test-driven-development, verification-before-completion
  - mattpocock/skills (MIT): tdd, code-review, grill-me
  - getsentry/skills (Apache-2.0): find-bugs, pr-writer
  - blader/humanizer (MIT): humanizer
  - anthropics/skills (Apache-2.0): skill-creator, webapp-testing
  - Not offered, with the reason: anthropics docx/pdf/pptx/xlsx
    (proprietary licence), doc-coauthoring (no licence file),
    trailofbits (CC-BY-SA, share-alike), vercel-labs/agent-skills (no
    licence), the superpowers bundle as a whole (it takes over the
    workflow), getsentry `commit` (Sentry's conventions).
- **Where they go:** each skill in `~/.agents/skills/<name>/` (read by
  Codex, Gemini CLI, OpenCode, Goose, pi and omp) and in
  `~/.claude/skills/<name>/` (Claude Code reads only its own). Copies, not
  links. yardo marks the folders it writes and changes or removes only
  those, leaving a skill the user edited alone.

## Capabilities

### New Capabilities
- `yardo-agent-skills`: skills yardo installs for the coding agents, and
  the no-attribution settings and git hook.

## Impact

- New `internal/assets/skills/` (yardo's and vendored skills, a
  `skills.lock.toml` with repo, path, commit, licence and a tree hash) and
  `scripts/update-skills`.
- Catalog: an "Agent skills" item (yardo's three, default on) and one item
  per community skill set (off by default), on the Agents tab.
- `go:agentsafe` gains Claude Code's `attribution`; a new `go:gitattrib`
  step sets the hook; `agent-notes.md` gains the attribution line.
- Nothing is fetched at install time: skills ship inside yardo, and an
  update is a reviewed change to the lock file.
