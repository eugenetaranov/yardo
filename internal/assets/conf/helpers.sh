# shellcheck shell=bash
# yardo shell helpers for git, Kubernetes, Terraform, AWS and TLS: sourced
# from ~/.zshrc and ~/.bashrc. Written by yardo ("Shell helpers"); a re-run
# overwrites this file, so put your own in ~/.config/yardo/local.sh.
# Each helper needs its tool only when it runs; most pick with fzf.

_yardo_confirm() {
  printf '%s [y/N] ' "$1"
  read -r _yardo_ans
  case $_yardo_ans in [yY]*) unset _yardo_ans; return 0 ;; esac
  unset _yardo_ans
  return 1
}

# --- git ---------------------------------------------------------------

# git-make-branch <ticket-URL-or-ID>: asks for a name, checks out
# <ticket>/<name-with-dashes> (DEVOPS-12 + "fix login" → devops-12/fix-login).
git-make-branch() {
  if [ -z "$1" ]; then
    echo "Usage: git-make-branch <ticket-URL-or-ID>"
    return 1
  fi
  local ticket name
  ticket=$(printf '%s\n' "$1" | grep -oE '[A-Z][A-Z0-9]*-[0-9]+' | head -n1)
  [ -n "$ticket" ] || ticket=$1
  printf 'Branch name: '
  read -r name
  [ -n "$name" ] || { echo "Branch name cannot be empty"; return 1; }
  git checkout -b "$(printf '%s' "$ticket" | tr '[:upper:]' '[:lower:]')/$(printf '%s' "$name" | tr ' ' '-')"
}

# The remote's default branch, checked out and pulled.
default_branch_git() {
  local b
  b=$(git rev-parse --abbrev-ref origin/HEAD 2>/dev/null) || {
    echo "origin/HEAD unknown: git remote set-head origin --auto"
    return 1
  }
  git checkout "${b#origin/}" && git pull
}

# Pick one of the 20 most recent local branches (not the default one).
recent_branches() {
  local def b
  def=$(git rev-parse --abbrev-ref origin/HEAD 2>/dev/null)
  def=${def#origin/}
  b=$(git for-each-ref --count=20 --sort=-committerdate \
        --format='%(refname:short)' refs/heads/ | grep -vxF "${def:-}" |
      fzf --prompt='Branch: ' --height=15 --reverse \
          --preview='git log --oneline --color=always -15 {}') || return
  git checkout "$b"
}

# Pick any branch, local or remote; a remote one is checked out tracking.
fzf-git-checkout() {
  git rev-parse HEAD >/dev/null 2>&1 || return
  local b
  b=$(git branch -a --format='%(refname:short)' | grep -v '/HEAD$' |
      fzf --prompt='Checkout: ' --height=40% --reverse) || return
  if git show-ref --verify --quiet "refs/heads/$b"; then
    git checkout "$b"
  else
    git checkout --track "$b"
  fi
}

# --- Kubernetes --------------------------------------------------------

if command -v kubectl >/dev/null 2>&1; then
  alias kev='kubectl get events --sort-by=.metadata.creationTimestamp'
  alias kgn='kubectl get nodes --sort-by=.metadata.creationTimestamp'
  alias kdn='kubectl describe node'
  alias kgpres='kubectl get pods -o custom-columns="NAME:.metadata.name,CPU:.spec.containers[*].resources.requests.cpu,MEMORY:.spec.containers[*].resources.requests.memory"'
fi
command -v stern >/dev/null 2>&1 && alias sternf='stern --only-log-lines --no-follow'

# Pick a kubeconfig under ~/.kube and export KUBECONFIG for this shell.
k8s_config_switch() {
  local f
  f=$(find ~/.kube -type f \( -name config -o -name '*.yaml' -o -name '*.yml' -o -name '*.conf' \) \
        -not -path '*/cache/*' 2>/dev/null | sed "s|^$HOME/.kube/||" | sort |
      fzf --prompt='kubeconfig> ' --height=40% --reverse) || return
  export KUBECONFIG="$HOME/.kube/$f"
  echo "KUBECONFIG=$KUBECONFIG"
}

# keit <pod> [shell]: a shell in the pod (bash, else sh).
keit() {
  [ -n "$1" ] || { echo "Usage: keit <pod> [shell]"; return 1; }
  if [ -n "$2" ]; then
    kubectl exec -ti "$1" -- "$2"
  else
    kubectl exec -ti "$1" -- sh -c 'command -v bash >/dev/null && exec bash || exec sh'
  fi
}

# kgpf <words…>: pods whose line has every word.
kgpf() {
  local out w
  out=$(kubectl get pods)
  for w in "$@"; do
    out=$(printf '%s\n' "$out" | grep -i -- "$w")
  done
  printf '%s\n' "$out"
}

# A throwaway debug pod (removed on exit) in the namespace of a pod you pick.
kdebug() {
  local image="${KDEBUG_IMAGE:-nicolaka/netshoot:latest}" ns
  ns=$(kubectl get pods -A --no-headers \
        -o custom-columns='NS:.metadata.namespace,POD:.metadata.name,NODE:.spec.nodeName,STATUS:.status.phase' |
      fzf --prompt='Pod (picks its namespace) > ' \
          --header="context: $(kubectl config current-context) — image: $image" \
          --height=60% --reverse | awk '{print $1}')
  [ -n "$ns" ] || { echo "No pod selected."; return 1; }
  local pod="kdebug-$(whoami)-$RANDOM"
  echo "Launching $image in namespace '$ns' as pod '$pod' (removed on exit)"
  kubectl run "$pod" --rm -it --restart=Never -n "$ns" --image="$image" -- sh
}

# Each node's AMI (EKS).
kgnami() {
  kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.providerID}{"\n"}{end}' |
    while IFS=$'\t' read -r node pid; do
      printf '%s\t%s\n' "$node" "$(aws ec2 describe-instances --instance-ids "${pid##*/}" \
        --query 'Reservations[0].Instances[0].ImageId' --output text)"
    done
}

