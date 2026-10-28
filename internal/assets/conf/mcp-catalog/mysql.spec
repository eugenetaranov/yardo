# MySQL — schema and queries, read-only unless allowed below
# (benborla/mcp-server-mysql, run with npx in Node's image).
# https://github.com/benborla/mcp-server-mysql
DESCRIPTION="MySQL: schema and read-only queries"
KIND=docker
IMAGE=node:24.19.0-slim
IMAGE_ARGS="npx -y @benborla29/mcp-server-mysql@2.0.9"
REQUIRED="MYSQL_HOST MYSQL_USER MYSQL_PASS"
# Host networking, so a database on this VM is reachable at 127.0.0.1.
DOCKER_ARGS="--network host"
skeleton() {
  cat <<'SKEL'
# MySQL — a user with SELECT only is best
# (https://dev.mysql.com/doc/refman/8.4/en/create-user.html).
MYSQL_HOST=
MYSQL_PORT=3306
MYSQL_USER=
MYSQL_PASS=

# Optional: one database (empty: every database the user can see)
#MYSQL_DB=

# Queries are read-only; allow writes only if you mean it
#ALLOW_INSERT_OPERATION=true
#ALLOW_UPDATE_OPERATION=true
#ALLOW_DELETE_OPERATION=true
SKEL
}
