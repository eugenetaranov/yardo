## REMOVED Requirements

### Requirement: Updates in one step
**Reason**: The Updates screen is replaced by the Updates tab on Tools, where the user picks which tools to update.
**Migration**: The home's "Install N updates" row opens the tab with every update queued: enter, then `a`, updates them all (see "Install updates opens the Updates tab").

## ADDED Requirements

### Requirement: Install updates opens the Updates tab
Enter on the home's "Install N updates" row SHALL open Tools on the Updates tab with every update queued (pinned ones excepted); `a` there SHALL start the update, and `q` SHALL go back home with the queue kept. There SHALL be no other button or question between the list and the run. yardo SHALL NOT have an Updates screen apart from that tab.

#### Scenario: Two keys
- **WHEN** the user presses enter on "Install 3 updates", then `a`
- **THEN** the run updating the three starts

#### Scenario: Leave some out
- **WHEN** the user presses enter on "Install 3 updates", space on node, then `a`
- **THEN** the run updates the other two only