k8s_list_running_images() {
  kubectl get pods -A -o custom-columns='NAMESPACE:.metadata.namespace,POD:.metadata.name,IMAGE:.spec.containers[*].image,IMAGE_ID:.status.containerStatuses[*].imageID'
}

# Deployments whose pods restarted, most restarts first.
# k8s_pods_restarted_pods_stats [namespace]
k8s_pods_restarted_pods_stats() {
  local scope=-A
  [ -n "$1" ] && scope="--namespace=$1"
  kubectl get pods "$scope" -o json | jq -r '
      [ .items[]
        | { ns: .metadata.namespace,
            rs: (.metadata.ownerReferences[]? | select(.kind=="ReplicaSet") | .name),
            restarts: ([.status.containerStatuses[]?.restartCount] | add // 0) }
        | select(.rs != null)
        | .deploy = (.rs | sub("-[a-f0-9]+$"; "")) ]
      | group_by(.ns + "/" + .deploy)
      | map({key: (.[0].ns + "/" + .[0].deploy), restarts: ([.[].restarts] | add)})
      | map(select(.restarts > 0))
      | sort_by(-.restarts)
      | .[] | [.key, .restarts] | @tsv' | column -t
}

# k8s_node_terminate <node>…: force-drain and delete each node, then
# terminate its EC2 instance. Asks first.
k8s_node_terminate() {
  [ $# -gt 0 ] || { echo "Usage: k8s_node_terminate <node-name>…"; return 1; }
  _yardo_confirm "Drain, delete and terminate $* in $(kubectl config current-context)?" || return 1
  local node id
  for node in "$@"; do
    echo "== $node"
    kubectl get node "$node" >/dev/null 2>&1 || { echo "not in the cluster, skipped"; continue; }
    kubectl get pods -A --field-selector "spec.nodeName=$node" -o json |
      jq -r '.items[] | select(.metadata.deletionTimestamp != null) | "\(.metadata.namespace) \(.metadata.name)"' |
      while read -r ns pod; do
        kubectl delete pod -n "$ns" "$pod" --grace-period=0 --force
      done
    kubectl drain "$node" --ignore-daemonsets --delete-emptydir-data --force \
      --grace-period=0 --disable-eviction --timeout=30s || echo "drain failed or timed out, going on"
    kubectl patch node "$node" -p '{"metadata":{"finalizers":[]}}' --type=merge >/dev/null 2>&1
    kubectl delete node "$node" --force --grace-period=0
    id=$(aws ec2 describe-instances --filters "Name=private-dns-name,Values=$node" \
          --query 'Reservations[].Instances[].InstanceId' --output text 2>/dev/null)
    if [ -z "$id" ] || [ "$id" = None ]; then
      echo "no EC2 instance (already terminated?)"
      continue
    fi
    aws ec2 terminate-instances --instance-ids "$id" >/dev/null &&
      echo "terminated $id" || echo "failed to terminate $id"
  done
}

# k8s_delete_ns <namespace>: a namespace stuck in Terminating, finalizers
# stripped (Rancher's and Fleet's too) and deletion forced. Asks first.
k8s_delete_ns() {
  local ns=$1 r
  [ -n "$ns" ] || { echo "Usage: k8s_delete_ns <namespace>"; return 1; }
  _yardo_confirm "Strip finalizers and force-delete namespace $ns in $(kubectl config current-context)?" || return 1
  for r in bundledeployments.fleet.cattle.io bundles.fleet.cattle.io \
           clusters.provisioning.cattle.io clusters.management.cattle.io \
           projectroletemplatebindings.management.cattle.io \
           clusterroletemplatebindings.management.cattle.io \
           rolebindings roles secrets configmaps serviceaccounts; do
    kubectl get "$r" -n "$ns" -o name 2>/dev/null |
      xargs -r -I {} kubectl patch {} -n "$ns" -p '{"metadata":{"finalizers":[]}}' --type=merge >/dev/null 2>&1
  done
  kubectl get namespace "$ns" -o json | jq 'del(.metadata.finalizers) | .spec.finalizers = []' |
    kubectl replace --raw "/api/v1/namespaces/$ns/finalize" -f - >/dev/null && echo "finalized $ns"
}

# --- Terraform ---------------------------------------------------------

# tfreset [workspace]: a fresh init -upgrade, then a workspace (picked).
tfreset() {
  local ws
  rm -rf .terraform .terraform.lock.hcl
  terraform init -upgrade || return
  if [ -n "$1" ]; then
    ws=$1
  else
    ws=$(terraform workspace list | sed 's/[* ]//g' | grep -v '^$')
    if [ "$(printf '%s\n' "$ws" | wc -l)" -gt 1 ]; then
      ws=$(printf '%s\n' "$ws" | fzf --prompt='Workspace: ') || return
    fi
  fi
  terraform workspace select "$ws"
}

alias tf-format='terraform fmt -recursive'

# tflint with the terraform ruleset's style rules only.
tfl() {
  tflint \
    --only=terraform_deprecated_interpolation \
    --only=terraform_deprecated_index \
    --only=terraform_comment_syntax \
    --only=terraform_documented_outputs \
    --only=terraform_documented_variables \
    --only=terraform_typed_variables \
    --only=terraform_naming_convention \
    --only=terraform_required_version \
    --only=terraform_required_providers \
    --only=terraform_workspace_remote \
    --only=terraform_unused_declarations "$@"
}

# --- AWS ---------------------------------------------------------------

# Pick a profile from ~/.aws/config and export AWS_PROFILE.
aws-profile() {
  local p
  p=$(awk '/^\[default\]/ {print "default"}
           /^\[profile / {gsub(/^\[profile |\]$/, ""); print}' ~/.aws/config |
      sort | fzf --prompt='AWS profile > ' --height=40% --reverse) || return
  export AWS_PROFILE="$p"
  echo "AWS_PROFILE=$AWS_PROFILE"
}

aws-list-rds() {
  aws rds describe-db-clusters --query 'DBClusters[*].[DBClusterIdentifier,Endpoint]' --output text |
    column -t | fzf --prompt='RDS > '
}

# Pick an EC2 instance: prints its id, name and addresses.
aws-ec2-list() {
  aws ec2 describe-instances --output text \
    --query 'Reservations[].Instances[].[InstanceId, Tags[?Key==`Name`].Value | [0], State.Name, PrivateIpAddress, PublicIpAddress]' |
    column -t | fzf --header='id  name  state  private  public' --prompt='Instance: '
}

# ec2ctl <start|stop|status|restart> <instance-id>; restart is a stop and
# a start (a new host), waiting for each.
ec2ctl() {
  local action=$1 id=$2
  if [ -z "$action" ] || [ -z "$id" ]; then
    echo "Usage: ec2ctl <start|stop|status|restart> <instance-id>"
    return 1
  fi
  case $action in
    start) aws --no-cli-pager ec2 start-instances --instance-ids "$id" >/dev/null ;;
    stop) aws --no-cli-pager ec2 stop-instances --instance-ids "$id" >/dev/null ;;
    status)
      aws ec2 describe-instances --instance-ids "$id" \
        --query 'Reservations[].Instances[].State.Name' --output text ;;
    restart|reboot)
      echo "Stopping…"
      aws --no-cli-pager ec2 stop-instances --instance-ids "$id" >/dev/null &&
        aws ec2 wait instance-stopped --instance-ids "$id" || return
      echo "Starting…"
      aws --no-cli-pager ec2 start-instances --instance-ids "$id" >/dev/null &&
        aws ec2 wait instance-running --instance-ids "$id" && echo "Running." ;;
    *)
      echo "Usage: ec2ctl <start|stop|status|restart> <instance-id>"
      return 1 ;;
  esac
}

# --- TLS ---------------------------------------------------------------

# openssl_info_text <cert.pem>: the certificate, decoded.
openssl_info_text() { openssl x509 -text -noout -in "$1"; }

# openssl_connect <host> [port]: the chain the server sends (SNI on).
openssl_connect() {
  openssl s_client -showcerts -servername "$1" -connect "$1:${2:-443}" </dev/null
}
