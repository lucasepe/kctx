#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
HOST_PORT="${HOST_PORT:-8888}"
NODE_PORT="${NODE_PORT:-30088}"
ARGOCD_HOST_PORT="${ARGOCD_HOST_PORT:-8443}"
ARGOCD_NODE_PORT="${ARGOCD_NODE_PORT:-30443}"

# Print a visible step message to stderr.
log() {
  printf '\n==> %s\n' "$*" >&2
}

# Print an error message and stop the script.
fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

# Ensure that a required command is available before continuing.
require() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

require kind
require kubectl

CLUSTER_CREATED="false"

if kind get clusters | grep -Fxq "${CLUSTER_NAME}"; then
  log "reusing existing kind cluster '${CLUSTER_NAME}'"
else
  log "creating kind cluster '${CLUSTER_NAME}'"
  CLUSTER_CREATED="true"
  kind create cluster --name "${CLUSTER_NAME}" --config - <<YAML
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    # Map local host ports into the kind node for NodePort services.
    extraPortMappings:
      - containerPort: ${NODE_PORT}
        hostPort: ${HOST_PORT}
        protocol: TCP
      - containerPort: ${ARGOCD_NODE_PORT}
        hostPort: ${ARGOCD_HOST_PORT}
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
ArgoCD:    https://localhost:${ARGOCD_HOST_PORT} -> NodePort ${ARGOCD_NODE_PORT}
Created:   ${CLUSTER_CREATED}

Next:
  scripts/install-online-boutique.sh
  scripts/install-argocd.sh

If you deploy kctx serve inside the cluster, expose it with a Service using:
  type: NodePort
  nodePort: ${NODE_PORT}
  targetPort: 8080

If this script reused an existing cluster created before the ArgoCD mapping was added,
run scripts/kind-down.sh and scripts/kind-up.sh to recreate it with the new port map.

EOF
