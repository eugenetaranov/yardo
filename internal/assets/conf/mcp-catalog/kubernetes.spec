# Kubernetes — resources, logs, events, Helm releases, through a kubeconfig
# you put next to the env file (Flux159/mcp-server-kubernetes; Docker).
# https://github.com/Flux159/mcp-server-kubernetes
DESCRIPTION="Kubernetes: pods, deployments, logs, events and Helm (read-only)"
KIND=docker
IMAGE=flux159/mcp-server-kubernetes:v4.2.1
# Root in the container: ~/.mcp/kubernetes is readable by the gateway's
# user only, not by the image's own user.
DOCKER_ARGS="--user 0:0"
REQUIRED="KUBECONFIG"
skeleton() {
  cat <<'SKEL'
# Kubernetes — copy a kubeconfig to ~/.mcp/kubernetes/config, ideally for a
# read-only service account
# (https://kubernetes.io/docs/reference/access-authn-authz/rbac/), then set
# KUBECONFIG=/mcp/config here (the folder is mounted at /mcp). The image has
# kubectl, helm, and the gcloud and aws auth plugins.
KUBECONFIG=

# Read-only tools only; set to false to let agents apply, scale and delete
ALLOW_ONLY_READONLY_TOOLS=true

# Optional: a context other than the kubeconfig's current one
#K8S_CONTEXT=
SKEL
}
