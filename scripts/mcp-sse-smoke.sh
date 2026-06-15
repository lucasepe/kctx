#!/usr/bin/env bash
set -euo pipefail

URL="${1:-${MCP_URL:-http://localhost:8888/mcp/sse}}"
NAMESPACE="${2:-${NAMESPACE:-boutique}}"
SERVICE="${3:-${SERVICE:-frontend}}"
POD="${POD:-}"
DUMP_NAMESPACE="${DUMP_NAMESPACE:-true}"

workdir="$(mktemp -d)"
events="${workdir}/events.log"
curl_pid=""

cleanup() {
  if [ -n "${curl_pid}" ]; then
    kill "${curl_pid}" 2>/dev/null || true
  fi
  rm -rf "${workdir}"
}
trap cleanup EXIT

curl -fsS -N "${URL}" >"${events}" &
curl_pid="$!"

endpoint=""
for _ in $(seq 1 100); do
  endpoint="$(sed -n 's#^data: \(/mcp/message?sessionId=.*\)$#\1#p' "${events}" | head -n1)"
  if [ -n "${endpoint}" ]; then
    break
  fi
  sleep 0.1
done

if [ -z "${endpoint}" ]; then
  echo "failed to read MCP SSE endpoint from ${URL}" >&2
  cat "${events}" >&2 || true
  exit 1
fi

base="${URL%/mcp/sse}"
message_url="${base}${endpoint}"

post() {
  local label="$1"
  local payload="$2"
  echo ">>> ${label}"
  curl -fsS \
    -X POST "${message_url}" \
    -H 'Content-Type: application/json' \
    -d "${payload}" \
    -o /dev/null
}

post initialize '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"kctx-smoke","version":"dev"}}}'
post tools/list '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
post get_namespace_health "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"get_namespace_health\",\"arguments\":{\"namespace\":\"${NAMESPACE}\"}}}"
post trace_service "{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"tools/call\",\"params\":{\"name\":\"trace_service\",\"arguments\":{\"namespace\":\"${NAMESPACE}\",\"name\":\"${SERVICE}\"}}}"

next_id=5
if [ -n "${POD}" ]; then
  post get_pod_graph "{\"jsonrpc\":\"2.0\",\"id\":${next_id},\"method\":\"tools/call\",\"params\":{\"name\":\"get_pod_graph\",\"arguments\":{\"namespace\":\"${NAMESPACE}\",\"name\":\"${POD}\"}}}"
  next_id=$((next_id + 1))
fi

if [ "${DUMP_NAMESPACE}" = "true" ]; then
  post dump_namespace "{\"jsonrpc\":\"2.0\",\"id\":${next_id},\"method\":\"tools/call\",\"params\":{\"name\":\"dump_namespace\",\"arguments\":{\"namespace\":\"${NAMESPACE}\"}}}"
fi

sleep 2

echo
echo "SSE responses:"
grep '^data: {' "${events}" | sed 's/^data: //'

for expected in get_namespace_health explain_resource trace_service get_pod_graph dump_namespace; do
  if ! grep -q "\"name\":\"${expected}\"" "${events}"; then
    echo "warning: tools/list output did not show ${expected}" >&2
  fi
done

if grep -q '"isError":true' "${events}"; then
  echo "one or more MCP tool calls returned isError=true" >&2
  exit 1
fi

echo
echo "smoke test completed"
