## Why

The catalog pinned every MCP server's image tag and npx or uvx package version. A release could therefore ship versions that were already behind upstream, and they stayed behind until someone bumped the pins by hand. yardo should run current servers. It should still run the same thing between runs and say exactly what each one runs.

## What Changes

- **BREAKING (policy)**: catalog specs name an image by its tag (`:latest`, or a floating tag like `node:lts-slim`) and a package by its name alone. The spec test now fails on a pin.
- mcp-sync resolves each catalog server when it's first synced and again on `mcp-sync update`, which every yardo run and `sudo yardo mcp update` call. It pulls the tag and records the digest, asks npm or PyPI for the package's newest version, and records both in `~/.mcp/<name>/.resolved`. A server runs exactly what was recorded until the next update. A lookup that fails keeps the previous version.
- Servers from elsewhere aren't resolved: they run the digest that was approved.
- Rows show what's installed. MCP servers show the package or image version and a short digest. Skills show their own version, or the yardo that installed them, and a short hash of their files. Checked add-ons show what was checked: the skill's source, ref and short commit, or the image's tag and short digest.
- `scripts/mcp-audit` and `task mcp-audit` go away: there's no pin to audit.

## Capabilities

### New Capabilities

### Modified Capabilities
- `yardo-catalog`: MCP server specs are no longer pinned. Instead, what they run is resolved on each run.

## Impact

- `internal/assets/mcp-sync`: `update`, `resolve`, render substitution, source output
- Catalog specs in `internal/assets/mcp-catalog` and `internal/assets/conf/mcp-catalog`
- `internal/steps/mcp.go`: the sync step runs `mcp-sync update`
- `cmd/yardo`: `mcp update`, version and short-hash display, `addon.What`
- Each yardo run now asks Docker registries, npm and PyPI for what's newest
