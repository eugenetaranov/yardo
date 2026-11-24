## Why

An add-on that was fine when approved can turn bad later. A new commit
can add a fetch-and-run, an image tag can be pushed again with new
content, a remote MCP server can change its tool descriptions, or an
agent can edit an installed skill. A container that got past every check
can also still do anything Docker allows. This is phase 5 of 5. Updates
go through the same checks as installs, looking at what changed, and
yardo notices changes it didn't make. Containers from elsewhere also run
with limits by default.

## What Changes

- **Updates are offered, never applied on their own.** yardo looks for a
  newer commit on a skill's branch or tag, or a new digest for a server's
  image tag. A row reads "update available". **Update** fetches the new
  version into the holding area and runs every check. The report shows
  what changed since the approved version. The approved version stays in
  use until the new one is approved.
- **The review sees the change.** The report lists added, changed and
  removed files with a diff. Claude is given the diff, plus the new
  SKILL.md and every new or changed script in full.
- **Approved copies are kept** with the approval record, so a diff and a
  Restore are always possible.
- **Changes yardo didn't make are flagged**:
  - A checked skill whose files no longer match its approval reads
    "changed since checked" on the Skills screen and the home. The next
    steps are **Check it again** or **Restore the approved version**.
  - A server whose image digest doesn't match isn't registered.
  - A remote server whose tool list changed is turned off until its new
    tools are checked.
- **Limits on containers from elsewhere**: no extra capabilities, no new
  privileges, a read-only file system with a scratch `/tmp`, limits on
  memory and processes, an unprivileged user, and no network unless the
  approved spec says it needs one. yardo's own catalog servers get the
  same limits where they still work, and each exception is named in its
  spec.
- **CLI**: `sudo yardo addons outdated` and `sudo yardo addons update
  [name …]`.

## Capabilities

### New Capabilities

### Modified Capabilities
- `yardo-addon-review`: updates, approved copies, changes made outside
  yardo, remote tool lists.
- `yardo-addon-safety`: limits on running MCP containers.

## Impact

- `internal/addon`: outdated check, diff, approved copies, drift check,
  tool-list hash.
- `internal/assets/mcp-sync` (`render`): limit flags; register remote
  servers only when their tool list hash matches.
- `internal/assets/mcp-catalog/*.spec`: `LIMITS` exceptions where a
  server needs them.
- `cmd/yardo`: rows, Update, Check it again, Restore, home rows, CLI.
- README: Updating add-ons, what the limits are.
