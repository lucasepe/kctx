#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
NAMESPACE="${NAMESPACE:-boutique}"
TIMEOUT="${TIMEOUT:-300s}"
MANIFEST_URL="${MANIFEST_URL:-https://raw.githubusercontent.com/GoogleCloudPlatform/microservices-demo/main/release/kubernetes-manifests.yaml}"

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

# The kind cluster must already exist before installing demo workloads.
kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found; run scripts/kind-up.sh first"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

# Create the target namespace in an idempotent way.
log "creating namespace '${NAMESPACE}'"
kubectl create namespace "${NAMESPACE}" --dry-run=client --output yaml | kubectl apply -f -

# Apply the Online Boutique Kubernetes manifests into the target namespace.
log "installing Online Boutique"
kubectl apply --namespace "${NAMESPACE}" -f "${MANIFEST_URL}"

# Wait until every pod in the namespace reports Ready.
log "waiting for Online Boutique pods"
kubectl wait --namespace "${NAMESPACE}" --for=condition=Ready pod --all --timeout="${TIMEOUT}"

# Capture the frontend pod name for copy-pasteable kctx examples.
FRONTEND_POD="$(kubectl get pod --namespace "${NAMESPACE}" -l app=frontendpath='{.items[0].metadata.name}')"

cat <<EOF

Online Boutique is ready.

Namespace:    ${NAMESPACE}
Frontend pod: ${FRONTEND_POD}

Try:
  go run . explain pod ${FRONTEND_POD} --namespace ${NAMESPACE}
  go run . graph pod ${FRONTEND_POD} --namespace ${NAMESPACE}
  go run . trace service frontend --namespace ${NAMESPACE}
  go run . health namespace ${NAMESPACE}
  go run . dump namespace ${NAMESPACE}

EOF
