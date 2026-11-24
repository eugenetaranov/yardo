## 1. Specs read without root

- [x] 1.1 `mcp.Load` runs the reader as `nobody` with `env -i` when it is root, and fails when it can't drop privileges. Verify: `mcp_test.go` case (skipped unless root) where a spec's `touch` outside `/tmp` fails and its fields still load; a spec echoing an exported variable reads empty.
- [x] 1.2 Check that every catalog spec and the TUI's MCP screens still load. Verify: `task test`; `sudo yardo status` on a VM lists the same servers as before.

- [x] 1.3 The bash installer's `spec_field` and `spec_skeleton` (still run by an apply) read specs the same way: as nobody, with an empty environment (found on the VM: a team spec's command ran as root during `--profile` apply). Verify: `TestYardoShSpecRead` (as root on Linux); on the VM the spec's marker file is made by `nobody` and `/root/pwned` isn't.

## 2. Docker options refused

- [x] 2.1 `mcp-sync` checks a spec's `DOCKER_ARGS` against the deny list (yardo's own specs may name `HOST_ACCESS` exceptions), skips registering a match, and reports `refused <option>` in `status`. Verify: `mcpsync_test.go` cases for `--privileged`, `--network=host`, `--net host`, `-v /:/h`, `-v /var/run/docker.sock:/s`, `-v $HOME/.ssh:/s`, `-p 80:80`, `--cap-add SYS_ADMIN`; a `-v ~/.mcp/<name>/x:/x` passes.
- [x] 2.2 The TUI and `yardo mcp status` show `refused` with the option, like `failed`. Verify: status test with a refused server; the home doesn't offer Fill in for it.
- [x] 2.3 Every catalog spec passes. Verify: test that renders each catalog spec with dummy keys and finds no refusal.
- [x] 2.4 README MCP section: which options are refused and why. Verify: README reads with it.

## 3. Skill writes and names

- [x] 3.1 `skillsStep.write` writes under `os.OpenRoot` of each skills folder; a link as the folder, the skill folder or a file in it is skipped with a run note. Verify: `skills_test.go` cases for each link placement, the target unchanged.
- [x] 3.2 Skill names are checked before any write. Verify: `skills_test.go` for `../x`, `X`, `-a`, a 65-character name; existing shipped names pass.

## 4. Archify pinned

- [x] 4.1 Read `tt-a1i/archify` at a current commit, find the skill's folder and licence, add a `fetch = true` lock entry with its tree hash. Verify: `scripts/update-skills archify` reproduces the same hash.
- [x] 4.2 The skills step downloads the tarball for a `fetch` entry, unpacks only its path (regular files only), checks the hash, and installs with the marker; the archify item uses it and drops `npx` and `node`. Verify: `skills_test.go` with a served tarball: a match installs, a changed byte installs nothing and the run says so, a link in the tarball is refused.
- [x] 4.3 Lock test covers `fetch` entries (licence named, hash present). Verify: `task test`.
- [x] 4.4 README and the item's `source`: pinned commit, no installer. Verify: Tools shows the new source.

## 5. Agent denies and the Docker group

- [x] 5.1 `ClaudeDeny` adds `~/.aws/**`, `~/.kube/**`, `~/.config/gh/**`, `~/.docker/config.json`, `~/.git-credentials`; `Plan` text and agent notes say which. Verify: `agentsafe_test.go` checks the merged file, existing rules kept.
- [x] 5.2 Docker's item text says that in the docker group, anything run as you, agents and skills included, can act as root. Verify: Tools › Docker shows it.

## 6. Integration

- [x] 6.1 `task test` and `task lint` pass.
- [x] 6.2 On a VM: apply agent skills, archify and the MCP servers; check skills installed, `yardo mcp status` unchanged for catalog servers, and a test spec with `--privileged` refused. Verify: output recorded in the PR.
