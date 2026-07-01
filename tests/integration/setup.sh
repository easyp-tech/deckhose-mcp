#!/usr/bin/env bash
# Integration test setup: Kind + Deckhouse CE + MCP server.
#
# Transport modes (via TRANSPORT env, default: stdio):
#   stdio — build local binary, run as subprocess via FIFOs
#   sse   — build Docker image, load into Kind, deploy to d8-system, port-forward 8080
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-d8}"
KUBE_CONTEXT="kind-${KIND_CLUSTER_NAME}"
BINARY_PATH="$SCRIPT_DIR/deckhouse-harness"
TRANSPORT_MODE="${TRANSPORT:-stdio}"
IMAGE_NAME="${IMAGE_NAME:-deckhouse-mcp}"

info()  { echo "==> $*"; }
error() { echo "ERROR: $*" >&2; exit 1; }

# --- Prerequisites -----------------------------------------------------------
info "Checking prerequisites..."
case "$TRANSPORT_MODE" in
  stdio)
    for cmd in kind kubectl jq go; do
      command -v "$cmd" >/dev/null 2>&1 || error "$cmd is not installed"
    done
    ;;
  sse)
    for cmd in kind kubectl jq go docker; do
      command -v "$cmd" >/dev/null 2>&1 || error "$cmd is not installed"
    done
    ;;
  *)
    error "Unknown TRANSPORT='$TRANSPORT_MODE'. Use 'stdio' or 'sse'."
    ;;
esac
info "All prerequisites satisfied (transport: $TRANSPORT_MODE)."

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

# --- Start MCP server --------------------------------------------------------
case "$TRANSPORT_MODE" in
  stdio)
    info "Building MCP server binary (stdio mode)..."
    go build -o "$BINARY_PATH" "$ROOT_DIR/cmd/deckhouse-harness"
    info "Binary built at $BINARY_PATH"
    echo "stdio" > "$SCRIPT_DIR/.transport-mode"
    ;;

  sse)
    info "Building Docker image (SSE mode)..."
    docker build -t "$IMAGE_NAME:local" "$ROOT_DIR"
    info "Loading image into Kind..."
    kind load docker-image "$IMAGE_NAME:local" --name "$KIND_CLUSTER_NAME"

    info "Applying deployment manifests..."
    kubectl --context "$KUBE_CONTEXT" apply -f "$ROOT_DIR/deploy/rbac.yaml"
    kubectl --context "$KUBE_CONTEXT" apply -f "$ROOT_DIR/deploy/deployment.yaml"
    kubectl --context "$KUBE_CONTEXT" apply -f "$ROOT_DIR/deploy/service.yaml"

    info "Rolling out deployment..."
    kubectl --context "$KUBE_CONTEXT" rollout restart deployment/deckhouse-mcp -n d8-system
    kubectl --context "$KUBE_CONTEXT" rollout status deployment/deckhouse-mcp -n d8-system --timeout=120s

    info "Starting port-forward 8080:8080..."
    kubectl --context "$KUBE_CONTEXT" port-forward svc/deckhouse-mcp 8080:8080 -n d8-system \
      > "$SCRIPT_DIR/.port-forward.log" 2>&1 &
    echo $! > "$SCRIPT_DIR/.port-forward.pid"
    sleep 3  # give port-forward time to bind
    info "Port-forward PID: $(cat "$SCRIPT_DIR/.port-forward.pid")"
    echo "sse" > "$SCRIPT_DIR/.transport-mode"
    ;;
esac

# Export variables for test.sh (sourced via Taskfile env).
echo "$KUBE_CONTEXT" > "$SCRIPT_DIR/.kube-context"
echo "$BINARY_PATH" > "$SCRIPT_DIR/.binary-path"

info "Setup complete (transport: $TRANSPORT_MODE). Run 'task integration:test' to start tests."
