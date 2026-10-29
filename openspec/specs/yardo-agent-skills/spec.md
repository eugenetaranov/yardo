# yardo-agent-skills Specification

## Purpose
TBD - created by archiving change agent-skills. Update Purpose after archive.

## Requirements

### Requirement: No AI attribution in commits
yardo SHALL keep coding agents from adding attribution to commits and pull requests: Claude Code's settings SHALL have `attribution.commit` and `attribution.pr` set to empty strings, merged so the user's other settings stay, and the agent notes SHALL say not to add such lines.

#### Scenario: Claude Code settings
- **WHEN** the agent skills item is applied for a user with Claude Code
- **THEN** `~/.claude/settings.json` has `"attribution": {"commit": "", "pr": ""}`, and its other keys are as they were

### Requirement: Agent trailers stripped by a git hook
On Git 2.54 or later, yardo SHALL set a global config-based `commit-msg` hook that removes trailer lines naming a coding agent (`noreply@anthropic.com`, `noreply@openai.com`, `cursoragent@cursor.com`, "Generated with [Claude Code]" or "[Codex]") and no other line. yardo SHALL NOT set a global `core.hooksPath`.

#### Scenario: Agent trailer stripped, person's kept
- **WHEN** a commit message ends with `Co-Authored-By: Claude <noreply@anthropic.com>` and `Co-authored-by: Ada <ada@example.com>`
- **THEN** the hook leaves `Co-authored-by: Ada <ada@example.com>` and removes the Claude line

#### Scenario: Repo hooks still run
- **WHEN** a repo has its own `.git/hooks/commit-msg`, or husky
- **THEN** that hook still runs alongside yardo's

#### Scenario: Older git on Ubuntu
- **WHEN** `git --version` is older than 2.54 on Ubuntu
- **THEN** git is upgraded from the git-core PPA (its signing key checked against its fingerprint), and the hook is set

#### Scenario: Older git elsewhere
- **WHEN** `git --version` is older than 2.54 and git can't be upgraded (Debian)
- **THEN** no hook is set, and `~/yardo.txt` says why

#### Scenario: Codex attribution
- **WHEN** Codex is installed
- **THEN** `~/yardo.txt` says Codex's attribution is a ChatGPT account setting, and where to turn it off

### Requirement: yardo's skills
yardo SHALL ship `git-commits`, `pr-descriptions` and `jira-tickets` as skills in the agentskills.io format, with only portable frontmatter (`name`, `description`, `license`, `compatibility`, `metadata`), each under 100 lines. `jira-tickets` SHALL ask for a title, 2–5 plain sentences and at most 5 acceptance criteria, with no headings, bold labels, emojis or boilerplate sections.

#### Scenario: Installed for every agent
- **WHEN** the agent skills item is applied
- **THEN** each skill is in `~/.agents/skills/<name>/SKILL.md` and `~/.claude/skills/<name>/SKILL.md`

#### Scenario: Valid skills
- **WHEN** the tests run
- **THEN** every shipped skill passes the agentskills.io rules (name matches its folder, lowercase and hyphens, description at most 1024 characters)

### Requirement: Community skills, opt-in and pinned
Community skills SHALL be offered as catalog items off by default, each vendored at a full commit with its licence files, listed in `skills.lock.toml` (repo, path, commit, SPDX licence, tree sha256). A skill without a licence that permits redistribution SHALL NOT be vendored. yardo SHALL NOT fetch skills at install time.

#### Scenario: Lock matches files
- **WHEN** the tests run
- **THEN** each vendored skill's tree hash equals its lock entry, and its licence file is present

### Requirement: yardo changes only its own skill folders
Each skill folder yardo writes SHALL carry a `.yardo-skill` marker with the content hash. yardo SHALL update a folder only while it still matches its marker; a folder the user changed, or made without a marker, SHALL be left alone. An unticked skill SHALL stay installed, as every unticked item does.

#### Scenario: Edited skill kept
- **WHEN** the user edited `~/.claude/skills/jira-tickets/SKILL.md` and yardo ships a new version
- **THEN** yardo leaves that copy, and updates the one in `~/.agents/skills` it still owns

#### Scenario: A skill of the user's own
- **WHEN** `~/.claude/skills/git-commits/` exists without yardo's marker
- **THEN** yardo leaves it as it is
