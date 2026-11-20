## ADDED Requirements

### Requirement: VPN logins are idempotent and named
The Tailscale and NetBird login steps SHALL treat a client that is already logged in as done, whatever key was typed, and SHALL record the join, so `yardo forget` logs the VM out (`tailscale logout`; `netbird down` and `netbird logout`). NetBird's first login SHALL set `--hostname` to the VM's hostname (Tailscale names the node after it already). Tailscale SHALL be set with `--accept-routes=false` and `--operator=<user>`; tags come from a tagged auth key, which the key prompt suggests. On a joined VM, Tailscale's settings SHALL be applied with `tailscale set` when they've drifted, never by logging in again.

#### Scenario: Already on the tailnet
- **WHEN** `tailscale status --json` reports BackendState `Running` and a new auth key was typed
- **THEN** yardo doesn't run `tailscale up`, and notes that the VM is already joined and how to rejoin

#### Scenario: First join
- **WHEN** Tailscale isn't logged in and an auth key was typed on VM payzoff
- **THEN** yardo runs `tailscale up` with the key from a file, `--accept-routes=false` and `--operator=<user>`

#### Scenario: Settings drifted on a joined VM
- **WHEN** a joined VM accepts subnet routes, or has no operator
- **THEN** yardo runs `tailscale set --accept-routes=false --operator=<user>`

#### Scenario: NetBird awaiting approval
- **WHEN** after `netbird up` the peer is waiting for approval
- **THEN** the run's notes say to approve it in the NetBird dashboard

### Requirement: connect uses the VPN name
`yardo connect` SHALL put the VM's VPN first: its address, with its DNS name beside it (Tailscale `Self.DNSName`, NetBird's FQDN). The commands and URLs it prints SHALL use the VPN address (a name needs the laptop to resolve it, and another VPN's DNS can take names over); it SHALL say the name works too. It SHALL print a `~/.ssh/config` block with one `Host` for the VM, the VPN address as `HostName`, and `HostKeyAlias`, `ServerAliveInterval` and `ServerAliveCountMax`. It SHALL say to use it at home too, since VPN-only services aren't on the LAN.

#### Scenario: Tailscale VM
- **WHEN** the VM is on a tailnet as `payzoff.example.ts.net` at 100.64.1.7
- **THEN** connect says to use 100.64.1.7 at home and away, that `payzoff.example.ts.net` works too, prints `ssh://<user>@100.64.1.7` and the MCP URL at that address, and a `Host payzoff` block with `HostName 100.64.1.7`

#### Scenario: NetBird VM
- **WHEN** the VM is connected to NetBird as `payzoff.netbird.cloud`
- **THEN** connect names it beside the NetBird address, alone or with Tailscale's

#### Scenario: No VPN name
- **WHEN** the VPN client reports no DNS name
- **THEN** connect uses the VPN address and says why there's no name

#### Scenario: Expiring key
- **WHEN** the VM is an untagged Tailscale node with key expiry on
- **THEN** connect says when the key expires and how to turn expiry off for the VM

### Requirement: No topology that bypasses the VPN interfaces
yardo SHALL NOT set a subnet route, an exit node or a router port forward on a VM it sets up. Its docs SHALL recommend a client on each VM, reached by its VPN name, and SHALL say why a /24 route of the home LAN breaks yardo's VPN-only services and collides with other networks.

#### Scenario: Login flags
- **WHEN** yardo logs a VM into Tailscale
- **THEN** no `--advertise-routes`, `--advertise-exit-node` or `--accept-routes=true` is passed

### Requirement: Client VPNs keep the VM reachable
While the WireGuard or OpenVPN client on a VM is connected, replies from the VM's local address and traffic to its local network SHALL NOT go into that tunnel, whatever the profile routes: a hook run when the tunnel comes up (`wg-quick@`, `openvpn-client@`, `openvpn@`) routes them through the local gateway (table 5400). Tailscale's and NetBird's own routing rules come first, so they keep working too.

#### Scenario: ssh while connected out
- **WHEN** the VM's WireGuard client is up with `AllowedIPs = 0.0.0.0/0`
- **THEN** ssh to the VM from the LAN and over Tailscale still works
