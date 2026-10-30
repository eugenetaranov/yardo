## 1. Tailscale (first)

- [x] 1.1 `tailscale_up`: Check = BackendState `Running` and prefs kept; first join `up --auth-key=file:… --accept-routes=false --operator=<user>`; joined: `tailscale set` when prefs drifted; a key typed for a joined VM: note, not re-run (no `--hostname`: the node is named after the OS hostname already)
- [x] 1.3 Tags from a tagged auth key: the key prompt says so (no tags prompt)
- [x] 1.4 Tests with a fake `tailscale`/`netbird`: fresh, joined, joined with a key, tags, `set` on re-run

## 2. `yardo connect`, Tailscale

- [x] 2.1 The VPN name: `Self.DNSName` (Tailscale); the address when there's none (NetBird: 3.2)
- [x] 2.2 Every printed command and URL by name; a `~/.ssh/config` block (Host, HostName, User, HostKeyAlias, ServerAliveInterval 15, ServerAliveCountMax 8)
- [x] 2.3 Notes: the same name at home and away; MCP isn't on the LAN; Tailscale node key expiry (untagged) and how to turn it off; MagicDNS off
- [x] 2.4 Tests: Tailscale, none
- [ ] 2.5 payzoff: test plan 1–6 with Tailscale

## 3. NetBird

- [ ] 3.1 `netbird_up`: Check = connected; `--hostname`; after the login, a note when the peer awaits approval
- [ ] 3.2 `connect`: the NetBird FQDN; tests with NetBird, and both
- [ ] 3.3 A setup key with an auto-group: the item's prompt says so
- [ ] 3.4 `netbird_up` records the join, and `yardo forget` runs `netbird down` + deregisters it (as it does `tailscale logout`): pmox delete doesn't log VMs out, so forget is the only way a deleted VM leaves the network

## 4. WireGuard and OpenVPN servers

- [ ] 4.1 Ask the endpoint (public name/address; the VM's address by default) and the tunnel (split, default; or all traffic)
- [ ] 4.2 `yardo-vpn`: Endpoint/`remote` from the answer; split: `AllowedIPs = 10.9.0.0/24`, no DNS line; OpenVPN: no `redirect-gateway`/DNS push
- [ ] 4.3 A change of endpoint or tunnel rewrites the users' profiles, with a note to re-import
- [ ] 4.4 Note: forward UDP 51820/1194 to the VM's LAN address; check NAT loopback or add a local DNS entry for the endpoint
- [ ] 4.5 `connect`: 10.9.0.1/10.8.0.1 as the VM's name, in the ssh block
- [ ] 4.6 Tests: profiles for split and full, endpoint by name, rewrite

## 5. Client VPNs

- [ ] 5.1 WireGuard client: policy rules keep the LAN, 100.64.0.0/10 and 10.8/10.9.0.0/24 off the tunnel
- [ ] 5.2 OpenVPN client: `route … net_gateway` exclusions for the same
- [ ] 5.3 Tests; payzoff: ssh by LAN and Tailscale with a client up

## 6. Docs

- [ ] 6.1 README "Away from home, and back" (Tailscale part written): per-VM client, no port forwards or /24 routes, keys (tagged or reusable, not ephemeral), sample Tailscale grants and NetBird policy, one mesh client on the laptop, self-hosted NetBird on a VPS, tmux

- [ ] 6.2 README: WireGuard/OpenVPN servers at home: dynamic DNS, the port forward, NAT loopback or split DNS, on-demand rules as a fallback
