## Context

The catalog pinned image tags and npx or uvx package versions, and `task mcp-audit` compared those pins with upstream before each minor release. The pins went stale between releases.

## Decisions

- **Resolve, don't float.** Specs name a tag or a package name. mcp-sync turns them into a digest and an exact version and records both in `~/.mcp/<name>/.resolved`. Registration uses exactly those. Floating `npx -y pkg` would fetch whatever is newest on every container start, because `/tmp` is a fresh tmpfs each time. That would make a server's version change silently and make every start slower.
- **When.** A server is resolved when it has no `.resolved` yet, and on `mcp-sync update`. The yardo run's sync step and `sudo yardo mcp update` call `update`. Plain syncs, which the home screen runs to register pending servers, and status checks never resolve.
- **Where the version comes from.** Images: `docker pull` the tag, then the `RepoDigests` entry for the image's repository. npm: `registry.npmjs.org/<name>/latest` (scoped names as `@scope%2Fname`). PyPI: `pypi.org/pypi/<name>/json`, `.info.version`. The package is the first word after `npx` or `uvx` that isn't a flag. One that already names a version is left as it is.
- **Substitution.** `render` replaces the `IMAGE` word and the package word in the final argument list, so specs and `image_args` don't change.
- **Failure.** A lookup that fails keeps the recorded value. With nothing recorded, the server runs by tag and name, as before pins existed.
- **Servers from elsewhere** are skipped. Their approval is of one digest, and they update through `sudo yardo addons update`.

## Risks / Trade-offs

- [A new upstream major breaks a server, such as aws 1.8 → 2.0] → it shows as a failed registration with the error, as any spec bug would. `.resolved` names the version so it can be reported. A spec may pin a known-good tag again (the spec test would need an exception).
- [Each run contacts registries] → one pull and one HTTP request per catalog server per run, with timeouts. Failures are non-fatal.
- [Supply-chain trust moves from yardo's review of a pin to upstream's latest] → accepted. Catalog servers are well-known vendors' and they run under the container limits. Add-ons from elsewhere stay pinned and checked.
