#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
ARGOCD_NAMESPACE="${ARGOCD_NAMESPACE:-argocd}"
APP_NAME="${APP_NAME:-guestbook}"
DEST_NAMESPACE="${DEST_NAMESPACE:-guestbook}"
DELETE_DEST_NAMESPACE="${DELETE_DEST_NAMESPACE:-true}"

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

# The kind cluster must already exist before deleting demo resources.
kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

log "deleting ArgoCD Application '${APP_NAME}'"
kubectl delete application "${APP_NAME}" --namespace "${ARGOCD_NAMESPACE}" --ignore-not-found

if [ "${DELETE_DEST_NAMESPACE}" = "true" ]; then
  log "deleting destination namespace '${DEST_NAMESPACE}'"
  kubectl delete namespace "${DEST_NAMESPACE}" --ignore-not-found
else
  log "leaving destination namespace '${DEST_NAMESPACE}' in place"
fi

cat <<EOF

ArgoCD demo cleanup requested.

Application:       ${ARGOCD_NAMESPACE}/${APP_NAME}
Destination ns:    ${DEST_NAMESPACE}
Deleted namespace: ${DELETE_DEST_NAMESPACE}

EOF
