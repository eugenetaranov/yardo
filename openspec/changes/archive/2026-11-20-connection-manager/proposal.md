## Why

Connection today is a read-only page: it prints how to reach the machine
and that's all. Everything else about remote access is somewhere else or
nowhere. Logging a VPN in is under Tools → the VPN's details. Stopping
Tailscale for a while, bringing a WireGuard or OpenVPN profile up, or
taking a machine off a network for good are shell commands you have to
know. And the laptop side (ssh, Docker, Claude Code, Codex) is one long
text to scroll and select by hand. tui-flow also took Connection out of Go
to, so it's reachable only through the home's panel.

## What Changes

- **Connection is back in Go to**, and becomes the one place for remote
  access. The home's panel stays as its summary; enter on either opens it.
- **The screen has two parts.** *Networks*: every way this machine is
  connected. *From your laptop*: what to do on the laptop to use it.
- **Networks, one row each:** Tailscale, NetBird, and each WireGuard and
  OpenVPN client profile on the machine. A row shows whether it's
  connected, its address and name, and whether it starts at boot. The
  local network is listed too, with nothing to do on it.
- **Connect and disconnect:** space on a row stops or starts it
  (`tailscale down`/`up`, `netbird down`/`up`, the profile's systemd
  unit). It runs in the background, the screen stays, and the row shows
  the result. `b` turns "start at boot" on or off for a profile.
- **It warns before cutting you off.** When your own session comes in
  through the network you're about to stop or remove, yardo says so first
  ("you're connected through Tailscale: stopping it ends this session;
  you'd get back in over 192.168.0.105 or the console") and asks. The
  default answer is no.
- **First-time setup, from the row:**
  - Tailscale and NetBird not logged in: enter asks for the auth or setup
    key. With no key, Tailscale shows its login link to open in a browser,
    and the row turns connected once you've approved it.
  - WireGuard and OpenVPN: `n` adds a profile. You give it a name and
    either a file on the machine or the profile pasted in. yardo checks
    it, puts it where the client reads it (root only), brings it up and
    reports whether it connected. A VPN that isn't installed yet is
    installed first, the way Tools does it.
- **Remove completely:** `d` on a row, asked first, saying exactly what
  goes:
  - Tailscale: logs out, and says the machine stays listed in your
    tailnet's admin console until you delete it there.
  - NetBird: disconnects and logs out, which deregisters the peer.
  - A WireGuard or OpenVPN profile: stopped, taken off boot, and its file
    (with its private key) deleted.
  - The software itself stays; uninstalling it is still Tools' job, and
    the question says so.
- **From your laptop, a row per thing to set up:** SSH, Docker, the MCP
  gateway for Claude Code, for Codex, and for other clients (the URL and
  header). Each row opens to its exact lines, with the address already
  filled in, and says how you sign in (your SSH key; the gateway's token
  and where it's kept).
- **`c` copies** the selected command or block to the laptop's clipboard,
  through the terminal. Where the terminal doesn't allow that, it says how
  to turn it on, and the lines are still there to select.
- **Gateway tokens:** from the MCP row, `n` makes a token for a named
  client (laptop, ci), and `d` revokes one. Tokens are never shown on the
  screen; `c` copies one, and the row says which file holds it.
- **One script for the laptop:** `yardo connect --script` prints a shell
  script that adds the ssh Host block, the Docker context and the Claude
  Code server on the laptop. The screen shows the one line that fetches
  it, to read before running.
- Tools' "Log in" and "Log out" on a VPN stay, and do the same as the
  screen.

Not in this change: running a VPN server on the machine (retired), editing
a profile's contents, Tailscale's own features (exit nodes, serve, SSH),
and changing who can reach the gateway and container ports (that stays in
Settings → Network access; the screen links to it).

## Capabilities

### New Capabilities
- `yardo-connections`: the Connection screen as the place to see, start,
  stop, set up and remove this machine's networks (Tailscale, NetBird,
  WireGuard and OpenVPN profiles), and to get the laptop's side set up:
  ssh, Docker, the MCP gateway and its tokens.

### Modified Capabilities
- `yardo-tui-screens`: "Connection is one thing": Go to lists Connection
  again.

## Impact

- `internal/ui`: a Connection view of its own (`connection.go`), taken out
  of `menu.go`'s `connect` screen; the Go to entry; hits, keys and help
  for it; an OSC 52 copy helper.
- `cmd/yardo/connect.go`: the networks' status (Tailscale, NetBird,
  `wg-quick@*`, `openvpn-client@*`), the laptop rows as data, `--script`.
- `internal/steps`: start, stop and remove for each network; adding a
  WireGuard or OpenVPN profile (validate, install, enable); the session's
  way in (which interface the user's ssh came through).
- `internal/assets/mcp-sync`: revoking a client's token, if it can't yet.
- `cmd/yardo/main.go`: the screen's actions run in the background like
  the MCP servers' switches; typed questions (a key, a profile) leave the
  screen like any question.
- README: "Reaching this machine from a laptop" and "Away from home".
