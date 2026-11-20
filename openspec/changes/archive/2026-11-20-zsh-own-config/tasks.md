## 1. The split

- [x] 1.1 `zshrcStep` renders the template (with a "written by yardo" header and the drop-ins that exist) to `~/.config/yardo/zshrc`, and keeps the block first in `~/.zshrc`; Check is the block present and yardo's file equal; no `.zshrc.orig`. Verify: `shell_test.go`: no `.zshrc` (created, block only), own `.zshrc` (kept under the block), re-run with an alias added (unchanged), block deleted (back on top).
- [x] 1.2 Without the zsh config, the block carries the drop-in lines; with it, yardo's file does; the catalog's `line` steps into `~/.zshrc` go, `.bashrc`'s stay. Verify: catalog test that no step targets `~/.zshrc` with `line`; `shell_test.go` for `shellcfg` with and without `zshcfg`.
- [x] 1.3 Order check: a zsh run in a temp HOME with fake oh-my-zsh shows the user's alias and `EDITOR` below the block winning, and `~/.zshrc.local` still loaded. Verify: a test that runs `zsh -ic` (skipped where zsh is missing).

## 2. Moving machines over

- [x] 2.1 A yardo-written `.zshrc` (the marker, no block) is saved once as `~/.zshrc.before-yardo-split`, replaced by the block, with a note naming it; old drop-in lines are removed from a user's own file by exact match. Verify: `shell_test.go` for both, and a second run that saves and notes nothing.
- [x] 2.2 A user's own file that loads oh-my-zsh: the note; a `.zshrc` that is a symlink: not written through, the note with the line to add. Verify: `shell_test.go` for each.
- [x] 2.3 Uninstall removes the block and yardo's file, leaving the rest. Verify: `remove_test.go` case.

## 3. Docs and checks

- [x] 3.1 README "Your own settings": `.zshrc` is yours, what the block is, `.zshrc.local` still works; the Removing tools paragraph. Verify: README reads with it.
- [x] 3.2 `task test` and `task lint` pass.
- [ ] 3.3 On a VM set up with the current release: add an alias to `.zshrc`, upgrade and run; check the saved file, the note, a working prompt and plugins, the alias after a second run. Verify: the steps' output recorded in the PR.
