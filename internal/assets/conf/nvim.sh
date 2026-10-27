# shellcheck shell=bash
# yardo's Neovim drop-in, sourced from ~/.bashrc and ~/.zshrc. Written by
# yardo (the Neovim item); re-running it overwrites this file.
#
# nvim is your editor. sudo can't see it (it comes from your mise, and
# sudo keeps its own PATH), so `sudo nvim FILE` edits FILE through
# sudoedit: your nvim, with your config and plugins, runs as you on a
# copy, and sudo writes it back as root when you save. Root never runs a
# binary from your home.

command -v nvim >/dev/null 2>&1 || return 0

export EDITOR=nvim VISUAL=nvim
# sudoedit looks the editor up itself: give it the full path.
SUDO_EDITOR=$(command -v nvim) && export SUDO_EDITOR

sudo() {
  if [ "${1:-}" != nvim ]; then
    command sudo "$@"
    return
  fi
  shift
  for _yardo_a in "$@"; do
    case "$_yardo_a" in
      -*)
        unset _yardo_a
        echo "sudo nvim: only files, no options (it edits through sudoedit)" >&2
        return 2
        ;;
    esac
  done
  unset _yardo_a
  if [ $# -eq 0 ]; then
    echo "usage: sudo nvim FILE... (edits as root through sudoedit)" >&2
    return 2
  fi
  command sudoedit "$@"
}
