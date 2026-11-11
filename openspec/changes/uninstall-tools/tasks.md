## 1. Undo per step type

- [x] 1.1 `Undo` for file/template (only when content matches), line, block, link (only when it points at yardo's target), sysctl (drop-in, reload), systemctl (stop, disable), git_clone (only when clean); each skips what's already gone
- [x] 1.2 `Kept` grows `Files` and `Lines`: a target another staying item writes stays
- [x] 1.3 `steps.Plan(env, item, kept)`: takes off / keeps (and who uses it) / leaves; `Remove` executes the plan; script, steps in reverse, packages, repos, mise
- [x] 1.4 `catalog.undone` = the step types with an Undo
- [x] 1.5 Tests: each Undo, edited-by-hand kept, shared file kept, Remove run twice

## 2. Catalog

- [x] 2.1 `keep_say`, `remove_with`; validation: a visible item is removable or has `keep_say`; `remove_with` names an item that exists and is removable
- [x] 2.2 AWS: `remove = "dpkg -r session-manager-plugin 2>/dev/null || true"`, `remove_say` (leaves `~/.aws`)
- [x] 2.3 NetBird: `remove_with` on the login; the client's `remove` also deletes `/etc/netbird` and `/var/lib/netbird`; `remove_say` names the dashboard peer. Same for Tailscale (`tailscale logout` first)
- [x] 2.4 Go through the other non-removable visible items: a `remove` script where it's simple (rust, omp, docs, k8stools, neovim, devlimits, zram, journald…), `keep_say` where undoing defeats it (sshharden, sudopw, zshbin, gitid, tzlocale…)
- [x] 2.5 Test: every visible item in the shipped catalog passes validation

## 3. Board

- [x] 3.1 `Pending = "uninstall"`, `✗ uninstall` mark; `d` toggles it; no refusal while queued; `remove_with` resolves to the client
- [x] 3.2 Footer on an installed row: `d uninstall`, `space stop managing`; enter panel: "Uninstall (d)"
- [x] 3.3 Confirmation: per uninstall, takes off / keeps (by whom) / leaves / stops managing; an add that re-installs it says so
- [x] 3.4 `BoardResult.Uninstalls`; apply order uninstalls → stop managing → adds; one line per item after; failed = `partial`; autoremove hint
- [x] 3.5 Installed tab: a login under its client
- [x] 3.6 Tests: queue NetBird and AWS, confirm text, one fails

## 4. CLI

- [x] 4.1 `yardo remove <item>… [--yes] [--dry-run]`; names by id or short label, case-insensitive; validate all before removing any
- [x] 4.2 Exit codes: unknown or not removable → 2, nothing done; a removal failed → 1
- [x] 4.3 Tests: dry run, not removable, mixed

## 5. Docs and check

- [x] 5.1 README "Removing tools" and `--help`
- [ ] 5.2 On a VM with NetBird and AWS: remove both from the board; then reinstall, and remove again with `yardo remove --yes`; check nothing of theirs is left (`dpkg -l`, `mise ls`, rc files, `/etc/netbird`)
