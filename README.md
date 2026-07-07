# Deckhouse Harness

MCP server for managing [Deckhouse Kubernetes Platform](https://deckhouse.ru/docs) (Community Edition).

Supports **dual transport**:
- **stdio** (default): local process, newline-delimited JSON on stdin/stdout.
- **SSE** (HTTP): set `LISTEN_ADDR=:8080` (or use `-listen :8080`) to serve via `mcp.NewSSEHandler`.

Authenticates to Kubernetes using in-cluster config (when running inside a Pod) or `~/.kube/config` / `KUBECONFIG` (for local execution).

## Features

**Diagnostics** (read-only, 11 tools)
- `deckhouse_GetClusterStatus` — aggregated cluster health: nodes, modules, releases, unhealthy pods
- `deckhouse_ListNodes` — cluster nodes with filtering by group, status, role
- `deckhouse_ListNodeGroups` — NodeGroup resources with status and conditions
- `deckhouse_ListStaticInstances` — StaticInstance resources with filtering by group and phase
- `deckhouse_ListUnhealthyPods` — pods not in Running/Succeeded state
- `deckhouse_GetNode` — detailed node info with conditions, capacity, events
- `deckhouse_GetNodeGroup` — full NodeGroup spec with member node names
- `deckhouse_GetDeckhouseLogs` — Deckhouse controller pod logs with grep/tail/since
- `deckhouse_GetNodeEvents` — Kubernetes Events for a specific node
- `deckhouse_GetStaticInstance` — detailed StaticInstance info
- `deckhouse_GetPodLogs` — logs for a specific pod and container

**Modules** (7 tools)
- `deckhouse_ListModuleConfigs` — ModuleConfig resources with enabled/disabled filter
- `deckhouse_GetModuleConfig` — full spec and status of a single ModuleConfig
- `deckhouse_EnableModule` / `deckhouse_DisableModule` — toggle module enabled state
- `deckhouse_ListModules` — runtime Module resources
- `deckhouse_UpdateModuleSettings` — RFC 7396 JSON Merge Patch on module settings
- `deckhouse_SetModuleMaintenance` — toggle module maintenance mode

**Releases** (3 tools)
- `deckhouse_ListDeckhouseReleases` — DeckhouseRelease resources with phase filter
- `deckhouse_GetDeckhouseRelease` — full release details with requirements
- `deckhouse_ApproveRelease` — approve a pending release

**Nodes** (13 tools, write)
- `deckhouse_CreateSSHCredentials` / `deckhouse_DeleteSSHCredentials`
- `deckhouse_CreateStaticInstance` / `deckhouse_DeleteStaticInstance`
- `deckhouse_AddWorkerNode` — composite: SSHCredentials → StaticInstance → wait for Running
- `deckhouse_RemoveNode` — composite: drain → delete StaticInstance
- `deckhouse_CreateNodeGroup` / `deckhouse_DeleteNodeGroup`
- `deckhouse_WaitNodeReady` — poll StaticInstance until Running or timeout
- `deckhouse_CordonNode` / `deckhouse_UncordonNode`
- `deckhouse_DrainNode` — cordon + eviction loop with PDB awareness
- `deckhouse_CreateNodeGroupConfiguration` — bash script bound to NodeGroups

**Config** (3 tools)
- `deckhouse_GetClusterConfiguration` — read ClusterConfiguration YAML
- `deckhouse_GetStaticClusterConfiguration` — read StaticClusterConfiguration YAML
- `deckhouse_UpdateKubernetesVersion` — patch kubernetesVersion with retry-on-conflict

**Sources** (6 tools)
- `deckhouse_ListModuleSources` / `deckhouse_CreateModuleSource` / `deckhouse_DeleteModuleSource`
- `deckhouse_ListModuleUpdatePolicies` / `deckhouse_CreateModuleUpdatePolicy`
- `deckhouse_ListModuleReleases` — module releases with phase filter

## Tech Stack

- **Go 1.26** with [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) (stdio + SSE transports)
- **Protobuf + [protoc-gen-mcp](https://github.com/easyp-tech/protoc-gen-mcp)** — proto-first tool generation
- **[EasyP](https://github.com/easyp-tech/easyp)** — proto linting, codegen, dependency management
- **client-go** — typed client for core resources, dynamic client for Deckhouse CRDs

## Prerequisites

- Go 1.26+
- [EasyP](https://github.com/easyp-tech/easyp) (`brew install easyp-tech/tap/easyp`)
- [go-task](https://taskfile.dev) (`brew install go-task`)
- A Kubernetes cluster with Deckhouse CE installed
- `~/.kube/config` or `KUBECONFIG` env pointing to the cluster

## Quick Start

```bash
# Generate protobuf code
task generate

# Build
go build -o deckhouse-harness ./cmd/deckhouse-harness

# Run tests
task test

# Run the server (connects to cluster via kubeconfig)
./deckhouse-harness
```

## Transport Modes

The server supports two transport modes, selectable via the `TRANSPORT` env var or CLI flags:

### stdio (default)

Local process mode — newline-delimited JSON on stdin/stdout. Used by MCP clients like Claude Desktop, Cursor, and `mcp` CLI. Logs go to stderr.

```bash
./deckhouse-harness                    # stdio mode (default)
TRANSPORT=stdio ./deckhouse-harness    # explicit
```

### SSE (HTTP)

SSE transport for in-cluster deployment or remote access. Uses `mcp.NewSSEHandler` + `http.Server` with graceful shutdown. Multiple clients can connect concurrently.

```bash
TRANSPORT=sse ./deckhouse-harness                           # SSE on :8080
TRANSPORT=sse LISTEN_ADDR=:9090 ./deckhouse-harness         # SSE on :9090
./deckhouse-harness -listen :8080                           # via CLI flag
```

### In-Cluster Deployment (SSE)

Docker image and Kubernetes manifests are provided for deploying the server inside a cluster (e.g. in the `d8-system` namespace alongside Deckhouse):

```bash
# Build and load into Kind
task docker:build
task docker:load

# Deploy
kubectl apply -f deploy/rbac.yaml -f deploy/deployment.yaml -f deploy/service.yaml

# Port-forward for local testing
kubectl port-forward svc/deckhouse-harness 8080:8080 -n d8-system
```

The Deployment sets `TRANSPORT=sse` and uses in-cluster config for Kubernetes auth. See `deploy/` for manifests (Deployment, Service, RBAC).

## Connecting an MCP Client

By default the server uses **stdio transport**. Configure your MCP client to launch the binary.

**SSE / HTTP mode:** Start the server with `-listen :8080` (or `LISTEN_ADDR=:8080` env). The binary uses `urfave/cli/v3` so `--help` shows associated env vars. Configure MCP clients that support HTTP/SSE to the base URL.

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS):

```json
{
  "mcpServers": {
    "deckhouse": {
      "command": "/path/to/deckhouse-harness",
      "env": {
        "KUBECONFIG": "/Users/you/.kube/config",
        "LOG_LEVEL": "INFO"
      }
    }
  }
}
```

### Cursor / VS Code MCP

```json
{
  "mcpServers": {
    "deckhouse": {
      "command": "/path/to/deckhouse-harness"
    }
  }
}
```

### Manual (pipe JSON-RPC)

```bash
echo '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./deckhouse-harness
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TRANSPORT` | `stdio` | Transport mode: `stdio` (local) or `sse` (HTTP in-cluster). If empty, auto-selects based on `LISTEN_ADDR`. |
| `LISTEN_ADDR` | `:8080` | SSE listen address (only when `TRANSPORT=sse` or `LISTEN_ADDR` is set) |
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig file (or in-cluster config when running inside a Pod) |
| `LOG_LEVEL` | `INFO` | Log verbosity: `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `LOG_OUTPUT` | `stderr` | Log destination: `stderr`, `file`, `discard` |
| `LOG_FILE` | — | Log file path (required when `LOG_OUTPUT=file`) |

**Stdio mode:** stdout is reserved for the MCP protocol (logs never go there).

**SSE mode:** the server listens on the given address and serves MCP over HTTP/SSE. Multiple clients can connect concurrently.

## Development

### Available Tasks

```
task generate        # easyp mod download + easyp generate
task lint            # easyp lint
task build           # go build -o deckhouse-harness ./cmd/deckhouse-harness
task test            # go test ./...
task docker:build    # build Docker image (deckhouse-harness:local)
task docker:load     # load Docker image into Kind cluster
task integration     # full integration test cycle (requires Kind)
```

### Proto-First Workflow

All MCP tools are defined in `.proto` files under `proto/deckhouse/v1/`:

| File | Service | Purpose |
|------|---------|---------|
| `diagnostics.proto` | `DiagnosticsAPI` | Read-only cluster status, nodes, pods, logs, events |
| `modules.proto` | `ModulesAPI` | ModuleConfig management, enable/disable, settings |
| `releases.proto` | `ReleasesAPI` | DeckhouseRelease listing and approval |
| `nodes.proto` | `NodesAPI` | Node provisioning, drain/cordon, NodeGroup |
| `config.proto` | `ConfigAPI` | Cluster configuration, Kubernetes version |
| `sources.proto` | `SourcesAPI` | ModuleSource, ModuleUpdatePolicy, ModuleRelease |

After editing `.proto` files, regenerate:

```bash
task generate
```

### Implementing Handlers

Each handler implements a generated `*ToolHandler` interface:

```go
type DiagnosticsAPIToolHandler interface {
    GetClusterStatus(ctx context.Context, req *emptypb.Empty) (*GetClusterStatusResponse, error)
    ListNodes(ctx context.Context, req *ListNodesRequest) (*ListNodesResponse, error)
    // ...
}
```

Handlers live in `internal/handler/` and receive a `k8s.Client` interface for all Kubernetes operations.

### Testing

```bash
task test    # unit tests (mock k8s.Client, no cluster needed)
```

Tests use a mock `k8s.Client` with function fields — no external mock libraries.

## Project Structure

```
proto/deckhouse/v1/          # .proto files — single source of truth
├── diagnostics.proto        # DiagnosticsAPI (11 RPCs)
├── modules.proto            # ModulesAPI (7 RPCs)
├── releases.proto           # ReleasesAPI (3 RPCs)
├── nodes.proto              # NodesAPI (13 RPCs)
├── config.proto             # ConfigAPI (3 RPCs)
└── sources.proto            # SourcesAPI (6 RPCs)
cmd/deckhouse-harness/main.go    # dual-mode entrypoint (stdio default, SSE via TRANSPORT/LISTEN_ADDR)
internal/handler/            # Tool handler implementations
internal/k8s/client.go       # Kubernetes client interface
deploy/                      # Kubernetes manifests (Deployment, Service, RBAC)
Dockerfile                   # Multi-stage build (golang:1.26 → distroless)
tests/integration/           # Integration tests (Kind + Deckhouse CE, dual transport)
Taskfile.yml                 # Build tasks (go-task)
easyp.yaml                   # Proto config
```

## License

[MIT](LICENSE)
