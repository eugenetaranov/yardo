# yardo-agent-skills Specification

## Purpose
The agent setup yardo installs for coding agents: the notes describing the machine, safety settings for unattended agents, and the shared skills in ~/.agents/skills.

## Requirements

### Requirement: No AI attribution in commits
yardo SHALL keep coding agents from adding attribution to commits and pull requests on every machine, in an item that is always in the plan and can't be unticked or removed: Claude Code's settings SHALL have `attribution.commit` and `attribution.pr` set to empty strings, merged so the user's other settings stay, and the agent notes SHALL say not to add such lines.

#### Scenario: Claude Code settings
- **WHEN** the agent skills item is applied for a user with Claude Code
- **THEN** `~/.claude/settings.json` has `"attribution": {"commit": "", "pr": ""}`, and its other keys are as they were

### Requirement: Agent trailers stripped by a git hook
On Git 2.54 or later, yardo SHALL set a global config-based `commit-msg` hook that removes trailer lines naming a coding agent (Claude, Codex, Cursor, Gemini, OpenCode, Goose, Aider, Copilot, Amp and others, by name or by their noreply address; "Generated with [Claude Code]" or "[Codex]"; Amp's thread link) and no other line. yardo SHALL NOT set a global `core.hooksPath`.

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
yardo SHALL ship `git-commits`, `git-branches`, `pr-descriptions` and `jira-tickets` as skills in the agentskills.io format, with only portable frontmatter (`name`, `description`, `license`, `compatibility`, `metadata`), each under 100 lines. `jira-tickets` SHALL ask for a title, 1–3 plain sentences and 3–5 acceptance criteria, each an observable pass/fail result and required on every story, task and bug, with no headings, bold labels, emojis or boilerplate sections. `git-branches` SHALL name a branch `<KEY>-<short-description>` when there's a Jira ticket (its key as Jira shows it, then 2–5 lowercase words joined with hyphens), else the description alone; a project's convention comes first: one its docs state, a prefix in `git config yardo.branchPrefix` (per repo or global, which `git-make-branch` uses too), or a prefix its recent branches share.

#### Scenario: Installed for every agent
- **WHEN** the agent skills item is applied
- **THEN** each skill is in `~/.agents/skills/<name>/SKILL.md` and `~/.claude/skills/<name>/SKILL.md`

#### Scenario: Valid skills
- **WHEN** the tests run
- **THEN** every shipped skill passes the agentskills.io rules (name matches its folder, lowercase and hyphens, description at most 1024 characters)

### Requirement: Community skills, opt-in and pinned
Community skills SHALL be offered as catalog items off by default, each vendored at a full commit with its licence files, listed in `skills.lock.toml` (repo, path, commit, SPDX licence, tree sha256). A skill without a licence that permits redistribution SHALL NOT be vendored. yardo SHALL NOT fetch skills at install time, except one too big to ship inside yardo (Archify): that one SHALL be listed in `skills.lock.toml` the same way, fetched from its repository at the locked commit, and installed only when its tree hash matches the lock. A fetched skill whose hash doesn't match SHALL NOT be installed, and the run SHALL say so.

#### Scenario: Lock matches files
- **WHEN** the tests run
- **THEN** each vendored skill's tree hash equals its lock entry, and its licence file is present

#### Scenario: Archify installed
- **WHEN** the archify item is applied
- **THEN** `~/.claude/skills/archify` and `~/.agents/skills/archify` hold the locked commit's skill, each with yardo's marker, and no `npx` command ran

#### Scenario: Archify changed upstream
- **WHEN** the files fetched for Archify's locked commit don't match its tree hash
- **THEN** nothing is installed, and the run says Archify's files didn't match

### Requirement: yardo changes only its own skill folders
Each skill folder yardo writes SHALL carry a `.yardo-skill` marker with the content hash. yardo SHALL update a folder only while it still matches its marker; a folder the user changed, or made without a marker, SHALL be left alone. An unticked skill SHALL stay installed, as every unticked item does.

#### Scenario: Edited skill kept
- **WHEN** the user edited `~/.claude/skills/jira-tickets/SKILL.md` and yardo ships a new version
- **THEN** yardo leaves that copy, and updates the one in `~/.agents/skills` it still owns

#### Scenario: A skill of the user's own
- **WHEN** `~/.claude/skills/git-commits/` exists without yardo's marker
- **THEN** yardo leaves it as it is

### Requirement: Skill writes don't follow links
yardo SHALL NOT write a skill's files through a symbolic link. When `~/.agents/skills`, `~/.claude/skills`, a skill folder or anything under it is a link, yardo SHALL skip that skill in that place, leave the link as it is, and the run SHALL say which path and why.

#### Scenario: Skills folder links elsewhere
- **WHEN** `~/.agents/skills` is a link to `/etc/systemd/system` and the agent skills item is applied
- **THEN** nothing is written under `/etc/systemd/system`, the skills go to `~/.claude/skills` as usual, and the run says `~/.agents/skills` is a link and was skipped

#### Scenario: A link inside a skill folder
- **WHEN** `~/.claude/skills/git-commits/SKILL.md` is a link to `~/.ssh/config`
- **THEN** yardo doesn't write through it, and `~/.ssh/config` is unchanged

### Requirement: Skill names are checked
yardo SHALL install a skill only when its name is 1–64 characters of lowercase letters, digits and hyphens, not starting or ending with a hyphen, as agentskills.io requires. Any other name SHALL be an error naming the skill.

#### Scenario: A path for a name
- **WHEN** a skill is named `../x.service.d`
- **THEN** it isn't installed, and the run says the name isn't valid
