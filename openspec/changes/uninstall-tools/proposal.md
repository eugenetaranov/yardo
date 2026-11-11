## Why

You install a VM with the defaults, then find you don't need NetBird or
the AWS tools. yardo should take them off cleanly. Today it mostly can't:

- **Most items can't be uninstalled.** `d` works only when every step is
  `apt`, `apt_repo`, `mise` or `note`, or the item has a `remove` script.
  43 visible items fail that test, AWS among them: its `file` (aws.sh),
  two `line`s (.bashrc, .zshrc) and the `run` that installs AWS's
  session-manager-plugin .deb. `d` on AWS says "yardo can't uninstall
  AWS tools… space stops managing it", and the CLI, the .deb and the
  rc lines stay.
- **NetBird is two rows, and only one can be removed.** The login
  (`netbird`, Logins tab) is a `go:netbird_up` step, so `d` on it says
  it can't be removed. You have to know to go to Net → NetBird and press
  `d` there. Removing that also leaves the daemon's state
  (`/etc/netbird`, `/var/lib/netbird`) and the peer in the NetBird
  dashboard, and doesn't say so.
- **Two verbs that sound alike.** Space "stops managing" (queued, it
  stays installed), `d` "removes" (runs now, one at a time, and refuses
  while anything is queued). Removing NetBird and AWS is two trips, and
  space, the key you'd try first, does the thing that leaves them on the
  machine.
- **The question doesn't say what goes.** "Removing it uninstalls it."
  You can't see which packages and tools go, which stay because
  something else uses them (fzf stays for Shell helpers), or what is
  left behind.
- **No way from the shell.** There's `yardo status` and `yardo mcp`, but
  no `yardo remove aws netbird`, so it can't be scripted or run from
  pmox over ssh.

## What Changes

- **Undo for yardo's own steps.** Removing an item also undoes its
  `file`/`template` (delete the file, unless it was edited by hand),
  `line` (delete the line), `block` (delete the managed block), `link`
  (delete the symlink if it points where yardo put it), `sysctl` (delete
  the drop-in, reload) and `systemctl` (stop and disable what the item
  enabled). `git_clone` deletes the clone only when it has no local
  changes. These step types count as undone for `Removable`.
- **`remove` scripts for the rest.** `run`, `go:*`, `legacy`, `cred`,
  `git_config` and `user_shell` steps still need the item's `remove`
  script. AWS gets one: `dpkg -r session-manager-plugin`. Items whose
  `remove` would undo what they're for (sshharden, sudopw, the shell
  itself) say why with a new `keep_say`, and catalog validation fails a
  visible item that's neither removable nor has `keep_say`.
- **A login removes its client.** `remove_with = "netbird-install"` on
  the NetBird login (and the same for Tailscale): `d` on the login row
  asks to remove NetBird and takes both items off. NetBird's `remove`
  script also deletes its state, and the question says the peer stays in
  the dashboard until you delete it there.
- **`d` queues an uninstall, like space queues a change.** Several items
  can be queued; `a` applies them in one go with adds and stop-managing.
  The row shows `✗ uninstall` (space shows `− stop managing`). The enter
  panel gets "Uninstall (d)" above "Stop managing it". `d` no longer
  refuses while something is queued.
- **The question says what goes and what stays.** Before applying, each
  queued uninstall lists what it takes off (packages that are installed,
  mise tools, files, rc lines, its own script's `remove_say`), what it
  keeps and why ("fzf: Shell helpers use it"), what it leaves (your
  `~/.aws`, the NetBird peer), and what stops working (items that need
  it). It never touches the user's own data.
- **After, it says how each went.** One line per item: removed, or
  failed with the step and the log. A failed item stays installed and
  shows `partial`; `d` again picks up where it stopped (every undo
  skips what's already gone). Other queued items still run.
- **`yardo remove <item>…`.** By id or by the label's short name (`aws`,
  `netbird`), several at once. It prints the same plan and asks; `--yes`
  skips the question, `--dry-run` prints the plan only. Exit code non-zero
  if any item failed or can't be removed.
- README and `--help`: removing tools.

Not in this change: `pmox yardo remove <vm> <item>` (a follow-up: it
runs `yardo remove --yes` over ssh); removing the NetBird peer or
Tailscale node from their control servers (needs an API token yardo
doesn't keep); undo for `go:*` steps beyond the scripts named here.

## Capabilities

### New Capabilities
- `yardo-uninstall`: what removing an item takes off, keeps and leaves;
  the board's `d` and queue; `yardo remove`.

## Impact

- `internal/catalog/catalog.go` (`keep_say`, `remove_with`, `undone`,
  validation), `internal/steps/remove.go` (undo per step type, the plan),
  `internal/steps/{file,line,block,link,sysctl,systemctl,git}.go` (undo),
  `cmd/yardo/remove.go` (queue, CLI), `cmd/yardo/main.go` (`remove`
  verb), `internal/ui/board.go` (`d` queues, confirm panel, enter panel),
  catalog data (AWS, NetBird, Tailscale, `keep_say` on the rest), README.
- Items already installed: nothing changes until you remove one. Files
  written by an older yardo are compared against what this one would
  write, so one changed since then counts as edited by hand and is kept.
