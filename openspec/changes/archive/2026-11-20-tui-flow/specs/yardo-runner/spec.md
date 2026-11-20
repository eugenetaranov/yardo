# Spec Delta

## MODIFIED Requirements

### Requirement: Log and summary
Every command the runner executes SHALL be appended with its output to `/var/log/yardo.log`. On failure, the terminal SHALL show the last lines of the failing command's output. The progress line SHALL count the steps (`12 of 43 · <step>`). What the run prints after its steps (result lines, failing output, notes, hints) SHALL be kept for the home's Last run screen (see `yardo-last-run`). The run SHALL write `~/yardo.txt`, grouping what it installed by mise, apt, and other sources, followed by the "Secrets to fill in" section from `yardo-credentials`.

#### Scenario: Failure output
- **WHEN** a step's command fails
- **THEN** the terminal shows its last output lines and the log path

#### Scenario: Kept for Last run
- **WHEN** a step's command fails and the home comes back
- **THEN** Last run shows the same last output lines and the log path
