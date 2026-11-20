# yardo-tui-screens Specification

## Purpose
The screens `sudo yardo` shows after the first run: one program whose views are the home, Tools, Settings, Fix, Updates, Connection and MCP servers, what each keeps as you move between them, and the key map they all share.

## Requirements

### Requirement: One program, one screen
The home, Tools, Settings (Network access, Look), Fix, Fill in, Updates, Connection and the MCP servers screens SHALL be views of one program in one alternate screen. Moving between them SHALL NOT leave and re-enter the alternate screen, and SHALL NOT read the machine's status again. Only a prompt that asks a question and a run SHALL leave the screen.

#### Scenario: Home to Tools and back
- **WHEN** the user presses enter on Tools, then `q`
- **THEN** the home shows again without "Checking what's set up" and without the terminal's own screen showing in between

### Requirement: Screens keep their state
Leaving a screen for another SHALL keep its state until the program ends: the Tools board's queue, tab, each tab's cursor and its filter; the home's cursor; the MCP list's cursor. The home's Tools row SHALL say how many changes are queued. `q` on the home with a queue SHALL ask once; `q` again SHALL quit and drop it.

#### Scenario: Queue kept across a hop
- **WHEN** the user ticks Java on Tools, presses `q`, then enter on Tools
- **THEN** Java is still queued and the board is on the tab it was left on

#### Scenario: Quit with a queue
- **WHEN** the user has one change queued and presses `q` on the home
- **THEN** the home says 1 change isn't applied and that `q` again quits without it

### Requirement: Search typed on the home is live
Typing a letter or `/` on the home SHALL show the Tools board filtered by what's typed, with each further key narrowing it, in the same screen.

#### Scenario: Type on the home
- **WHEN** the user types `ne` on the home
- **THEN** the Tools board shows filtered to `ne` with the cursor in the search field

### Requirement: One key map
Every screen SHALL take the same keys for the same things: ↑↓ (`j` `k`) move; ←→ (`h` `l`) switch tabs on Tools, pick a choice on a Network access or Look row, and move along Go to on the home; tab goes to the next panel or tab; enter opens or acts on the row; space ticks (queue, tick to fix, on or off); `a` applies what's queued or changed; `n` adds; `d` deletes or uninstalls, asking first; `e` edits; `/` searches; `?` shows the keys; `q` goes back, and quits on the home; ctrl+c quits anywhere.

#### Scenario: a on MCP servers
- **WHEN** the user marks a server off on MCP servers and presses `a`
- **THEN** the change is saved, as Save did

#### Scenario: n on MCP servers
- **WHEN** the user presses `n` on MCP servers
- **THEN** the catalog of servers to add opens

#### Scenario: s is no key
- **WHEN** the user presses `s` on any screen
- **THEN** nothing happens

#### Scenario: Left on the home
- **WHEN** the cursor is on a Needs you row and the user presses →
- **THEN** the cursor moves to the first Go to entry

#### Scenario: Vim keys on the home
- **WHEN** the cursor is on Go to's Tools entry and the user presses `l`
- **THEN** the cursor moves to the next Go to entry

### Requirement: The footer lists every key the screen takes
Each screen's bottom line SHALL list every key that does something on it, with what it does, in the same place and style on every screen, and SHALL show the apply key in the accent colour while something is queued or changed. A toast SHALL take the line above the footer, never the footer itself.

#### Scenario: d on Tools
- **WHEN** the cursor is on an installed tool on Tools
- **THEN** the footer lists `d uninstall`

#### Scenario: Flash keeps the keys
- **WHEN** the board says "Nothing queued" after `a`
- **THEN** that line is above the footer and the footer still lists the keys

### Requirement: Every screen has the same header
Each screen's top line SHALL show yardo's name, the host, the user and the screen's path from the home (`Home › Tools › Dev`, `Home › Settings › Look`), in the same style on every screen.

#### Scenario: Tools' path
- **WHEN** the user is on the Dev tab of Tools
- **THEN** the header ends with `Home › Tools › Dev`

