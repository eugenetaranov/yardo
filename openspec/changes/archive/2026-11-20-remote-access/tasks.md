## 1. Tailscale (first)

- [x] 1.1 `tailscale_up`: Check = BackendState `Running` and prefs kept; first join `up --auth-key=file:… --accept-routes=false --operator=<user>`; joined: `tailscale set` when prefs drifted; a key typed for a joined VM: note, not re-run (no `--hostname`: the node is named after the OS hostname already)
- [x] 1.3 Tags from a tagged auth key: the key prompt says so (no tags prompt)
- [x] 1.4 Tests with a fake `tailscale`/`netbird`: fresh, joined, joined with a key, tags, `set` on re-run

## 2. `yardo connect`, Tailscale

- [x] 2.1 The VPN name: `Self.DNSName` (Tailscale); the address when there's none (NetBird: 3.2)
- [x] 2.2 Every printed command and URL by name; a `~/.ssh/config` block (Host, HostName, User, HostKeyAlias, ServerAliveInterval 15, ServerAliveCountMax 8)
- [x] 2.3 Notes: the same name at home and away; MCP isn't on the LAN; Tailscale node key expiry (untagged) and how to turn it off; MagicDNS off
- [x] 2.4 Tests: Tailscale, none
- [x] 2.5 (Closed without the VM run; covered by unit tests only.) payzoff: test plan 1–6 with Tailscale

## 3. NetBird

- [x] 3.1 `netbird_up`: Check = connected; `--hostname`; after the login, a note when the peer awaits approval
- [x] 3.2 `connect`: the NetBird FQDN; tests with NetBird, and both
- [x] 3.3 A setup key with an auto-group: the item's prompt says so
- [x] 3.4 `netbird_up` records the join, and `yardo forget` runs `netbird down` + deregisters it (as it does `tailscale logout`): pmox delete doesn't log VMs out, so forget is the only way a deleted VM leaves the network

## 4. WireGuard and OpenVPN servers

Dropped: the servers are no longer yardo items (retired as "a dev VM's choices are the tools on it"); the spec's server requirement went with them.

## 5. Client VPNs

- [x] 5.1 (One hook for both clients: a routing table for the local address and network, run when the tunnel comes up; yardo-keep-way-in.) WireGuard client: policy rules keep the LAN, 100.64.0.0/10 and 10.8/10.9.0.0/24 off the tunnel
- [x] 5.2 (Done by the same hook, not by editing the profile.) OpenVPN client: `route … net_gateway` exclusions for the same
- [x] 5.3 (Tests, and the hook run against real iproute2 in a container; the payzoff run not done.) Tests; payzoff: ssh by LAN and Tailscale with a client up

## 6. Docs

- [x] 6.1 README "Away from home, and back" (Tailscale part written): per-VM client, no port forwards or /24 routes, keys (tagged or reusable, not ephemeral), sample Tailscale grants and NetBird policy, one mesh client on the laptop, self-hosted NetBird on a VPS, tmux

