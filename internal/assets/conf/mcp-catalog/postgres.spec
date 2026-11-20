# Postgres — schema, queries, explain plans and health checks (Postgres MCP Pro; Docker).
# https://github.com/crystaldba/postgres-mcp
DESCRIPTION="Postgres: schema, read-only queries, explain plans and index advice"
KIND=docker
IMAGE=crystaldba/postgres-mcp:0.3.0
REQUIRED="DATABASE_URI"
# Host networking, so a database on this VM is reachable at 127.0.0.1.
DOCKER_ARGS="--network host"
# Read-only transactions; explain plans, health and index advice stay.
# To let agents write, change it to --access-mode=unrestricted.
IMAGE_ARGS="--access-mode=restricted"
skeleton() {
  cat <<'SKEL'
# Postgres — a connection URI, postgresql://user:password@host:5432/dbname
# (https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING-URIS).
# For a database on this VM use 127.0.0.1, not
# "localhost" (the image rewrites localhost to the Docker bridge).
# Use a read-only role where you can. Agents only read (restricted
# mode): to let them write, set --access-mode=unrestricted in
# /usr/local/share/mcp-catalog/postgres.spec's IMAGE_ARGS, or your own copy.
DATABASE_URI=
SKEL
}
