## Context

The team's three reviews (Tailscale, NetBird, security/ops) agree:

| Topology | Home | Away | Fits yardo's firewall |
|---|---|---|---|
| Client per VM, reached by its VPN name | direct over the LAN | direct or relayed | yes: traffic arrives on tailscale0/wt0 |
| Subnet router / routing peer for 192.168.0.0/24 | hairpins or bypasses, depending on OS | collides with 192.168.0.0/24 networks | no: arrives on eth0, SNATed |
| Client on the Proxmox host | — | — | an internet-facing agent on the hypervisor |
| yardo's WireGuard/OpenVPN server on the VM, split tunnel | via NAT loopback or split DNS for the endpoint | via the forwarded UDP port | yes: traffic arrives on yardo-wg/tun0 |

## Decisions

- **One name, the VPN's.** The client on the VM reports its own DNS name:
  Tailscale `tailscale status --json` → `Self.DNSName`, NetBird `netbird
  status --json` → `fqdn`. `connect` uses it everywhere. `HostKeyAlias`
  keeps known_hosts stable if the name's address ever changes.
- **No LAN path for VPN-only services.** We keep "vpn" meaning "VPN
  interfaces only". A laptop at home already takes the direct LAN path
  inside the tunnel, so opening the LAN would only add a second name.
- **Check is "logged in", not "no key".** The key is asked only when the
  client isn't logged in. A key typed for a joined VM is ignored, with a
  note: "already on the tailnet as payzoff; `sudo tailscale logout` first
  to rejoin".
- **`tailscale set` for settings on a joined VM.** `up` refuses partial
  flag changes, and `set` applies only what's given.
- **`--accept-routes=false` explicitly.** The default is false on Linux,
  but an earlier `up --accept-routes` would stick. A VM on 192.168.0.0/24
  that accepts that route sends its own LAN traffic through the router.
- **Tags are optional.** A tagged key needs the tag in the policy's
  tagOwners, and many users have none. Untagged nodes' keys expire
  (180 days by default), so `connect` warns and says how to turn it off.
- **Servers: one address, one endpoint name.** The VM's tunnel address
  (10.9.0.1, 10.8.0.1) is its name for every service. The endpoint is the
  only thing that differs between home and away, and it is a single DNS
  name: public (dynamic DNS) away; at home the router's NAT loopback, or a
  local DNS answer with the LAN address. The laptop's VPN app stays on.
  WireGuard on-demand rules (connect except on the home Wi-Fi) are the
  fallback where the router can't loop back. They're documented, not
  required.
- **Split tunnel by default for new servers.** "Reach my VM" needs only
  10.9.0.0/24 or 10.8.0.0/24. A full tunnel at home sends everything via
  the VM for nothing, and it's what breaks when the endpoint is
  unreachable.
- **Port forwards: only the VPN's UDP port.** Unlike ssh or the MCP
  port, WireGuard doesn't answer unauthenticated packets, and OpenVPN uses
  tls-crypt.
- **Clients don't take the way in.** A client VPN on the VM excludes the
  home LAN, 100.64.0.0/10 and the yardo servers' subnets from its routes.
- **OpenSSH stays.** Tailscale SSH and NetBird SSH are opt-in later.
  OpenSSH carries yardo's hardening, `ssh -A`, docker contexts and VS Code.

## Risks

- NetBird's JSON field names change between versions: parse leniently;
  with no name, `connect` falls back to the address.
- MagicDNS off in a tailnet: `Self.DNSName` is still set, but the laptop
  can't resolve it. `connect` says to turn on MagicDNS, or use the 100.x
  address in `HostName`.
- macOS with "Use Tailscale subnets" on and someone else's /24 route:
  out of yardo's reach; the README says to avoid /24 routes.

## Test plan (on payzoff, from the laptop)

1. At home: `tailscale ping payzoff` shows "via 192.168.0.x" (direct).
   Then ssh, the MCP URL by name, a dev port through `ssh -L`, and iperf3
   at close to LAN speed.
2. Phone hotspot: the same, plus `ping -D -s 1252` (MTU) and a large scp.
3. Switching network: tmux with a running command over ssh, switch Wi-Fi
   to the hotspot. The session carries on, or reconnects to the same tmux.
4. Lid closed for 30+ minutes: VS Code Remote-SSH reconnects without a
   prompt.
5. Negative: from another LAN device, the MCP port is dropped. A scan of
   the home public IP shows nothing open.
6. The laptop removed from the tailnet: access ends at once.
7. WireGuard server (later phase): the same 1–5 with `ssh 10.9.0.1`; at
   home with the endpoint name resolving to the LAN address; the laptop's
   other traffic doesn't go through the VM (split).
8. A client VPN up on the VM: ssh from the LAN and over Tailscale still works.
