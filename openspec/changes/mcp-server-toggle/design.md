## Context

Each user has their own MCPJungle gateway (`mcpjungle@<user>`, pinned
0.4.6). Agents are wired to it once, as a single `mcpjungle` MCP server;
every server registered on the gateway shows through it as
`<name>__<tool>`. `mcp-sync`, run as the gateway's service user
(`mcp-<user>`, which owns `~/.mcp`), registers the enabled servers
(`~/.mcp/servers`, plus every `~/.mcp/<name>/spec`), re-registers one with
`register --force` when its rendered config changes, and deregisters the
ones dropped. yardo reads `mcp-sync status` for the menu, the home and
`yardo status`.

MCPJungle 0.4.6, checked on a throwaway gateway (2026-11-20):

- `disable server <name>`: its tools leave `tools/list` on `/mcp`, a call
  to one fails with "tool not found", other servers are untouched.
- The disabled state survives a gateway restart (it's in the database).
- `register --force` of a disabled server makes it enabled again.
- `enable server <name>`: its tools are back at once.

## Goals / Non-Goals

**Goals:**
- One key per switch, applied in about a second, with no yardo run.
- yardo, not the gateway, is the record of what's off, so a
  re-registration can't turn a server back on.

**Non-Goals:**
- A security boundary. In development mode the gateway's admin API takes
  no token, and a user yardo created has passwordless sudo, so an agent
  set on it can turn a server back on. This keeps tools out of everyday
  sessions; it doesn't lock them away.
- Off by default for servers that can write, timers that turn a server off
  again, switching single tools, or per-agent or per-session sets of
  servers.
- A switch on the home: typing there searches the tools, and the MCP
  servers screen is one key away.

## Decisions

**Disable on the gateway, not deregister or stop it.** Deregistering
loses nothing on disk, but turning on again re-registers (a container
start, seconds, and it can fail on settings). Stopping the gateway takes
every server away and makes every agent show a failed MCP connection at
start. `disable server` is instant both ways and hides the tools from
`tools/list`, which is what the context goal needs.

**Markers in `~/.mcp`, owned by the service user.** `~/.mcp/<name>/off`
marks a server off; `~/.mcp/off` marks all of them off. A server is off
when either exists. They live in the server's directory, so `mcp-sync
rename` moves the choice and `mcp-sync remove` deletes it with nothing
extra. They aren't in yardo's answers, so applying picks or a profile
never changes them. Written only through mcp-sync (as the service user),
like the rest of `~/.mcp`.

**mcp-sync keeps the gateway in line with the markers.** A third file,
`~/.mcp/<name>/.disabled`, records that mcp-sync disabled the server on
the gateway. At the end of every sync, for each registered server:

- off and no `.disabled` → `disable server`, write `.disabled`;
- on and `.disabled` → `enable server`, remove `.disabled`.

A successful `register` removes `.disabled` (the gateway just enabled it),
so a server re-registered while off is disabled again in the same sync.
Without the stamp, a sync would have to enable every server that's on,
each time, which would also undo any tool a user disabled by hand with
`mcpjungle disable tool`.

`mcp-sync off [name …]` / `on [name …]` write or remove the markers (no
name: `~/.mcp/off`) and then do only that reconciling step, not a full
sync. If the gateway doesn't answer, the markers stay written, it says the
switch applies once the gateway runs, and exits 0; yardo's sync step, which
runs on every apply and Fix, reconciles later.

**`mcp-sync status` says `off` first.** For an enabled server with a
marker, the line is `<name>\toff\t<own|all>` (`own` when it has its own
marker, `all` when only `~/.mcp/off` makes it off), before the checks that
would say `waiting` or `failed`; a first line `*\toff` says all are off.
yardo maps it to the row state `off`, which wins over secret, failed,
pending and new, so a server that's off never makes a problem row.

**The menu.** Space, and a server screen's Turn it off/on, end the menu
with `MCPAct{Do: "off"|"on", Entry}` (Entry empty for All servers), as
Try again does. yardo runs `mcp-sync off|on`, then reopens the list with a
toast; there's no run screen. Turning one server on while `~/.mcp/off`
exists is refused in the menu with a toast. The found-server action `on`
becomes `adopt`, labelled Add to yardo's list, and that state
becomes `unlisted`, read "not on yardo's list".

## Risks / Trade-offs

- [An agent session already open may keep listing the tools it saw at
  start] → the toast says "new agent sessions won't see its tools"; a
  gateway that sends `tools/list_changed` is a bonus, not relied on.
- [A server turned off is gone for the laptop's client too] → intended:
  one choice per gateway. The spec says every client.
- [A user turns a server off with `mcpjungle disable server` directly]
  → yardo doesn't see it (no marker); the gateway keeps it off until it
  re-registers. Not worth reading the gateway's state back for.
- [`register --force` behaviour changes in a later MCPJungle] → the
  stamp only ever adds a `disable` call that's harmless when already
  disabled; a test on the menu side covers the state mapping, and the
  check script's steps go in the tasks as a manual check on upgrades.

## Migration Plan

Nothing to migrate: no marker means on, as every server is today. Rolling
back yardo leaves servers that were off disabled on the gateway until they
re-register or are enabled by hand; the markers are ignored by an older
mcp-sync.

## Open Questions

- Does MCPJungle 0.4.6 send `notifications/tools/list_changed` to open
  sessions? Only changes the toast's wording.
