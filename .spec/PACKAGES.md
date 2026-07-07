<!-- generated: 2026-07-07, template: core.md -->
# Packages Reference — Deckhouse Harness

## Application Layer

### `cmd/deckhouse-harness`

**Entry point and wiring.** Creates all dependencies and starts the server in one of two transport modes.

| File | Description |
|------|-------------|
| `main.go` | K8s config (in-cluster or kubeconfig), MCP server, handler registration, dual transport (stdio default for local, SSE HTTP for in-cluster), graceful shutdown |

Key calls:
- `k8s.New(cfg)` — builds the K8s client
- `pb.Register*Tools(server, handler)` — registers MCP tools (6 calls: Diagnostics, Modules, Releases, Nodes, Config, Sources)
- stdio transport (default) — MCP over stdin/stdout for local use
- `mcp.NewSSEHandler(...)` — wraps MCP server for HTTP/SSE transport (in-cluster)
- `httpServer.Shutdown(ctx)` — 30s graceful shutdown

---

## Handler Layer

### `internal/handler`

**MCP tool handler implementations.** Each file implements a generated `*APIToolHandler` interface.

| File | Description |
|------|-------------|
| `diagnostics.go` | `DiagnosticsHandler` (11) — `GetClusterStatus`, `ListNodes`, `ListNodeGroups`, `ListStaticInstances`, `ListUnhealthyPods`, `GetNode`, `GetNodeGroup`, `GetDeckhouseLogs`, `GetNodeEvents`, `GetStaticInstance`, `GetPodLogs` + helpers |
| `modules.go` | `ModulesHandler` (7) — `ListModuleConfigs`, `GetModuleConfig`, `EnableModule`, `DisableModule`, `ListModules`, `UpdateModuleSettings`, `SetModuleMaintenance` |
| `releases.go` | `ReleasesHandler` (3) — `ListDeckhouseReleases`, `GetDeckhouseRelease`, `ApproveRelease` |
| `nodes.go` | `NodesHandler` (13) — `CreateSSHCredentials`, `DeleteSSHCredentials`, `CreateStaticInstance`, `DeleteStaticInstance`, `AddWorkerNode` (composite, polls), `RemoveNode` (composite: cordon+drain+delete), `CreateNodeGroup`, `DeleteNodeGroup`, `WaitNodeReady` (polling), `CordonNode`, `UncordonNode`, `DrainNode` (composite), `CreateNodeGroupConfiguration` |
| `config.go` | `ConfigHandler` (3) — `GetClusterConfiguration`, `GetStaticClusterConfiguration`, `UpdateKubernetesVersion` |
| `sources.go` | `SourcesHandler` (6) — `ListModuleSources`, `CreateModuleSource`, `DeleteModuleSource`, `ListModuleUpdatePolicies`, `CreateModuleUpdatePolicy`, `ListModuleReleases` |
| `mock_client_test.go` | `mockClient` — function-field test double for `k8s.Client` (36 function fields) |
| `diagnostics_test.go` | Unit tests for `DiagnosticsHandler` |
| `modules_test.go` | Unit tests for `ModulesHandler` |
| `releases_test.go` | Unit tests for `ReleasesHandler` |
| `nodes_test.go` | Unit tests for `NodesHandler` |
| `config_test.go` | Unit tests for `ConfigHandler` |
| `sources_test.go` | Unit tests for `SourcesHandler` |
| `errors_test.go` | Unit tests for error cases |

**Total: 134 unit tests** across test files.

Key patterns:
- Implements generated interface (e.g., `pb.DiagnosticsAPIToolHandler`)
- All K8s calls through `k8s.Client` field
- Helpers for unstructured fields: `unstructuredNestedString`, `unstructuredNestedInt64`, etc.
- `pollInterval = 30s`, `defaultTimeoutSeconds = 900`

---

## K8s Client Layer

### `internal/k8s`

**Kubernetes API abstraction.** Isolates all `client-go` usage behind an interface.

| File | Description |
|------|-------------|
| `client.go` | `Client` interface (36 methods: 11 core + 25 CRD), `client` struct, GVR constants, all K8s method implementations |

