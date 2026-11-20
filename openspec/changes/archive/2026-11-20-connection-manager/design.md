## Context

Today:
- **The screen.** `menu.go`'s `connect` screen draws `yardo connect`'s
  text (`printConnect`) and the Keys to add; its only action is `d`
  dismissing a key. The home's panel shows three `ConnectRow`s. Go to has
  no Connection entry (tui-flow removed it).
- **Addresses.** `connect.go` lists the machine's addresses by interface
  (`addresses`, `vpnOf`: `tailscale0`, `wt0`, `wg*`, `tun*`) and names
  them from `tailscale status --json` and `netbird status --json`
  (`withNames`). The VPN address comes first in every line it prints.
- **Logins.** `tailscale_up` and `netbird_up` are item steps run by an
  apply, with the key asked before the run. Tools' details offer "Log in"
  and "Log out" (the item's `remove` script: `tailscale logout`,
  `netbird down`).
- **Clients.** The WireGuard and OpenVPN client items only install the
  package (and the keep-way-in hook). Profiles are the user's to place in
  `/etc/wireguard/<name>.conf` or `/etc/openvpn/client/<name>.conf` and
  start with `wg-quick@<name>` or `openvpn-client@<name>`.
- **Tokens.** `mcp-sync client <name>` makes a gateway token in
  `~/.mcp/clients/<name>.token`. Nothing revokes one.
- **Background actions.** The MCP servers screen already runs its
  switches in the background and reports into the row (`waitCmd`).
- **Clipboard.** `clipboardHint` explains OSC 52 for Neovim and tmux;
  yardo itself never copies.
- **Root.** `asRoot` re-runs yardo through sudo, keeping a short list of
  environment variables. `SSH_CONNECTION` isn't on it.

## Goals / Non-Goals

**Goals:**
- One screen to see, start, stop, set up and remove the machine's
  networks, and to set the laptop up.
- Never cut the user's own session without saying so first.
- Actions that don't need typing never leave the screen.

**Non-Goals:**
- A command-line twin (`yardo vpn up|down`): `tailscale`, `netbird` and
  `systemctl` are that already. Can follow if asked for.
- Editing a profile's contents, split-tunnel choices, or DNS settings.
- VPN servers on the machine.
- Reaching into the laptop: yardo runs on the machine, so the laptop's
  side is lines to paste and one script, never something yardo does there.

## Decisions

- **A view of its own.** `internal/ui/connection.go` holds a `Connection`
  view (rows, sections, hits, keys, help), as `Board` and `Menu` are views
  of `App`. `menu.go`'s `connect` screen and its text rendering go. The
  screen is rows, not a text dump: *Networks* first, then *From your
  laptop*, then *Keys to add* (as now, with `d`). Alternative considered:
  growing the menu's `connect` case. Rejected: the menu is already the
  largest file, and this screen has its own background actions.
- **Networks as data, from the system.** `cmd/yardo` builds
  `[]ui.Network{Kind, Name, State, Addr, DNS, Boot, Via}`:
  - Tailscale: `tailscale status --json` (`Running`, `Stopped`,
    `NeedsLogin`).
  - NetBird: `netbird status --json`.
  - Profiles: the files in `/etc/wireguard/*.conf` and
    `/etc/openvpn/client/*.conf`, with `systemctl is-active` and
    `is-enabled` of their units. A profile started some other way (plain
    `wg-quick up`) shows by its interface, as today.
  - Read once when the screen opens and again after each action; no
    polling, except while waiting for a Tailscale login link (every 2s,
    stopped on leaving the screen).
- **Stop is not log out.** Stop: `tailscale down`, `netbird down`,
  `systemctl stop <unit>`. Start: `tailscale up` (its saved settings),
  `netbird up`, `systemctl start`. Boot: `systemctl enable`/`disable`
  for profiles; Tailscale and NetBird remember their own state across
  reboots, so they get no `b`.
