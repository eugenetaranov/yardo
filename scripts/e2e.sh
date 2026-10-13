#!/bin/sh
# End-to-end run of yardo on a throwaway Linux machine over SSH: copy the
# build for its architecture, apply a profile with every catalog item (or
# ITEMS="cli tmux …"; MCP="github …" picks MCP servers and wires agents;
# TZ_SET=Europe/Berlin sets the timezone), verify, and re-run.
#   HOST=ubuntu@10.0.0.5 ./scripts/e2e.sh      (passwordless sudo there)
set -eu
HOST=${HOST:?HOST=user@host of a throwaway machine}
ITEMS=${ITEMS:-$(go run ./cmd/yardo --list | cut -f1 | tr '\n' ' ')}
profile=$(mktemp "${TMPDIR:-/tmp}/yardo-e2e.XXXXXX")
trap 'rm -f "$profile"' EXIT
{
  printf 'picks = ['
  sep=
  for id in $ITEMS; do printf '%s"%s"' "$sep" "$id"; sep=', '; done
  printf ']\n'
  if [ -n "${TZ_SET:-}" ]; then printf '[system]\ntz = "%s"\n' "$TZ_SET"; fi
  if [ -n "${MCP:-}" ]; then
    printf '[mcp]\nwire_agents = true\nservers = ['
    sep=
    for n in $MCP; do printf '%s"%s"' "$sep" "$n"; sep=', '; done
    printf ']\n'
  fi
} > "$profile"

arch=$(ssh "$HOST" uname -m)
case $arch in x86_64) a=amd64 ;; aarch64) a=arm64 ;; *) echo "no build for $arch" >&2; exit 1 ;; esac
scp -q "bin/yardo-linux-$a" "$HOST:/tmp/yardo"
scp -q "$profile" "$HOST:/tmp/yardo-e2e.toml"
ssh "$HOST" 'sudo install -m 0755 /tmp/yardo /usr/local/bin/yardo'
echo "== apply"
ssh "$HOST" 'sudo yardo --profile /tmp/yardo-e2e.toml --yes'
echo "== verify"
ssh "$HOST" 'sudo yardo --verify'
echo "== re-run (should change nothing)"
ssh "$HOST" 'sudo yardo --yes' | tee /dev/stderr | grep -q '^✓ ' && { echo "the re-run changed something" >&2; exit 1; } || true
echo "e2e ok"
