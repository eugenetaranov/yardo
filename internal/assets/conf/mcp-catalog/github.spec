# GitHub — repos, issues, pull requests, Actions (GitHub's official server; Docker).
# https://github.com/github/github-mcp-server
DESCRIPTION="GitHub: repositories, issues, pull requests and Actions"
KIND=docker
IMAGE=ghcr.io/github/github-mcp-server:v2.0.2
REQUIRED="GITHUB_PERSONAL_ACCESS_TOKEN"
skeleton() {
  cat <<'SKEL'
# A fine-grained token reaches only the repositories you pick. Make one at
# https://github.com/settings/personal-access-tokens/new
#   Resource owner: you, or the organization whose repositories these are
#     (it may have to approve the token)
#   Repository access: the repositories the agents work on, or all of them
#   Repository permissions:
#     Contents: Read and write        code, branches, commits
#     Pull requests: Read and write   open, review and comment on PRs
#     Issues: Read and write          read and file issues
#     Actions: Read-only              workflow runs and their logs
#     Workflows: Read and write       only to change .github/workflows
#     (Metadata: Read-only comes with any of them)
# A classic token (https://github.com/settings/tokens) reaches every repository
# you can, in any organization: scopes repo, plus workflow and read:org if needed.
GITHUB_PERSONAL_ACCESS_TOKEN=

# GitHub Enterprise Server or ghe.com only: GITHUB_HOST=https://github.example.com

# Optional: limit the tools (comma-separated toolsets, e.g. repos,issues,pull_requests)
#GITHUB_TOOLSETS=

# Optional: read-only tools only
#GITHUB_READ_ONLY=1
SKEL
}
