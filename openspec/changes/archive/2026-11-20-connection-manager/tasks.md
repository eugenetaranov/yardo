## 1. The screen and its data

- [x] 1.1 `ui.Connection` view (`internal/ui/connection.go`): sections Networks, From your laptop, Keys to add; rows, cursor, paging, hits, `Keys()`/`Help()`, opened from the home's panel and from a Go to entry; `menu.go`'s `connect` screen removed. Verify: `connection_test.go` view at 120 and 80 columns; `app_test.go` opens it both ways; `q` goes home.
- [x] 1.2 Networks as data: `[]ui.Network` built in `cmd/yardo` from `tailscale status --json`, `netbird status --json`, `/etc/wireguard/*.conf` and `/etc/openvpn/client/*.conf` with their units' active/enabled state; the local network last. Verify: `main_test.go` with a fake exec and temp dirs: Tailscale running, NetBird needing login, one profile up and one down.
- [x] 1.3 Laptop rows as data: `printConnect`'s sections become `[]ui.Setup`; `yardo connect` prints from the same data. Verify: existing `TestPrintConnect*` pass unchanged in what they assert; a new test that the screen's Claude Code row and `yardo connect` have the same line.

## 2. Start, stop, boot

- [x] 2.1 Space stops or starts the row's network in the background (`tailscale down/up`, `netbird down/up`, `systemctl stop/start <unit>`), the row showing "stopping…", then the state or the failure's last line; the networks read again after. `b` on a profile enables or disables its unit. Verify: `connection_test.go` key tests with a fake action; `steps` test for the commands per kind.
- [x] 2.2 The session's way in: `asRoot` keeps `SSH_CONNECTION`; the network whose interface has its local address is marked; stopping or removing it opens the warning dialog (default no) naming the way back; without the variable on an ssh session, every stop warns. Verify: `main_test.go` for the address → network match; `connection_test.go`: space on the marked row opens the dialog, on another doesn't.

## 3. First-time setup

- [x] 3.1 Enter on Tailscale or NetBird not logged in: the key prompt, then the login as a one-item apply (existing steps). Verify: `main_test.go` asserts the apply's item and that a logged-in row doesn't ask.
- [x] 3.2 Tailscale with no key: the login link on the row (`AuthURL`, or `tailscale up`'s output), `c` copies it, a 2s poll turns it connected and stops on leaving. Verify: `connection_test.go` with a fake status going NeedsLogin → Running.
- [x] 3.3 `n` adds a profile: kind, name (checked), a file or pasted text; a `vpn_profile` step validates, writes it 0600, enables and starts the unit, waits for a handshake, and reports; an existing name or a bad profile is refused with nothing written; a full-tunnel profile says so first; the client item is installed first when missing. Verify: `steps` tests: good and bad WireGuard and OpenVPN profiles, existing name, the file's mode, the contents absent from the log.

## 4. Remove

- [x] 4.1 `d` on a network: the dialog says what goes and what stays per kind; then logout (Tailscale), down + logout (NetBird), or stop + disable + delete (profile), and the join's record dropped. Verify: `steps` tests for each kind's commands and the deleted file; `connection_test.go` dialog text for Tailscale names the admin console.

## 5. The laptop's side

- [x] 5.1 `c` copies the selected line or block with OSC 52 (wrapped under tmux), says so, and says once how to allow it; a token is read from its file when copied, never drawn. Verify: test of the escape sequence written for a block, with and without `TMUX`; a view test that no token text appears.
- [x] 5.2 Gateway tokens: the clients listed by name and file; `n` makes one (`mcp-sync client <name>`), `d` revokes one (`mcp-sync client-rm <name>`, new), both in the background. Verify: `mcp-sync` shell test for `client-rm`; `connection_test.go` list and keys.
- [x] 5.3 `yardo connect --script`: the Host block once, the Docker context when missing, `claude mcp add` when claude is there, no token inside; the screen's row shows the line that saves it to a file. Verify: run the script twice against a temp HOME with fake `ssh`, `docker` and `claude`: second run changes nothing, exit 0; grep the script for a token.

## 6. Docs and checks

- [x] 6.1 Tools' VPN details point to Connection; "Log in"/"Log out" use the same code. Verify: `board_test.go` details text.
- [x] 6.2 README: "Reaching this machine from a laptop" and "Away from home" rewritten around the screen (start/stop, the warning, adding a profile, removing, `c`, tokens, the script); the home sample's Go to line. Verify: README reads with them.
- [x] 6.3 `task test` and `task lint` pass.
- [ ] 6.4 On a VM, over ssh: stop Tailscale from a LAN session (no question) and from a Tailscale session (warned); add a WireGuard profile from a file and see it connect; remove it; make and revoke a token and check the gateway refuses it; run the script on the laptop twice. Verify: the steps' output recorded in the PR.
