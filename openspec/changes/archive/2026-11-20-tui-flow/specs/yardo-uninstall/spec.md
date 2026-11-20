# Spec Delta

## MODIFIED Requirements

### Requirement: The board queues uninstalls
On the Tools board, space on an installed, removable item SHALL queue it for uninstall, marked `[−]` and "will uninstall"; space again SHALL take it off the queue. `d` SHALL queue the row's uninstall and open the question at once, and SHALL be listed in the footer. `a`, the apply key every screen shares (see `yardo-tui-screens`), SHALL apply uninstalls, then forgets, then adds, in one run. A pending row SHALL show no health mark, and the queue line SHALL name each change in words.

#### Scenario: Untick
- **WHEN** the user presses space on installed AWS tools, then `a`, then `y`
- **THEN** AWS tools is uninstalled

#### Scenario: d
- **WHEN** the user presses `d` on NetBird
- **THEN** the question to uninstall NetBird opens

#### Scenario: d in the footer
- **WHEN** the cursor is on an installed tool
- **THEN** the footer lists `d uninstall`

#### Scenario: Toggle
- **WHEN** the user presses space twice on AWS tools
- **THEN** AWS tools is queued, then taken off the queue

#### Scenario: NetBird and AWS together
- **WHEN** the user presses space on NetBird, space on AWS tools, then `a`
- **THEN** one question lists both, and one run removes both

#### Scenario: Re-added by an add
- **WHEN** an uninstall is queued with an add that needs that item
- **THEN** the question says the add brings it back

### Requirement: Typing filters the board
Text typed on the home or after `/` SHALL filter the board's rows, in the same screen, each key narrowing it. Enter or ↓ SHALL leave the text and move into the rows with the filter kept, where every key works; esc SHALL clear the filter.

#### Scenario: Typed, then untick
- **WHEN** the user types `netb` on the home, presses enter, then space
- **THEN** NetBird is queued for uninstall

#### Scenario: Live from the home
- **WHEN** the user types `n` then `e` on the home
- **THEN** the board shows filtered to `n`, then to `ne`, without the screen restarting
