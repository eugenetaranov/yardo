# Figma — design data (layouts, styles, components) for building from a
# design (Framelink's Figma-Context-MCP, run with npx in Node's image;
# Figma's own server takes only OAuth or the desktop app).
# https://github.com/GLips/Figma-Context-MCP
DESCRIPTION="Figma: read files, frames, layouts and styles to build from a design"
KIND=docker
IMAGE=node:24.19.0-slim
IMAGE_ARGS="npx -y figma-developer-mcp@0.13.2 --stdio"
REQUIRED="FIGMA_API_KEY"
skeleton() {
  cat <<'SKEL'
# Figma — a personal access token (read-only scopes are enough):
# Figma → Settings → Security, see https://www.figma.com/developers/api#access-tokens
FIGMA_API_KEY=
SKEL
}
