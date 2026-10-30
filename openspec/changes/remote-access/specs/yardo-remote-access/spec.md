## ADDED Requirements

### Requirement: VPN logins are idempotent and named
The Tailscale and NetBird login steps SHALL treat a client that is already logged in as done, whatever key was typed. A first login SHALL set the client's hostname to the VM's hostname. Tailscale SHALL be set with `--accept-routes=false` and `--operator=<user>`, plus `--advertise-tags` when tags were given. On a joined VM, Tailscale's settings SHALL be applied with `tailscale set`, never by logging in again.

#### Scenario: Already on the tailnet
- **WHEN** `tailscale status --json` reports BackendState `Running` and a new auth key was typed
- **THEN** yardo doesn't run `tailscale up`, and notes that the VM is already joined and how to rejoin

#### Scenario: First join
- **WHEN** Tailscale isn't logged in and an auth key was typed on VM payzoff
- **THEN** yardo runs `tailscale up` with the key from a file, `--hostname=payzoff`, `--accept-routes=false` and `--operator=<user>`

#### Scenario: NetBird awaiting approval
- **WHEN** after `netbird up` the peer is waiting for approval
- **THEN** `~/yardo.txt` says to approve it in the NetBird dashboard

### Requirement: connect uses the VPN name
`yardo connect` SHALL print the VM's VPN DNS name (Tailscale `Self.DNSName`, NetBird's FQDN) and use it in every command and URL it prints, with the address only when there's no name. It SHALL print a `~/.ssh/config` block with one `Host` for the VM, its VPN name as `HostName`, and `HostKeyAlias`, `ServerAliveInterval` and `ServerAliveCountMax`. It SHALL say to use that name at home too, since VPN-only services aren't on the LAN.

#### Scenario: Tailscale VM
- **WHEN** the VM is on a tailnet as `payzoff.example.ts.net`
- **THEN** connect prints `ssh <user>@payzoff.example.ts.net`, the MCP URL by that name, and a `Host payzoff` block with `HostName payzoff.example.ts.net`

#### Scenario: No VPN name
- **WHEN** the VPN client reports no DNS name
- **THEN** connect uses the VPN address and says why

#### Scenario: Expiring key
- **WHEN** the VM is an untagged Tailscale node with key expiry on
- **THEN** connect says when the key expires and how to turn expiry off for the VM

### Requirement: No topology that bypasses the VPN interfaces
yardo SHALL NOT set a subnet route, an exit node or a router port forward on a VM it sets up. Its docs SHALL recommend a client on each VM, reached by its VPN name, and SHALL say why a /24 route of the home LAN breaks yardo's VPN-only services and collides with other networks.

#### Scenario: Login flags
- **WHEN** yardo logs a VM into Tailscale
- **THEN** no `--advertise-routes`, `--advertise-exit-node` or `--accept-routes=true` is passed

### Requirement: yardo's VPN servers work away from home
The WireGuard and OpenVPN server items SHALL ask for the endpoint clients connect to (a DNS name or address, the VM's address by default) and SHALL put it in every client profile. They SHALL ask for the tunnel: split (only the VPN's subnet; the default) or all traffic. Their note SHALL name the one UDP port to forward on the router. A change of either answer SHALL rewrite every user's profile and say to re-import it.

#### Scenario: Split tunnel by default
- **WHEN** a WireGuard user profile is made with the default answers
- **THEN** its `AllowedIPs` is `10.9.0.0/24` and it sets no DNS

#### Scenario: Endpoint by name
- **WHEN** the endpoint is `home.example.net`
- **THEN** each WireGuard profile has `Endpoint = home.example.net:51820` and each OpenVPN profile `remote home.example.net 1194`

#### Scenario: Server address in connect
- **WHEN** the WireGuard server is set up
- **THEN** connect uses `10.9.0.1` as the VM's address in the ssh block and the URLs

### Requirement: Client VPNs keep the VM reachable
While the WireGuard or OpenVPN client on a VM is connected, traffic to the home LAN, to 100.64.0.0/10 and to yardo's VPN subnets SHALL NOT go into that tunnel.

#### Scenario: ssh while connected out
- **WHEN** the VM's WireGuard client is up with `AllowedIPs = 0.0.0.0/0`
- **THEN** ssh to the VM from the LAN and over Tailscale still works
