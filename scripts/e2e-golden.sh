#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
EMPTY_NAMESPACE="${EMPTY_NAMESPACE:-kctx-e2e-golden-empty}"
NAMESPACE="${NAMESPACE:-kctx-e2e-golden}"
RBAC_NAMESPACE="${RBAC_NAMESPACE:-kctx-e2e-golden-rbac}"
RBAC_SERVICEACCOUNT="${RBAC_SERVICEACCOUNT:-kctx-golden-denied}"
OUT_DIR="${OUT_DIR:-/tmp/kctx-e2e-golden}"
GOLDEN_DIR="${GOLDEN_DIR:-testdata/e2e/golden}"
KCTX_BIN="${KCTX_BIN:-./kctx}"
DENIED_KUBECONFIG="${DENIED_KUBECONFIG:-/tmp/kctx-e2e-golden-denied.kubeconfig}"
WAIT_SECONDS="${WAIT_SECONDS:-45}"
UPDATE_GOLDEN="${UPDATE_GOLDEN:-0}"

ACTUAL_DIR="${OUT_DIR}/actual"
NORMALIZED_DIR="${OUT_DIR}/normalized"

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

capture_success_json() {
  local name="$1"
  shift
  log "capturing ${name}"
  "$@" >"${ACTUAL_DIR}/${name}.json" 2>"${ACTUAL_DIR}/${name}.stderr"
  printf '0\n' >"${ACTUAL_DIR}/${name}.exitcode"
}

capture_failure_json() {
  local name="$1"
  shift
  local code
  log "capturing expected failure ${name}"
  set +e
  "$@" >"${ACTUAL_DIR}/${name}.json" 2>"${ACTUAL_DIR}/${name}.stderr"
  code="$?"
  set -e
  printf '%s\n' "${code}" >"${ACTUAL_DIR}/${name}.exitcode"
  if [ "${code}" -eq 0 ]; then
    fail "expected ${name} to fail"
  fi
}

normalize_json() {
  local name="$1"
  GOCACHE="${GOCACHE:-/private/tmp/kctx-go-build}" go run ./internal/e2e/normalize <"${ACTUAL_DIR}/${name}.json" >"${NORMALIZED_DIR}/${name}.json"
}

compare_or_update() {
  local name="$1"
  local actual="${NORMALIZED_DIR}/${name}.json"
  local golden="${GOLDEN_DIR}/${name}.json"

  if [ "${UPDATE_GOLDEN}" = "1" ] || [ "${UPDATE_GOLDEN}" = "true" ]; then
    mkdir -p "${GOLDEN_DIR}"
    cp "${actual}" "${golden}"
    log "updated ${golden}"
    return
  fi

  if [ ! -f "${golden}" ]; then
    fail "missing golden file ${golden}; run UPDATE_GOLDEN=1 scripts/e2e-golden.sh to create it"
  fi

  diff -u "${golden}" "${actual}"
}

wait_for_bad_image_signal() {
  local deadline
  local reason
  deadline=$((SECONDS + WAIT_SECONDS))
  while [ "${SECONDS}" -lt "${deadline}" ]; do
    reason="$(kubectl get pod --namespace "${NAMESPACE}" kctx-bad-image-pod --output jsonpath='{.status.containerStatuses[0].state.waiting.reason}' 2>/dev/null || true)"
    if [ -n "${reason}" ] && [ "${reason}" != "ContainerCreating" ]; then
      log "bad image pod reached waiting reason '${reason}'"
      return
    fi
    sleep 3
  done
  log "bad image pod did not expose a waiting reason within ${WAIT_SECONDS}s; continuing anyway"
}

create_denied_kubeconfig() {
  local token
  local server
  local ca_data

  log "creating denied ServiceAccount kubeconfig"
  kubectl create namespace "${RBAC_NAMESPACE}" --dry-run=client --output yaml | kubectl apply -f -
  kubectl create serviceaccount "${RBAC_SERVICEACCOUNT}" --namespace "${RBAC_NAMESPACE}" --dry-run=client --output yaml | kubectl apply -f -

  token="$(kubectl create token "${RBAC_SERVICEACCOUNT}" --namespace "${RBAC_NAMESPACE}" --duration=15m)"
  server="$(kubectl config view --raw --minify --output jsonpath='{.clusters[0].cluster.server}')"
  ca_data="$(kubectl config view --raw --minify --output jsonpath='{.clusters[0].cluster.certificate-authority-data}')"

  cat >"${DENIED_KUBECONFIG}" <<YAML
apiVersion: v1
kind: Config
clusters:
  - name: denied
    cluster:
      server: ${server}
      certificate-authority-data: ${ca_data}
users:
  - name: denied
    user:
      token: ${token}
contexts:
  - name: denied
    context:
      cluster: denied
      user: denied
      namespace: default
current-context: denied
YAML
}

