## Why

Every MCP server yardo registers is in every agent session: its tools fill
the agent's context, and sensitive ones (AWS, Kubernetes, a database) can be
called during everyday work that has nothing to do with them. The only way
to stop that today is deleting the server, or editing yardo's list and
re-running, which loses or re-registers its settings. A server that's
rarely needed should be one key away from off, and back on.

## What Changes

- **On and off, per server:** on the MCP servers screen, space turns the
  selected server off or on, straight away (no run screen). A server that's
  off stays registered with its settings; the gateway hides its tools from
  every client and refuses calls to them.
- **All servers:** a row at the top of the list turns every server off in
  one key, and back on: each server returns to its own choice.
- **The choice persists** across syncs, re-registrations, renames, gateway
  restarts and yardo runs. It isn't part of yardo's answers or profiles: it
  is the machine's day-to-day state, not its setup.
- **A server that's off doesn't nag:** no "Fill in" or "Try again" rows on
  the home for it, and `yardo status` doesn't count it as a problem.
- **CLI:** `sudo yardo mcp on|off [server …]` (no server: all of them).
- **mcp-sync:** `mcp-sync on|off [name …]`; `mcp-sync status` says `off`.
- A server found in `~/.mcp` but not on yardo's list no longer reads
  "off": it reads "not on yardo's list", and its action is **Add it to
  yardo's servers** (was "Turn it on").

## Capabilities

### New Capabilities

- `yardo-mcp-toggle`: turning MCP servers on and off (each, or all), how
  that persists, what agents see, and how the MCP servers screen, the home,
  `yardo status` and `yardo mcp` show and change it.

### Modified Capabilities

None. No existing spec covers the MCP servers screen or mcp-sync's
commands.

## Impact

- `internal/assets/mcp-sync`: `on`/`off` commands, the markers in `~/.mcp`,
  turning off again after each sync, the `off` status.
- `cmd/yardo/mcpservers.go`: the `off` state in the menu's rows; on/off
  actions; `yardo mcp on|off`.
- `internal/ui/mcpmenu.go`, `internal/ui/board.go`: the All servers row,
  space, the server screen's action, the state words.
- `internal/ui/home.go`, `cmd/yardo/status.go`: off servers in the MCP
  panel and summary; no problem rows for them.
- Needs mcpjungle's `disable server` / `enable server` (in the pinned
  0.4.6; checked).
- Agents' wiring is unchanged: they keep one `mcpjungle` entry.
