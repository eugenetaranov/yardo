# yardo-connections Specification

## Purpose

The Connection screen: where a user sees how this machine is connected, starts, stops, sets up and removes its networks (Tailscale, NetBird, WireGuard and OpenVPN profiles), and gets the laptop's side set up to use its ssh, Docker and MCP gateway.

## Requirements

### Requirement: Networks are listed with their state
The Connection screen SHALL list every network of the machine: Tailscale and NetBird when installed, each WireGuard and OpenVPN client profile on it, and the local network. Each row SHALL say whether it's connected, its address and name when it has them, and, for a profile, whether it starts at boot. A VPN that's installed but not set up SHALL say so and how to set it up.

#### Scenario: Tailscale up, a WireGuard profile down
- **WHEN** Tailscale is connected and the profile `office` isn't started
- **THEN** the screen lists Tailscale as connected with its address and name, and `office` as stopped

#### Scenario: Not set up
- **WHEN** NetBird is installed and not logged in
- **THEN** its row says it isn't set up, and that enter sets it up

### Requirement: Starting and stopping a network
Space on a network's row SHALL stop it when it's connected and start it when it's stopped, without logging it out or deleting anything. It SHALL run in the background with the screen kept, and the row SHALL show the new state, or what failed. `b` on a profile's row SHALL turn starting at boot on or off.

#### Scenario: Stop Tailscale
- **WHEN** the user presses space on a connected Tailscale and confirms
- **THEN** Tailscale is disconnected, still logged in, and the row says stopped

#### Scenario: Start fails
- **WHEN** starting the profile `office` fails
- **THEN** the row says it failed, with the reason's last line, and Last run or the log has the rest

### Requirement: A warning before cutting the user off
When the user's own session reaches the machine through a network, stopping or removing that network SHALL first say that it ends this session and how to get back in (another address of the machine, or its console), and SHALL ask, with no as the default. Stopping any other network SHALL NOT ask.

#### Scenario: Connected through Tailscale
- **WHEN** the user's ssh came in over Tailscale and they press space on Tailscale
- **THEN** yardo warns that this session will end, names the local address as the way back, and stops Tailscale only on yes

#### Scenario: Connected over the local network
- **WHEN** the user's ssh came in over the local network and they press space on Tailscale
- **THEN** Tailscale stops without a question

### Requirement: Setting a network up for the first time
Enter on a Tailscale or NetBird row that isn't logged in SHALL ask for its key and log in; for Tailscale with no key given, the screen SHALL show the login link and turn the row connected once the login is approved. `n` SHALL add a WireGuard or OpenVPN profile from a name and either a file on the machine or pasted text: yardo SHALL check that it's a profile of that kind, store it readable by root only, start it, and say whether it connected. A profile that fails the check SHALL be refused with the reason and nothing stored. Adding a profile for a VPN that isn't installed SHALL install it first.

#### Scenario: A WireGuard profile from a file
- **WHEN** the user adds a profile named `office` from `~/office.conf`
- **THEN** `office` is stored for WireGuard, readable by root only, started, and listed as connected

#### Scenario: Not a profile
- **WHEN** the file given has no `[Interface]` section
- **THEN** yardo says it isn't a WireGuard profile and stores nothing

#### Scenario: Tailscale by link
- **WHEN** the user presses enter on Tailscale and gives no key
- **THEN** the screen shows Tailscale's login link, and the row says connected once the login is approved in a browser

### Requirement: Removing a network completely
`d` on a network's row SHALL ask first, saying what goes, then remove it: Tailscale is logged out, NetBird is disconnected and logged out, a profile is stopped, taken off boot and its file deleted. The question SHALL say what stays (the software, and for Tailscale the machine's entry in the admin console until it's deleted there). After it, the row SHALL be gone or say not set up.

#### Scenario: Remove a profile
- **WHEN** the user presses `d` on `office` and confirms
- **THEN** `office` is stopped, no longer starts at boot, its file is deleted, and it's off the list

#### Scenario: Tailscale's console entry
- **WHEN** the user presses `d` on Tailscale
- **THEN** the question says the machine stays listed in the tailnet's admin console until deleted there

### Requirement: The laptop's side, ready to use
The screen SHALL have a row for each thing to set up on the laptop: SSH, Docker, and the MCP gateway for Claude Code, for Codex and for other clients. Each SHALL open to the exact lines to run or paste, with this machine's address (its VPN address first) filled in, and SHALL say how the user signs in. `c` SHALL copy the selected command or block to the laptop's clipboard through the terminal, and SHALL say how to allow that where the terminal refuses it. A token SHALL never be drawn on the screen.

#### Scenario: Claude Code
- **WHEN** the user opens the Claude Code row with the gateway reachable from the VPNs
- **THEN** it shows the `claude mcp add` line with the gateway's URL, and where the token is kept

#### Scenario: Copy
- **WHEN** the user presses `c` on the ssh config block
- **THEN** the block is on the laptop's clipboard, and the screen says so

#### Scenario: Gateway for this machine only
- **WHEN** the gateway isn't open beyond the machine
- **THEN** the MCP rows show the ssh tunnel to use, and that Settings → Network access opens it

### Requirement: Gateway tokens per client
From the MCP gateway's rows, `n` SHALL make a token for a named client and `d` SHALL revoke one, asked first. The screen SHALL list the clients with a token, by name and file, never by value.

#### Scenario: A token for CI
- **WHEN** the user presses `n` and names the client `ci`
- **THEN** a token for `ci` exists, the list has `ci` with its file, and the token isn't shown

#### Scenario: Revoke
- **WHEN** the user revokes `laptop`'s token and confirms
- **THEN** the gateway refuses that token, and `laptop` is off the list

### Requirement: One script for the laptop
`yardo connect --script` SHALL print a shell script for the laptop that adds the ssh Host block (once), creates the Docker context and adds the gateway to Claude Code when it's installed, and changes nothing else. The script SHALL contain no token; it SHALL say how it reads one at run time. The Connection screen SHALL show the line that fetches the script.

#### Scenario: Run twice
- **WHEN** the script is run on a laptop that already has the Host block and the context
- **THEN** it adds neither again and exits 0
