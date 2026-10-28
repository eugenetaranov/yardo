# Supabase — projects, database schema and queries, logs, docs (Supabase's
# hosted MCP server, with a personal access token).
# https://github.com/supabase/mcp
DESCRIPTION="Supabase: projects, tables, SQL queries (read-only), logs and docs"
KIND=http
# read_only: queries run as a read-only Postgres user. To limit it to one
# project, use your own copy of this spec with &project_ref=<ref>.
URL="https://mcp.supabase.com/mcp?read_only=true"
AUTH=bearer
AUTH_TOKEN_VAR=SUPABASE_ACCESS_TOKEN
REQUIRED="SUPABASE_ACCESS_TOKEN"
skeleton() {
  cat <<'SKEL'
# Supabase — a personal access token: https://supabase.com/dashboard/account/tokens
SUPABASE_ACCESS_TOKEN=
SKEL
}
