## Context

Removal today (`internal/steps/remove.go`): run the item's `remove`
script, purge its apt packages that are installed and that no staying
item or base package needs, delete its apt repo files, `mise unuse -g`
its tools. `Removable()` is true only with a `remove` script or when
every step is `apt`/`apt_repo`/`mise`/`note`. The board's `d` asks y/n
and runs at once; space queues "stop managing".

The user's case: drop NetBird and AWS from a VM set up with them.

| Item | Steps | Today |
|---|---|---|
| `aws` | mise ×2, apt, run (.deb), file, line ×2 | can't uninstall |
| `netbird` (login) | go:netbird_up | can't uninstall |
| `netbird-install` | apt_repo, apt, run + `remove` | uninstalls; leaves state |

## Decisions

- **Undo is per step type, next to its Apply.** Each step type that
  yardo fully owns gets an `Undo(ctx, env)`: file/template, line, block,
  link, sysctl, systemctl, git_clone. `Remove` walks the item's steps in
  reverse and undoes each, after the `remove` script and before the
  packages (a unit is stopped before its package goes). `undone` in the
  catalog becomes "has an Undo".
- **Never delete what the user changed.** A `file` is deleted only when
  its content is what yardo would write now; otherwise it's kept and the
  result says "kept ~/.config/yardo/aws.sh (edited by hand)". A
  `git_clone` with local changes is kept the same way. A `line` is
  deleted by exact match only. A `link` only when it still points at
  yardo's target.
- **Shared targets are kept like packages.** `Kept` grows `Files` and
  `Lines`: a file or rc line another staying item also writes stays.
- **Two words, two marks.** "Uninstall" (`d`, `✗`) takes it off the
  machine; "Stop managing" (space, `−`) leaves it installed. The footer
  shows both on an installed row. Space keeps its meaning, so muscle
  memory and the team-profile warning still hold.
- **One queue.** `Pending` gains `"uninstall"`. `BoardApply` returns
  `Uninstalls` with `Adds` and `Removes`. Order: uninstalls, then
  stop-managing, then adds (an add that needs an uninstalled item puts
  it back, and the confirm panel says so).
- **The plan is computed, not guessed.** `steps.Plan(env, item, kept)`
  returns what `Remove` would do (only packages `dpkg-query` reports
  installed, only files that exist), and `Remove` executes that plan.
  The confirm panel, `yardo remove` and `--dry-run` all print it.
- **`remove_with` over merging rows.** The login and the client stay
  separate items (the login's key is asked separately, and you can log
  out and keep the client), but `d` on a login means the client. The
  Installed tab shows the login under its client, indented.
- **`keep_say` is required, not a fallback message.** Validation makes
  every visible item either removable or explained, so "can't uninstall"
  always says why: "sshharden: undoing it reopens password logins; untick
  to stop managing".
- **The user's data is out of scope by rule.** `~/.aws`, `~/.kube`,
  `~/.ssh`, shell history, Docker volumes: never in a plan. Where an item
  leaves something worth knowing, `remove_say` names it.

## Risks

- **apt autoremove isn't run.** Dependencies pulled in by a purged
  package stay. Running it could take packages the user installed by
  hand that apt marks auto. The result says "`sudo apt autoremove` frees
  N more packages" when apt reports any.
- **Older yardo files count as edited.** A file whose template changed
  between versions is kept, not deleted. Acceptable: keeping is the safe
  side, and it's said.
- **A failed remove script mid-way.** The item stays picked and shows
  `partial`; every Undo and the scripts must tolerate a second run
  (`|| true`, existence checks), which the tests check by running Remove
  twice.
