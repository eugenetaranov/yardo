# OpenSearch — search indices, mappings, cluster health (the OpenSearch
# project's opensearch-mcp-server-py, run with uvx in uv's image; Docker).
# https://github.com/opensearch-project/opensearch-mcp-server-py
DESCRIPTION="OpenSearch: search indices, read mappings and cluster health (read-only)"
KIND=docker
IMAGE=ghcr.io/astral-sh/uv:0.13.9-python3.13-trixie-slim
REQUIRED="OPENSEARCH_URL"
# uvx fetches its dependencies (scipy among them) at start: more than the
# 256 MB scratch /tmp holds, so its root stays writable.
LIMITS="writable"
# Read-only unless the env file says OPENSEARCH_SETTINGS_ALLOW_WRITE=true
# (the server's own default is to write; an env file made before this
# line has no such setting, so it's set here, on the command).
image_args() {
  w=$(envget OPENSEARCH_SETTINGS_ALLOW_WRITE "$ENV_FILE")
  [ "$w" = true ] || w=false
  echo env
  echo "OPENSEARCH_SETTINGS_ALLOW_WRITE=$w"
  echo uvx
  echo opensearch-mcp-server-py@0.12.0
}
skeleton() {
  cat <<'SKEL'
# OpenSearch — the cluster's URL, and a read-only user if it has security on
# (https://docs.opensearch.org/latest/security/access-control/users-roles/).
# With its port: https://search.example.com:9200 (none means 443, or 80
# for http).
OPENSEARCH_URL=

# Agents only read; true lets them write (index, delete, change settings)
OPENSEARCH_SETTINGS_ALLOW_WRITE=false

# Basic auth ...
#OPENSEARCH_USERNAME=
#OPENSEARCH_PASSWORD=
# ... or none (a cluster without the security plugin)
#OPENSEARCH_NO_AUTH=true
# ... or AWS IAM (Amazon OpenSearch Service): keys (the container has no
# ~/.aws, so no profile), with a session token for temporary ones
#AWS_REGION=
#AWS_ACCESS_KEY_ID=
#AWS_SECRET_ACCESS_KEY=
#AWS_SESSION_TOKEN=
# ... and for OpenSearch Serverless
#AWS_OPENSEARCH_SERVERLESS=true

#OPENSEARCH_SSL_VERIFY=true
SKEL
}
