# AWS — the AWS MCP Server, AWS's managed one (it replaces the AWS API MCP
# server, aws-api), reached through the MCP Proxy for AWS, which signs each
# request with your credentials (SigV4); Docker.
# https://docs.aws.amazon.com/agent-toolkit/latest/userguide/getting-started-aws-mcp-server.html
# https://github.com/aws/mcp-proxy-for-aws
DESCRIPTION="AWS: call AWS APIs, search the docs and follow AWS's guides (AWS's managed MCP server)"
KIND=docker
IMAGE=public.ecr.aws/mcp-proxy-for-aws/mcp-proxy-for-aws:1.7.0
REQUIRED="AWS_REGION"
# Files in ~/.mcp/aws/ are mounted at /mcp: a config/credentials pair
# dropped there is picked up via AWS_PROFILE.
DOCKER_ARGS="-e AWS_CONFIG_FILE=/mcp/config -e AWS_SHARED_CREDENTIALS_FILE=/mcp/credentials"
# The server's endpoint: in AWS_REGION when it has one there, else
# AWS_MCP_ENDPOINT_REGION's or us-east-1 (requests are signed for it);
# AWS_REGION is the default region of what the agent does.
image_args() {
  region=$(envget AWS_REGION "$ENV_FILE")
  endpoint=$(envget AWS_MCP_ENDPOINT_REGION "$ENV_FILE")
  if [ -z "$endpoint" ]; then
    case $region in
      us-east-1|us-west-2|ap-southeast-1|ap-southeast-2|ap-northeast-1|eu-central-1|eu-west-1|eu-west-2) endpoint=$region ;;
      *) endpoint=us-east-1 ;;
    esac
  fi
  echo "https://aws-mcp.$endpoint.api.aws/mcp"
  echo --region
  echo "$endpoint"
  echo --metadata
  echo "AWS_REGION=$region"
  [ "$(envget READ_OPERATIONS_ONLY "$ENV_FILE")" = false ] || echo --read-only
}
skeleton() {
  cat <<'SKEL'
# AWS — AWS's managed MCP server. What the agent calls is checked against
# these credentials' own permissions: use a least-privilege (ideally
# read-only, e.g. the ReadOnlyAccess policy) IAM user or role:
# https://console.aws.amazon.com/iam/home#/users
# The region your agent works in by default:
AWS_REGION=

# Either static keys here ...
#AWS_ACCESS_KEY_ID=
#AWS_SECRET_ACCESS_KEY=
# ... or put `config` and `credentials` files next to this env file and pick
# a profile:
#AWS_PROFILE=

# Only read operations: write tools are hidden (false shows them).
READ_OPERATIONS_ONLY=true

# The server's own region, when AWS_REGION isn't one it runs in
# (us-east-1, us-west-2, eu-west-1, eu-west-2, eu-central-1,
# ap-southeast-1, ap-southeast-2, ap-northeast-1; else us-east-1):
#AWS_MCP_ENDPOINT_REGION=
SKEL
}
