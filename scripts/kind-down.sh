#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kctx-lab}"

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

# Delete the lab cluster only when it exists.
if kind get clusters | grep -Fxq "${CLUSTER_NAME}"; then
  log "deleting kind cluster '${CLUSTER_NAME}'"
  kind delete cluster --name "${CLUSTER_NAME}"
else
  log "kind cluster '${CLUSTER_NAME}' does not exist"
fi
