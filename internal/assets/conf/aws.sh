# yardo: AWS profile switching, installed with the AWS tools item.
# Sourced from ~/.zshrc and ~/.bashrc. oh-my-zsh's aws_profiles (which
# lists them, and which asp and its completion use) is left alone.

# aws_profile: pick a profile (~/.aws/config and ~/.aws/credentials), with
# its settings beside it (keys hidden), and switch this shell to it,
# through oh-my-zsh's asp when it's there (the prompt follows).
# aws_profile NAME switches straight to NAME.
aws_profile() {
  local profiles p s
  profiles=$(cat ~/.aws/config ~/.aws/credentials 2>/dev/null |
    sed -n 's/^\[\(profile \)\{0,1\}\([^]]*\)\].*/\2/p' | sort -u)
  if [ -n "$1" ]; then
    p=$1
  else
    [ -n "$profiles" ] || { echo "No profiles in ~/.aws/config or ~/.aws/credentials"; return 1; }
    p=$(printf '%s\n' "$profiles" |
        fzf --prompt='AWS profile > ' --height=40% --reverse \
            --header="now: ${AWS_PROFILE:-default}" \
            --preview="awk -v p={} '/^\\[/ {on = (\$0 == \"[\" p \"]\" || \$0 == \"[profile \" p \"]\")} on' ~/.aws/config ~/.aws/credentials 2>/dev/null | grep -viE 'secret|token|access_key'" \
            --preview-window=right,50%) || return
  fi
  if type asp >/dev/null 2>&1; then
    asp "$p" || return
  else
    printf '%s\n' "$profiles" | grep -qxF -- "$p" || { echo "No AWS profile $p" >&2; return 1; }
    export AWS_PROFILE="$p" AWS_DEFAULT_PROFILE="$p"
  fi
  echo "AWS_PROFILE=$AWS_PROFILE"
  # boto3 (rds-mysql, most SDK tools) stops without a region; aws s3 ls
  # doesn't, so say so now.
  if [ -z "${AWS_REGION:-}${AWS_DEFAULT_REGION:-}" ] &&
     ! awk -v p="$p" '/^\[/ {on = ($0 == "[" p "]" || $0 == "[profile " p "]")} on && /^[[:space:]]*region[[:space:]]*=/ {f = 1} END {exit !f}' ~/.aws/config 2>/dev/null; then
    case $p in default) s=default ;; *) s="profile $p" ;; esac
    echo "No region for $p: add 'region = <region>' under [$s] in ~/.aws/config (or export AWS_REGION)." >&2
  fi
}
alias aws-profile=aws_profile
