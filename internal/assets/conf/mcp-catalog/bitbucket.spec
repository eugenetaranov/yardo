# Bitbucket Cloud — repositories, pull requests, comments and diffs
# (aashari/mcp-server-atlassian-bitbucket, run with npx in Node's image;
# mcp-atlassian, behind jira and confluence, has no Bitbucket).
# https://github.com/aashari/mcp-server-atlassian-bitbucket
DESCRIPTION="Bitbucket: repositories, pull requests, comments and diffs"
KIND=docker
IMAGE=node:lts-slim
IMAGE_ARGS="npx -y @aashari/mcp-server-atlassian-bitbucket"
REQUIRED="ATLASSIAN_USER_EMAIL ATLASSIAN_API_TOKEN"
skeleton() {
  cat <<'SKEL'
# Bitbucket Cloud — an API token with scopes (Jira's token won't do):
# https://id.atlassian.com/manage-profile/security/api-tokens
#   Create API token with scopes → Bitbucket, and tick each of these
#   (none implies another: without read:pullrequest no pull request
#   shows, 403, though the repository does):
#     read:user:bitbucket          who you are ("my pull requests")
#     read:workspace:bitbucket     your workspaces
#     read:repository:bitbucket    repositories, code, commits, diffs
#     read:pullrequest:bitbucket   pull requests, and commenting on them
#   To open, approve or merge pull requests too:
#     write:pullrequest:bitbucket
#   Missed one: make a new token with all of them, then
#   sudo yardo mcp edit bitbucket.
ATLASSIAN_USER_EMAIL=
ATLASSIAN_API_TOKEN=

# Optional: the workspace to use when none is named
#BITBUCKET_DEFAULT_WORKSPACE=
SKEL
}
