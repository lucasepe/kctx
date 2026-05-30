#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"
NAMESPACE="${NAMESPACE:-kctx-system}"
APP_NAME="${APP_NAME:-kctx-serve}"
IMAGE="${IMAGE:-kctx:kind}"
HOST_PORT="${HOST_PORT:-8888}"
NODE_PORT="${NODE_PORT:-30088}"
CONTAINER_PORT="${CONTAINER_PORT:-8080}"
VERBOSE="${VERBOSE:-false}"
TIMEOUT="${TIMEOUT:-120s}"

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

require docker
require kind
require kubectl

kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found; run scripts/kind-up.sh first"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

COMMIT_HASH="dev"
if command -v git >/dev/null 2>&1; then
  COMMIT_HASH="$(git rev-parse --short HEAD 2>/dev/null || printf dev)"
fi

log "building Docker image '${IMAGE}'"
docker build \
  --build-arg "COMMIT_HASH=${COMMIT_HASH}" \
  --tag "${IMAGE}" \
  .

log "loading image into kind cluster '${CLUSTER_NAME}'"
kind load docker-image "${IMAGE}" --name "${CLUSTER_NAME}"

log "deploying '${APP_NAME}' into namespace '${NAMESPACE}'"
kubectl create namespace "${NAMESPACE}" --dry-run=client --output yaml | kubectl apply -f -

kubectl apply -f - <<YAML
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${APP_NAME}
  namespace: ${NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ${APP_NAME}
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ${APP_NAME}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ${APP_NAME}
subjects:
  - kind: ServiceAccount
    name: ${APP_NAME}
    namespace: ${NAMESPACE}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${APP_NAME}
  namespace: ${NAMESPACE}
  labels:
    app.kubernetes.io/name: ${APP_NAME}
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: ${APP_NAME}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: ${APP_NAME}
    spec:
      serviceAccountName: ${APP_NAME}
      containers:
        - name: kctx
          image: ${IMAGE}
          imagePullPolicy: Never
          args:
            - serve
            - --listen
            - :${CONTAINER_PORT}
          env:
            - name: VERBOSE
              value: "${VERBOSE}"
          ports:
            - name: http
              containerPort: ${CONTAINER_PORT}
              protocol: TCP
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 2
            periodSeconds: 5
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 10
            periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: ${APP_NAME}
  namespace: ${NAMESPACE}
  labels:
    app.kubernetes.io/name: ${APP_NAME}
spec:
  type: NodePort
  selector:
    app.kubernetes.io/name: ${APP_NAME}
  ports:
    - name: http
      port: ${CONTAINER_PORT}
      targetPort: http
      nodePort: ${NODE_PORT}
YAML

log "waiting for rollout"
kubectl rollout status "deployment/${APP_NAME}" --namespace "${NAMESPACE}" --timeout="${TIMEOUT}"

cat <<EOF

kctx serve is ready.

Cluster:    ${CLUSTER_NAME}
Namespace:  ${NAMESPACE}
Image:      ${IMAGE}
URL:        http://localhost:${HOST_PORT}

Try:
  curl http://localhost:${HOST_PORT}/healthz
  curl http://localhost:${HOST_PORT}/version
  curl http://localhost:${HOST_PORT}/health/namespace/default

If you installed Online Boutique:
  curl http://localhost:${HOST_PORT}/health/namespace/boutique
  curl http://localhost:${HOST_PORT}/trace/service/boutique/frontend

EOF
