## 1. Holding area and fetching

- [x] 1.1 `internal/addon`: held items under `/var/lib/yardo/quarantine/<user>/<id>` (root 0700, files 0600), with meta, list, remove. Verify: `addon_test.go` checks modes and that a held item is removed with nothing left.
- [x] 1.2 Skills from GitHub: ref to commit, tarball, unpack only the path, regular files only, offer each `SKILL.md` folder. Verify: tests against a served tarball: branch resolved to commit, a link or `..` entry refused, three skills offered.
- [x] 1.3 Images: pull, digest, inspect; never create or run. Verify: test with a fake `docker` on PATH that records calls: only `pull` and `image inspect`.
- [x] 1.4 Spec parser: data-only vs code, re-quoting writer; image or URL form to a data spec. Verify: parser tests for quoting, comments, a function, `$(…)`, a here-doc.

## 2. Basic checks

- [x] 2.1 Block rules with ids and next steps: pin, links and paths, programs, archives, hidden Unicode, size, name, frontmatter hooks and tools, Docker allow list, MOUNT_* on servers from elsewhere. Verify: a testdata case per rule, plus a clean skill with no findings.
- [x] 2.2 Warning rules with file:line: web addresses, fetch-and-run, sudo and crontab, secret paths, hide or ignore phrasing, HTML comments, near names, package install scripts, root image. Verify: a testdata case per rule with the expected line.

## 3. skill-veil

- [x] 3.1 Catalog item `skillveil`: pinned release, sha256 for amd64 and arm64. Verify: `task test` catalog checks; on a VM `skill-veil --version` prints the pinned version.
- [x] 3.2 Runner: copy held files for nobody, run with no network, no home, a 60 s limit; map block to needs OK and require_approval to note; failure to "not scanned". Verify: test with a fake binary for each outcome; on a VM, a skill that tries `curl` from skill-veil's runner can't connect (check with a stub that tries).

## 4. Report, decision, install

- [x] 4.1 Report model: verdict from all layers, findings sorted by severity, fixed next steps; fixes (remove a file, drop an option, edit a copy) re-run the checks. Verify: tests for the verdict order and that a fix re-checks.
- [x] 4.2 Install: re-hash, then write through the skills step with a `checked` marker, or write the spec and register by digest; record the approval; typed OK when needed. Verify: tests for a changed held file (re-check, no install), the record's fields, and a wrong typed name.
- [x] 4.3 `mcp-sync` registers a server from elsewhere only when its spec hash is in the approval record. Verify: `mcpsync_test.go` with a spec edited after approval: not registered, status says so.
- [x] 4.4 Decline removes files and the pulled image; uninstall and delete remove the record entry. Verify: tests for each.

## 5. Screens and CLI

- [x] 5.1 Skills screen: `n` to add (pick from a repo), held and checked rows, report view with Install, Fix and check again, Decline and remove; Check it on a skill that isn't yardo's (turned off while checked). Verify: screen model tests for each path.
- [x] 5.2 MCP servers: From elsewhere (image, URL, spec), form, held row, report view. Verify: screen model tests; a held server isn't in the gateway's tool list (mcp-sync test).
- [x] 5.3 `sudo yardo skills add`, `sudo yardo mcp add`, `sudo yardo addons`; `--yes` installs only reports with nothing needing OK. Verify: `main_test.go` cases.
- [x] 5.4 README: Adding skills and servers from elsewhere, what's checked, what isn't (image layers, code fetched at run time). Verify: README reads with it.

## 6. Integration

- [x] 6.1 `task test` and `task lint` pass.
- [x] 6.2 On a VM: add a clean public skill, one with a planted `curl … | sh` and a `~/.aws` read, and an MCP image with `--privileged` in its spec. Check the reports, decline the bad ones, install the clean one, and see it listed as checked. Verify: output and screenshots in the PR. (Done on a throwaway VM: internal-comms from anthropics/skills installed and listed as checked; a planted skill with `curl … | sh` and `~/.aws` flagged by both layers and declined; a spec with `--privileged` and `-v /:/host` blocked; a spec with a `curl | sh` function flagged and declined; mcp/time pulled by digest, installed, registered; a spec edited after approval refused and deregistered; skill-veil's sandbox has no network. Output in the session; no PR, committed on main.)
