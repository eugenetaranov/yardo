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

### Requirement: No size limit from user-data
The files pmox installs SHALL include the full mise registry snapshot (names, aliases and descriptions), and the user-data size SHALL no longer depend on yardo.

#### Scenario: Full registry on the VM
- **WHEN** yardo is pushed to a VM
- **THEN** `/usr/local/share/yardo/mise-registry.tsv` equals `internal/bootstrap/mise-registry.tsv`

