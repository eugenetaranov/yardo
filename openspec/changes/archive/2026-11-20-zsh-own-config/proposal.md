## Why

yardo owns `~/.zshrc`: it rewrites the whole file on every run, so an
alias or export you add there is gone after the next `sudo yardo` (your
first version is kept once, as `.zshrc.orig`, and that's all). The way
around it, `~/.zshrc.local`, works, but you have to know it exists.
Everyone's habit, and every tool's install note ("add this line to your
`.zshrc`"), points at `.zshrc`, and those lines get lost.

## What Changes

- **`~/.zshrc` is yours.** yardo keeps one small marked block at its top
  that loads yardo's zsh setup, and never touches another line of it.
  Aliases, exports and tools' install lines go below the block, as
  anywhere else, and stay through every run.
- **yardo's zsh setup moves to a file of its own**,
  `~/.config/yardo/zshrc`: oh-my-zsh and its plugins, the prompt,
  completions, and the lines for yardo's other shell files (shell.sh,
  helpers, agent keys, Neovim, AWS). yardo rewrites that file freely; its
  first line says so.
- **Yours wins.** Your lines come after yardo's setup, so an alias or
  option you set overrides yardo's.
- **`~/.zshrc.local` keeps working**, for machines that use it.
- **Machines set up before**, on their next run:
  - a `.zshrc` yardo wrote is saved once as `~/.zshrc.before-yardo-split`
    and replaced by the block, and the run says so, with where the old
    file is, in case you had added lines to it;
  - lines yardo added to `.zshrc` for its other files are taken out (the
    new file has them).
- **A `.zshrc` of your own on a new install is kept**: the block goes on
  top, and nothing is moved to `.orig`. If it loads oh-my-zsh itself, the
  run says that it now loads twice and which lines to take out.
- **Uninstalling the zsh config** removes the block and yardo's file, and
  leaves the rest of `.zshrc` alone.
- **Home's "Restore" no longer fires for your own edits** to `.zshrc`:
  only a missing block counts as changed.

Not in this change: `.bashrc` (yardo already only adds lines to it),
`.tmux.conf` and Neovim's `init.lua` (they keep their `.local` files; the
same split could follow), and migrating lines out of an old edited
`.zshrc` automatically.

## Capabilities

### New Capabilities

### Modified Capabilities
- `yardo-shell-config`: yardo's zsh setup lives in its own file, loaded
  from a block at the top of the user's `.zshrc`, which yardo otherwise
  never changes.

## Impact

- `internal/steps/shell.go` (`zshrcStep`, `Dropins`): render to
  `~/.config/yardo/zshrc`, keep the block in `~/.zshrc`, migrate.
- `internal/assets/conf/zshrc.tmpl`: a header; unchanged otherwise.
- `internal/catalog/data/*.toml`: the `line` steps into `~/.zshrc`
  (20-shell, 50-cloud, 80-ai) go; `.bashrc`'s stay. `remove` for the zsh
  config.
- README "Your own settings"; the `session-on-login` change's file is
  sourced from yardo's file, first.
