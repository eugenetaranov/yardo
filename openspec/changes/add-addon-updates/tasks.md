## 1. Approved copies and drift

- [ ] 1.1 Install keeps an approved copy under `/var/lib/yardo/approved/<user>/<kind>/<name>/`; uninstall and delete remove it. Verify: `addon_test.go` for keep, replace on a new approval, and removal.
- [ ] 1.2 Drift: `checked` skills compared with the record's hash on each run and screen refresh; home row and Skills state; Restore and Check it again. Verify: tests for an edited skill (flagged), Restore (bytes equal the copy), and Check it again (re-approved, flag gone).

## 2. Updates

- [ ] 2.1 Outdated check for skills (commit of the given ref) and images (digest of the given tag), cached for an hour; pinned commits never outdated. Verify: tests with a fake GitHub API and registry.
- [ ] 2.2 Update: fetch to the holding area, diff against the approved copy, all checks, Claude given the diff and full new scripts; approved version stays until approval. Verify: tests for a declined update (installed unchanged), an approved one (installed and copy replaced), and the Claude input containing the diff and full changed script.
- [ ] 2.3 Rows read update available; Update on the add-on's screen; `sudo yardo addons outdated` and `update [name …]`, with `--yes` installing only reports with nothing needing OK. Verify: screen model and `main_test.go` cases.

## 3. Remote tool lists

- [ ] 3.1 Record the sorted tool list hash on approval; on sync, a mismatch turns the server off with reason tools-changed; Check it again checks the new list. Verify: `mcpsync_test.go` with a fake `mcpjungle` whose tool list changes; check against MCPJungle's real output on a VM.

## 4. Container limits

- [ ] 4.1 `render` adds the limits and `--network none` unless `NETWORK=1` for servers from elsewhere; `LIMITS` in catalog specs drops the named ones; the spec parser refuses `LIMITS` from elsewhere. Verify: `mcpsync_test.go` for the rendered arguments in each case.
- [ ] 4.2 Try each catalog server with every limit on a VM; add `LIMITS` lines with a reason only where needed. Verify: a table of servers and results in the PR; each server's tools answer through the gateway.
- [ ] 4.3 README: Updating add-ons, the limits and the `LIMITS` exceptions. Verify: README reads with it.

## 5. Integration

- [ ] 5.1 `task test` and `task lint` pass.
- [ ] 5.2 On a VM: approve a skill from a test repo, push a commit adding `curl … | sh`, update and decline; edit the installed skill and Restore; approve a server from elsewhere and check `docker inspect` shows the limits and no network. Verify: output in the PR.
