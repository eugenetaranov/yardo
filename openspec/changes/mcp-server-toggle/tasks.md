## 1. mcp-sync

- [ ] 1.1 Add `mcp-sync off [name …]` and `mcp-sync on [name …]`: write or remove `~/.mcp/<name>/off` (no name: `~/.mcp/off`), validating names as the other commands do, then reconcile only those servers (1.2); with the gateway down, keep the markers, say the switch applies once the gateway runs, exit 0. Update the usage header. Verify: `internal/assets/mcpsync_test.go` cases with a fake `mcpjungle` (fakeBin) assert the marker files and the `disable server`/`enable server` calls, and exit 0 with a fake gateway that's down.
- [ ] 1.2 Reconcile at the end of every sync: for each registered enabled server, off (its marker or `~/.mcp/off`) without `.disabled` → `disable server`, write `.disabled`; on with `.disabled` → `enable server`, remove it; a successful `register` removes `.disabled`. Verify: test that a server re-registered (changed env) while off gets `register --force` then `disable server` in one sync, and that a sync with nothing changed makes no `disable`/`enable` call.
- [ ] 1.3 `mcp-sync status`: an enabled server that's off prints `<name>\toff\t<all|>` before the `waiting`/`failed` checks. Verify: test with an off server missing a required key reports `off`, not `waiting`.
- [ ] 1.4 `rename` and `remove` keep working with the markers (they move or go with the directory); `rename` drops `.disabled` with the deregistration. Verify: test renaming an off server leaves `<new>/off` and no `.disabled`.

## 2. yardo: states and actions

- [ ] 2.1 Map the `off` status to the row state `off` in `mcpRows` (wins over secret, failed, pending, new, unchecked); keep whether it's off by itself or by all; rename the found-server state `off` to `unlisted`. Update `MCPServer`'s doc comment. Verify: `cmd/yardo/mcpservers_test.go` table cases for an off server with an empty key, an off failed one, and a found unlisted one.
- [ ] 2.2 Handle `MCPAct{Do: "off"|"on"}` (Entry empty: all) by running `mcp-sync off|on` and returning a toast ("jira-work off: new agent sessions won't see its tools", "all servers off", the gateway-down wording); rename the found-server action `on` to `adopt`. Verify: tests with the fake `mcpSync` hook assert the args and the toast, and the existing `Do: "on"` test is moved to `adopt`.
- [ ] 2.3 Off servers make no home problem rows (Fill in, Try again, Fix) and no `yardo status` problem; `yardo status --json` gives their state `off`. Verify: `cmd/yardo/dashboard_test.go` / status tests with an off server that has an empty value.
- [ ] 2.4 `sudo yardo mcp on|off [server …]`: unknown server → error listing the servers; prints each server's state after; `yardo mcp status` shows off servers; update the unknown-subcommand message and `--help`. Verify: tests for one server, none (all), and an unknown name.

## 3. The MCP servers screen

- [ ] 3.1 Add the All servers row above the servers (`● on · 3 of 4` / `○ off`), dim the servers' rows while all are off, and render a server that's off as `○ off`; the rows' positions don't move as the selection does. Verify: `internal/ui/menu_test.go` view tests for the row and the dimmed list.
- [ ] 3.2 Space on a server or All servers ends the menu with the on/off act; space on an unlisted server does nothing; space on a server while all are off shows the "turn all servers on first" toast; add `space on/off` to the keys line; the cursor indexes (Add row, `mcpSel`, delete from the list) account for the new row. Verify: menu key tests for each case, and the existing `e`/`d`/enter tests still pass.
- [ ] 3.3 A server's screen: Turn it off / Turn it on as its first action for yardo's servers; found servers show Add it to yardo's servers; the state word for `unlisted` reads "not on yardo's list". Verify: menu tests on `mcpActions` and `mcpWord`.
- [ ] 3.4 The home's MCP servers panel lists off servers apart (`○ name  off`), says "all servers off" on the gateway line when they are, and `mcpSummary` doesn't count them as not ready. Verify: home view test.

## 4. Docs and check

- [ ] 4.1 README: the MCP servers screen's space key and All servers, `yardo mcp on|off`, and that off hides tools from every client but isn't a lock. Verify: README section reads with the new keys.
- [ ] 4.2 `task test` and `task lint` pass.
- [ ] 4.3 On a VM (pmox, or `scripts/e2e.sh`): register two servers, turn one off from the screen, check a new Claude Code session doesn't list its tools, edit its settings (re-registers), check it's still off, turn all off and on, check `yardo status`. Verify: the steps' output recorded in the PR.
