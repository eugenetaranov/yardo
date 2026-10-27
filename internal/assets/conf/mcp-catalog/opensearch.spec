# OpenSearch — search indices, mappings, cluster health (the OpenSearch
# project's opensearch-mcp-server-py, run with uvx in uv's image; Docker).
# https://github.com/opensearch-project/opensearch-mcp-server-py
DESCRIPTION="OpenSearch: search indices, read mappings and cluster health"
KIND=docker
IMAGE=ghcr.io/astral-sh/uv:0.13.6-python3.13-trixie-slim
IMAGE_ARGS="uvx opensearch-mcp-server-py@0.12.0"
REQUIRED="OPENSEARCH_URL"
skeleton() {
  cat <<'SKEL'
# OpenSearch — the cluster's URL, and a read-only user if it has security on
# (https://docs.opensearch.org/latest/security/access-control/users-roles/).
OPENSEARCH_URL=

# Basic auth ...
#OPENSEARCH_USERNAME=
#OPENSEARCH_PASSWORD=
# ... or none (a cluster without the security plugin)
#OPENSEARCH_NO_AUTH=true
# ... or AWS IAM (Amazon OpenSearch Service): static keys or a profile
#AWS_REGION=
#AWS_ACCESS_KEY_ID=
#AWS_SECRET_ACCESS_KEY=

#OPENSEARCH_SSL_VERIFY=true
SKEL
}
