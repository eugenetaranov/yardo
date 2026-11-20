# yardo-github-access Specification

## Purpose
How a machine logs in to GitHub, GitLab and Bitbucket: the gh login, tokens, and the user's own forwarded agent.

## Requirements

### Requirement: SSO hint after a key upload
After uploading a key in `key` mode, yardo SHALL print, and add to `~/yardo.txt`, a note that keys for SAML SSO organizations must be authorized at https://github.com/settings/keys ("Configure SSO").

#### Scenario: Key uploaded
- **WHEN** a key is uploaded
- **THEN** the run output contains the SSO note with the settings link

### Requirement: Git email from verified addresses
When the git email is not set, yardo SHALL read the GitHub account's verified emails and offer them, primary first. The noreply address SHALL be used only when the user picks it or it is the only one. Under `--yes`, the primary verified email SHALL be used.

#### Scenario: Work email chosen
- **WHEN** the account has verified `me@acme.com` (primary) and a noreply address
- **THEN** the choice lists `me@acme.com` first, and choosing it sets `user.email`

### Requirement: GitHub host keys pinned
yardo SHALL add github.com's published host keys from a checked-in file to the user's `known_hosts`. It SHALL NOT use `ssh-keyscan`.

#### Scenario: Fresh VM
- **WHEN** `github` runs on a VM without a github.com entry in `known_hosts`
- **THEN** the entries added equal the checked-in keys, and no network scan is made

### Requirement: SSH commit signing
A `gitsign` item on the Accounts tab, needing `github`, SHALL set:
- `gpg.format=ssh`
- `commit.gpgsign=true` and `tag.gpgsign=true`
- `user.signingkey`, which is the forwarded agent's first key (`key::…`) in agent mode, and `~/.ssh/id_ed25519.pub` in key mode
- `gpg.ssh.allowedSignersFile`, pointing to `~/.config/git/allowed_signers` with a line for the git email

It SHALL upload the signing key with `gh ssh-key add --type signing`. If gh lacks `admin:ssh_signing_key`, it SHALL add the scope with `gh auth refresh`, or leave a note when it cannot prompt. In token mode, it SHALL set the git options and note the settings page for adding the signing key by hand.

#### Scenario: Signed commit
- **WHEN** `gitsign` is applied in key mode and the user commits
- **THEN** `git log --show-signature -1` reports a good signature for the git email

#### Scenario: Unattended
- **WHEN** `gitsign` runs under `--yes` and gh lacks the signing scope
- **THEN** git is configured, no prompt is shown, and a note gives the `gh auth refresh -s admin:ssh_signing_key` command
