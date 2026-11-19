# Kubernetes — resources, logs, events, Helm releases, through a kubeconfig
# of yours under ~/.kube (Flux159/mcp-server-kubernetes; Docker).
# https://github.com/Flux159/mcp-server-kubernetes
DESCRIPTION="Kubernetes: pods, deployments, logs, events and Helm (read-only)"
KIND=docker
IMAGE=flux159/mcp-server-kubernetes:v4.2.1
# Your ~/.kube and ~/.aws, read-only: the kubeconfig, and for EKS what its
# aws eks get-token reads (profiles, SSO logins: aws sso login here renews
# them for the server too).
MOUNT_KUBE=1
MOUNT_AWS=1
# Root in the container (it wins over your user, the mounts': files in
# ~/.mcp/<name>, the gateway's user's only, stay readable for a
# KUBECONFIG=/mcp/... of before).
DOCKER_ARGS="--user 0:0"
REQUIRED="KUBECONFIG"
skeleton() {
  cat <<'SKEL'
# Kubernetes — a kubeconfig of yours under ~/.kube (~/.kube/config, say),
# ideally a read-only service account's or role's
# (https://kubernetes.io/docs/reference/access-authn-authz/rbac/).
# ~/.kube and ~/.aws are mounted read-only, so EKS's aws eks get-token
# works with your AWS CLI profiles and SSO logins (when one runs out: aws
# sso login --profile <it>, here). The image has kubectl, helm, and the
# gcloud and aws auth plugins.
KUBECONFIG=

# Read-only tools only; set to false to let agents apply, scale and delete
ALLOW_ONLY_READONLY_TOOLS=true

# Optional: a context other than the kubeconfig's current one
#K8S_CONTEXT=
SKEL
}
