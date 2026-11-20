## Purpose

How yardo finds newer versions of the tools it installed, without making anyone wait, shows them on a tab of their own, and updates only the ones the user picks.

## ADDED Requirements

### Requirement: Updates are checked in the background
yardo SHALL check apt and mise for newer versions of the user's items in the background: when the screen opens and the last check is over a day old, and when the user presses `u` on the home or the Updates tab. No screen SHALL wait for the check, and every key SHALL keep working while it runs. A run that uses apt SHALL wait for a check still running, saying so. The result SHALL be kept between runs.

#### Scenario: Screen opens with an old check
- **WHEN** `sudo yardo` opens and the last check is 30 hours old
- **THEN** the home is usable at once, and a new check runs behind it

#### Scenario: Check again
- **WHEN** the user presses `u` on the Updates tab
- **THEN** the tab says "Checking for updates…" and fills in with what the check finds, without leaving the tab

### Requirement: The Updates tab
Tools SHALL have an Updates tab next to Installed while a check is running or has found newer versions. It SHALL list one row per tool with a newer version: its name, the version installed and the version out, and where it comes from (mise or apt). Extra mise tools SHALL be listed too. With no newer versions and no check running, the tab SHALL NOT be shown.

#### Scenario: Three updates
- **WHEN** a check found newer node, gh and uv
- **THEN** the Updates tab lists node, gh and uv, each with its versions, e.g. `node 22.12 → 24.11`

#### Scenario: Nothing newer
- **WHEN** a check finds nothing newer
- **THEN** Tools has no Updates tab, and the toast says everything is current

### Requirement: Updating the tools picked
Space on an Updates row SHALL queue that tool for update (`[↑]`) or take it off the queue, using the same queue as adds and uninstalls. `a` SHALL apply the whole queue in one run, and that run SHALL update only the queued tools, each to its latest version. When the run is done, the tools it updated SHALL leave the list, and the others SHALL stay until the next check. The run SHALL report like any run: the progress line, the toast, Last run.

#### Scenario: One of three
- **WHEN** the Updates tab lists node, gh and uv, and the user queues gh and presses `a`
- **THEN** only gh is updated, and the tab still lists node and uv

#### Scenario: With an add
- **WHEN** the user queues Java to add and gh to update, then presses `a`
- **THEN** one run installs Java and updates gh

### Requirement: Pinned versions stay
A tool whose version a team profile pins SHALL be listed with "pinned by your profile" and SHALL NOT be queued for update.

#### Scenario: Pinned terraform
- **WHEN** a profile pins terraform and a newer terraform is out
- **THEN** the Updates tab lists terraform as pinned, and space on it does nothing but say why
