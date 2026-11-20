## Why

Adding GitHub, GitLab or Bitbucket asks nothing about SSH: git uses
`~/.ssh/id_ed25519` (made when missing), so one key ends up on every host
and the user never chose it. yardo's settings for the hosts go in a block at
the top of the user's own `~/.ssh/config`, where they win over the user's
own entries for those hosts. And Bitbucket Cloud now serves Git over SSH
only on `ssh.bitbucket.org` (`bitbucket.org` is HTTPS only), which yardo
still connects to and trusts on first use.

## What Changes

- **Adding a git host asks for its key**, one page: make a new key, use a
  key already on this machine, paste one (or send it with `yardo gitkey`),
  or no key here (a forwarded agent or HTTPS).
- **Keys yardo makes or is given live in `~/.ssh/identities/<provider>`**
  (`github`, `gitlab`, `bitbucket`), never overwritten without asking:
  an existing key there is shown (type, fingerprint, comment) with Use it
  or Replace it (the old one kept aside). A key the user picks from
  `~/.ssh` stays where it is.
- **yardo's SSH settings move to their own file**, `~/.ssh/config.d/yardo-git`,
  included by one line at the top of `~/.ssh/config`; the old managed block
  moves there. A host the user's config already sets is left to the user,
  and yardo says so.
- **Bitbucket over `ssh.bitbucket.org`**: `bitbucket.org` and
  `ssh.bitbucket.org` both reach it (existing remotes keep working), with
  Bitbucket's published host keys pinned in `known_hosts`.
- **Setting: Accept new hosts' SSH keys** (off by default): ssh trusts a
  host it hasn't seen before without the fingerprint prompt, still refusing
  a changed key. Applied in `~/.ssh/config.d/yardo-defaults`, included at
  the end of `~/.ssh/config`, so the user's own setting wins.
- `--yes` and profiles ask nothing: the key in `~/.ssh/identities/<provider>`
  is used, made when missing, never replaced.
- **BREAKING (spec only)**: the `yardo-github-access` requirement "GitHub
  access modes" (agent / key / token, asked as `gh_mode`) is removed; the
  code no longer asks it, and this change's question replaces it for every
  host. Machines set up before keep their keys where they are.

## Capabilities

### New Capabilities

- `yardo-git-ssh`: how a machine reaches its git hosts over SSH: choosing
  each host's key, where keys live, yardo's SSH config file and how it
  coexists with the user's, and the hosts' pinned host keys.

### Modified Capabilities

- `yardo-github-access`: "GitHub access modes" is removed (replaced by
  `yardo-git-ssh`'s key choice).

## Impact

- `internal/steps/gitkey.go`: keys per host from the answers, the
  identities directory, the config file and Include line, migration of the
  old block, the user's own Host entries, Bitbucket's HostName and host
  keys.
- `internal/answers`: each git host records its key choice (path or none);
  old modes (`own`, `key`, `sent`, `token`) are read as before.
- `cmd/yardo/ask.go` and `internal/ui`: the key page and the
  existing-key page, asked when a host is added or changed.
- `cmd/yardo/main.go` (`yardo gitkey`): a sent key goes to the host's
  identity file.
- A checked-in `bitbucket_known_hosts`, beside `github_known_hosts`.
- `internal/catalog/data/90-git.toml`: the items' labels and sources.
