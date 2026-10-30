## Why

Your VMs run on Proxmox in the home LAN (payzoff is 192.168.0.x). Away
from home you want to reach each VM's ssh, MCP gateway and dev ports.
Back home it should just work again: the same name, LAN speed, nothing to
switch.

yardo already installs Tailscale and NetBird on the VM and logs in with a
key. What's missing is the rest of the path:

- **`yardo connect` prints IPs, not names.** The 100.x address and the
  192.168.0.x one are each right in only one place, so the laptop ends up
  with two ssh hosts, which is the toggling to avoid.
- **The LAN IP half works and half doesn't.** In "vpn" mode the firewall
  lets the MCP port and Docker ports in only on the VPN interfaces
  (`firewall.go`). ssh by LAN IP works at home, but MCP by LAN IP is
  dropped there, and both break when you leave.
- **The login steps don't know the VM is already joined.** Check only
  asks "was a key typed this run?", so a new key on a joined VM runs
  `up` again. The VM's name is left to chance (`payzoff-1` after a
  collision). Tailscale's `--accept-routes` and operator are unset.
- **Nothing says which topology works with yardo's firewall.** A subnet
  router or NetBird routing peer for 192.168.0.0/24 forwards traffic to
  the VM's LAN interface with the source rewritten, and yardo's VPN-only
  rules drop it. The route also collides with the many cafés and hotels on
  192.168.0.0/24.

A team review (Tailscale, NetBird, security and operations) settled on a
mesh client on every VM, always reached by its VPN DNS name. At home the
client finds the LAN path itself (direct WireGuard over 192.168.0.x, no
relay). The tailnet address doesn't change when the laptop changes
network, so the name, known_hosts and VS Code's Remote-SSH host stay the
same everywhere.

Every VPN yardo offers has to work for this, not only the mesh ones. Today:

- **The WireGuard and OpenVPN servers aren't reachable away from home.**
  Their profiles point at the VM's own address (192.168.0.x for payzoff).
  Away from home they need a public name and a UDP port forwarded on the
  router, and yardo asks for neither.
- **They're full tunnels.** All the laptop's traffic goes through the home
  VM (`AllowedIPs = 0.0.0.0/0`, `redirect-gateway`), when reaching the VM
  only needs its own subnet.
- **The client VPNs can cut off inbound access.** The WireGuard and
  OpenVPN clients connect the VM *out* to another network. A full tunnel
  there can send replies to LAN ssh into that tunnel.

So the rule is the same for every VPN: one address or name, the VPN's,
used both at home and away. Tailscale comes first, then NetBird, then the
WireGuard and OpenVPN servers, then the clients.

## What Changes

- **VPN logins know the VM and name it:**
  - Check skips when the client is already logged in (`tailscale status
    --json` BackendState `Running`, `netbird status` connected), whatever
    key was typed.
  - NetBird: `--hostname` is the VM's hostname (Tailscale already uses it).
  - Tailscale also gets `--accept-routes=false` (a VM on the LAN mustn't
    route its own LAN through a subnet router) and `--operator=<user>`
    (`tailscale serve` without sudo).
  - Tags come from a tagged auth key (the prompt says so; no tags
    prompt). On a joined VM, drifted settings go through `tailscale set`.
  - NetBird: after the login, if the peer waits for approval, say so and
    where to approve it.
- **`yardo connect` by name:**
  - It prints the MagicDNS name (`Self.DNSName`) or NetBird FQDN, and
    uses it in every command it prints: ssh, the docker context, the MCP
    URL and the agent configs.
  - It prints a ready `~/.ssh/config` block for the laptop: one `Host`,
    `HostName` the VPN name, `HostKeyAlias`, `ServerAliveInterval`.
  - It says why to use the name, not 192.168.0.x: the same name at home
    and away, and MCP is blocked on the LAN.
  - Tailscale: it warns when the VM's node key expires (an untagged
    node), and says how to turn expiry off.
- **WireGuard and OpenVPN servers reachable from anywhere:**
  - The item asks for the endpoint: a public DNS name or address (dynamic
    DNS for a home line), the VM's address by default. It goes into every
    profile's `Endpoint`/`remote`.
  - It asks for the tunnel: "this VM and its VPN" (split:
    `AllowedIPs = 10.9.0.0/24`, no `redirect-gateway`, no DNS push; the
    new default) or "all traffic" (as now).
  - Its note says what to forward on the router (UDP 51820 or 1194 to the
    VM's LAN address), and nothing else.
  - At home the laptop reaches the endpoint through the router's NAT
    loopback, or through split DNS or a router DNS entry for the
    endpoint name to the VM's LAN address. The README says how, and
    `yardo connect` says to check it.
  - Changing endpoint or tunnel rewrites existing users' profiles, and
    the note says to re-import them.
- **Client VPNs keep the way back in:** the WireGuard and OpenVPN clients
  on the VM keep the routes to the LAN and the mesh VPNs off the tunnel
  (WireGuard: a policy rule for the LAN, tailnet and NetBird ranges;
  OpenVPN: `route` exclusions), so ssh still reaches the VM while it's
  connected.
- **`connect` for every VPN:** for the WireGuard and OpenVPN servers, the
  VM's tunnel address (10.9.0.1, 10.8.0.1) is the one name, in the same
  ssh block.
- **README, "Reaching your VMs away from home":**
  - A client on each VM; no router port forwards; no /24 subnet route
    (if one is needed, a /32 for the Proxmox UI, from an LXC).
  - Keys: tagged or reusable, never ephemeral for VMs.
  - A sample Tailscale grants policy and NetBird policy.
  - Only one mesh client on the laptop (both use 100.64.0.0/10).
  - A self-hosted NetBird belongs on a VPS, not on the home LAN.
  - tmux for sessions that last.
- Tests: the login steps' Check, Apply and flags with a fake
  `tailscale`/`netbird`; `connect` with names, without them, and with
  both VPNs.

Not in this change: Tailscale SSH and NetBird SSH (OpenSSH stays: agent
forwarding, docker `ssh://` contexts and VS Code all work over it); a
`tailscale serve` HTTPS mode for the MCP gateway; an nft rule limiting
ssh to lo, VPN and LAN; mosh's UDP range on the VPN interfaces; a
subnet-router item. Each is a follow-up once this is in use.

## Impact

- `internal/steps/system.go` (tailscale_up, netbird_up),
  `cmd/yardo/connect.go`, the VPN items' prompts (tags; endpoint and
  tunnel for the servers), `conf/yardo-vpn` and `openvpn-server.conf`
  (endpoint, split tunnel), the client items, README.
- Existing WireGuard/OpenVPN users: their profiles stay full-tunnel until
  the item is applied again with the new answers.
- No change for a VM already joined: the new Check sees it and skips;
  `--hostname`/`--accept-routes`/`--operator` are applied with `tailscale
  set` on the next run.
