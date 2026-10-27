# shellcheck shell=bash
# yardo shell drop-in, sourced at the end of ~/.zshrc and ~/.bashrc.
# Written by yardo ("Shell niceties"); re-running it overwrites this
# file, so put your own changes in your rc file after the source line.
# Each hook runs only if its tool is installed.

if [ -n "${ZSH_VERSION:-}" ]; then _yardo_sh=zsh; else _yardo_sh=bash; fi

command -v zoxide >/dev/null 2>&1 && eval "$(zoxide init "$_yardo_sh")"
command -v direnv >/dev/null 2>&1 && eval "$(direnv hook "$_yardo_sh")"

# fzf: Ctrl-R history, Ctrl-T files, Alt-C cd. Ubuntu's fzf (0.44) has no
# `fzf --zsh`, so load the scripts it ships. With oh-my-zsh's
# zsh-fzf-history-search, Ctrl-R stays with that plugin.
if command -v fzf >/dev/null 2>&1; then
  for _f in /usr/share/doc/fzf/examples/key-bindings."$_yardo_sh" \
            /usr/share/doc/fzf/examples/completion."$_yardo_sh"; do
    [ -r "$_f" ] && . "$_f"
  done
  unset _f
  # Give Ctrl-R back to zsh-fzf-history-search when oh-my-zsh loaded it.
  if [ "$_yardo_sh" = zsh ] && (( ${+widgets[fzf_history_search]} )); then
    bindkey '^R' fzf_history_search
  fi
fi

if command -v eza >/dev/null 2>&1; then
  alias ls='eza --group-directories-first'
  alias ll='eza -l --git --group-directories-first'
  alias la='eza -la --git --group-directories-first'
  alias lt='eza --tree --level=2'
fi
command -v bat >/dev/null 2>&1 && alias cat='bat --paging=never --style=plain'

# Terraform/OpenTofu: download each provider once per VM, not once per
# working directory.
if command -v terraform >/dev/null 2>&1 || command -v tofu >/dev/null 2>&1; then
  export TF_PLUGIN_CACHE_DIR="$HOME/.terraform.d/plugin-cache"
  [ -d "$TF_PLUGIN_CACHE_DIR" ] || mkdir -p "$TF_PLUGIN_CACHE_DIR"
fi

# kubecolor: colored kubectl output, keeping kubectl's completion.
if command -v kubecolor >/dev/null 2>&1; then
  alias kubectl=kubecolor
  if [ "$_yardo_sh" = zsh ] && (( $+functions[compdef] )); then
    compdef kubecolor=kubectl
  fi
fi

if command -v nvim >/dev/null 2>&1; then
  export EDITOR=nvim VISUAL=nvim
else
  export EDITOR=vim VISUAL=vim
fi

unset _yardo_sh

# Your own settings for zsh and bash: yardo never writes local.sh.
[ -r ~/.config/yardo/local.sh ] && . ~/.config/yardo/local.sh
