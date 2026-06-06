#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
ARGOCD_NAMESPACE="${ARGOCD_NAMESPACE:-argocd}"
APP_NAME="${APP_NAME:-guestbook}"
DEST_NAMESPACE="${DEST_NAMESPACE:-guestbook}"

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

# The kind cluster and ArgoCD namespace must already exist.
kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found; run scripts/kind-up.sh first"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

kubectl get namespace "${ARGOCD_NAMESPACE}" >/dev/null 2>&1 || fail "namespace '${ARGOCD_NAMESPACE}' not found; run scripts/install-argocd.sh first"
kubectl get crd applications.argoproj.io >/dev/null 2>&1 || fail "ArgoCD Application CRD not found; run scripts/install-argocd.sh first"

log "creating ArgoCD Application '${APP_NAME}'"
kubectl apply --namespace "${ARGOCD_NAMESPACE}" -f - <<YAML
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ${APP_NAME}
spec:
  project: default
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps.git
    targetRevision: HEAD
    path: guestbook
  destination:
    server: https://kubernetes.default.svc
    namespace: ${DEST_NAMESPACE}
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
YAML

cat <<EOF

ArgoCD Application is configured.

Application:       ${ARGOCD_NAMESPACE}/${APP_NAME}
Destination ns:    ${DEST_NAMESPACE}

Try:
  kubectl get applications.argoproj.io --namespace ${ARGOCD_NAMESPACE}
  kubectl describe application ${APP_NAME} --namespace ${ARGOCD_NAMESPACE}
  go run . explain applications.argoproj.io ${APP_NAME} --namespace ${ARGOCD_NAMESPACE}
  go run . health namespace ${DEST_NAMESPACE}
  go run . dump namespace ${DEST_NAMESPACE}

EOF
