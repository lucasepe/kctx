#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
ARGOCD_NAMESPACE="${ARGOCD_NAMESPACE:-argocd}"
APP_NAME="${APP_NAME:-guestbook}"
DEST_NAMESPACE="${DEST_NAMESPACE:-guestbook}"
OUT_DIR="${OUT_DIR:-/tmp/kctx-e2e}"
KCTX_BIN="${KCTX_BIN:-./kctx}"

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

ensure_kctx() {
  if [ -x "${KCTX_BIN}" ]; then
    return
  fi
  if [ "${KCTX_BIN}" != "./kctx" ]; then
    fail "KCTX_BIN '${KCTX_BIN}' is not executable"
  fi
  log "building ./kctx"
  GOCACHE="${GOCACHE:-/private/tmp/kctx-go-build}" go build -o ./kctx .
}

capture_success() {
  local name="$1"
  shift
  log "capturing ${name}"
  "$@" >"${OUT_DIR}/${name}.json" 2>"${OUT_DIR}/${name}.stderr"
  printf '0\n' >"${OUT_DIR}/${name}.exitcode"
}

capture_failure() {
  local name="$1"
  shift
  local code
  log "capturing expected failure ${name}"
  set +e
  "$@" >"${OUT_DIR}/${name}.stdout" 2>"${OUT_DIR}/${name}.stderr"
  code="$?"
  set -e
  printf '%s\n' "${code}" >"${OUT_DIR}/${name}.exitcode"
  if [ "${code}" -eq 0 ]; then
    fail "expected ${name} to fail"
  fi
}

require kind
require kubectl
require go

kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found; run scripts/kind-up.sh first"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

ensure_kctx
mkdir -p "${OUT_DIR}"

kubectl get namespace "${ARGOCD_NAMESPACE}" >/dev/null 2>&1 || fail "namespace '${ARGOCD_NAMESPACE}' not found; run scripts/install-argocd.sh first"
kubectl get application "${APP_NAME}" --namespace "${ARGOCD_NAMESPACE}" >/dev/null 2>&1 || fail "Application '${ARGOCD_NAMESPACE}/${APP_NAME}' not found; run scripts/create-argocd-application.sh first"

capture_success "argocd-application" \
  "${KCTX_BIN}" explain applications.argoproj.io "${APP_NAME}" --namespace "${ARGOCD_NAMESPACE}"

capture_success "argocd-appproject" \
  "${KCTX_BIN}" explain appprojects.argoproj.io default --namespace "${ARGOCD_NAMESPACE}"

capture_success "argocd-destination-health" \
  "${KCTX_BIN}" health namespace "${DEST_NAMESPACE}"

capture_success "argocd-destination-dump" \
  "${KCTX_BIN}" dump namespace "${DEST_NAMESPACE}"

cat <<EOF

ArgoCD e2e outputs captured.

Directory: ${OUT_DIR}

Files:
  ${OUT_DIR}/argocd-application.json
  ${OUT_DIR}/argocd-appproject.json
  ${OUT_DIR}/argocd-destination-health.json
  ${OUT_DIR}/argocd-destination-dump.json

EOF
