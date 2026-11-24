# Slack — channels, threads, search, users (korotovsky/slack-mcp-server;
# Docker). Slack's own server needs an OAuth app approved by an admin.
# https://github.com/korotovsky/slack-mcp-server
DESCRIPTION="Slack: read channels, threads and DMs, search messages (posting off)"
KIND=docker
IMAGE=ghcr.io/korotovsky/slack-mcp-server:latest
IMAGE_ARGS="--transport stdio"
REQUIRED="SLACK_MCP_XOXP_TOKEN"
# Its read tools only, unless SLACK_MCP_ENABLED_TOOLS lists others: with
# no list the server also offers joining and leaving channels, marking
# them read, and editing user groups and saved items. An env file of
# before that lets the agent post (SLACK_MCP_ADD_MESSAGE_TOOL) keeps
# posting.
image_args() {
  tools=$(envget SLACK_MCP_ENABLED_TOOLS "$ENV_FILE")
  if [ -z "$tools" ]; then
    tools=conversations_history,conversations_replies,conversations_search_messages,conversations_unreads,channels_list,channels_me,users_search,usergroups_list,usergroups_me,saved_list
    [ -z "$(envget SLACK_MCP_ADD_MESSAGE_TOOL "$ENV_FILE")" ] || tools=$tools,conversations_add_message
  fi
  echo --enabled-tools
  echo "$tools"
}
skeleton() {
  cat <<'SKEL'
# Slack — a user token (xoxp-…): create an app at https://api.slack.com/apps,
# add user token scopes (channels:history, channels:read, groups:history,
# im:history, search:read, users:read, …) under OAuth & Permissions, install
# it, and copy the User OAuth Token. Its scopes decide what the agent reads.
SLACK_MCP_XOXP_TOKEN=

# The tools the agent gets: reading only. To let it post, add
# conversations_add_message (to every channel the token can post in).
SLACK_MCP_ENABLED_TOOLS=conversations_history,conversations_replies,conversations_search_messages,conversations_unreads,channels_list,channels_me,users_search,usergroups_list,usergroups_me,saved_list
SKEL
}
