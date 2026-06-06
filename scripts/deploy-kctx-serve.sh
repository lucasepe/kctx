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
REQUEST_TIMEOUT="${REQUEST_TIMEOUT:-30s}"
KUBE_API_BUDGET="${KUBE_API_BUDGET:-100}"
TIMEOUT="${TIMEOUT:-120s}"

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

require docker
require kind
require kubectl

# The kind cluster must already exist with the kctx NodePort mapped by kind-up.sh.
kind get clusters | grep -Fxq "${CLUSTER_NAME}" || fail "kind cluster '${CLUSTER_NAME}' not found; run scripts/kind-up.sh first"

log "using kind context"
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

# Include the current Git commit in the image metadata when Git is available.
COMMIT_HASH="dev"
if command -v git >/dev/null 2>&1; then
  COMMIT_HASH="$(git rev-parse --short HEAD 2>/dev/null || printf dev)"
fi

# Build the local development image and load it into kind because kind cannot pull it from a registry.
log "building Docker image '${IMAGE}'"
docker build \
  --build-arg "COMMIT_HASH=${COMMIT_HASH}" \
  --tag "${IMAGE}" \
  .

log "loading image into kind cluster '${CLUSTER_NAME}'"
kind load docker-image "${IMAGE}" --name "${CLUSTER_NAME}"

# Create the namespace before applying the service account, RBAC, deployment, and service.
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
            - name: REQUEST_TIMEOUT
              value: "${REQUEST_TIMEOUT}"
            - name: KUBE_API_BUDGET
              value: "${KUBE_API_BUDGET}"
          ports:
            - name: http
              containerPort: ${CONTAINER_PORT}
              protocol: TCP
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            initialDelaySeconds: 2
            periodSeconds: 5
          livenessProbe:
            httpGet:
              path: /livez
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

# Wait until the deployment is available before printing usage examples.
log "waiting for rollout"
kubectl rollout status "deployment/${APP_NAME}" --namespace "${NAMESPACE}" --timeout="${TIMEOUT}"

cat <<EOF

kctx serve is ready.

Cluster:    ${CLUSTER_NAME}
Namespace:  ${NAMESPACE}
Image:      ${IMAGE}
URL:        http://localhost:${HOST_PORT}

Try:
  curl http://localhost:${HOST_PORT}/readyz
  curl http://localhost:${HOST_PORT}/version
  curl http://localhost:${HOST_PORT}/health/namespace/default

If you installed Online Boutique:
  curl http://localhost:${HOST_PORT}/health/namespace/boutique
  curl http://localhost:${HOST_PORT}/trace/service/boutique/frontend

EOF