apply_fixtures() {
  log "creating deterministic namespaces"
  kubectl create namespace "${EMPTY_NAMESPACE}" --dry-run=client --output yaml | kubectl apply -f -
  kubectl create namespace "${NAMESPACE}" --dry-run=client --output yaml | kubectl apply -f -

  log "resetting immutable fixture objects"
  kubectl delete pod kctx-bad-image-pod --namespace "${NAMESPACE}" --ignore-not-found --wait=true
  kubectl delete pvc kctx-pending-pvc --namespace "${NAMESPACE}" --ignore-not-found --wait=true
  kubectl delete event --all --namespace "${NAMESPACE}" --ignore-not-found

  log "applying deterministic golden fixtures"
  kubectl apply --namespace "${NAMESPACE}" -f - <<YAML
apiVersion: v1
kind: Secret
metadata:
  name: kctx-secret
  labels:
    app: kctx-golden
    example.com/token: should-not-leak
type: Opaque
stringData:
  password: should-not-leak
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: kctx-config
  labels:
    app: kctx-golden
data:
  api-key: should-not-leak
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: kctx-pending-pvc
spec:
  storageClassName: ""
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: Pod
metadata:
  name: kctx-bad-image-pod
  labels:
    app: kctx-bad-image
    example.com/token: should-not-leak
spec:
  restartPolicy: Never
  volumes:
    - name: config
      configMap:
        name: kctx-config
    - name: secret
      secret:
        secretName: kctx-secret
  containers:
    - name: app
      image: ghcr.io/lucasepe/kctx-e2e-image-does-not-exist:never
      volumeMounts:
        - name: config
          mountPath: /etc/kctx-config
        - name: secret
          mountPath: /etc/kctx-secret
---
apiVersion: v1
kind: Service
metadata:
  name: kctx-orphan-service
  labels:
    app: kctx-golden
spec:
  type: ClusterIP
  selector:
    app: no-pod-should-match-this-selector
  ports:
    - name: http
      port: 80
      targetPort: 8080
YAML
}

require kind
require kubectl
require go
require diff

kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found; run scripts/kind-up.sh first"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

ensure_kctx
mkdir -p "${ACTUAL_DIR}" "${NORMALIZED_DIR}"

apply_fixtures
wait_for_bad_image_signal
create_denied_kubeconfig

capture_success_json "health-empty-namespace" \
  "${KCTX_BIN}" health namespace "${EMPTY_NAMESPACE}"

capture_success_json "health-unhealthy-namespace" \
  "${KCTX_BIN}" health namespace "${NAMESPACE}"

capture_success_json "dump-unhealthy-namespace" \
  "${KCTX_BIN}" dump namespace "${NAMESPACE}"

capture_success_json "explain-bad-image-pod" \
  "${KCTX_BIN}" explain pod kctx-bad-image-pod --namespace "${NAMESPACE}"

capture_success_json "trace-orphan-service" \
  "${KCTX_BIN}" trace service kctx-orphan-service --namespace "${NAMESPACE}"

capture_failure_json "error-not-found-pod" \
  "${KCTX_BIN}" explain pod kctx-pod-does-not-exist --namespace "${NAMESPACE}"

capture_failure_json "error-forbidden-health" \
  "${KCTX_BIN}" --kubeconfig "${DENIED_KUBECONFIG}" health namespace default

for name in \
  health-empty-namespace \
  health-unhealthy-namespace \
  dump-unhealthy-namespace \
  explain-bad-image-pod \
  trace-orphan-service \
  error-not-found-pod \
  error-forbidden-health
do
  normalize_json "${name}"
  compare_or_update "${name}"
done

log "validating JSON contract"
scripts/validate-json-contract.sh

cat <<EOF

Golden e2e completed.

Actual outputs:     ${ACTUAL_DIR}
Normalized outputs: ${NORMALIZED_DIR}
Golden outputs:     ${GOLDEN_DIR}
Update mode:        ${UPDATE_GOLDEN}

Cleanup:
  GOLDEN_NAMESPACE=${NAMESPACE} GOLDEN_EMPTY_NAMESPACE=${EMPTY_NAMESPACE} scripts/e2e-golden-cleanup.sh

EOF
