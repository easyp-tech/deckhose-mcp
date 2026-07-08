#!/usr/bin/env bash
# Invoke a single deckhouse-harness tool over JSON-RPC (uses shared robust helpers).
#
# Usage:
#   mcp-call.sh <tool_name> [args_json]
#
# This is the preferred one-shot caller for the e2e skill and manual testing.
# It now sources the shared helpers for reliability.

set -euo pipefail

TOOL="${1:?Usage: mcp-call.sh <tool_name> [args_json]}"
ARGS="${2:-{}}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
BINARY="$PROJECT_ROOT/tests/integration/deckhouse-harness"

if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: binary not found at $BINARY — run setup.sh first." >&2
  exit 1
fi

# Make sure we use the correct kubeconfig (setup now writes a real path)
if [ -z "${KUBECONFIG:-}" ]; then
  if [ -f "$PROJECT_ROOT/tests/integration/.kube-context" ]; then
    STORED="$(cat "$PROJECT_ROOT/tests/integration/.kube-context" 2>/dev/null || true)"
    if [ -f "$STORED" ]; then
      export KUBECONFIG="$STORED"
    fi
  fi
fi

# Source the stdio helpers.
export BINARY_PATH="$BINARY"
# helpers live next to the integration test scripts
source "$PROJECT_ROOT/tests/integration/mcp_helpers.sh" || {
  echo "ERROR: could not source mcp_helpers.sh" >&2
  exit 1
}

# One fresh connection per call (as per skill contract)
mcp_connect
mcp_initialize

result=$(mcp_call_tool "" "$TOOL" "$ARGS")
echo "$result"

mcp_disconnect
