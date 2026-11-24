## Context

- Phases 3–4 hold, check (basic, skill-veil, Claude) and record each
  approval: source, pin, tree hash, findings. Skills from elsewhere carry
  a `checked` marker. Servers from elsewhere register from an approved
  spec with `IMAGE=<repo>@sha256:…`.
- `mcp-sync` talks to MCPJungle through `mj` (`mcpjungle` with the
  gateway user's config). Its `render` builds `docker run -i --rm …`.
- Phase 1 refuses dangerous Docker options for every server.

## Goals / Non-Goals

**Goals:**
- No add-on changes under the user without a check and their OK.
- A container that got through is boxed in by default.

**Non-Goals:**
- Updating yardo's vendored skills this way. They update with yardo's
  releases, reviewed in the repo.
- Per-container egress filtering (an allow list of hosts). It's either no
  network or the default network.
- Checking automatically on a timer. Checks run when yardo runs or the
  screen opens.

## Decisions

- **Outdated check**:
  - **Skills:** one GitHub API call per source (the ref the user gave:
    branch, tag, or default branch). The commit is compared with the
    approved one. A pinned commit given by the user is never outdated.
  - **Images:** `docker buildx imagetools inspect` or the registry's
    manifest HEAD for the tag the user gave, comparing digests, without
    pulling.
  
  Results are cached for an hour in `/var/lib/yardo/addons-outdated.json`.
- **Approved copies**: on approval, phase 3's install writes the bytes it
  installs to `/var/lib/yardo/approved/<user>/<kind>/<name>/` too (root
  0700), and the record points to it. The diff is computed in Go (line
  diff, text files only; binaries can't exist, because phase 3 blocks
  them).
- **Update input to Claude** (phase 4's input, plus):
  - a "changes since approved" section with the unified diff;
  - the full new SKILL.md;
  - the full text of every added or changed script, meaning `.sh`, `.py`,
    `.js` and `.ts`, or any file with a shebang.
  
  The same size limits apply, and the diff goes first.
- **Drift**: the skills step's existing ownership check (marker hash vs
  content) is extended for `checked` skills to compare with the approval
  record's tree hash. A mismatch is a home row ("changed since checked")
  and a Skills row state. Restore copies the approved copy back as the
  user, through `os.Root`. It isn't turned off by itself: the user's own
  edits are common, and the flag is enough at this scope.
- **Remote tool lists**: after registering a remote server,
  `mj list tools --server <name>` (or the gateway's JSON equivalent)
  gives names, descriptions and input schemas. Their sha256 is stored in
  the record on approval. Each sync compares them. A mismatch runs
  `mcp-sync off <name>` with reason `tools-changed`, and the row offers
  Check it again. That runs phase 3–4's checks on a text file of the tool
  list.
- **Container limits** in `render`, before spec options:
  - `--cap-drop ALL`;
  - `--security-opt no-new-privileges`;
  - `--read-only --tmpfs /tmp:rw,nosuid,size=256m`;
  - `--pids-limit 256 --memory 1g`;
  - `--user 65534:65534`, unless `MOUNT_AWS` or `MOUNT_KUBE` set a user;
  - `--network none` for servers from elsewhere unless `NETWORK=1`.
  
  `HOME=/tmp` is set so `npx` images can write a cache. Catalog specs can
  set `LIMITS="writable root-user …"` to name what they go without. The
  parser refuses `LIMITS` in specs from elsewhere.
- **Rolling limits out to the catalog**: each catalog spec is tried with
  every limit on a VM. Only those that fail get a `LIMITS` line, and
  the line has a comment saying why.

## Risks / Trade-offs

- [A catalog server breaks under the limits] → per-spec `LIMITS`, tried
  first on a VM, as part of the tasks; `sudo yardo mcp restart` applies
  changes.
- [Registry rate limits on outdated checks] → one HEAD per image per hour.
- [A user edits a checked skill on purpose] → the flag shows on the home
  until they pick Check it again, which re-approves their version.
- [`mj list tools` output format differs across MCPJungle versions] →
  normalised to sorted JSON before hashing; yardo pins MCPJungle.

## Migration Plan

- Approvals from phase 3 without a kept copy: the first update does a full
  review (no diff), then keeps a copy. Drift works from the recorded hash
  alone.
- Limits for catalog servers ship after the VM trial, in the same release.
  `sudo yardo mcp restart` after upgrading picks them up. The release
  notes say so.

## As built

- Copies live at `/var/lib/yardo/approved/<user>/<kind>/<name>/` (root,
  0700/0600), next to the approval record.
- An image's latest digest comes from `docker buildx imagetools inspect`
  (registry only, nothing pulled), not a hand-made HEAD request.
- A remote server's tools come from MCPJungle's REST API
  (`/api/v0/tools?server=`), normalised with jq, not from `mj list tools`.
- mcp-sync's `off` file holds the reason on its first line (`own`,
  `tools-changed`); an empty one reads as `own`.
- The scratch `/tmp` is `exec`: npx and uvx run what they fetch from
  there. VM trial of the catalog under every limit: azure, bitbucket,
  confluence, figma, github, grafana, jira, k8s, playwright, postgres and
  sequential-thinking register; aws starts and stops only for want of
  real credentials;
  docker needs `root-user` (the socket), opensearch `writable` (uvx's
  dependencies outgrow 256 MB); mysql and slack fail with placeholder
  settings with or without limits. An image from elsewhere whose program
  only root may read (mcp/time) won't start as nobody: the basic check
  says so.
