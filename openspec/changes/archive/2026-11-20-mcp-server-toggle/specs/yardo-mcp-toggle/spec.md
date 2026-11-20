## Purpose

Turning the gateway's MCP servers on and off, each or all at once, so a server that's rarely needed stays out of agents' context and out of their reach until it's needed, without losing its settings.

## ADDED Requirements

### Requirement: A server that's off is hidden from every client
A server that's off SHALL stay registered with the gateway, with its settings in `~/.mcp/<name>`, and the gateway SHALL leave its tools out of the tools it lists to every client (the machine's agents and any other client, a laptop's included) and SHALL refuse calls to them. Servers that are on SHALL be unaffected. Turning a server on SHALL make its tools listed and callable again without registering it again.

#### Scenario: Turned off
- **WHEN** servers `github` and `aws` are registered and the user turns `aws` off
- **THEN** a new agent session lists `github__*` tools and no `aws__*` tool, and a call to `aws__call_aws` fails as an unknown tool

#### Scenario: Turned back on
- **WHEN** the user turns `aws` on again
- **THEN** a new agent session lists the `aws__*` tools, and `~/.mcp/aws/env` is as it was

### Requirement: The choice persists
Whether a server is off SHALL survive: a sync of the servers (its re-registration with new settings included), a rename (the choice moves with it), a restart of the gateway or the machine, and a yardo run that applies the user's picks or a profile. The choice SHALL NOT be stored in yardo's answers or exported in profiles. Deleting a server SHALL delete its choice with its settings.

#### Scenario: Settings edited while off
- **WHEN** `jira` is off and the user edits its settings, so it registers again
- **THEN** `jira` is still off afterwards

#### Scenario: Profile applied
- **WHEN** `aws` is off and `sudo yardo --profile team.toml --yes` runs with `aws` in the profile
- **THEN** `aws` is still off

#### Scenario: Renamed
- **WHEN** `jira-work` is off and the user renames it `jira-acme`
- **THEN** `jira-acme` is off

### Requirement: All servers off at once
The user SHALL be able to turn all servers off in one action, and back on. While all are off, every server SHALL be off, a server added meanwhile included. Turning all back on SHALL return each server to its own choice: one the user turned off by itself SHALL stay off. While all are off, turning one server on SHALL NOT be possible; yardo SHALL say to turn all servers on first.

#### Scenario: Off and back
- **WHEN** `jira` is off by itself, and the user turns all servers off, then on
- **THEN** `github` and `aws` are on and `jira` is off

#### Scenario: Added while all are off
- **WHEN** all servers are off and the user adds `grafana`
- **THEN** `grafana` registers and is off

### Requirement: Switching on the MCP servers screen
On the MCP servers screen, space SHALL mark the selected server off or on, and a row above the servers, **All servers**, SHALL mark them all, shown at once as not saved. **Save** (a button under the list, or `s`) SHALL apply every change marked, in the background, the screen staying open, and SHALL then say what changed. Leaving with changes not saved SHALL ask once first. The keys line SHALL include `space on/off`. A server's own screen SHALL offer **Turn it off** or **Turn it on**.

#### Scenario: Mark, then save
- **WHEN** the cursor is on `jira-work`, the user presses space, then `s`
- **THEN** `jira-work` reads `○ off` and not saved at once, and after `s` it is off, the screen still the MCP servers list, saying `jira-work` is off

#### Scenario: Leaving unsaved
- **WHEN** a change is marked and the user presses `q`
- **THEN** the screen says it isn't saved and stays; `q` again leaves without it

#### Scenario: A found server
- **WHEN** the cursor is on a server found in `~/.mcp` that isn't on yardo's list and the user presses space
- **THEN** nothing is marked

#### Scenario: A server's screen
- **WHEN** the user opens `aws`, which is on
- **THEN** its first action is Turn it off

### Requirement: The MCP servers screen shows what's off
Each server's row SHALL say whether it's off (`○ off`). The All servers row SHALL say whether all are off, else how many servers are on. While all are off, the servers' rows SHALL be shown dimmed, so what will come back stays visible.

#### Scenario: Some off
- **WHEN** `jira-work` is off and three other servers are on
- **THEN** `jira-work` reads `○ off` and All servers reads 3 of 4 on

#### Scenario: All servers
- **WHEN** the cursor is on All servers and the user presses space
- **THEN** All servers reads off and every server's row is dimmed

### Requirement: A found server isn't called off
A server found in `~/.mcp` that isn't on yardo's list SHALL read "not on yardo's list", not "off", and its action to adopt it SHALL be **Add to yardo's list**.

#### Scenario: Found
- **WHEN** `~/.mcp/opensearch` exists and `opensearch` isn't on yardo's list
- **THEN** its row reads "not on yardo's list" and its screen offers Add to yardo's list

### Requirement: A server that's off doesn't ask for attention
A server that's off SHALL NOT produce rows on the home (Fill in, Try again, Fix), SHALL NOT count as a problem in `yardo status` (text or JSON: its empty values aren't among the MCP servers item's inputs), SHALL be shown as off by `yardo mcp status`, and SHALL be shown as off on the home's MCP servers panel, apart from the servers that are ready or need something. When all servers are off, the panel SHALL say so on its gateway line.

#### Scenario: Missing a value
- **WHEN** `jira` is off and its `JIRA_API_TOKEN` is empty
- **THEN** the home has no Fill in JIRA_API_TOKEN row and `yardo status` reports no problem for `jira`

#### Scenario: All off on the home
- **WHEN** all servers are off
- **THEN** the home's MCP servers panel says all servers are off

### Requirement: Switching from the command line
`sudo yardo mcp off [server …]` and `sudo yardo mcp on [server …]` SHALL turn the named servers off or on, and with no server, all servers. They SHALL print each server's state afterwards. An unknown server SHALL be an error listing the servers. `yardo mcp status` SHALL say which servers are off.

#### Scenario: One server
- **WHEN** the user runs `sudo yardo mcp off aws`
- **THEN** `aws` is off, and the output says so

#### Scenario: All
- **WHEN** the user runs `sudo yardo mcp off`
- **THEN** all servers are off

#### Scenario: Unknown
- **WHEN** the user runs `sudo yardo mcp off nope`
- **THEN** it fails, naming the servers there are

### Requirement: Switching while the gateway is down
When the gateway isn't answering, a switch SHALL still be recorded, and it SHALL take effect when the servers are next synced. yardo SHALL say so rather than fail.

#### Scenario: Gateway stopped
- **WHEN** the gateway is stopped and the user turns `aws` off
- **THEN** the screen says `aws` will be off once the gateway runs, and after the gateway is fixed, `aws`'s tools aren't listed
