## Purpose

How yardo keeps the skills and MCP servers it installs from reaching beyond what they are for: how specs are read, which Docker options a server may have, and what the user is told about the risks that remain.

## ADDED Requirements

### Requirement: MCP specs are read without root
When yardo reads an MCP spec's fields (its description, image or URL, required keys and env skeleton), it SHALL do so as an unprivileged user with an empty environment, never as root or as the user. This SHALL hold for yardo's catalog specs, team specs a profile ships, and every screen, command and run that reads them.

#### Scenario: A spec that runs a command
- **WHEN** a team spec in the catalog has `touch /root/pwned` above its fields and the TUI opens
- **THEN** `/root/pwned` doesn't exist, and the spec's fields are read as before

#### Scenario: Nothing of the caller's environment
- **WHEN** `sudo yardo` runs with `AWS_SECRET_ACCESS_KEY` set and a spec prints `$AWS_SECRET_ACCESS_KEY` in its description
- **THEN** the description shown is empty where the variable was

### Requirement: Dangerous Docker options are refused
A Docker MCP server SHALL NOT be registered when its spec's Docker options include `--privileged`, a host namespace, `--cap-add`, a device, a published port, a security option that turns off confinement, or a mount of `/`, a system folder, the Docker socket, or a home outside the server's own `~/.mcp/<name>`. Its status SHALL be `refused`, naming the option. yardo's own mounts (`MOUNT_AWS`, `MOUNT_KUBE`) SHALL stay allowed.

#### Scenario: Privileged
- **WHEN** a spec sets `DOCKER_ARGS="--privileged"` and the servers sync
- **THEN** the server isn't registered, and `yardo mcp status` shows it refused for `--privileged`

#### Scenario: The host's root
- **WHEN** a spec sets `DOCKER_ARGS="-v /:/host"`
- **THEN** the server isn't registered, and its status names the mount

#### Scenario: The catalog's servers
- **WHEN** every spec in yardo's catalog is synced with its required keys filled in
- **THEN** none is refused

### Requirement: yardo's own specs name their exceptions
A spec yardo ships in its catalog MAY name, in `HOST_ACCESS`, the host access it needs beyond what's allowed: the Docker socket (the Docker server) or the host's network (a database on this machine). Only those options SHALL then be allowed for it. `HOST_ACCESS` SHALL count for nothing in a team's spec, a spec in `~/.mcp`, or a spec from elsewhere.

#### Scenario: The Docker server
- **WHEN** yardo's `docker` server, whose spec has `HOST_ACCESS=docker-socket`, is synced
- **THEN** it is registered with the Docker socket mounted

#### Scenario: A team spec
- **WHEN** a team's spec mounts the Docker socket and sets `HOST_ACCESS=docker-socket`
- **THEN** the server is refused for the socket mount

### Requirement: The Docker group's risk is named
Wherever yardo offers to put the user in the docker group, it SHALL say plainly that anything running as the user, coding agents and their skills included, can then act as root on the machine.

#### Scenario: Docker's screen
- **WHEN** the user opens Docker on the Tools screen
- **THEN** it says that in the docker group, anything run as them, agents and skills included, can act as root