- **The session's way in.** `asRoot` keeps `SSH_CONNECTION`. Its third
  field is the machine's address the session came to; the network whose
  interface has that address is `Via`. With no `SSH_CONNECTION` (console,
  or a sudo that dropped it), nothing is `Via` and no warning is needed on
  a console; when the variable is missing but `SSH_TTY`/`SUDO_*` suggest
  ssh, yardo warns on every stop ("yardo can't tell which network you're
  on"). The way back named in the warning is the first other address in
  rank order.
- **First-time setup.**
  - Tailscale/NetBird with a key: the existing prompts and steps, run as a
    one-item apply, so Check, notes and Last run all work as they do now.
  - Tailscale without a key: `tailscale up --timeout=1s` prints the login
    URL (also in `status --json` as `AuthURL`); the row shows it, `c`
    copies it, and the poll turns the row connected.
  - Profiles: `n` asks the kind (when both are installed), a name
    (`[a-z0-9-]`, 15 characters at most: it becomes an interface name)
    and the source. A new step type validates (WireGuard: `[Interface]`
    with `PrivateKey` and a `[Peer]`; OpenVPN: a `remote` line), writes
    the file 0600 root, enables and starts the unit, and waits up to 15s
    for a handshake (`wg show <name> latest-handshakes`) or OpenVPN's
    "Initialization Sequence Completed". No handshake is reported as "up,
    but nothing answered yet", not a failure. An existing name is refused,
    not overwritten.
- **Remove.** Tailscale: `tailscale logout`. NetBird: `netbird down`,
  `netbird logout`. Profile: stop, disable, delete the file. Each also
  drops the join's record (`created.json`), so `yardo forget` doesn't try
  again. The login items stay picked, so Tools still shows the VPN
  installed, "not logged in".
- **Laptop rows as data.** `printConnect`'s sections become
  `[]ui.Setup{What, How, Lines, Auth}`; `yardo connect` prints the same
  data as text, so the two can't drift. Rows: SSH (the Host block),
  Docker (context), MCP for Claude Code, for Codex, other clients (URL
  and header), and the script.
- **`c` copies with OSC 52.** One helper writes
  `ESC ] 52 ; c ; <base64> BEL` (wrapped for tmux when `TMUX` is set).
  There's no reply to read, so yardo says "copied, if your terminal
  allows it" and, the first time, how to allow it (`clipboardHint`'s
  text). A token is copied by reading its file at that moment; it's never
  put in the view's state or drawn.
- **Tokens.** `n` runs `mcp-sync client <name>` in the background. `d`
  needs `mcp-sync client-rm <name>` (new: delete the gateway's client,
  then the token and json files). The list is the `*.token` files.
- **The script.** `yardo connect --script` prints POSIX sh: append the
  Host block when `ssh -G <name>` doesn't already resolve it;
  `docker context create` when missing and docker is there; `claude mcp
  add` when claude is there, reading the token with `ssh <name> sudo cat
  <file>` at run time. The screen shows
  `ssh <user>@<addr> yardo connect --script > yardo-<name>.sh`, then `sh
  yardo-<name>.sh`: saved to a file to read first, never piped to sh.
- **Tools keeps Log in and Log out.** They call the same start, set-up
  and remove code. The details also say "more on Connection".

## Risks / Trade-offs

- **Cutting the session anyway.** `SSH_CONNECTION` can be stale inside
  tmux or zellij (attached from another network). The warning covers the
  common case; the README says a console or the local network is the way
  back. → Mitigation: stopping a network never disables it at boot, so a
  reboot from the hypervisor brings Tailscale and enabled profiles back.
- **A full-tunnel profile on first start** changes the machine's default
  route. The keep-way-in hook (remote-access) keeps ssh over the local
  network working; a session that came in over another client tunnel
  could still drop. The add question says so when the profile routes
  everything (`AllowedIPs = 0.0.0.0/0`, `redirect-gateway`).
- **OSC 52 limits.** Some terminals cap the size or refuse it. Blocks
  here are small; the lines stay on screen to select by hand.
- **Profiles hold private keys.** Stored 0600 root, never logged (the
  step logs the name, not the contents), deleted on remove. A pasted
  profile goes through a temp file, 0600, removed after.
- **Tailscale's login link in `status`** differs between versions. →
  Fall back to parsing `tailscale up`'s output; with neither, the row says
  to run `sudo tailscale up` in another terminal.
- **Scope.** This is the largest screen after Tools. The tasks are
  ordered so each group ships something usable on its own.
