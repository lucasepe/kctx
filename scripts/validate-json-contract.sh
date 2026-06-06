#!/usr/bin/env bash
set -euo pipefail

SCHEMA_DIR="${SCHEMA_DIR:-schemas/kctx.io/v1alpha1}"
GOLDEN_DIR="${GOLDEN_DIR:-testdata/e2e/golden}"

if [ "$#" -gt 0 ]; then
  exec env GOCACHE="${GOCACHE:-/private/tmp/kctx-go-build}" go run ./internal/contract/validate --schemas "${SCHEMA_DIR}" "$@"
fi

files=()
while IFS= read -r file; do
  files+=("${file}")
done < <(find "${GOLDEN_DIR}" -maxdepth 1 -type f -name '*.json' | sort)
if [ "${#files[@]}" -eq 0 ]; then
  printf 'error: no golden JSON files found in %s\n' "${GOLDEN_DIR}" >&2
  exit 1
fi

env GOCACHE="${GOCACHE:-/private/tmp/kctx-go-build}" go run ./internal/contract/validate --schemas "${SCHEMA_DIR}" "${files[@]}"
