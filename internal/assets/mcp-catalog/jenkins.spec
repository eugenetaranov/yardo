# Jenkins — via the Jenkins "MCP Server" plugin, which serves MCP over HTTP
# from Jenkins itself (install the plugin on your controller first).
# https://plugins.jenkins.io/mcp-server/
DESCRIPTION="Jenkins: jobs, builds and logs"
KIND=http
URL_VAR=JENKINS_URL
URL_SUFFIX=/mcp-server/mcp
AUTH=basic
AUTH_USER_VAR=JENKINS_USER
AUTH_TOKEN_VAR=JENKINS_API_TOKEN
REQUIRED="JENKINS_URL JENKINS_USER JENKINS_API_TOKEN"
skeleton() {
  cat <<'SKEL'
# Jenkins — the MCP Server plugin 0.178 or later (two security advisories
# before it). An API token: <your Jenkins>/me/security → API Token
# (https://www.jenkins.io/doc/book/using/remote-access-api/)
JENKINS_URL=
JENKINS_USER=
JENKINS_API_TOKEN=
SKEL
}
