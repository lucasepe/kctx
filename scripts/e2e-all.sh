#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
OUT_DIR="${OUT_DIR:-/tmp/kctx-e2e}"

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

mkdir -p "${OUT_DIR}"

log "running ArgoCD e2e capture"
scripts/e2e-argocd.sh

log "running unhealthy resource e2e capture"
scripts/e2e-unhealthy.sh

log "running error e2e capture"
scripts/e2e-errors.sh

cat <<EOF

All e2e captures completed.

Directory: ${OUT_DIR}

Suggested review:
  ls -lh ${OUT_DIR}
  jq '.summary // empty' ${OUT_DIR}/unhealthy-health.json
  jq '.resource.status // empty' ${OUT_DIR}/argocd-application.json
  for f in ${OUT_DIR}/*.stderr; do [ -s "\$f" ] && printf '\n--- %s ---\n' "\$f" && cat "\$f"; done

EOF
