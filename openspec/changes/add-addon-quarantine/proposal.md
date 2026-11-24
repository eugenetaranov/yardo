## Why

Today the only skills and MCP servers yardo installs are its own, which
were reviewed in its repository. Users want to add others, like skills
a, b and c from GitHub or MCP servers 1–4 from Docker Hub. A skill is
instructions an agent follows, plus scripts it runs. An MCP server is a
container that receives your tokens. Either can steal credentials or run
`curl | sh` on the dev VM. This change is phase 3 of 5. Nothing from
elsewhere reaches an agent until it has been downloaded into a holding
area, checked, shown to the user and approved.

## What Changes

- **Adding from elsewhere**:
  - Skills, from GitHub (`owner/repo[/path][@ref]`), several at once.
  - MCP servers, from a Docker image or an HTTP URL, with settings
    filled in on a form, or from a spec file.
  
  Each is pinned when it's fetched: a commit, an image digest, or an
  https URL.
- **A holding area**, `/var/lib/yardo/quarantine/<user>/`, owned by root
  and outside the home. Agents can't see it, and nothing in it is run.
  Docker images are pulled and inspected, never started.
- **Basic checks**, built into yardo, that can't be overridden:
  - the source isn't pinned;
  - the files include links, paths that leave the folder, programs,
    archives or hidden Unicode;
  - it's too big;
  - its name is taken;
  - a skill grants itself hooks or shell access;
  - a server's Docker options are outside a short allowed list;
  - a server from elsewhere asks for `~/.aws` or `~/.kube`.
  
  Warnings, each with file and line:
  - web addresses;
  - fetch-and-run;
  - paths to secrets;
  - text telling the agent to hide things;
  - names close to existing ones;
  - an image that runs as root.
- **skill-veil** (a pinned release, checked by sha256) scans each skill
  and spec for prompt injection, credential theft, fetch-and-run, typosquats
  and the like. It runs offline, as a user with no home and no network.
- **Specs with shell code** are shown line by line and approved or
  declined spec by spec. Plain `KEY="value"` specs need no extra step.
- **A report** for each add-on: what it is, where it's from, and each
  finding with its file, line and what to do. The next steps are
  **Install**, **Fix and check again** (remove a file, drop an option,
  edit a copy) or **Decline and remove**. A finding that needs the user's
  OK makes them type the add-on's name to install.
- **An approval record**, owned by root, keeps what was approved (source,
  pin, hash of the files, findings, who and when). Install writes exactly
  the bytes that were checked.
- **Checking a skill already there.** A skill on the Skills screen that
  isn't yardo's can be checked the same way, and adopted if approved.
- **CLI**: `sudo yardo skills add`, `sudo yardo mcp add`, `sudo yardo
  addons` (pending and approved).

Not in this change: the AI review (phase 4), updates and drift, and
limits on running containers (phase 5).

## Capabilities

### New Capabilities
- `yardo-addon-review`: the path every skill or MCP server from elsewhere
  takes before it reaches an agent: fetch pinned, hold, check, report,
  decide, record, install.

### Modified Capabilities
- `yardo-skills-toggle`: the Skills screen adds skills (`n`), shows
  whether each was checked, and offers Check it.
- `yardo-mcp-toggle`: MCP servers' catalog offers "From elsewhere".

## Impact

- New `internal/addon/` package: fetch (GitHub tarball, image
  pull and inspect, URL), holding area, checks, skill-veil runner, approval
  record, install.
- New catalog item `skillveil` (80-ai.toml), a needs-item of adding
  skills or servers from elsewhere.
- `cmd/yardo/skills.go`, `cmd/yardo/mcpservers.go`: add, report and
  decide screens; `cmd/yardo/main.go`: `skills add`, `mcp add`, `addons`.
- `internal/assets/mcp-sync`: register a server from elsewhere only from
  its approved spec, with its image by digest.
- `/var/lib/yardo/quarantine`, `/var/lib/yardo/approved/<user>.toml`.
- README: Adding skills and servers from elsewhere.
