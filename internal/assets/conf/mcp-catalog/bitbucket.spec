# Bitbucket Cloud — repositories, pull requests, comments and diffs
# (aashari/mcp-server-atlassian-bitbucket, run with npx in Node's image;
# mcp-atlassian, behind jira and confluence, has no Bitbucket).
# https://github.com/aashari/mcp-server-atlassian-bitbucket
DESCRIPTION="Bitbucket: repositories, pull requests, comments and diffs"
KIND=docker
IMAGE=node:24.19.0-slim
IMAGE_ARGS="npx -y @aashari/mcp-server-atlassian-bitbucket@3.1.0"
REQUIRED="ATLASSIAN_USER_EMAIL ATLASSIAN_API_TOKEN"
skeleton() {
  cat <<'SKEL'
# Bitbucket Cloud — an API token with scopes (Jira's token won't do):
# https://id.atlassian.com/manage-profile/security/api-tokens
#   Create API token with scopes → Bitbucket → repository, workspace
#   (and pullrequest to comment on or open pull requests)
ATLASSIAN_USER_EMAIL=
ATLASSIAN_API_TOKEN=

# Optional: the workspace to use when none is named
#BITBUCKET_DEFAULT_WORKSPACE=
SKEL
}
