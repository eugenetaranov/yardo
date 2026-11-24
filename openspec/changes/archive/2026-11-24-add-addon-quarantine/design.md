## Context

- After phase 1, specs are read as `nobody`. Dangerous Docker options are
  refused at registration. Skill writes use `os.Root`.
- After phase 2, skills can be held off; the Skills screen exists.
- yardo installs release binaries as `run` steps with a pinned version
  and a sha256 per architecture (mcpjungle in `80-ai.toml`).
- skill-veil v0.3.0 (MIT, Rust) ships Linux amd64 and arm64 tarballs of
  about 4 MB, plus `checksums.txt`. It works offline with built-in rules.
  `skill-veil scan-package <dir> --format json` gives allow, require
  approval or block, with findings by rule id. Its network and AI features
  are opt-in. Its "semantics" mode is opt-in at build time and isn't in
  the release.
- The security and AI reviews made for this plan list the checks used
  below.

## Goals / Non-Goals

**Goals:**
- Simple, blunt attacks are stopped or shown before install: credential
  theft, fetch-and-run, a container onto the host, a skill that lies about
  what it does.
- Every decision is the user's, with the evidence in front of them.

**Non-Goals:**
- Running add-ons in a sandbox to watch them.
- Signatures, CVE scans, reputation lookups.
- Re-checking yardo's own vendored skills on the VM. They're reviewed and
  hashed in the repo.

## Decisions

- **One package, `internal/addon`, one held item per add-on**:
  `/var/lib/yardo/quarantine/<user>/<id>/` holds `files/` (0600, root),
  `meta.json` (kind, name, source, pin, tree hash) and `report.json`.
  `id` is random. All work on it is in Go, as root, through `os.Root`.
- **Fetching**:
  - **Skills:** resolve `@ref` (default branch when none) to a commit
    with GitHub's API (`/repos/{o}/{r}/commits/{ref}`, with the user's
    `gh` token when there is one, for the rate limit), download the codeload
    tarball, unpack only `path/` in Go, and take regular files only.
    `SKILL.md` folders under the path are the skills offered to pick.
  - **Images:** `docker pull` by tag, take `RepoDigests`, and
    `docker image inspect` for the config. `docker create` and `run` are
    never used.
  - **URLs:** https only; nothing is fetched from them until install.
- **Check order**: basic checks → skill-veil → (phase 4: Claude). Each
  layer adds findings. The verdict is the worst of them:
  `block` (basic, can't be overridden) > `needs OK` > `note`.
  Fixes change `files/` and re-run every layer.
- **Basic checks are Go rules with ids**, each with a fixed next step.
  Limits: 1 MB a file, 5 MB and 300 files in all. "Programs" means ELF,
  Mach-O, PE or wasm magic. Archives means zip, tar, gz, whl or jar
  magic. Allowed Docker options for servers from elsewhere: `-e K=V`,
  `--tmpfs`, `--memory`, `--cpus`, `--network none`, `--read-only`,
  `--cap-drop`, `--security-opt no-new-privileges`. Phase 1's deny list
  still applies to every server.
- **skill-veil as a catalog item** `skillveil`: a pinned release with a
  sha256 per architecture, in `/usr/local/bin/skill-veil`. It's added to
  the plan when the user first adds from elsewhere. It runs as
  `setpriv --reuid=nobody --regid=nogroup --clear-groups` under
  `systemd-run --pipe --wait -p PrivateNetwork=yes -p ProtectHome=yes
  -p RuntimeMaxSec=60`, with `HOME` set to an empty temporary folder, so
  `init` and caches can't reach anything. The held files are copied for it
  into a folder `nobody` can read, which is removed afterwards. Its
  `block` becomes **needs OK**, not a hard block, because it can raise
  false alarms. Its `require_approval` becomes a **note** with its rule
  id.
- **MCP specs from elsewhere**: a parser accepts `KEY="value"`,
  `KEY='value'` and comments. Anything else marks the spec as code. A data
  spec is written back by yardo, re-quoted, as `~/.mcp/<name>/spec`. A
  code spec is stored as given after the user's OK. Its hash is recorded,
  and `mcp-sync` refuses a spec whose hash isn't in the record (the
  record is readable by the gateway user). The form for an image or URL
  produces a data spec: `KIND`, `IMAGE=<repo>@sha256:…`, `URL`,
  `DESCRIPTION`, `REQUIRED`, `NETWORK=0|1`.
- **Approval record**: TOML at `/var/lib/yardo/approved/<user>.toml`,
  written atomically, entries keyed by kind and name. `/var/lib/yardo` is
  root's alone (0700), so the server specs' hashes go to a second list the
  gateway user can read: `/etc/mcpjungle/<user>.approved` (0644, a
  `name hash` line each), which `mcp-sync` checks. A skill installs
  through the skills step's `write` with yardo's usual marker; the Skills
  screen tells "checked" from "yardo's" by the approval record, not the
  marker.
- **mcp-sync current**: an install from elsewhere first brings the
  installed `mcp-sync` up to date (the gateway item's `go:mcp_files`
  step), since an older one doesn't check the list. `mcp-sync NAME…`
  saves its names as the list, so a sync after an install passes them all.
- **Decline and images**: a pulled image is removed on decline only when
  yardo pulled it and it wasn't there before; a spec's image is never
  removed.
- **Typed OK**: the add-on's name, as in GitHub's dangerous actions.

## Risks / Trade-offs

- [skill-veil false alarms annoy] → needs OK, not a block; the rule id is
  shown so it can be judged.
- [skill-veil itself is hostile, or goes unmaintained] → pinned and
  hashed, run as nobody with no network and no home. Updating it is a
  yardo release. It can be dropped without changing anything else.
- [GitHub API rate limit with no token] → 60 requests an hour; the error
  says to log in with `gh`.
- [Image layers aren't checked] → the report says so plainly; phase 5
  limits what a container can do.
- [A user adds a skill outside yardo (`npx skills add`)] → it shows as not
  checked on the Skills screen, with Check it.

## Migration Plan

New paths only. Existing skills not yardo's show as not checked; nothing
is turned off on its own.
