# Grafana — dashboards, datasources, Prometheus and Loki queries, alerts,
# incidents (Grafana Labs' mcp-grafana; Docker).
# https://github.com/grafana/mcp-grafana
DESCRIPTION="Grafana: dashboards, Prometheus and Loki queries, alerts and incidents (read-only)"
KIND=docker
IMAGE=grafana/mcp-grafana:2.3.0
# Read-only by default: drop --disable-write here to let agents edit
# dashboards, alert rules and incidents.
# No usage reports to Grafana Labs (they're on by default).
IMAGE_ARGS="-t stdio --disable-write --usage-stats=disabled"
REQUIRED="GRAFANA_URL GRAFANA_SERVICE_ACCOUNT_TOKEN"
skeleton() {
  cat <<'SKEL'
# Grafana — a service account token (Viewer is enough read-only):
# <your Grafana>/org/serviceaccounts, see https://grafana.com/docs/grafana/latest/administration/service-accounts/
GRAFANA_URL=
GRAFANA_SERVICE_ACCOUNT_TOKEN=

# Optional: a Grafana org other than the token's default
#GRAFANA_ORG_ID=
SKEL
}
