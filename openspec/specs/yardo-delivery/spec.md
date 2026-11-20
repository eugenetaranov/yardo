# yardo-delivery Specification

## Purpose
How yardo reaches a machine and stays current: a release per version, an
install script that checks it, `yardo version up`, and cloud-init for an
unattended first boot.

## Requirements

### Requirement: Static binary per architecture
Each release SHALL publish yardo as a static binary for linux/amd64 and linux/arm64 (GoReleaser), with everything it needs embedded in it, and a `checksums.txt` of the archives.

#### Scenario: arm64 machine
- **WHEN** yardo is installed on a machine whose `uname -m` is `aarch64`
- **THEN** `/usr/local/bin/yardo` is the linux/arm64 build, and `yardo --version` prints the release's version

### Requirement: Install script
`install.sh`, published with each release, SHALL download the release for the machine's architecture, check it against the release's `checksums.txt`, and install it at `/usr/local/bin/yardo`. `--version vX.Y.Z` (or `YARDO_VERSION`) SHALL pick a release. An unsupported architecture or a checksum mismatch SHALL stop it with an error, installing nothing.

#### Scenario: Pinned version
- **WHEN** `install.sh --version v0.1.0` runs
- **THEN** yardo v0.1.0 is installed, checked against v0.1.0's `checksums.txt`

### Requirement: Upgrade in place
`yardo version up` SHALL download the latest release, check it against `checksums.txt` and that it runs, and replace the binary (as root), leaving `/var/lib/yardo` untouched. `--check` SHALL only say whether a newer release is out. A Homebrew install SHALL upgrade through `brew`.

#### Scenario: Upgrade keeps answers
- **WHEN** a machine has saved answers and `sudo yardo version up` runs
- **THEN** the binary is the latest release, and `/var/lib/yardo/answers.json` is unchanged

### Requirement: Unattended first boot
Installed from cloud-init's `runcmd`, `yardo --profile <file or URL> --yes --user <name>` SHALL set the machine up without a terminal, asking nothing.

#### Scenario: cloud-init
- **WHEN** a machine boots with `runcmd` installing yardo and running `yardo --profile https://example.com/team.toml --yes --user ubuntu`
- **THEN** the profile's items are installed for `ubuntu` and saved as its answers

### Requirement: Team catalog on the machine
yardo SHALL read extra catalog items from `/etc/yardo/catalog.d/*.toml` and extra MCP server specs from the machine's MCP catalog, however they got there (config management, an image, a copy by hand).

#### Scenario: Company MCP server
- **WHEN** `acme-docs.spec` is in the machine's MCP catalog
- **THEN** the MCP servers screen offers `acme-docs`
