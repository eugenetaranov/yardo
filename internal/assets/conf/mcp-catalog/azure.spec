# Azure — resources across Azure services: storage, Cosmos DB, AKS, Key
# Vault, monitor queries, … (Microsoft's Azure MCP Server; Docker).
# https://github.com/microsoft/mcp/tree/main/servers/Azure.Mcp.Server
DESCRIPTION="Azure: list and inspect resources, query logs and metrics (read-only)"
KIND=docker
IMAGE=mcr.microsoft.com/azure-sdk/azure-mcp:3.0.0
# The image starts the server; drop --read-only to let agents change things.
IMAGE_ARGS="--read-only"
REQUIRED="AZURE_TENANT_ID AZURE_CLIENT_ID AZURE_CLIENT_SECRET"
skeleton() {
  cat <<'SKEL'
# Azure — a service principal with the Reader role:
#   az ad sp create-for-rbac --name yardo-mcp --role Reader --scopes /subscriptions/<id>
# (or https://portal.azure.com → App registrations). tenant, appId, password:
AZURE_TENANT_ID=
AZURE_CLIENT_ID=
AZURE_CLIENT_SECRET=

# Optional: the default subscription
#AZURE_SUBSCRIPTION_ID=

AZURE_MCP_COLLECT_TELEMETRY=false
SKEL
}
