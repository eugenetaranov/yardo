## 1. Unpin

- [x] 1.1 Catalog specs: images by tag (`:latest`, `node:lts-slim`, `uv:python3.13-trixie-slim`), packages without versions. Verify: `TestMCPSpecsUnpinned`, `TestMCPSpecsValid`.
- [x] 1.2 Remove `scripts/mcp-audit` and `task mcp-audit`; README says how servers stay current.

## 2. Resolve

- [x] 2.1 mcp-sync: `update` prefix, `resolve`, `.resolved`, render substitution, pull of the resolved image, `source` with version and digest. Verify: `TestMCPSyncResolve` (first sync, plain sync keeps, update moves, registry down keeps, source line).
- [x] 2.2 The yardo run's sync step runs `mcp-sync update`; `sudo yardo mcp update`. Verify: `TestMCPSyncAlways`.

## 3. Show what's installed

- [x] 3.1 `addon.What` for checked add-ons (source, ref and 7-character commit; tag and 7-character digest; spec hash). Verify: `TestWhat`.
- [x] 3.2 Every skill row and line: version (front matter, else yardo's) and a 7-character files hash; MCP rows: version and 7-character digest. Verify: `TestSkillVersion`, `TestParseSources`.

## 4. Integration

- [x] 4.1 `task test` and `task lint` pass.
