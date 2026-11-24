# Playwright — drive a headless Chromium: open pages, click, fill, screenshot (Microsoft; Docker).
# https://github.com/microsoft/playwright-mcp
DESCRIPTION="Playwright: a headless browser to open pages, click, fill forms and take screenshots"
KIND=docker
IMAGE=mcr.microsoft.com/playwright/mcp:latest
REQUIRED=""
DOCKER_ARGS="--init"
skeleton() {
  echo "# Playwright needs no credentials."
}
