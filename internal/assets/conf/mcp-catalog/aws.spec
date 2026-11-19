# AWS — the AWS MCP Server, AWS's managed one (it replaces the AWS API MCP
# server, aws-api), reached through the MCP Proxy for AWS, which signs each
# request with your credentials (SigV4); Docker.
# https://docs.aws.amazon.com/agent-toolkit/latest/userguide/getting-started-aws-mcp-server.html
# https://github.com/aws/mcp-proxy-for-aws
DESCRIPTION="AWS: call AWS APIs, search the docs and follow AWS's guides (AWS's managed MCP server)"
KIND=docker
IMAGE=public.ecr.aws/mcp-proxy-for-aws/mcp-proxy-for-aws:1.7.0
REQUIRED="AWS_REGION"
# The image's healthcheck looks for a process named aws.* holding a socket;
# the proxy, run over stdio, is mcp-proxy-for-aws-cli: it would always say
# unhealthy, working or not.
DOCKER_ARGS="--no-healthcheck"
# Your ~/.aws, read-only: the AWS CLI's profiles and its SSO logins (aws
# sso login here renews them for the server too, as it runs).
MOUNT_AWS=1
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
  [ "$(envget READ_OPERATIONS_ONLY "$ENV_FILE")" != true ] || echo --read-only
  profile=$(envget AWS_PROFILE "$ENV_FILE")
  if [ -n "$profile" ]; then
    echo --profile
    echo "$profile"
  fi
}
skeleton() {
  cat <<'SKEL'
# AWS — AWS's managed MCP server. What the agent calls is checked against
# the credentials' own permissions, nothing else: use a read-only role or
# permission set (ReadOnlyAccess, ViewOnlyAccess), not an admin one.
# The region your agent works in by default:
AWS_REGION=

# A profile of the AWS CLI's (~/.aws/config: an SSO login, a role, keys);
# commented out: its default. When an SSO login runs out: aws sso login
# --profile <it>, here; the server takes the new one as it runs. To make
# one: aws configure sso
# https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sso.html
#AWS_PROFILE=

# Or keys of their own, here (a dedicated IAM user's, read-only), which
# win over the profile:
#AWS_ACCESS_KEY_ID=
#AWS_SECRET_ACCESS_KEY=

# false: the agent calls AWS APIs as the profile's role, reads and writes
# alike: what the role allows is the limit (a read-only role,
# ReadOnlyAccess or ViewOnlyAccess, keeps it to reading).
# true: AWS's docs, guides and regions only, not your account: the tool
# that calls AWS APIs (run_script) is hidden, as it can write too.
READ_OPERATIONS_ONLY=false

# The server's own region, when AWS_REGION isn't one it runs in
# (us-east-1, us-west-2, eu-west-1, eu-west-2, eu-central-1,
# ap-southeast-1, ap-southeast-2, ap-northeast-1; else us-east-1):
#AWS_MCP_ENDPOINT_REGION=
SKEL
}
