#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
NAMESPACE="${NAMESPACE:-kctx-e2e-unhealthy}"
OUT_DIR="${OUT_DIR:-/tmp/kctx-e2e}"
KCTX_BIN="${KCTX_BIN:-./kctx}"
WAIT_SECONDS="${WAIT_SECONDS:-45}"

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

wait_for_bad_image_signal() {
  local deadline
  local reason
  deadline=$((SECONDS + WAIT_SECONDS))
  while [ "${SECONDS}" -lt "${deadline}" ]; do
    reason="$(kubectl get pod --namespace "${NAMESPACE}" -l app=kctx-bad-image --output jsonpath='{.items[0].status.containerStatuses[0].state.waiting.reason}' 2>/dev/null || true)"
    if [ -n "${reason}" ] && [ "${reason}" != "ContainerCreating" ]; then
      log "bad image pod reached waiting reason '${reason}'"
      return
    fi
    sleep 3
  done
  log "bad image pod did not expose a waiting reason within ${WAIT_SECONDS}s; continuing anyway"
}

require kind
require kubectl
require go

kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found; run scripts/kind-up.sh first"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

ensure_kctx
mkdir -p "${OUT_DIR}"

log "creating unhealthy namespace '${NAMESPACE}'"
kubectl create namespace "${NAMESPACE}" --dry-run=client --output yaml | kubectl apply -f -

log "applying artificial unhealthy resources"
kubectl apply --namespace "${NAMESPACE}" -f - <<YAML
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kctx-bad-image
  labels:
    app: kctx-bad-image
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kctx-bad-image
  template:
    metadata:
      labels:
        app: kctx-bad-image
    spec:
      containers:
        - name: app
          image: ghcr.io/lucasepe/kctx-e2e-image-does-not-exist:never
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: kctx-orphan-service
spec:
  type: ClusterIP
  selector:
    app: no-pod-should-match-this-selector
  ports:
    - name: http
      port: 80
      targetPort: 8080
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: kctx-pending-pvc
spec:
  storageClassName: kctx-missing-storage-class
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
YAML

wait_for_bad_image_signal

BAD_POD="$(kubectl get pod --namespace "${NAMESPACE}" -l app=kctx-bad-image --output jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
if [ -z "${BAD_POD}" ]; then
  fail "bad image pod was not created"
fi

capture_success "unhealthy-health" \
  "${KCTX_BIN}" health namespace "${NAMESPACE}"

capture_success "unhealthy-dump" \
  "${KCTX_BIN}" dump namespace "${NAMESPACE}"

capture_success "unhealthy-explain-bad-image-pod" \
  "${KCTX_BIN}" explain pod "${BAD_POD}" --namespace "${NAMESPACE}"

capture_success "unhealthy-trace-orphan-service" \
  "${KCTX_BIN}" trace service kctx-orphan-service --namespace "${NAMESPACE}"

cat <<EOF

Unhealthy e2e resources are ready and outputs were captured.

Namespace: ${NAMESPACE}
Directory: ${OUT_DIR}

Files:
  ${OUT_DIR}/unhealthy-health.json
  ${OUT_DIR}/unhealthy-dump.json
  ${OUT_DIR}/unhealthy-explain-bad-image-pod.json
  ${OUT_DIR}/unhealthy-trace-orphan-service.json

Cleanup:
  scripts/e2e-cleanup.sh

EOF
