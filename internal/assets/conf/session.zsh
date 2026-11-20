# yardo: on an ssh login, you're in your tmux or zellij session: the one
# running is attached, else one starts. Written by yardo; a re-run
# rewrites it. Which one: ~/.config/yardo/session (tmux, zellij or off;
# missing: asked once), also sudo yardo › Settings › Session on login.
# Skip it once: ssh -t vm YARDO_SESSION=off zsh.
# Sourced first, before the prompt: it may ask, and hands the terminal
# over. The multiplexer runs as a child: detaching gives the plain shell.
() {
  # A person at a terminal over ssh, not inside a session already, not an
  # editor's terminal or a script.
  [[ -o interactive && -t 0 && -t 1 ]] || return 0
  [[ -n $SSH_CONNECTION && -z $TMUX && -z $ZELLIJ ]] || return 0
  [[ $TERM_PROGRAM != vscode && $TERM != dumb && $YARDO_SESSION != off ]] || return 0
  (( ${#${(k)parameters[(I)(VSCODE_*|CURSOR_*|JETBRAINS_*|TERMINAL_EMULATOR)]}} )) && return 0

  local f=~/.config/yardo/session use= key
  local -a have running
  [[ -r $f ]] && use=$(<$f)
  [[ $use == off ]] && return 0
  (( $+commands[tmux] )) && have+=(tmux)
  (( $+commands[zellij] )) && have+=(zellij)
  (( $#have )) || return 0

  # zellij's sessions still running, as it lists them (EXITED ones can't be
  # attached without resurrecting).
  local -a zs
  if (( ${have[(Ie)zellij]} )); then
    zs=(${${(f)"$(zellij list-sessions --no-formatting 2>/dev/null)"}:#*EXITED*})
    zs=(${zs%% *})
  fi
  (( ${have[(Ie)tmux]} )) && tmux has-session 2>/dev/null && running+=(tmux)
  (( $#zs )) && running+=(zellij)

  if (( ! ${have[(Ie)$use]} )); then
    if (( $#have == 1 )); then
      use=$have[1]
    elif (( $#running == 1 )); then
      use=$running[1]
    else
      local verb=start
      (( $#running )) && verb="attach to"
      print -n "Session: $verb [t]mux  [z]ellij  [n]ever (change in sudo yardo › Settings) "
      read -k1 key
      print
      case $key in
        t) use=tmux ;;
        z) use=zellij ;;
        n) use=off ;;
        *) return 0 ;;
      esac
      mkdir -p ${f:h} && print -r -- $use >| $f
      [[ $use == off ]] && return 0
    fi
  fi

  local rc
  if [[ $use == tmux ]]; then
    if (( ${running[(Ie)tmux]} )); then tmux attach; else tmux new -s main; fi
  else
    if (( $#zs )); then zellij attach $zs[1]; else zellij attach -c main; fi
  fi
  rc=$?
  (( rc )) && print -u2 "yardo: $use didn't start (exit $rc): this is the plain shell."
  return 0
}
