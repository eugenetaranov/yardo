## ADDED Requirements

### Requirement: Removing an item undoes yardo's own steps
Removing an item SHALL run its `remove` script, then undo its `file`, `template`, `line`, `block`, `link`, `sysctl`, `systemctl` and `git_clone` steps in reverse order, then purge its apt packages, delete its apt repo files and take off its mise tools. Anything a staying item or the base set also uses SHALL be kept. Removing SHALL NOT touch the user's own data (`~/.aws`, `~/.kube`, `~/.ssh`, shell history, Docker volumes).

#### Scenario: AWS tools
- **WHEN** the user removes `aws` and Shell helpers stays installed
- **THEN** aws-cli, aws-ssm-connect, ssmsearch and rds-mysql are taken off mise, session-manager-plugin and mariadb-client are removed, `~/.config/yardo/aws.sh` and its lines in `.bashrc` and `.zshrc` are deleted, fzf and curl stay, and `~/.aws` is untouched

#### Scenario: Removing twice
- **WHEN** a removal failed part way and the user removes the item again
- **THEN** the steps already undone are skipped and the rest run

### Requirement: Undo never deletes what the user changed
A `file` or `template` SHALL be deleted only when its content is what yardo would write. A `line` SHALL be deleted by exact match only. A `link` SHALL be deleted only when it points at yardo's target. A `git_clone` SHALL be deleted only when it has no local changes. `systemctl` SHALL stop and disable only the units the item enabled. What is kept this way SHALL be named in the result.

#### Scenario: File edited by hand
- **WHEN** the user removes an item whose file was changed since yardo wrote it
- **THEN** the file stays, and the result says it was kept because it was edited by hand

#### Scenario: Clone with local changes
- **WHEN** the user removes an item whose git clone has uncommitted changes
- **THEN** the clone stays, and the result says why

### Requirement: Every visible item is removable or says why not
A visible item in the shipped catalog SHALL be removable (each step has an undo, or the item has a `remove` script), SHALL set `keep_say`, or SHALL set `remove_with`. Catalog validation SHALL fail a shipped item that is none of these; an item from `/etc/yardo/catalog.d` SHALL load either way, so an older local file keeps working. `d` on an item that can't be removed SHALL show its `keep_say` and say that space stops managing it.

#### Scenario: Hardening item
- **WHEN** the user presses `d` on SSH hardening
- **THEN** the board says why yardo won't undo it, and that space stops managing it

#### Scenario: Validation
- **WHEN** a shipped catalog item has a `run` step, no `remove` and no `keep_say`
- **THEN** loading the catalog fails naming the item

#### Scenario: Local item
- **WHEN** an item in `/etc/yardo/catalog.d` has a `run` step, no `remove` and no `keep_say`
- **THEN** the catalog loads, and `d` on it says yardo can't uninstall it

### Requirement: A login is removed with its client
An item MAY set `remove_with` to the item it logs in to. `d` on it SHALL ask to remove that item, and removing SHALL take off both. NetBird's removal SHALL delete its state (`/etc/netbird`, `/var/lib/netbird`), and its question SHALL say the peer stays in the NetBird dashboard until deleted there. Tailscale SHALL do the same for its node. The Installed tab SHALL show a login under its client.

#### Scenario: NetBird from the login row
- **WHEN** the user presses `d` on the NetBird login
- **THEN** the board asks to remove NetBird, saying it disconnects, uninstalls the client and deletes its state, and that the peer stays in the dashboard

### Requirement: The board queues uninstalls
On the Tools board, `d` on an installed, removable item SHALL queue it for uninstall, marked `✗ uninstall`; `d` again SHALL take it off the queue. `d` SHALL work while other changes are queued. `a` SHALL apply uninstalls, then stop-managing, then adds, in one run.

#### Scenario: Toggle
- **WHEN** the user presses `d` twice on AWS tools
- **THEN** AWS tools is queued, then taken off the queue

#### Scenario: NetBird and AWS together
- **WHEN** the user presses `d` on NetBird, `d` on AWS tools, then `a`
- **THEN** one confirmation lists both, and one run removes both

#### Scenario: Re-added by an add
- **WHEN** an uninstall is queued with an add that needs that item
- **THEN** the confirmation says the add installs it again

### Requirement: Uninstall and stop managing are told apart
Space SHALL keep queuing stop-managing, marked `− stop managing`, which leaves the item installed. The footer SHALL show both `d uninstall` and `space stop managing` on an installed row. The enter panel of an installed, removable item SHALL offer "Uninstall (d)" above "Stop managing it".

#### Scenario: Footer
- **WHEN** the cursor is on an installed row
- **THEN** the footer shows `d uninstall` and `space stop managing`

### Requirement: The confirmation says what goes, stays and is left
Before applying, the confirmation SHALL list for each queued uninstall what it takes off (installed packages, mise tools, files, rc lines, the item's `remove_say`), what it keeps and which staying item uses it, what it leaves behind, and which installed items need it and stop being managed. The list SHALL come from the same plan the removal executes.

#### Scenario: Shared package
- **WHEN** AWS tools is queued for uninstall and Shell helpers is installed
- **THEN** the confirmation lists fzf under kept, used by Shell helpers

### Requirement: Each removal reports how it went
After applying, the board SHALL show one line per uninstalled item: removed, or failed with the step and the log path. A failed item SHALL stay installed and show `partial`. A failure SHALL NOT stop the other queued items. When apt reports packages it could autoremove, the result SHALL say so with `sudo apt autoremove`.

#### Scenario: One fails
- **WHEN** NetBird's removal fails and AWS's succeeds
- **THEN** AWS is gone, NetBird shows `partial` with the failed step, and the log path is shown

### Requirement: yardo remove from the shell
`yardo remove <item>…` SHALL remove the named items, each by id or by its label's short name, case-insensitive. It SHALL print the same plan as the board and ask before removing; `--yes` SHALL skip the question and `--dry-run` SHALL print the plan and change nothing. An unknown name, or one that can't be removed, SHALL be reported with its `keep_say` and the command SHALL exit non-zero without removing anything. A failed removal SHALL make it exit non-zero after the others have run.

#### Scenario: Dry run
- **WHEN** the user runs `yardo remove aws netbird --dry-run`
- **THEN** yardo prints what each would take off, keep and leave, and changes nothing

#### Scenario: Not removable
- **WHEN** the user runs `yardo remove sshharden aws`
- **THEN** yardo prints sshharden's `keep_say`, removes nothing and exits non-zero
