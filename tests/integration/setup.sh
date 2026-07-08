#!/usr/bin/env bash
# Integration test setup: Kind + Deckhouse CE + MCP server.
#
# The server is stdio-only: the tests build a local binary and run it as a
# subprocess via FIFOs (see mcp_helpers.sh).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-d8}"
KUBE_CONTEXT="kind-${KIND_CLUSTER_NAME}"
BINARY_PATH="$SCRIPT_DIR/deckhouse-harness"

info()  { echo "==> $*"; }
error() { echo "ERROR: $*" >&2; exit 1; }

# --- Prerequisites -----------------------------------------------------------
info "Checking prerequisites..."
for cmd in kind kubectl jq go; do
  command -v "$cmd" >/dev/null 2>&1 || error "$cmd is not installed"
done
info "All prerequisites satisfied."

# --- Kind cluster with Deckhouse CE ------------------------------------------
if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
  info "Kind cluster '${KIND_CLUSTER_NAME}' already exists, reusing."
else
  info "Creating Kind cluster with Deckhouse CE (this takes ~15 minutes)..."
  bash -c "$(curl -Ls https://raw.githubusercontent.com/deckhouse/deckhouse/main/tools/kind-d8.sh)"
fi

# Wait for Deckhouse to be ready (moduleconfig 'deckhouse' must exist).
info "Waiting for Deckhouse to be ready..."
for i in $(seq 1 60); do
  if kubectl --context "$KUBE_CONTEXT" get moduleconfigs deckhouse >/dev/null 2>&1; then
    info "Deckhouse is ready."
    break
  fi
  if [ "$i" -eq 60 ]; then
    error "Timeout waiting for Deckhouse to become ready."
  fi
  sleep 10
done

# Apply fixtures for consistent test data (NodeGroups, ModuleConfigs, Releases).
# Use --validate=false because the kind-d8 environment has admission policies
# that can reject certain heritage-labeled or stub objects.
info "Applying test fixtures..."
kubectl --context "$KUBE_CONTEXT" apply --validate=false -f "$ROOT_DIR/tests/integration/fixtures.yaml" 2>&1 | tail -5 || true

# --- Build MCP server binary (stdio) -----------------------------------------
info "Building MCP server binary..."
go build -o "$BINARY_PATH" "$ROOT_DIR/cmd/deckhouse-harness"
info "Binary built at $BINARY_PATH"

# Export variables for test.sh and skill scripts.
# Store the *kubeconfig file path* (not the context name) so that
# KUBECONFIG=... works directly in other scripts.
KUBECONFIG_FILE="${KUBECONFIG:-$HOME/.kube/config}"
echo "$KUBECONFIG_FILE" > "$SCRIPT_DIR/.kube-context"
echo "$BINARY_PATH" > "$SCRIPT_DIR/.binary-path"

info "Setup complete. Run 'task integration:test' to start tests."
info "For skill E2E/manual: export KUBECONFIG=$(cat "$SCRIPT_DIR/.kube-context")"
