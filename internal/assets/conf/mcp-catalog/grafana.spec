# Grafana — dashboards, datasources, Prometheus and Loki queries, alerts,
# incidents (Grafana Labs' mcp-grafana; Docker).
# https://github.com/grafana/mcp-grafana
DESCRIPTION="Grafana: dashboards, Prometheus and Loki queries, alerts and incidents"
KIND=docker
IMAGE=grafana/mcp-grafana:2.1.1
# Read-only by default: drop --disable-write here to let agents edit
# dashboards, alert rules and incidents.
IMAGE_ARGS="-t stdio --disable-write"
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