`Client` interface — current methods:
```go
// Core resources (typed) — 11
ListNodes(ctx) ([]corev1.Node, error)
GetNode(ctx, name) (*corev1.Node, error)
CordonNode(ctx, name) error
UncordonNode(ctx, name) error
ListPods(ctx, namespace) ([]corev1.Pod, error)
DeletePod(ctx, namespace, name) error
EvictPod(ctx, namespace, name) error
ListNodeEvents(ctx, nodeName) ([]corev1.Event, error)
GetPodLogs(ctx, namespace, pod, container, tail, since) (string, error)
GetSecret(ctx, namespace, name) (*corev1.Secret, error)
UpdateSecret(ctx, secret) (*corev1.Secret, error)

// Deckhouse CRDs (dynamic/unstructured) — 25
ListNodeGroups(ctx) ([]unstructured.Unstructured, error)
GetNodeGroup(ctx, name) (*unstructured.Unstructured, error)
CreateNodeGroup(ctx, obj) (*unstructured.Unstructured, error)
DeleteNodeGroup(ctx, name) error
ListStaticInstances(ctx) ([]unstructured.Unstructured, error)
GetStaticInstance(ctx, name) (*unstructured.Unstructured, error)
CreateStaticInstance(ctx, obj) (*unstructured.Unstructured, error)
DeleteStaticInstance(ctx, name) error
ListModuleConfigs(ctx) ([]unstructured.Unstructured, error)
GetModuleConfig(ctx, name) (*unstructured.Unstructured, error)
UpdateModuleConfig(ctx, obj) (*unstructured.Unstructured, error)
PatchModuleConfig(ctx, name, patch) (*unstructured.Unstructured, error)
ListDeckhouseReleases(ctx) ([]unstructured.Unstructured, error)
GetDeckhouseRelease(ctx, name) (*unstructured.Unstructured, error)
PatchDeckhouseRelease(ctx, name, patch) (*unstructured.Unstructured, error)
CreateSSHCredentials(ctx, obj) (*unstructured.Unstructured, error)
DeleteSSHCredentials(ctx, name) error
ListModules(ctx) ([]unstructured.Unstructured, error)
ListModuleSources(ctx) ([]unstructured.Unstructured, error)
CreateModuleSource(ctx, obj) (*unstructured.Unstructured, error)
DeleteModuleSource(ctx, name) error
ListModuleUpdatePolicies(ctx) ([]unstructured.Unstructured, error)
CreateModuleUpdatePolicy(ctx, obj) (*unstructured.Unstructured, error)
ListModuleReleases(ctx) ([]unstructured.Unstructured, error)
CreateNodeGroupConfiguration(ctx, obj) (*unstructured.Unstructured, error)
```

Note: `ListNodeGroups`/`ListStaticInstances` return an actionable error
`"CRD deckhouse.io/v1/nodegroups not registered (is node-manager module enabled?)"`
when the CRD is absent (e.g. the node-manager module is disabled).

GVR constants (10 Deckhouse CRDs):
```go
NodeGroupGVR              // deckhouse.io/v1/nodegroups
StaticInstanceGVR         // deckhouse.io/v1alpha2/staticinstances
SSHCredentialsGVR         // deckhouse.io/v1alpha2/sshcredentials
ModuleConfigGVR           // deckhouse.io/v1alpha1/moduleconfigs
DeckhouseReleaseGVR       // deckhouse.io/v1alpha1/deckhouserelease
ModuleGVR                 // deckhouse.io/v1alpha1/modules
ModuleSourceGVR           // deckhouse.io/v1alpha1/modulesources
ModuleUpdatePolicyGVR     // deckhouse.io/v1alpha1/moduleupdatepolicies
ModuleReleaseGVR          // deckhouse.io/v1alpha1/modulereleases
NodeGroupConfigurationGVR // deckhouse.io/v1alpha1/nodegroupconfigurations
```

---

## Proto / Generated Layer

### `proto/deckhouse/v1`

**Source of truth for MCP tools.** Do not manually edit `*.pb.go` or `*.mcp.go`.

| File | Description |
|------|-------------|
| `diagnostics.proto` | Block A: `DiagnosticsAPI` service — 11 RPCs |
| `diagnostics.pb.go` | Generated protobuf types for diagnostics |
| `diagnostics.mcp.go` | Generated: `DiagnosticsAPIToolHandler` interface + `RegisterDiagnosticsAPITools()` |
| `modules.proto` | Block B: `ModulesAPI` — 7 RPCs |
| `modules.pb.go` | Generated types |
| `modules.mcp.go` | Generated: `ModulesAPIToolHandler` + registration |
| `releases.proto` | Block C: `ReleasesAPI` — 3 RPCs |
| `releases.pb.go` | Generated types |
| `releases.mcp.go` | Generated: `ReleasesAPIToolHandler` + registration |
| `nodes.proto` | Block D: `NodesAPI` — 13 RPCs |
| `nodes.pb.go` | Generated types |
| `nodes.mcp.go` | Generated: `NodesAPIToolHandler` + registration |
| `config.proto` | Block E: `ConfigAPI` — 3 RPCs |
| `config.pb.go` | Generated types |
| `config.mcp.go` | Generated: `ConfigAPIToolHandler` + registration |
| `sources.proto` | Block F: `SourcesAPI` — 6 RPCs |
| `sources.pb.go` | Generated types |
| `sources.mcp.go` | Generated: `SourcesAPIToolHandler` + registration |

Regenerate everything: `task generate` (runs `easyp mod download && easyp generate`).

---

## Deploy / Integration

### `deploy/`

| File | Description |
|------|-------------|
| `deployment.yaml` | K8s Deployment — 1 replica, `d8-system` namespace, resource limits |
| `rbac.yaml` | ServiceAccount + ClusterRole + ClusterRoleBinding (all P0–P3 permissions) |
| `service.yaml` | K8s Service (ClusterIP, port 8080) |

### `tests/integration/`

| File | Description |
|------|-------------|
| `setup.sh` | Creates Kind cluster, loads CRDs, applies fixtures |
| `test.sh` | Sends MCP tool calls, validates responses |
| `teardown.sh` | Deletes Kind cluster |
| `crds.yaml` | Deckhouse CRD definitions for local testing |
| `fixtures.yaml` | Sample K8s resources (nodes, nodegroups, etc.) |
