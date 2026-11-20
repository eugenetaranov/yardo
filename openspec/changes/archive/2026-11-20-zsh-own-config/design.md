## Context

- `zshrcStep` (`internal/steps/shell.go`) renders `zshrc.tmpl` to
  `~/.zshrc`, appends a source line for each of `Dropins` whose file
  exists, keeps the first pre-existing file as `.zshrc.orig`, and its
  Check is byte equality, so any edit reads as drifted and "Restore"
  overwrites it.
- Other items add `line` steps into `~/.zshrc` and `~/.bashrc` (shell.sh,
  helpers, agent-env, nvim.sh, aws.sh); the template rewrite drops them
  and `zshrcStep` puts them back.
- The catalog has a `block` step: a marked block kept first in a file,
  the rest of the file kept, undone on uninstall.
- The template ends by sourcing `~/.zshrc.local`. Its first lines are
  Powerlevel10k's instant prompt, which must run before anything prints.

## Goals / Non-Goals

**Goals:**
- `~/.zshrc` behaves the way every user and install note expects.
- Nothing changes in what a new shell loads, or in which order.

**Non-Goals:**
- Carrying a user's edits out of an old yardo-written `.zshrc` by
  guessing which lines were theirs.
- The same split for tmux and Neovim.

## Decisions

- **The block is at the top, not the bottom.** The instant prompt has to
  come first, and the user's lines have to come after oh-my-zsh to win.
  Alternative considered: yardo's file sourced last. Rejected: yardo's
  aliases would override the user's, the opposite of what's asked.
- **The block:**
  ```
  # >>> yardo (managed: sudo yardo rewrites this block; your lines go below)
  [ -r ~/.config/yardo/zshrc ] && source ~/.config/yardo/zshrc
  # <<< yardo
  ```
  Guarded, so a deleted yardo file leaves a working shell.
- **Everything of yardo's goes through its one file.** With the zsh
  config installed, `~/.config/yardo/zshrc` sources each drop-in that
  exists (what `zshrcStep.want` appends today). Without the zsh config
  (zsh only, or the user's own setup), the block itself holds the drop-in
  lines, so `shellcfg` and the others still work. `Dropins` keeps one
  list; `zshrcStep` and the block render from it. The `line` steps that
  target `~/.zshrc` leave the catalog; `.bashrc`'s stay.
- **`.zshrc.local` stays sourced, last in yardo's file.** It then loads
  before the user's lines in `.zshrc`, which is fine: both are the
  user's. Nobody has to move anything.
- **Check is "the block is there and yardo's file matches".** Edits
  outside the block are no longer drift.
- **Recognising a yardo-written `.zshrc`:** it contains the template's
  marker comment `# yardo: local overrides (keep last)` (in every
  released template) and no block. That one is saved to
  `~/.zshrc.before-yardo-split` (only when that name is free) and
  replaced by the block. A file with neither marker nor block is the
  user's own: block on top, file kept. A file with the block: left.
- **Old drop-in lines are removed by exact match** against `Dropins`'
  lines, anywhere outside the block, on files of the user's own that an
  older yardo appended to.
- **Double oh-my-zsh:** the user's own file is searched for
  `oh-my-zsh.sh`; a match adds the note. yardo doesn't edit it.
- **Uninstall:** the block's own undo, plus deleting yardo's file.
  `.zshrc.before-yardo-split` and `.zshrc.orig` are left.

## Risks / Trade-offs

- **Edits made inside an old yardo-written `.zshrc` stop applying** after
  the move. → They're in the saved file, the run's note says so, and
  Last run keeps the note. Today those edits are lost outright on the
  next run, so this is no worse.
- **A dotfiles repo that ships its own `.zshrc`** (the dotfiles item,
  stow) replaces the file, block included. → The next run puts the block
  back on top of the stowed file only if it's a regular file; for a
  symlink into the repo the run notes the line to add to the repo, and
  doesn't write through the link.
- **Tools that append to `.zshrc`** now simply work, which is the point;
  they load after yardo's setup.
- **Two changes touch the top of yardo's file** (this and
  session-on-login). → Whichever lands second sources `session.zsh`
  first in `~/.config/yardo/zshrc`.
