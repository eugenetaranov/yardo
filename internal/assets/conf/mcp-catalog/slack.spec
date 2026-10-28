# Slack — channels, threads, search, users (korotovsky/slack-mcp-server;
# Docker). Slack's own server needs an OAuth app approved by an admin.
# https://github.com/korotovsky/slack-mcp-server
DESCRIPTION="Slack: read channels, threads and DMs, search messages (posting off)"
KIND=docker
IMAGE=ghcr.io/korotovsky/slack-mcp-server:v1.3.0
IMAGE_ARGS="--transport stdio"
REQUIRED="SLACK_MCP_XOXP_TOKEN"
skeleton() {
  cat <<'SKEL'
# Slack — a user token (xoxp-…): create an app at https://api.slack.com/apps,
# add user token scopes (channels:history, channels:read, groups:history,
# im:history, search:read, users:read, …) under OAuth & Permissions, install
# it, and copy the User OAuth Token. Its scopes decide what the agent reads.
SLACK_MCP_XOXP_TOKEN=

# Optional: let the agent post (true, or a comma-separated list of channel IDs)
#SLACK_MCP_ADD_MESSAGE_TOOL=
SKEL
}
