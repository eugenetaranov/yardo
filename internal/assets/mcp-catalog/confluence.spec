# Confluence alone via mcp-atlassian (Docker); jira.spec covers Jira with
# Confluence optional.
# https://github.com/sooperset/mcp-atlassian
DESCRIPTION="Confluence: search, read and update pages"
KIND=docker
IMAGE=ghcr.io/sooperset/mcp-atlassian:latest
REQUIRED="CONFLUENCE_URL CONFLUENCE_USERNAME CONFLUENCE_API_TOKEN"
skeleton() {
  cat <<'SKEL'
# Confluence — create an API token at https://id.atlassian.com/manage-profile/security/api-tokens
# Cloud: CONFLUENCE_URL=https://<your-site>.atlassian.net/wiki
# Or leave these empty: after jiractl configure, yardo fills them in on its
# next run (the URL too, on Atlassian Cloud).
CONFLUENCE_URL=
CONFLUENCE_USERNAME=
CONFLUENCE_API_TOKEN=

# Optional: expose read-only tools only
#READ_ONLY_MODE=true
SKEL
}
