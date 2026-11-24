## ADDED Requirements

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

## MODIFIED Requirements

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
