#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
GOLDEN_NAMESPACE="${GOLDEN_NAMESPACE:-kctx-e2e-golden}"
GOLDEN_EMPTY_NAMESPACE="${GOLDEN_EMPTY_NAMESPACE:-kctx-e2e-golden-empty}"
RBAC_NAMESPACE="${RBAC_NAMESPACE:-kctx-e2e-golden-rbac}"
DENIED_KUBECONFIG="${DENIED_KUBECONFIG:-/tmp/kctx-e2e-golden-denied.kubeconfig}"
OUT_DIR="${OUT_DIR:-/tmp/kctx-e2e-golden}"
DELETE_OUTPUTS="${DELETE_OUTPUTS:-false}"

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

kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

log "deleting namespace '${GOLDEN_NAMESPACE}'"
kubectl delete namespace "${GOLDEN_NAMESPACE}" --ignore-not-found

log "deleting namespace '${GOLDEN_EMPTY_NAMESPACE}'"
kubectl delete namespace "${GOLDEN_EMPTY_NAMESPACE}" --ignore-not-found

log "deleting namespace '${RBAC_NAMESPACE}'"
kubectl delete namespace "${RBAC_NAMESPACE}" --ignore-not-found

if [ -f "${DENIED_KUBECONFIG}" ]; then
  log "deleting denied kubeconfig '${DENIED_KUBECONFIG}'"
  rm -f "${DENIED_KUBECONFIG}"
fi

if [ "${DELETE_OUTPUTS}" = "true" ] && [ -d "${OUT_DIR}" ]; then
  log "deleting golden e2e outputs '${OUT_DIR}'"
  rm -rf "${OUT_DIR}"
else
  log "leaving golden e2e outputs in '${OUT_DIR}'"
fi
