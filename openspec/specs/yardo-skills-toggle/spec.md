# yardo-skills-toggle Specification

## Purpose
Turning the user's agent skills on and off, each or all at once, so a skill that's rarely needed, or not trusted yet, stays out of every agent's context without being lost.

## Requirements

### Requirement: A skill that's off is hidden from every agent
Turning a skill off SHALL move its folder out of `~/.claude/skills` and `~/.agents/skills`, wherever it is, into `~/.local/share/yardo/skills-off/`, keeping which of the two it came from. Turning it on SHALL put each folder back where it was, with its files, modes and yardo's marker unchanged.

#### Scenario: Turned off
- **WHEN** `tdd` is in both skills folders and the user turns it off
- **THEN** neither skills folder has `tdd`, and a new Claude Code or Codex session doesn't list it

#### Scenario: Turned back on
- **WHEN** the user turns `tdd` on again
- **THEN** both folders have `tdd`, byte for byte as before

#### Scenario: Only in one place
- **WHEN** `my-notes` is only in `~/.claude/skills` and is turned off, then on
- **THEN** it is back in `~/.claude/skills` only

### Requirement: Every skill is listed
The Skills screen and `yardo skills` SHALL list every skill folder in either skills folder or held off, once per name. Each SHALL show whether it's on, and where it's from: yardo, yardo but changed by the user, or not yardo's. A folder without a `SKILL.md` SHALL NOT be listed.

#### Scenario: Mixed sources
- **WHEN** `git-commits` is yardo's, `archify` was installed by another tool and `my-notes` was made by the user
- **THEN** all three are listed, `git-commits` as yardo's and the other two as not yardo's

### Requirement: The choice persists
A skill that's off SHALL stay off through a yardo run, a profile, a yardo update, and a reinstall of its item. When yardo updates a skill of its own that is off, it SHALL update the held copy, following the same rules as in the skills folders. Uninstalling a skill's item SHALL remove its held copy too.

#### Scenario: yardo runs
- **WHEN** `tdd` is off and `sudo yardo --profile team.toml --yes` applies the community skills
- **THEN** `tdd` is still off, and no skills folder has it

#### Scenario: Updated while off
- **WHEN** `jira-tickets` is off and yardo ships a new version of it
- **THEN** the held copy is the new version, and turning it on installs that

### Requirement: All skills off at once
The user SHALL be able to turn all skills off in one action, and back on. While all are off, a skill that yardo installs or that appears in a skills folder SHALL be turned off on the next yardo run or screen refresh. Turning all back on SHALL return each skill to its own choice. While all are off, turning one skill on SHALL NOT be possible, and yardo SHALL say to turn all skills on first.

#### Scenario: Off and back
- **WHEN** `humanizer` is off by itself, and the user turns all skills off, then on
- **THEN** every other skill is on and `humanizer` is off

### Requirement: Switching on the Skills screen
On the Skills screen, space SHALL mark the selected skill off or on, and an **All skills** row above the skills SHALL mark them all. Changes SHALL show at once as not saved. `a` SHALL apply every change marked and say what changed, with the screen staying open. Leaving with unsaved changes SHALL ask once first. A skill's own screen SHALL offer **Turn it off** or **Turn it on**.

#### Scenario: Mark, then apply
- **WHEN** the cursor is on `grill-me`, and the user presses space, then `a`
- **THEN** `grill-me` reads `○ off`, not saved, at once; after `a` it is off, and the screen says so

#### Scenario: Leaving unsaved
- **WHEN** a change is marked and the user presses `q`
- **THEN** the screen says it isn't saved and stays; `q` again leaves without it

### Requirement: Switching from the command line
`sudo yardo skills` SHALL list the skills with their state. `sudo yardo skills off [skill …]` and `sudo yardo skills on [skill …]` SHALL turn the named skills off or on, or all skills when none is named, and print each one's state afterwards. An unknown skill SHALL be an error that lists the skills there are.

#### Scenario: One skill
- **WHEN** the user runs `sudo yardo skills off tdd`
- **THEN** `tdd` is off, and the output says so

#### Scenario: Unknown
- **WHEN** the user runs `sudo yardo skills off nope`
- **THEN** it fails, naming the skills there are

### Requirement: A clash is not overwritten
When a skill is turned on and a folder of the same name has appeared in its skills folder meanwhile, yardo SHALL leave both as they are, keep the skill off, and say which folder is in the way.

#### Scenario: Same name came back
- **WHEN** `tdd` is off, another tool installs a new `~/.claude/skills/tdd`, and the user turns `tdd` on
- **THEN** the new folder is unchanged, the held copy is kept, and yardo says `~/.claude/skills/tdd` is in the way

### Requirement: Adding skills from the Skills screen
`n` on the Skills screen SHALL ask for one or more GitHub sources (`owner/repo[/path][@ref]`). When a source holds several skills, yardo SHALL list them to pick from. Each picked skill SHALL go through checking before it is installed, with its report shown on the screen.

#### Scenario: A repo with several skills
- **WHEN** the user presses `n` and enters `acme/skills`, which has `a`, `b` and `c`
- **THEN** yardo lists the three to pick, and checks each one picked

### Requirement: The Skills screen shows what was checked
Each skill's row SHALL say whether it's yardo's, checked (with the date it was approved), held (waiting for a decision), or not checked. A skill that isn't yardo's and isn't checked SHALL offer **Check it** on its screen.

#### Scenario: Not checked
- **WHEN** `extra` was installed by another tool
- **THEN** its row reads not checked, and its screen offers Check it
