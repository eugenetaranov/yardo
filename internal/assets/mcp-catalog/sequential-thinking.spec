# sequential-thinking — structured step-by-step reasoning (the MCP
# project's reference server, run with npx in Node's image; Docker; no
# credentials). Its own image, mcp/sequentialthinking, was never tagged.
# https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking
DESCRIPTION="Structured, step-by-step problem solving"
KIND=docker
IMAGE=node:lts-slim
IMAGE_ARGS="npx -y @modelcontextprotocol/server-sequential-thinking"
REQUIRED=""
skeleton() {
  echo "# sequential-thinking needs no credentials."
}
