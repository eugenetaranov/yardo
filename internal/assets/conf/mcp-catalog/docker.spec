# Docker — containers, images, networks and volumes on this machine
# (ckreiling/mcp-server-docker, run with uvx in uv's image).
# https://github.com/ckreiling/mcp-server-docker
#
# DANGER: it gets the Docker socket, which is root on this machine: an agent
# using it can run a privileged container or mount /. Enable it only for
# agents you'd give sudo.
DESCRIPTION="Docker: run, stop and inspect containers, pull and build images (root on this machine)"
KIND=docker
IMAGE=ghcr.io/astral-sh/uv:0.13.6-python3.13-trixie-slim
IMAGE_ARGS="uvx mcp-server-docker@0.3.0"
DOCKER_ARGS="-v /var/run/docker.sock:/var/run/docker.sock"
REQUIRED=""
skeleton() {
  cat <<'SKEL'
# Docker needs no credentials: it uses this machine's Docker socket, which is
# root on this machine. Enable it only for agents you'd give sudo.
SKEL
}
