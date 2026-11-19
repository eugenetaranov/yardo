# Confluence alone via mcp-atlassian (Docker); jira.spec covers Jira with
# Confluence optional.
# https://github.com/sooperset/mcp-atlassian
DESCRIPTION="Confluence: search, read and update pages"
KIND=docker
IMAGE=ghcr.io/sooperset/mcp-atlassian:0.23.1
REQUIRED="CONFLUENCE_URL CONFLUENCE_USERNAME CONFLUENCE_API_TOKEN"
skeleton() {
  cat <<'SKEL'
# Confluence — create an API token at https://id.atlassian.com/manage-profile/security/api-tokens
# Cloud: CONFLUENCE_URL=https://<your-site>.atlassian.net/wiki
CONFLUENCE_URL=
CONFLUENCE_USERNAME=
CONFLUENCE_API_TOKEN=

# Optional: expose read-only tools only
#READ_ONLY_MODE=true
SKEL
}
