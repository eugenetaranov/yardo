# yardo-git-ssh Specification

## Purpose
How a machine reaches its git hosts (GitHub, GitLab, Bitbucket) over SSH: which key each host uses and where it lives, yardo's SSH settings beside the user's own, and the hosts' host keys.

## Requirements

### Requirement: Adding a git host asks for its key
When GitHub, GitLab or Bitbucket is added, or its key is changed from its settings, yardo SHALL ask on one page: **Make a new key** (ed25519, no passphrase, at `~/.ssh/identities/<provider>`), **Use a key on this machine** (a private key under `~/.ssh`), **Paste a key** (saved to `~/.ssh/identities/<provider>`), or **No key here** (no key set: a forwarded agent, or HTTPS). `<provider>` SHALL be `github`, `gitlab` or `bitbucket`. A run that re-applies picks SHALL NOT ask again.

#### Scenario: New key
- **WHEN** the user adds Bitbucket and picks Make a new key
- **THEN** `~/.ssh/identities/bitbucket` and `bitbucket.pub` exist (mode 600 and 644, the directory 700), and yardo shows the public key and where to add it on Bitbucket

#### Scenario: Existing key
- **WHEN** the user adds GitHub and picks `~/.ssh/work_ed25519`
- **THEN** git to github.com uses `~/.ssh/work_ed25519`, and no file is written under `~/.ssh/identities`

#### Scenario: No key
- **WHEN** the user adds GitLab and picks No key here
- **THEN** yardo writes no key and no SSH settings for gitlab.com

#### Scenario: Re-run
- **WHEN** yardo runs again with GitHub still picked
- **THEN** it asks nothing about GitHub's key

### Requirement: A key there is never replaced without asking
When Make a new key or Paste a key would write `~/.ssh/identities/<provider>` and a key is already there, yardo SHALL say so and show its type, fingerprint and comment, and offer **Use it** (the default) or **Replace it**. Replacing SHALL first move the old key and its `.pub` to `<provider>.old-<date>`.

#### Scenario: Use the one there
- **WHEN** `~/.ssh/identities/github` exists and the user picks Make a new key, then Use it
- **THEN** the key file is unchanged and GitHub uses it

#### Scenario: Replace
- **WHEN** the user picks Replace it on 2026-11-20
- **THEN** the old key is at `~/.ssh/identities/github.old-2026-11-20` and a new key is at `~/.ssh/identities/github`

### Requirement: Pasted and sent keys are checked
A pasted key, and one sent with `ssh <machine> 'sudo yardo gitkey <provider>' < <key>`, SHALL be accepted only when it is an SSH private key, and SHALL go to `~/.ssh/identities/<provider>` with its `.pub` derived from it, following the requirement above when a key is there. A key with a passphrase SHALL be accepted with a warning that git will ask for the passphrase on each use. The key SHALL NOT be stored in yardo's answers, profiles or logs.

#### Scenario: Not a key
- **WHEN** the user pastes a public key
- **THEN** yardo says it isn't a private key and asks again

#### Scenario: Sent unattended
- **WHEN** a key is sent with `sudo yardo gitkey bitbucket` and no key is at `~/.ssh/identities/bitbucket`
- **THEN** it is written there and Bitbucket uses it

### Requirement: Unattended runs never ask and never replace
Under `--yes` or a profile, yardo SHALL NOT ask about keys. For a host with no key choice saved, it SHALL use `~/.ssh/identities/<provider>` when it exists and make it otherwise. It SHALL NOT replace a key.

#### Scenario: Profile on a fresh machine
- **WHEN** `sudo yardo --profile team.toml --yes` runs with `github` in the profile and no key under `~/.ssh/identities`
- **THEN** `~/.ssh/identities/github` is made and git to github.com uses it

