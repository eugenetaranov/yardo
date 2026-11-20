## Context

`internal/steps/gitkey.go` (`go:gitkey`, pulled in by `gitid`, `github`,
`gitlab`, `bitbucket`) decides each host's key from `answers.GitHost.Mode`:
`own` / `""` (`~/.ssh/id_ed25519`, made when missing; no settings unless
needed), `key` (`~/.ssh/<provider>`, or `id_ed25519` where only that
exists), `sent` (`~/.ssh/yardo_git_key`, from `yardo gitkey`), `token`
(GitHub over HTTPS, no SSH). Nothing asks for the mode now; it's set by
older answers or by `yardo gitkey`. The step writes a managed block
(`# >>> yardo gitkey (managed)`) first in `~/.ssh/config` with `WithBlock`,
pins GitHub's host keys from `github_known_hosts`, and trusts every other
host on first use (`StrictHostKeyChecking accept-new` in its block). Files
in `~/.ssh` are read and written as the user (`sshRead`, `sshWrite`).

Questions are asked in `cmd/yardo/ask.go` (`questions`, for what's new;
`change`, from an item's settings) with `internal/ui/prompts.go`'s
`Choose`, `Password`, `Confirm`. Settings rows are `ui.BoardSetting`
choices built in `cmd/yardo/status.go`.

## Goals / Non-Goals

**Goals:**
- One explicit key per host, chosen once, never silently replaced.
- yardo's SSH settings never edit, reorder or outrank the user's.
- Old machines unchanged in behaviour.

**Non-Goals:**
- Uploading keys for GitLab or Bitbucket (their APIs need tokens yardo
  doesn't hold); GitHub's upload stays as it is today (by gh, when logged
  in).
- Rewriting repos' remotes from `bitbucket.org` to `ssh.bitbucket.org`.
- Hosts other than the three items (`yardo gitkey --host` for other hosts
  keeps working as today).
- Passphrase-protected keys made by yardo (git must work unattended).

## Decisions

**Answers record a key path, not a mode.** `GitHost` gains
`Key string`: a path (`~/.ssh/identities/github`, `~/.ssh/work_ed25519`)
or `none`. When `Key` is set it wins; when it's empty, the old `Mode` is
read exactly as today (that's the "machines set up before" path, so no
answers migration is needed). A new host with no `Key` under `--yes` gets
`~/.ssh/identities/<provider>`. The private key itself is never in
answers: pasted keys travel in `creds.Secrets` (per provider, replacing
the single `GitKey`), like other secrets.
*Alternative*: new modes (`identity`, `path`, `none`) beside the old ones.
Rejected: a path says everything, and the old modes stay readable as-is.

**Point at a picked key; don't copy it.** One key file, one place to
rotate it. The `identities/` directory holds only keys yardo made or was
given.

**Replacing moves, never deletes.** `<provider>.old-<date>` (and `.pub`),
with a numeric suffix if that exists too.

**Two files, included at both ends.**
- `~/.ssh/config.d/yardo-git`, included before the first `Host`/`Match`:
  per-host settings must come before the user's `Host *` lines, or a
  user's `Host * / IdentityFile …` would be tried first. It never has a
  `Host *`, so the only hosts it affects are the git hosts.
- `~/.ssh/config.d/yardo-defaults`, included last, after `Match all`
  (an `Include` at the end would otherwise sit inside the user's last
  `Host` block and only apply to it). ssh takes the first value it finds,
  so anything the user sets wins over these defaults. It holds the
  accept-new setting.
Both Include lines are added once; yardo finds and keeps them where they
are if the user moved them. *Alternative*: keep the managed block. Rejected:
it rewrites the user's file on every change.

**The user's own Host entries are detected by `ssh -G`-free parsing.**
yardo reads `~/.ssh/config` minus its own lines, splits `Host` patterns,
and treats a host as the user's when a non-wildcard pattern matches it
(`github.com`, `bitbucket.org`, `ssh.bitbucket.org`, the GitLab host). A
`Host *` doesn't count. Includes of the user's own are not followed (said
in Risks). *Alternative*: `ssh -G` comparison; it can't tell which file
set a value.

**Bitbucket's entry.** `Host bitbucket.org ssh.bitbucket.org` with
`HostName ssh.bitbucket.org`, `User git`, the key. `known_hosts` gets the
checked-in `bitbucket_known_hosts` (Atlassian's published keys) for both
names, as `github_known_hosts` is today; no `accept-new` for Bitbucket
any more. GitLab's entry keeps `accept-new`, as today.

**Accept new hosts' keys is a Settings choice** (`ssh_accept_new`:
`""`/`off` = ask, `on` = accept new, refuse changed), written as
`StrictHostKeyChecking accept-new` in `yardo-defaults`. Off writes the
file empty, not `ask`, so ssh's own default applies.

**The page** is one `ui.Choose` with the four options and, for "Use a key
on this machine", a second `Choose` of the keys found (files in `~/.ssh`
whose first line is a private key header, not `*.pub`, not under
`identities/*.old-*`). Paste is a `ui.Password` loop. The existing-key
page is a `Choose` with Use it (default) / Replace it, its title showing
`ssh-keygen -lf` output.

## Risks / Trade-offs

- [The user's own `Include` files set a git host] → not detected; yardo's
  file wins for that host. The run's note says which hosts yardo set, and
  the user can add their own entry in `~/.ssh/config` to take over.
- [Bitbucket rotates its host keys] → pinned keys refuse; a yardo release
  updates `bitbucket_known_hosts`, as for GitHub. The refusal names the
  file.
- [Accept-new on a machine that connects to many new hosts] → it's opt-in,
  and a changed key is still refused.
- [A user removes the Include line] → yardo adds it back on the next run
  and says so; to opt out of yardo's settings, pick No key here.

## Migration Plan

On the first run: move the old block's hosts into `yardo-git`, remove the
block from `~/.ssh/config`, add the Include lines, add Bitbucket's pinned
keys. Keys stay where they are. Rollback: an older yardo writes its block
back at the top of `~/.ssh/config`; the Include lines then point at files
that duplicate it, which is harmless (first value wins, and they agree).
