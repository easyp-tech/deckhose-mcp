#!/usr/bin/env bash
# Integration test teardown: clean up test resources and MCP server.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-d8}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-d8}"

info()  { echo "==> $*"; }

# --- Stop MCP server ----------------------------------------------------------
if [ -f "$SCRIPT_DIR/deckhouse-harness" ]; then
  info "Removing MCP binary..."
  rm -f "$SCRIPT_DIR/deckhouse-harness"
fi

rm -f "$SCRIPT_DIR/.kube-context" "$SCRIPT_DIR/.binary-path"

# --- Delete test resources ----------------------------------------------------
info "Cleaning up integration test resources..."
kubectl --context "$KUBE_CONTEXT" delete staticinstances \
  integration-test-si integration-test-worker integration-test-delete-si 2>/dev/null || true
kubectl --context "$KUBE_CONTEXT" delete sshcredentials \
  integration-test-creds integration-test-worker-creds 2>/dev/null || true
kubectl --context "$KUBE_CONTEXT" delete nodegroups \
  integration-test-ng 2>/dev/null || true

# --- Optionally delete Kind cluster -------------------------------------------
if [ "${DELETE_CLUSTER:-}" = "true" ]; then
  info "Deleting Kind cluster '${KIND_CLUSTER_NAME}'..."
  kind delete cluster --name "$KIND_CLUSTER_NAME"
fi

info "Teardown complete."