### Requirement: yardo's SSH settings are in a file of their own
yardo SHALL write its SSH settings for the git hosts to `~/.ssh/config.d/yardo-git` (mode 600) and SHALL change `~/.ssh/config` only to add `Include config.d/yardo-git` before its first `Host` or `Match` line, once. A managed block an earlier yardo wrote in `~/.ssh/config` SHALL be removed from it, with every other line of the file unchanged. When no host needs settings, the file SHALL be empty and the Include line SHALL stay.

#### Scenario: User's config kept
- **WHEN** `~/.ssh/config` has the user's own `Host db` block and yardo sets up GitHub with a key
- **THEN** `~/.ssh/config` is the user's lines plus the Include line at the top, and the `Host github.com` settings are in `~/.ssh/config.d/yardo-git`

#### Scenario: Migrated
- **WHEN** `~/.ssh/config` has yardo's old `# >>> yardo gitkey (managed)` block and yardo runs
- **THEN** the block is gone from `~/.ssh/config`, its hosts are in `~/.ssh/config.d/yardo-git`, and `ssh -G github.com` gives the same IdentityFile as before

### Requirement: The user's own host entries win
When the user's `~/.ssh/config` (outside yardo's file and lines) has a `Host` entry matching a git host yardo would set up, yardo SHALL write no settings for that host, and SHALL say that the user's config sets it, naming the line.

#### Scenario: Own GitHub entry
- **WHEN** `~/.ssh/config` line 12 is `Host github.com` and the user adds GitHub with a new key
- **THEN** `~/.ssh/config.d/yardo-git` has no `github.com` entry, and yardo says `~/.ssh/config` sets github.com (line 12), with the new key's path to use there

### Requirement: Bitbucket over ssh.bitbucket.org
yardo's settings for Bitbucket SHALL send SSH for both `bitbucket.org` and `ssh.bitbucket.org` to `ssh.bitbucket.org`, so remotes naming either keep working. yardo SHALL add Bitbucket's published host keys, from a checked-in file, to the user's `known_hosts` for `ssh.bitbucket.org` and `bitbucket.org`, and SHALL NOT trust Bitbucket's host keys on first use.

#### Scenario: Old remote
- **WHEN** a repo's remote is `git@bitbucket.org:team/repo.git` and Bitbucket is set up with a key
- **THEN** `git fetch` connects to `ssh.bitbucket.org` and succeeds without a host key prompt

#### Scenario: Wrong host key
- **WHEN** `ssh.bitbucket.org` presents a key that isn't the pinned one
- **THEN** ssh refuses to connect

### Requirement: Accepting new hosts' keys is a setting
Settings SHALL have **Accept new hosts' SSH keys**, off by default, which a profile can set. On, ssh SHALL accept and remember an unseen host's key without asking, for every host, and SHALL still refuse a key that differs from the one remembered or pinned. A `StrictHostKeyChecking` the user sets in their own `~/.ssh/config` SHALL win over it.

#### Scenario: On
- **WHEN** the setting is on and the user runs `ssh new-box` for the first time
- **THEN** ssh connects without a fingerprint prompt and `new-box`'s key is in `known_hosts`

#### Scenario: Changed key
- **WHEN** the setting is on and github.com presents a key other than the pinned one
- **THEN** ssh refuses to connect

#### Scenario: User's own choice
- **WHEN** the setting is on and the user's `~/.ssh/config` has `Host prod-*` with `StrictHostKeyChecking yes`
- **THEN** ssh to `prod-1` still asks for its fingerprint

### Requirement: Machines set up before keep their keys
A git host set up by an earlier yardo SHALL keep using the key it uses (`~/.ssh/id_ed25519`, `~/.ssh/<provider>`, `~/.ssh/yardo_git_key`, or none), without being asked and without any key being made, moved or deleted; only its settings move to yardo's file.

#### Scenario: Upgrade
- **WHEN** a machine whose GitHub uses `~/.ssh/github` runs the new yardo
- **THEN** git to github.com still uses `~/.ssh/github`, and nothing is written under `~/.ssh/identities`
