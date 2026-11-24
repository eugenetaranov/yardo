# Jira (and optionally Confluence) via mcp-atlassian (Docker).
# https://github.com/sooperset/mcp-atlassian
DESCRIPTION="Jira and Confluence: search, read and update issues and pages"
KIND=docker
IMAGE=ghcr.io/sooperset/mcp-atlassian:latest
REQUIRED="JIRA_URL JIRA_USERNAME JIRA_API_TOKEN"
skeleton() {
  cat <<'SKEL'
# Jira — create an API token at https://id.atlassian.com/manage-profile/security/api-tokens
# Or leave these empty: after jiractl configure, yardo fills them in on its
# next run.
# Server or Data Center: a personal access token instead
# (JIRA_PERSONAL_TOKEN=…, JIRA_USERNAME and JIRA_API_TOKEN left empty).
JIRA_URL=
JIRA_USERNAME=
JIRA_API_TOKEN=
#JIRA_PERSONAL_TOKEN=

# Optional: Confluence through the same server (Cloud: https://<site>.atlassian.net/wiki)
#CONFLUENCE_URL=
#CONFLUENCE_USERNAME=
#CONFLUENCE_API_TOKEN=

# Optional: expose read-only tools only
#READ_ONLY_MODE=true

# Optional: the tool sets offered (default: the usual ones; all for every tool)
#TOOLSETS=default
SKEL
}
