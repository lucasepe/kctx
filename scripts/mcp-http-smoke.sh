#!/usr/bin/env bash
set -euo pipefail

URL="${1:-${MCP_URL:-http://localhost:8888/mcp}}"
NAMESPACE="${2:-${NAMESPACE:-boutique}}"
SERVICE="${3:-${SERVICE:-frontend}}"
POD="${POD:-}"
DUMP_NAMESPACE="${DUMP_NAMESPACE:-true}"
PROTOCOL_VERSION="${MCP_PROTOCOL_VERSION:-2025-06-18}"

workdir="$(mktemp -d)"
headers="${workdir}/headers.txt"
body="${workdir}/body.json"

cleanup() {
  rm -rf "${workdir}"
}
trap cleanup EXIT

post() {
  local label="$1"
  local payload="$2"
  shift 2

  echo ">>> ${label}"
  curl -fsS "${URL}" \
    -D "${headers}" \
    -o "${body}" \
    -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -H "MCP-Protocol-Version: ${PROTOCOL_VERSION}" \
    "$@" \
    -d "${payload}"
  cat "${body}"
  echo
}

post initialize '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"kctx-smoke","version":"dev"}}}'

session_id="$(awk -F': ' 'tolower($1)=="mcp-session-id" {print $2}' "${headers}" | tr -d '\r' | tail -n1)"
if [ -z "${session_id}" ]; then
  echo "initialize did not return Mcp-Session-Id" >&2
  exit 1
fi

post tools/list '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  -H "Mcp-Session-Id: ${session_id}"
post get_namespace_health "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"get_namespace_health\",\"arguments\":{\"namespace\":\"${NAMESPACE}\"}}}" \
  -H "Mcp-Session-Id: ${session_id}"
post trace_service "{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"tools/call\",\"params\":{\"name\":\"trace_service\",\"arguments\":{\"namespace\":\"${NAMESPACE}\",\"name\":\"${SERVICE}\"}}}" \
  -H "Mcp-Session-Id: ${session_id}"

next_id=5
if [ -n "${POD}" ]; then
  post get_pod_graph "{\"jsonrpc\":\"2.0\",\"id\":${next_id},\"method\":\"tools/call\",\"params\":{\"name\":\"get_pod_graph\",\"arguments\":{\"namespace\":\"${NAMESPACE}\",\"name\":\"${POD}\"}}}" \
    -H "Mcp-Session-Id: ${session_id}"
  next_id=$((next_id + 1))
fi

if [ "${DUMP_NAMESPACE}" = "true" ]; then
  post dump_namespace "{\"jsonrpc\":\"2.0\",\"id\":${next_id},\"method\":\"tools/call\",\"params\":{\"name\":\"dump_namespace\",\"arguments\":{\"namespace\":\"${NAMESPACE}\"}}}" \
    -H "Mcp-Session-Id: ${session_id}"
fi

curl -fsS -X DELETE "${URL}" -H "Mcp-Session-Id: ${session_id}" -o /dev/null

echo
echo "smoke test completed"