### Requirement: ? shows the keys
`?` on any screen SHALL open a keys screen listing every key of the screen it was pressed on, then the keys every screen shares, each with what it does; `q`, esc or `?` SHALL close it. The footer SHALL list `?`.

#### Scenario: ? on Tools
- **WHEN** the user presses `?` on Tools
- **THEN** a screen lists space, enter, `a`, `d`, `/`, ←→, tab, then the shared keys, and `q` returns to Tools on the same row

### Requirement: Every list pages the same way
On every list, PgUp and PgDn SHALL move a screenful, Home and `g` SHALL go to the first row, End and `G` to the last, and the list SHALL scroll to keep the cursor in view.

#### Scenario: G on Tools
- **WHEN** the user presses `G` on the Installed tab
- **THEN** the cursor is on the last row and the list has scrolled to show it

### Requirement: One cursor, one dialog
The row under the cursor SHALL look the same on every screen (one highlighted row, no second pointer character). Every question (apply, uninstall, delete, quit with a queue) SHALL be one framed dialog over the screen, its options as rows, answered with enter, `y` or `n`, and left with `q` or esc, nothing changed.

#### Scenario: Delete an MCP server
- **WHEN** the user presses `d` on an MCP server
- **THEN** the question is the same framed dialog the Tools uninstall question is, with `y`/`n` in its footer

### Requirement: The mouse works
A click on a row SHALL move the cursor to it, a double-click SHALL act on it as enter does, a click on a tab or a Go to entry SHALL go there, a click on a key in the footer SHALL press it, and the wheel SHALL scroll the list under the pointer. A terminal without mouse reporting SHALL lose nothing.

#### Scenario: Click a tab
- **WHEN** the user clicks DevOps in the tab bar
- **THEN** the DevOps tab opens

#### Scenario: Wheel on the home
- **WHEN** the user scrolls the wheel over Needs you
- **THEN** the cursor moves through its rows

### Requirement: Fix shows its details
The Fix screen SHALL show each problem's detail under its row without a key; enter SHALL fix the ticked rows. No key on it SHALL mean anything other than the shared map.

#### Scenario: Details shown
- **WHEN** Fix opens with a drifted file
- **THEN** the file's path is under its row at once

### Requirement: Check for updates from the home
`u` on the home SHALL start a check for updates in the background and say so. While updates were never checked, Needs you SHALL have a row "Check for updates" that enter starts. Settings SHALL NOT have a Check for updates row.

#### Scenario: u
- **WHEN** the user presses `u` on the home
- **THEN** the home says it's checking and, when done, shows what it found

### Requirement: Connection is one thing
The home's panel and the screen SHALL both be named Connection. Tab SHALL reach the panel, and enter on it SHALL open the screen. Go to SHALL list Connection too, and enter on it SHALL open the same screen.

#### Scenario: Open from the panel
- **WHEN** the user presses tab until the Connection panel is on, then enter
- **THEN** the Connection screen opens

#### Scenario: Open from Go to
- **WHEN** the user moves to Connection in Go to and presses enter
- **THEN** the Connection screen opens

### Requirement: Needs you has the full width
The Machine panel SHALL be two lines at every width, and Needs you SHALL be the full width of the screen, with its rows' notes on the same line as the row wherever they fit.

#### Scenario: Wide screen
- **WHEN** the terminal is 120 columns wide
- **THEN** Needs you is one box the width of the screen and Machine is two lines under it

### Requirement: Install updates opens the Updates tab
Enter on the home's "Install N updates" row SHALL open Tools on the Updates tab with every update queued (pinned ones excepted); `a` there SHALL start the update, and `q` SHALL go back home with the queue kept. There SHALL be no other button or question between the list and the run. yardo SHALL NOT have an Updates screen apart from that tab.

#### Scenario: Two keys
- **WHEN** the user presses enter on "Install 3 updates", then `a`
- **THEN** the run updating the three starts

#### Scenario: Leave some out
- **WHEN** the user presses enter on "Install 3 updates", space on node, then `a`
- **THEN** the run updates the other two only
