## 1. Approved copies and drift

- [x] 1.1 Install keeps an approved copy under `/var/lib/yardo/approved/<user>/<kind>/<name>/`; uninstall and delete remove it. Verify: `addon_test.go` for keep, replace on a new approval, and removal.
- [x] 1.2 Drift: `checked` skills compared with the record's hash on each run and screen refresh; home row and Skills state; Restore and Check it again. Verify: tests for an edited skill (flagged), Restore (bytes equal the copy), and Check it again (re-approved, flag gone).

## 2. Updates

- [x] 2.1 Outdated check for skills (commit of the given ref) and images (digest of the given tag), cached for an hour; pinned commits never outdated. Verify: tests with a fake GitHub API and registry.
- [x] 2.2 Update: fetch to the holding area, diff against the approved copy, all checks, Claude given the diff and full new scripts; approved version stays until approval. Verify: tests for a declined update (installed unchanged), an approved one (installed and copy replaced), and the Claude input containing the diff and full changed script.
- [x] 2.3 Rows read update available; Update on the add-on's screen; `sudo yardo addons outdated` and `update [name …]`, with `--yes` installing only reports with nothing needing OK. Verify: screen model and `main_test.go` cases.

## 3. Remote tool lists

- [x] 3.1 Record the sorted tool list hash on approval; on sync, a mismatch turns the server off with reason tools-changed; Check it again checks the new list. Verify: `mcpsync_test.go` with a fake `mcpjungle` whose tool list changes; check against MCPJungle's real output on a VM.

## 4. Container limits

- [x] 4.1 `render` adds the limits and `--network none` unless `NETWORK=1` for servers from elsewhere; `LIMITS` in catalog specs drops the named ones; the spec parser refuses `LIMITS` from elsewhere. Verify: `mcpsync_test.go` for the rendered arguments in each case.
- [x] 4.2 Try each catalog server with every limit on a VM; add `LIMITS` lines with a reason only where needed. Verify: a table of servers and results in the PR; each server's tools answer through the gateway.
- [x] 4.3 README: Updating add-ons, the limits and the `LIMITS` exceptions. Verify: README reads with it.

## 5. Integration

- [x] 5.1 `task test` and `task lint` pass.
- [x] 5.2 On a VM: approve a skill from a test repo, push a commit adding `curl … | sh`, update and decline; edit the installed skill and Restore; approve a server from elsewhere and check `docker inspect` shows the limits and no network. Verify: output in the PR.

## Notes

- 4.2: the table of results is in design.md, As built. Two changes came out of the trial. The scratch `/tmp` is `exec`. docker has `LIMITS="root-user"` and opensearch has `LIMITS="writable"`.
- 5.2, on VM 111:
  - A real update of internal-comms showed its diff and was installed.
  - The `curl … | sh` update was declined in `TestAddonsUpdate` against a fake GitHub, not a real test repo: pushing one needs a repo of the user's.
  - Edit, then `skills restore`, brought the approved files back.
  - A remote server whose tools changed went off; `addons recheck`, with the name typed, turned it on again.
  - thinkoff's container showed `net=none` and every limit.
