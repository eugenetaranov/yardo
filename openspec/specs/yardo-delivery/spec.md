# yardo-delivery Specification

## Purpose
How yardo reaches a VM: copied over SSH after boot rather than carried in cloud-init user-data, and refreshed on existing VMs with `pmox yardo push`.

## Requirements

### Requirement: yardo copied over SSH at launch
When yardo is enabled for the server, `pmox launch` SHALL install yardo's files after the SSH and boot waits and before any hook, by streaming one gzip tar over SSH and extracting it as root at `/`. pmox SHALL NOT add yardo's files to the VM's user-data. A failed copy SHALL print a warning naming the VM and `pmox yardo push <vm>`, and SHALL NOT fail the launch or remove the ready tag.

#### Scenario: Normal launch
- **WHEN** `pmox launch web1` runs with yardo enabled
- **THEN** the uploaded user-data has no `/usr/local/bin/yardo` entry, and after launch `/usr/local/bin/yardo` and `/usr/local/share/yardo/` exist on the VM

#### Scenario: Hook can use it
- **WHEN** `pmox launch web1 --post-create ./setup.sh` runs and `setup.sh` calls `sudo yardo --yes`
- **THEN** yardo is already installed when the hook starts

#### Scenario: Copy fails
- **WHEN** the SSH copy fails
- **THEN** launch exits 0 with the VM marked ready, and stderr has a warning that includes `pmox yardo push web1`

#### Scenario: No SSH wait
- **WHEN** `pmox launch web1 --no-wait-ssh` runs
- **THEN** no copy is attempted, and stderr says yardo was not installed and how to push it

### Requirement: `pmox yardo push`
`pmox yardo push <vm>` SHALL install or refresh yardo's files on a running VM using the same tarball as launch, and SHALL leave `/var/lib/yardo` untouched. It SHALL resolve the VM like `pmox shell` does, including the interactive picker when no VM is given on a terminal.

#### Scenario: Refresh keeps answers
- **WHEN** a VM has saved answers and `pmox yardo push web1` runs
- **THEN** the script and share dir match this pmox version, and `/var/lib/yardo/answers.json` is unchanged

### Requirement: Static binary per architecture
pmox SHALL install yardo as the static binary for the VM's architecture (from `uname -m`) at `/usr/local/bin/yardo`, with everything it needs embedded in it. pmox ships both builds in one copy, and the VM installs the one for its architecture. Until every item is ported, the bash yardo is copied too, to `/usr/local/share/yardo/yardo.sh`; the Go binary runs it for unported items and hands it the picker. The user-data size SHALL NOT depend on yardo.

#### Scenario: arm64 VM
- **WHEN** yardo is pushed to a VM whose `uname -m` is `aarch64`
- **THEN** `/usr/local/bin/yardo` is the linux/arm64 build, and `yardo --version` prints the pmox version

#### Scenario: Unsupported architecture
- **WHEN** the VM's architecture is neither x86_64 nor aarch64
- **THEN** the bash yardo is installed instead, and a warning names the architecture

### Requirement: Team catalog shipped by pmox
`pmox yardo push` (and the launch push) SHALL copy `~/.config/pmox/yardo/catalog.d/*.toml` to `/etc/yardo/catalog.d/`, and `~/.config/pmox/yardo/mcp-catalog/*.spec` to the VM's MCP catalog, when they exist. Files it copied earlier but that are gone locally SHALL be removed. Other files in those directories SHALL be left alone.

#### Scenario: Company MCP server
- **WHEN** `~/.config/pmox/yardo/mcp-catalog/acme-docs.spec` exists and `pmox yardo push web1` runs
- **THEN** the MCPJungle server list on web1 offers `acme-docs`

### Requirement: Apply and verify across VMs
- `pmox yardo apply [--all | --tag <t> | <vm>...] --profile <file>` (repeatable `--profile`) SHALL push to each selected running VM and run `sudo yardo --profile … --yes`, at most 4 at a time. It SHALL print one result line per VM, and exit non-zero if any VM failed.
- `pmox yardo verify` with the same selectors SHALL run `yardo --verify`, with the profiles when given, and report missing items and drift per VM.

#### Scenario: Team rollout
- **WHEN** `pmox yardo apply --tag team --profile team.toml` runs against three VMs and one is unreachable
- **THEN** two VMs report ok, one reports the SSH error, and the exit code is non-zero

#### Scenario: Drift
- **WHEN** one VM has had `starship` removed by hand
- **THEN** `pmox yardo verify --tag team --profile team.toml` reports `starship` missing for that VM only
