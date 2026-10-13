# Written by yardo ("Agent API keys"): exports the keys filled in in
# ~/.config/yardo/agent.env. Sourced by interactive shells only, so
# services and cron jobs don't get them.
if [ -r "$HOME/.config/yardo/agent.env" ]; then
  while IFS='=' read -r _yardo_k _yardo_v; do
    case "$_yardo_k" in '' | \#* | *[!A-Za-z0-9_]*) continue ;; esac
    [ -n "$_yardo_v" ] && export "$_yardo_k=$_yardo_v"
  done < "$HOME/.config/yardo/agent.env"
  unset _yardo_k _yardo_v
fi
