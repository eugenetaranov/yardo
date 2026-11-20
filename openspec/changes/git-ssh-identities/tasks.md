## 1. Answers and secrets

- [ ] 1.1 Add `Key` to `answers.GitHost` (a path or `none`; empty: the old `Mode` as today) in both `answers.go` and `store.go`, and `SSHAcceptNew` to the answers; profiles can set `ssh_accept_new`. Verify: `internal/answers` round-trip tests, and a test that answers saved before (each old mode, no `Key`) load unchanged.
- [ ] 1.2 Replace `creds.Secrets.GitKey` with a key per provider (keeping `--host` for other hosts on the old path); `yardo gitkey <provider>` and `--host github.com|bitbucket.org|<gitlab host>` fill it. Verify: `cmd/yardo/main_test.go` cases for `yardo gitkey bitbucket` and `--host bitbucket.org`, and that the key never appears in saved answers.

## 2. The gitkey step

- [ ] 2.1 Resolve each host's key: `Key` when set; else the old modes as now; a new host under `--yes` gets `~/.ssh/identities/<provider>`, made when missing. Make keys there with `~/.ssh/identities` at 700. Verify: `internal/steps/gitkey_test.go` table: new host, picked key, `none`, and each old mode giving the same IdentityFile as before.
- [ ] 2.2 Write a pasted or sent key to `~/.ssh/identities/<provider>` (checked as a private key, `.pub` derived, passphrase warned); when a key is there and replacing was chosen, move it and its `.pub` to `<provider>.old-<date>` first (numeric suffix on a clash); never replace unattended. Verify: tests for write, keep, replace (old files present), and a public key refused.
- [ ] 2.3 Write `~/.ssh/config.d/yardo-git` (600, the dir 700) with the hosts' entries, and add `Include config.d/yardo-git` before the first `Host`/`Match` of `~/.ssh/config` once; remove the old managed block, leaving every other byte. Verify: tests on configs with no file, a user `Host db` block, the old block at the top, and the Include moved by the user (kept where it is).
- [ ] 2.4 Skip hosts the user's `~/.ssh/config` sets (non-wildcard `Host` pattern matching, yardo's lines excluded) and note "~/.ssh/config sets <host> (line N): yardo leaves it to you; its key is <path>". Verify: tests for `Host github.com`, `Host *.bitbucket.org`, and `Host *` (not counted).
- [ ] 2.5 Bitbucket: `Host bitbucket.org ssh.bitbucket.org` / `HostName ssh.bitbucket.org`, no accept-new; add a checked-in `conf/bitbucket_known_hosts` (Atlassian's published keys, both names) to `known_hosts`, as GitHub's. Verify: unit test on the entry and the known_hosts merge; `internal/assets/credentials_test.go` checks the file's names and that no `ssh-keyscan` is used.
- [ ] 2.6 Write `~/.ssh/config.d/yardo-defaults` (`StrictHostKeyChecking accept-new` when the setting is on, empty when off) and add `Match all` + `Include config.d/yardo-defaults` at the end of `~/.ssh/config` once. Verify: tests for on, off, and a user config ending inside a `Host` block (the defaults apply to every host: checked with `ssh -G` in the test when ssh is on PATH).

## 3. Asking

- [ ] 3.1 The key page in `cmd/yardo/ask.go`: asked for `github`, `gitlab`, `bitbucket` when newly added (in `questions`, after the GitLab host) and from each item's settings (`change`); Make / Use a key on this machine (a second list of the private keys found under `~/.ssh`, `identities/*.old-*` excluded) / Paste / No key here. Verify: ask tests with fake prompts for each option, and that a re-run with the same picks asks nothing.
- [ ] 3.2 The existing-key page: shown when Make or Paste meets a key at `~/.ssh/identities/<provider>`, with `ssh-keygen -lf`'s type, fingerprint and comment; Use it (default) / Replace it. Verify: ask test that Use it leaves the file and Replace it sets the replace flag the step reads.
- [ ] 3.3 After a key is made or pasted, show the public key and where to add it (the existing NewKey notes; GitHub's upload by gh as today). Verify: the run report test includes the `.pub` path and the host's settings URL.
- [ ] 3.4 Settings: an **Accept new hosts' SSH keys** row (ask me / accept new, refuse changed) in `cmd/yardo/status.go`, applied like the other settings. Verify: a menu test that changing it saves `SSHAcceptNew` and queues the gitkey step.

## 4. Specs, docs and check

- [ ] 4.1 Update `internal/catalog/data/90-git.toml`'s labels and sources (keys in `~/.ssh/identities`, the config file), the `gitkey` step's doc comment, and the README's Git & SSH section and Settings list. Verify: `task test` (catalog tests) and the README reads with the new flow.
- [ ] 4.2 `task test` and `task lint` pass.
- [ ] 4.3 On a VM: add Bitbucket with a new key, add it on Bitbucket, `git clone git@bitbucket.org:<team>/<repo>` and `git@ssh.bitbucket.org:…` both work without a prompt; add GitHub with a key from `~/.ssh`; check `~/.ssh/config` only gained the Include lines; turn the setting on and `ssh` to a new host without a prompt. Verify: the output recorded in the PR.
