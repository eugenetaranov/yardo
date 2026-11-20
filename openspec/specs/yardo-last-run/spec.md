# yardo-last-run Specification

## Purpose
How a run's result reaches the user once the screen comes back: what the toast says, the Last run screen, the keys a run made, and the progress line while it runs.

## Requirements

### Requirement: The toast says how it went
After a run, the home's toast SHALL say whether it succeeded, naming what it fixed or installed, or failed, naming what failed. It SHALL NOT tell the user to quit to see the output.

#### Scenario: Failed run
- **WHEN** a run of Docker Engine fails
- **THEN** the toast reads `✗ Docker Engine failed · Last run shows what happened`

### Requirement: Last run is a screen
Go to SHALL have a Last run entry once a run has happened, with when and how it went. Its screen SHALL show each step's result line, the last lines of each failing step's output, the run's notes and hints, and the log's path. The home SHALL have a Needs you row "See why the last run failed" while the last run failed, opening the same screen.

#### Scenario: Failed step's output
- **WHEN** the user opens Last run after a run where apt failed
- **THEN** the screen shows the apt step marked failed, its last output lines, and `/var/log/yardo.log`

#### Scenario: Previous session
- **WHEN** `sudo yardo` opens the day after a run
- **THEN** Go to still has Last run, saying when that run was

### Requirement: Keys to add stay on Connection
A public key a run makes (a git host's, or the machine's) SHALL be listed on the Connection screen under Keys to add, with where to add it and the key alone on a line, until the user dismisses it with `d` or the host it is for reads working on Tools. The key SHALL NOT be shown only in the terminal's output.

#### Scenario: First run's GitHub key
- **WHEN** a first run makes a GitHub key and the home opens
- **THEN** Connection lists the key with "GitHub: add it at github.com/settings/keys" and the key on its own line

#### Scenario: Dismiss
- **WHEN** the user presses `d` on a key under Keys to add
- **THEN** the key leaves the list and the next `sudo yardo` doesn't list it

### Requirement: The progress line counts the steps
While a run is on, the spinner's line SHALL say which step this is of how many, the step's name and how long it has taken: `12 of 43 · Docker Engine (1m20s)`. Result lines SHALL keep the step's name without the count.

#### Scenario: Count
- **WHEN** a run of 43 steps is on its twelfth
- **THEN** the spinner reads `12 of 43 · <step> (<time>)`
