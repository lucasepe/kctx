#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
NAMESPACE="${NAMESPACE:-boutique}"
TIMEOUT="${TIMEOUT:-300s}"
MANIFEST_URL="${MANIFEST_URL:-https://raw.githubusercontent.com/GoogleCloudPlatform/microservices-demo/main/release/kubernetes-manifests.yaml}"

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

kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found; run scripts/kind-up.sh first"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

log "creating namespace '${NAMESPACE}'"
kubectl create namespace "${NAMESPACE}" --dry-run=client --output yaml | kubectl apply -f -

log "installing Online Boutique"
kubectl apply --namespace "${NAMESPACE}" -f "${MANIFEST_URL}"

log "waiting for Online Boutique pods"
kubectl wait --namespace "${NAMESPACE}" --for=condition=Ready pod --all --timeout="${TIMEOUT}"

FRONTEND_POD="$(kubectl get pod --namespace "${NAMESPACE}" -l app=frontend --output jsonpath='{.items[0].metadata.name}')"

cat <<EOF

Online Boutique is ready.

Namespace:    ${NAMESPACE}
Frontend pod: ${FRONTEND_POD}

Try:
  go run . explain pod ${FRONTEND_POD} --namespace ${NAMESPACE}
  go run . explain pod ${FRONTEND_POD} --namespace ${NAMESPACE} --output json
  go run . graph pod ${FRONTEND_POD} --namespace ${NAMESPACE} --output mermaid
  go run . trace service frontend --namespace ${NAMESPACE}
  go run . health namespace ${NAMESPACE}
  go run . dump namespace ${NAMESPACE} --output json

EOF
