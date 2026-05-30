#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
HOST_PORT="${HOST_PORT:-8888}"
NODE_PORT="${NODE_PORT:-30088}"

log() {
  printf '\n==> %s\n' "$*" >&2
}

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

require kind
require kubectl

if kind get clusters | grep -Fxq "${CLUSTER_NAME}"; then
  log "reusing existing kind cluster '${CLUSTER_NAME}'"
else
  log "creating kind cluster '${CLUSTER_NAME}'"
  kind create cluster --name "${CLUSTER_NAME}" --config - <<YAML
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: ${NODE_PORT}
        hostPort: ${HOST_PORT}
        protocol: TCP
YAML
fi

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

cat <<EOF

Kind cluster is ready.

Cluster:   ${CLUSTER_NAME}
Context:   kind-${CLUSTER_NAME}
Port map:  localhost:${HOST_PORT} -> NodePort ${NODE_PORT}

Next:
  scripts/install-online-boutique.sh

If you deploy kctx serve inside the cluster, expose it with a Service using:
  type: NodePort
  nodePort: ${NODE_PORT}
  targetPort: 8080

EOF
