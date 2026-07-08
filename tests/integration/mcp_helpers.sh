#!/usr/bin/env bash
# Shared MCP stdio transport helpers (robust FIFO subprocess).
# Sourced by test.sh, mcp-call.sh and skill scripts.
#
# Provides:
#   - mcp_connect / mcp_disconnect / mcp_send / mcp_request / mcp_initialize / mcp_call_tool
#
# Usage:
#   source tests/integration/mcp_helpers.sh
#   # then set KUBE_CONTEXT, BINARY_PATH, STDERR_LOG etc.
#   mcp_connect
#   mcp_initialize
#   result=$(mcp_call_tool "" "deckhouse_XXX" '{}')

# Note: we do NOT set -euo pipefail here so that sourcing does not change
# the caller's strict mode unexpectedly. Callers should have it.

# Defaults (can be overridden by caller)
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-d8}"
BINARY_PATH="${BINARY_PATH:-tests/integration/deckhouse-harness}"
STDERR_LOG="${STDERR_LOG:-/tmp/mcp-stderr.log}"

# Internal state
_MCP_SERVER_PID=""
_MCP_TMPDIR=""

_mcp_cleanup() {
  mcp_disconnect
  if [[ -n "${_MCP_TMPDIR}" && -d "${_MCP_TMPDIR}" ]]; then
    rm -rf "${_MCP_TMPDIR}"
  fi
}

# --------------------------------------------------------------------
# STDIO transport (FIFO subprocess)
# --------------------------------------------------------------------

mcp_connect() {
  _MCP_TMPDIR=$(mktemp -d)
  STDIN_FIFO="${_MCP_TMPDIR}/stdin"
  STDOUT_FIFO="${_MCP_TMPDIR}/stdout"
  mkfifo "$STDIN_FIFO" "$STDOUT_FIFO"

  KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}" \
    "$BINARY_PATH" < "$STDIN_FIFO" > "$STDOUT_FIFO" 2>"$STDERR_LOG" &
  _MCP_SERVER_PID=$!

  exec 3>"$STDIN_FIFO"
  exec 4<"$STDOUT_FIFO"
}

mcp_disconnect() {
  exec 3>&- 4<&- 2>/dev/null || true
  if [[ -n "${_MCP_SERVER_PID}" ]]; then
    kill "${_MCP_SERVER_PID}" 2>/dev/null || true
    wait "${_MCP_SERVER_PID}" 2>/dev/null || true
    _MCP_SERVER_PID=""
  fi
  if [[ -n "${_MCP_TMPDIR}" && -d "${_MCP_TMPDIR}" ]]; then
    rm -rf "${_MCP_TMPDIR}"
    _MCP_TMPDIR=""
  fi
}

mcp_send() {
  # printf %s (not echo): zsh's echo interprets backslash escapes, which would
  # turn an escaped \n inside a JSON string value (e.g. a PEM key) into a raw
  # newline and break the JSON-RPC message.
  printf '%s\n' "$1" >&3
}

mcp_recv() {
  local line
  IFS= read -r line <&4
  echo "$line"
}

mcp_request() {
  local method="$1"
  local params="${2:-null}"
  local id="${3:-1}"

  local body
  # -c: one JSON-RPC message per line so NDJSON framing is not broken by
  # pretty-print newlines or by newlines inside string values (e.g. PEM keys).
  body=$(jq -cn --arg method "$method" --argjson params "$params" --argjson id "$id" \
    '{"jsonrpc":"2.0","method":$method,"params":$params,"id":$id}')

  mcp_send "$body"

  local line
  # -t: never block the whole run forever if a response is slow or missing.
  while IFS= read -r -t "${MCP_READ_TIMEOUT:-30}" line <&4; do
    if echo "$line" | jq -e --argjson id "$id" 'select(.id == $id)' >/dev/null 2>&1; then
      echo "$line"
      return 0
    fi
  done

  echo '{"error":{"code":-1,"message":"timed out or EOF reading from server"}}' >&2
  return 1
}

# --------------------------------------------------------------------

mcp_initialize() {
  local resp
  resp=$(mcp_request "initialize" '{
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {"name": "integration-test", "version": "1.0.0"}
  }' 0) || return 1

  if echo "$resp" | jq -e '.error' >/dev/null 2>&1; then
    echo "ERROR: Initialize failed: $(echo "$resp" | jq -r '.error.message')" >&2
    return 1
  fi

  mcp_send '{"jsonrpc":"2.0","method":"notifications/initialized"}'
}

mcp_call_tool() {
  local _endpoint="$1"
  local tool_name="$2"
  local arguments="${3:-}"
  if [[ -z "$arguments" ]]; then arguments='{}'; fi
  local id="${4:-$((RANDOM % 10000 + 100))}"

  local params
  params=$(jq -cn --arg name "$tool_name" --argjson args "$arguments" \
    '{"name":$name,"arguments":$args}')

  local resp
  resp=$(mcp_request "tools/call" "$params" "$id") || return 1

  if echo "$resp" | jq -e '.error' >/dev/null 2>&1; then
    echo "TOOL ERROR: $(echo "$resp" | jq -r '.error.message')" >&2
    echo "$resp"
    return 1
  fi

  echo "$resp" | jq -r '.result.content[0].text // .result'
}

# Register cleanup
trap _mcp_cleanup EXIT
