#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
RBAC_NAMESPACE="${RBAC_NAMESPACE:-kctx-e2e-rbac}"
RBAC_SERVICEACCOUNT="${RBAC_SERVICEACCOUNT:-kctx-denied}"
UNSUPPORTED_NAMESPACE="${UNSUPPORTED_NAMESPACE:-kctx-e2e-unsupported}"
OUT_DIR="${OUT_DIR:-/tmp/kctx-e2e}"
KCTX_BIN="${KCTX_BIN:-./kctx}"
DENIED_KUBECONFIG="${DENIED_KUBECONFIG:-/tmp/kctx-e2e-denied.kubeconfig}"

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

apply_unsupported_crd_fixture() {
  log "creating unsupported CRD fixture"
  kubectl create namespace "${UNSUPPORTED_NAMESPACE}" --dry-run=client --output yaml | kubectl apply -f -
  kubectl apply -f - <<YAML
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.kctx.io
spec:
  group: kctx.io
  names:
    kind: Widget
    plural: widgets
    singular: widget
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true
YAML
  kubectl wait --for condition=Established crd/widgets.kctx.io --timeout=30s
  kubectl apply --namespace "${UNSUPPORTED_NAMESPACE}" -f - <<YAML
apiVersion: kctx.io/v1alpha1
kind: Widget
metadata:
  name: unsupported-demo
spec:
  message: no semantic adapter is registered for this CRD
YAML
}

require kind
require kubectl
require go

kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found; run scripts/kind-up.sh first"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

ensure_kctx
mkdir -p "${OUT_DIR}"

capture_failure "error-not-found-pod" \
  "${KCTX_BIN}" explain pod kctx-pod-does-not-exist --namespace default

apply_unsupported_crd_fixture

capture_failure "error-unsupported-widget" \
  "${KCTX_BIN}" explain widgets.kctx.io unsupported-demo --namespace "${UNSUPPORTED_NAMESPACE}"

create_denied_kubeconfig

capture_failure "error-rbac-forbidden-health" \
  "${KCTX_BIN}" --kubeconfig "${DENIED_KUBECONFIG}" health namespace default

capture_failure "error-rbac-forbidden-explain-pod" \
  "${KCTX_BIN}" --kubeconfig "${DENIED_KUBECONFIG}" explain pod kctx-pod-does-not-exist --namespace default

cat <<EOF

Error e2e outputs captured.

Directory: ${OUT_DIR}
Denied kubeconfig: ${DENIED_KUBECONFIG}

Files:
  ${OUT_DIR}/error-not-found-pod.stderr
  ${OUT_DIR}/error-not-found-pod.exitcode
  ${OUT_DIR}/error-unsupported-widget.stderr
  ${OUT_DIR}/error-unsupported-widget.exitcode
  ${OUT_DIR}/error-rbac-forbidden-health.stderr
  ${OUT_DIR}/error-rbac-forbidden-health.exitcode
  ${OUT_DIR}/error-rbac-forbidden-explain-pod.stderr
  ${OUT_DIR}/error-rbac-forbidden-explain-pod.exitcode

Cleanup:
  scripts/e2e-cleanup.sh

EOF
