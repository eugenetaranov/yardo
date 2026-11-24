# yardo-addon-review Specification

## Purpose
The path every skill and MCP server from outside yardo's catalog takes before an agent can use it: fetched at a fixed version into a holding area, checked, shown to the user, and installed only with their approval, exactly as checked.

## Requirements

### Requirement: Add-ons from elsewhere are held first
A skill from GitHub, or an MCP server from a Docker image, an HTTP URL or a spec file, SHALL be fetched into a holding area first. It SHALL NOT reach a skills folder, the MCP gateway or the catalog until the user approves it. Several add-ons MAY be added at once, and each SHALL be checked and decided on its own.

#### Scenario: Three skills
- **WHEN** the user adds `acme/skills/a`, `acme/skills/b` and `bob/c`
- **THEN** three reports are shown, one for each, and no skills folder has `a`, `b` or `c` yet

### Requirement: Sources are pinned when fetched
A GitHub ref SHALL be resolved to a full commit, and a Docker image tag to a digest, when the add-on is fetched. The commit or digest SHALL be what is checked, recorded and installed. An HTTP server's URL SHALL be https. A source that can't be pinned SHALL be refused.

#### Scenario: A branch
- **WHEN** the user adds `acme/skills/a@main`
- **THEN** the report shows the commit `main` pointed to, and that commit is what gets installed

#### Scenario: Plain http
- **WHEN** the user adds an MCP server at `http://example.com/mcp`
- **THEN** it is refused, saying the address must be https

### Requirement: The holding area is private and inert
The holding area SHALL be `/var/lib/yardo/quarantine/<user>/`, owned by root, mode 0700, outside the user's home. Its files SHALL NOT be executable. Nothing in it SHALL be run, sourced or loaded by an agent. A Docker image SHALL be pulled and inspected, never started, until approved.

#### Scenario: An agent looks
- **WHEN** a skill is held and an agent running as the user lists or reads the holding area
- **THEN** it is denied

#### Scenario: An image held
- **WHEN** an MCP image is held
- **THEN** `docker ps -a` shows no container from it

### Requirement: Basic checks can't be overridden
yardo SHALL refuse an add-on, with no way to install it as it is, when its source isn't pinned; it has links, absolute paths, `..`, programs, archives or hidden Unicode; it's too big; its name isn't valid or is taken; a skill's frontmatter has hooks or grants shell or write tools; or a server's Docker options are outside the allowed list or ask for `~/.aws` or `~/.kube`. The report SHALL say which check, where, and what would fix it.

#### Scenario: A program inside a skill
- **WHEN** a held skill contains `bin/helper`, an ELF file
- **THEN** the report shows it can't be installed because of `bin/helper`, with Remove this file and Decline as next steps

#### Scenario: Fixed and checked again
- **WHEN** the user picks Remove this file for `bin/helper`, and nothing else blocks
- **THEN** the skill is checked again without it, and Install is offered

### Requirement: Basic checks warn with file and line
yardo SHALL report as warnings, each with file and line:
- every web address;
- fetch-and-run (`curl`, `wget`, `| sh`, `base64 -d`, `eval`);
- `sudo` and `crontab`;
- paths to secrets (`~/.ssh`, `~/.aws`, `~/.kube`, `.env`, tokens);
- text asking an agent to ignore its instructions or to hide what it does;
- HTML comments in a SKILL.md;
- names one or two letters from an existing skill or server;
- install scripts in package files;
- an image that runs as root.

#### Scenario: A secret path
- **WHEN** a skill's SKILL.md line 42 says to read `~/.aws/credentials`
- **THEN** the report lists `SKILL.md:42` with the path, as a warning

### Requirement: skill-veil scans each add-on
yardo SHALL scan each held skill and spec with skill-veil, using the version yardo pins and checked by its sha256. The scan SHALL run as a user who can't read the user's home, with no network, a time limit, and none of skill-veil's online or AI features. Its findings SHALL be shown with file and line, marked as skill-veil's. A skill-veil block SHALL need the user's OK. A failed or timed-out scan SHALL be reported as "not scanned", as a warning.

