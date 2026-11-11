## MODIFIED Requirements

### Requirement: Every visible item is removable or says why not
A visible item in the shipped catalog SHALL be removable (each step has an undo, a Go step that plans its own, or the item has a `remove` script or no steps), SHALL set `keep_say`, or SHALL be `protected`. Catalog validation SHALL fail a shipped item that is none of these; an item from `/etc/yardo/catalog.d` SHALL load either way. Space on an installed item yardo can't uninstall SHALL say why and offer to forget it; on a `protected` item it SHALL only say why.

#### Scenario: Not removable
- **WHEN** the user presses space on ECC
- **THEN** the board says why yardo can't uninstall it and asks whether to forget it instead

#### Scenario: Protected
- **WHEN** the user presses space on zsh
- **THEN** the board says why and queues nothing

#### Scenario: Validation
- **WHEN** a shipped catalog item has a `run` step, no `remove`, no `keep_say` and isn't `protected`
- **THEN** loading the catalog fails naming the item

#### Scenario: Hardening item
- **WHEN** the user presses space on SSH hardening
- **THEN** the board says why yardo won't undo it, and offers to forget it

#### Scenario: Local item
- **WHEN** an item in `/etc/yardo/catalog.d` has a `run` step, no `remove` and no `keep_say`
- **THEN** the catalog loads, and space on it says yardo can't uninstall it

### Requirement: The board queues uninstalls
On the Tools board, space on an installed, removable item SHALL queue it for uninstall, marked `[−]` and "will uninstall"; space again SHALL take it off the queue. `d` SHALL queue the row's uninstall and open the question at once. `a` SHALL apply uninstalls, then forgets, then adds, in one run. A pending row SHALL show no health mark, and the queue line SHALL name each change in words.

#### Scenario: Untick
- **WHEN** the user presses space on installed AWS tools, then `a`, then `y`
- **THEN** AWS tools is uninstalled

#### Scenario: d
- **WHEN** the user presses `d` on NetBird
- **THEN** the question to uninstall NetBird opens

#### Scenario: Toggle
- **WHEN** the user presses space twice on AWS tools
- **THEN** AWS tools is queued, then taken off the queue

#### Scenario: NetBird and AWS together
- **WHEN** the user presses space on NetBird, space on AWS tools, then `a`
- **THEN** one question lists both, and one run removes both

#### Scenario: Re-added by an add
- **WHEN** an uninstall is queued with an add that needs that item
- **THEN** the question says the add brings it back

### Requirement: Uninstall and stop managing are told apart
Forgetting an item (yardo stops checking it; it stays installed) SHALL be offered only in its details panel and when yardo can't uninstall it, marked "will forget". The words SHALL be "uninstall" for what goes and "forget" for what stays, everywhere.

#### Scenario: Footer
- **WHEN** the cursor is on an installed row
- **THEN** the footer offers space to uninstall and does not offer forgetting

### Requirement: The confirmation says what goes, stays and is left
Before applying, the question SHALL be titled by what it does ("Uninstall 2 tools?" when only uninstalling), SHALL name each tool by its short name, and SHALL give for each: what its removal does, a `Stays:` line (what another item uses, what was edited, what its removal leaves), the tools that go with it, the tools forgotten with it, and a queued add that needs it back. The full list of what goes SHALL be one choice away. Indented lines SHALL stay indented when wrapped. `y` and `n` SHALL answer it.

#### Scenario: AWS tools
- **WHEN** AWS tools is queued and Shell helpers is installed
- **THEN** the question lists `Stays:` with fzf (Shell helpers uses it) and doesn't list package ids until asked

#### Scenario: Shared package
- **WHEN** AWS tools is queued for uninstall and Shell helpers is installed
- **THEN** fzf is not in what goes, even in the full list

### Requirement: Each removal reports how it went
After applying, the board SHALL show a panel naming how many were uninstalled and failed, with a line per item, until the user dismisses it. A failed item SHALL stay installed and the panel SHALL offer to retry it. When apt reports packages it could autoremove, the panel SHALL say so with `sudo apt autoremove`.

#### Scenario: One fails
- **WHEN** NetBird's removal fails and AWS's succeeds
- **THEN** the panel says 1 uninstalled, 1 failed, and offers Retry NetBird

### Requirement: yardo remove from the shell
`yardo remove <item>…` SHALL uninstall the named items, each by its short name or id, case-insensitive (a tool before its parts), and what needs them. It SHALL print the same plan as the board and ask; `--yes` SHALL skip the question and `--dry-run` SHALL print the plan and change nothing. An unknown or uninstalled name, or one yardo can't uninstall, SHALL be reported and the command SHALL exit 2 without removing anything. A failed removal SHALL make it exit 1 after the others have run.

#### Scenario: Dry run
- **WHEN** the user runs `yardo remove aws netbird --dry-run`
- **THEN** yardo prints what each would take off and keep, and what goes with it, and changes nothing

#### Scenario: Not removable
- **WHEN** the user runs `yardo remove github aws`
- **THEN** yardo says why it can't uninstall GitHub, removes nothing and exits 2

## REMOVED Requirements

### Requirement: A login is removed with its client
**Reason**: A login is now part of its client's row (see "A tool is one row").
**Migration**: `remove_with` becomes `part_of`.

## ADDED Requirements

### Requirement: A tool is one row
An item MAY set `part_of` to an item it needs. The board SHALL NOT list it; its parent's row SHALL show its health and the values it waits for, and its parent's details SHALL offer "Log in" when it isn't installed and "Log out (<parent> stays)" when it is and yardo can remove it. Fixing the parent SHALL fix its installed parts too. Uninstalling the parent SHALL uninstall its parts.

#### Scenario: NetBird
- **WHEN** NetBird and its login are installed
- **THEN** the board lists one NetBird row, and its details offer to log out

### Requirement: What needs a tool goes with it
Uninstalling an item SHALL uninstall the installed items that need it first, said in the question before anything changes. One yardo can't uninstall SHALL be forgotten instead, and said.

#### Scenario: Node
- **WHEN** the user uninstalls Node while Gemini CLI is installed
- **THEN** the question says Gemini CLI goes with it, and both are uninstalled

### Requirement: An uninstall sticks
When an uninstalled item is on a named profile, yardo SHALL record that it was uninstalled; applying a profile SHALL skip it with a warning naming it. Adding it on Tools SHALL clear the record.

#### Scenario: Team profile
- **WHEN** the user uninstalled AWS tools, which the team profile lists, and applies the profile again
- **THEN** AWS tools isn't installed, and the run says it was skipped because the user uninstalled it

### Requirement: Typing filters the board
Text typed on the home or after `/` SHALL filter the board's rows. Enter or ↓ SHALL leave the text and move into the rows with the filter kept, where every key works; esc SHALL clear the filter.

#### Scenario: Typed, then untick
- **WHEN** the user types `netb` on the home, presses enter, then space
- **THEN** NetBird is queued for uninstall
