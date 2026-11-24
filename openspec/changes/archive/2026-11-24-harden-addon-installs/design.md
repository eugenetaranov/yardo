## Context

- `mcp.Load` (`internal/mcp/mcp.go`) runs `bash -c '. spec; printf …;
  skeleton'` to read a spec. The TUI (`cmd/yardo/status.go`, `mcpSpecs`)
  and apply (`internal/steps/mcp.go`, `Missing` and the env skeleton)
  call it as root. The catalog dirs include `/usr/local/share/mcp-catalog`,
  where team specs a profile ships are written (`.team-files`).
- The bash installer (`yardo.sh`), still run by an apply, sources specs
  as root too (`spec_field`, `spec_skeleton`).
- `mcp-sync`'s `render` builds `docker run` from `DOCKER_ARGS`,
  `IMAGE_ARGS` and `image_args()`, word-split, unchecked. It runs as the
  gateway user `mcp-<you>`, which is in the docker group.
- `skillsStep.write` calls `writeFile` (`decl.go`), which uses
  `os.MkdirAll` and `os.CreateTemp` on `e.Path(...)` as root, then chowns.
  Both follow links.
- Archify is a `run` step: `npx -y skills@latest add https://archify.si`,
  as the user, marked `unverified`.
- `ClaudeDeny` (`internal/steps/agentsafe.go`) lists the read denies.

## Goals / Non-Goals

**Goals:**
- No spec text, from any source, runs as root.
- No spec can turn a container into a way onto the host.
- A link in the user's home can't redirect a root write.

**Non-Goals:**
- Stopping an agent from running `cat ~/.aws/credentials` through Bash:
  Read denies stop the obvious path only. Phase 3's checks and phase 4's
  review cover skills that ask for it.
- Limits on containers that pass (`--cap-drop`, read-only, network):
  phase 5, after catalog servers are tried with them.

## Decisions

- **Read specs as `nobody` with `env -i`.** `Load` runs
  `setpriv --reuid=nobody --regid=nogroup --clear-groups env -i
  PATH=/usr/bin:/bin bash -c …` when it is root, and runs as is otherwise
  (tests, a non-root `yardo status`). The spec file must be readable by
  `nobody`; the catalog is 0644 already. Alternatives considered:
  - Parse specs in Go, never in bash. Rejected for now: catalog specs use
    `skeleton()` and `image_args()` functions. Phase 3 parses data-only
    specs from elsewhere in Go.
  - Run as the user. Rejected: a spec could then read the user's files.
- **Only `DOCKER_ARGS` is checked.** `IMAGE_ARGS` and `image_args()` come
  after the image, so they go to the server program, not to Docker.
- **yardo's own specs name exceptions in `HOST_ACCESS`**
  (`docker-socket`, `host-network`). The Docker server mounts the socket by
  design, and the MySQL and Postgres servers use the host network to reach a
  database on the VM. These were found when the catalog test first ran.
  `HOST_ACCESS` counts only for a spec in the catalog that isn't listed in
  `.team-files`.
- **Docker options checked in `mcp-sync`, before `render`'s output is
  registered.** A deny list, matched on the split argument list (so
  `--network=host`, `--net host` and `--network host` are all caught):
  `--privileged`, `--network|--net host`, `--pid|--ipc|--uts|--userns
  host`, `--cap-add`, `--device*`, `-p|--publish*|-P`,
  `--security-opt` with `unconfined` or `label=disable`, and `-v|--volume|
  --mount` whose source is `/`, the Docker socket (`/var/run/docker.sock`,
  `/run/docker.sock`), or under `/home`, `/root` or `$home` except
  `$MCP_DIR/<name>`. `image_args()` output is checked too. The state is
  `refused <option>`, shown by `yardo mcp status` and the TUI like
  `failed`. Alternative considered: an allow list. Rejected for this phase:
  it would break specs people wrote for themselves. Phase 3 uses an allow
  list for specs from elsewhere.
- **Skill writes go through `os.Root`** (Go 1.26). Opening `os.OpenRoot`
  on `~/.agents/skills` (via `Lstat` first: a link is refused) and writing
  `name/rel` inside it can't escape by `..` or a link. A link inside a
  skill folder is reported, not followed or replaced. `writeFile` stays for
  other steps.
- **Archify through the skills step.** A `[[skill]]` lock entry with
  `fetch = true`: `repo`, `path`, `commit`, `sha256` and the licence.
  At apply, yardo lists the repo's tree at the commit (GitHub's API, one
  call) and downloads each file of `path/` from raw.githubusercontent.com,
  checking it against its git blob hash. Regular files only: a link or a
  submodule refuses the skill. It then computes `treeHash`, compares it
  with the lock, and installs through the same `write` as the vendored
  skills, marker included. (The repo is about 1.8 GB, so the commit's
  tarball is out.) Check compares the mark with the lock's hash and never
  downloads. `scripts/update-skills` handles `fetch` entries with a sparse
  checkout, recording the hash without vendoring. The item keeps `node`:
  Archify's scripts run on it. Alternative considered: vendoring 23 MB into the
  binary. Rejected for size.
- **Deny rules**: append to `ClaudeDeny`; the merge already keeps the
  user's rules.

## Risks / Trade-offs

- [A user spec relies on a refused option, e.g. `--network host` to reach
  a local service] → its status names the option; README says how to
  use the gateway's network instead. The user can't override: the
  option is root on the VM.
- [`setpriv` missing] → it's in util-linux on every supported release;
  `Load` fails closed (spec not loaded) if it's absent.
- [Archify's GitHub download is down] → the item fails with the download
  error and can be retried; nothing half-installed (written only after the
  hash matches).
- [Read deny on `~/.aws` stops an agent reading a profile name it needs]
  → it can still run `aws configure list-profiles`; the notes say so.

## Migration Plan

- Existing Archify installs (made by npx, no marker) are left alone, as
  any unmarked folder is, and the item reads as installed. The README says
  to delete both folders and apply Archify again to have the pinned copy.
- Rollback: revert; nothing is stored differently.

## Open Questions

- Archify's folder inside `tt-a1i/archify` (which `path` to lock) is to be
  read from the repo when the lock entry is made.