#### Scenario: skill-veil blocks
- **WHEN** skill-veil reports a credential-theft finding at `SKILL.md:88`
- **THEN** the report shows it as needing the user's OK, with `SKILL.md:88` and skill-veil named

#### Scenario: No network
- **WHEN** skill-veil runs
- **THEN** it can't open any network connection

### Requirement: Specs with code are decided spec by spec
An MCP spec from elsewhere that is only `KEY="value"` lines SHALL be checked as data and never run. A spec with anything else (functions, commands, substitutions) SHALL be shown in full with line numbers and the lines that aren't data marked, and SHALL need the user's OK for that spec. An approved spec SHALL be recorded by its hash, and a changed one SHALL need approving again.

#### Scenario: Data only
- **WHEN** a spec has only `DESCRIPTION=`, `KIND=`, `IMAGE=` and `REQUIRED=` lines
- **THEN** its report has no code to approve

#### Scenario: A function
- **WHEN** a spec defines `skeleton()` with a `curl` inside it
- **THEN** the report shows the spec with the function's lines marked, and the `curl` as a warning, and installing needs the user's OK

### Requirement: The report and the decision
Each held add-on SHALL have a report showing:
- its name and source, with the pin;
- whether it can be installed, needs the user's OK, or has only notes;
- every finding, with its file, line, what was found and what to do.

The user SHALL be able to Install, Fix and check again, or Decline and remove. Installing with any finding that needs the user's OK SHALL require typing the add-on's name.

#### Scenario: Needs OK
- **WHEN** a report has a skill-veil finding and the user picks Install
- **THEN** yardo asks them to type the skill's name, and installs only when it matches

#### Scenario: Declined
- **WHEN** the user picks Decline and remove
- **THEN** the add-on's files and pulled image are gone from the machine, and nothing was installed

### Requirement: Installed exactly as approved
Install SHALL write the very bytes that were checked, or register the very image digest and spec that were checked. A held add-on that changed after its check SHALL be checked again before install.

#### Scenario: Changed while held
- **WHEN** a held file differs from the hash in its report at install time
- **THEN** nothing is installed, and the add-on is checked again

### Requirement: Approvals are recorded
yardo SHALL keep each approval in `/var/lib/yardo/approved/<user>.toml`, owned by root and not writable by the user. Each entry SHALL hold:
- the source and its pin;
- the hash of the files or spec;
- the findings and the user's OK where given;
- who approved it, when, and yardo's version.

Uninstalling, or deleting a server, SHALL remove its entry.

#### Scenario: After install
- **WHEN** skill `a` is installed from `acme/skills@<commit>`
- **THEN** the record has `a` with that commit, its hash, its findings and the approver

### Requirement: Checking a skill already there
A skill in a skills folder that isn't yardo's SHALL be offered **Check it**. It SHALL be copied into the holding area and turned off while it is checked. If approved, it SHALL be recorded and turned back on. If declined, it SHALL stay off, with Delete it offered.

#### Scenario: A skill another tool installed
- **WHEN** the user picks Check it on `archify-extra`, which was installed by another tool, and approves it
- **THEN** it's on again, recorded, and listed as checked

### Requirement: Adding from the command line
`sudo yardo skills add <ref …>` and `sudo yardo mcp add <image|url|spec>` SHALL fetch, check and print each report, then ask for each add-on. With `--yes`, an add-on SHALL be installed only when nothing in its report needs the user's OK. `sudo yardo addons` SHALL list held and approved add-ons with their pins.

#### Scenario: Unattended
- **WHEN** `sudo yardo skills add acme/a acme/b --yes` runs, and `b` has a skill-veil finding
- **THEN** `a` is installed, `b` stays held, and the output says `b` needs the user's OK
