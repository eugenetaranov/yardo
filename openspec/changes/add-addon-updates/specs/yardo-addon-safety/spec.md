## ADDED Requirements

### Requirement: Containers run with limits
Each Docker MCP server SHALL run with:
- every capability dropped;
- no new privileges;
- a read-only file system with a scratch `/tmp`;
- limits on memory and processes;
- an unprivileged user, unless it mounts `~/.aws` or `~/.kube`.

A catalog server that can't run this way SHALL name, in its spec, each limit it goes without. A server from elsewhere SHALL NOT go without any.

#### Scenario: A server from elsewhere
- **WHEN** an approved server from elsewhere starts
- **THEN** its container has no capabilities, no new privileges and a read-only root, and runs as an unprivileged user

#### Scenario: A catalog exception
- **WHEN** a catalog spec says it needs a writable file system
- **THEN** its container is writable and keeps every other limit

### Requirement: No network unless approved
A Docker server from elsewhere SHALL have no network, unless its approved spec says it needs one. Changing that SHALL need approving the spec again.

#### Scenario: No network
- **WHEN** a server from elsewhere is approved without network and tries to reach the internet
- **THEN** its connection fails
