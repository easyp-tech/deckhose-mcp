<!-- generated: 2026-07-07, template: core.md -->
# Code Style — Deckhouse Harness

Project-specific Go and Proto conventions beyond standard `gofmt`.

## Go

### Package structure
- Each handler service = one file: `internal/handler/{service}.go`
- Exported types only: handler structs and constructors
- Unexported helpers (e.g., `unstructuredNestedString`) defined at package level, shared across files

### Constructor pattern
```go
type DiagnosticsHandler struct {
    client k8s.Client
}

func NewDiagnosticsHandler(client k8s.Client) *DiagnosticsHandler {
    return &DiagnosticsHandler{client: client}
}
```
No option structs, no variadic options — just the client.

### Unstructured field access
Use package-level helpers, never raw `obj.Object["key"]` chains:
```go
name, found, err := unstructuredNestedString(obj.Object, "metadata", "name")
count, found, err := unstructuredNestedInt64(obj.Object, "status", "ready")
```
Always check `found` before using the value.

### Error wrapping
Every K8s call is wrapped with operation context:
```go
nodes, err := h.client.ListNodes(ctx)
if err != nil {
    return nil, fmt.Errorf("listing nodes: %w", err)
}
```
Pattern: `"<verb>ing <resource>: %w"` — gerund form, lowercase, colon + `%w`.

### Base64 encoding
Always encode inside the handler, never accept pre-encoded from client:
```go
encodedKey := base64.StdEncoding.EncodeToString([]byte(req.PrivateKey))
```

### Unstructured object creation
Build the full `map[string]interface{}` inline:
```go
obj := &unstructured.Unstructured{
    Object: map[string]interface{}{
        "apiVersion": "deckhouse.io/v1alpha2",
        "kind":       "SSHCredentials",
        "metadata": map[string]interface{}{
            "name": req.Name,
        },
        "spec": map[string]interface{}{...},
    },
}
```

### Constants
Group related constants in a single `const` block:
```go
const (
    defaultSSHPort        = 22
    defaultTimeoutSeconds = 900
    pollInterval          = 30 * time.Second
)
```

### GVR constants
In `internal/k8s/client.go`, define as package-level `var`:
```go
var NodeGroupGVR = schema.GroupVersionResource{
    Group:    "deckhouse.io",
    Version:  "v1",
    Resource: "nodegroups",
}
```

### Idempotent write patterns
For enable/disable/approve operations, read current state first, return `previousState`:
```go
func (h *ModulesHandler) EnableModule(ctx context.Context, req *pb.EnableModuleRequest) (*pb.EnableModuleResponse, error) {
    mc, err := h.client.GetModuleConfig(ctx, req.Name)
    // ... read current enabled state ...
    // ... update to enabled=true ...
    return &pb.EnableModuleResponse{
        Success:       true,
        PreviousState: wasEnabled,
    }, nil
}
```

### Composite handler error reporting
On multi-step failure, return partial results:
```go
// If step 2 fails after step 1 succeeded
return nil, fmt.Errorf("creating static instance (SSHCredentials %q already created): %w", sshName, err)
```

## Proto

### Service options
```proto
service DiagnosticsAPI {
  option (mcp.options.v1.service) = {
    namespace: "deckhouse"
    description: "..."
  };
```

### Method annotations
Read-only:
```proto
option (mcp.options.v1.method) = {
  annotations: { read_only_hint: true }
};
```
Write:
```proto
option (mcp.options.v1.method) = {
  annotations: { destructive_hint: true }
};
```
Idempotent write:
```proto
option (mcp.options.v1.method) = {
  annotations: { read_only_hint: false, idempotent_hint: true }
};
```

### Field rules
- Required fields: singular (no `optional`) — optional/filter fields: `optional`
- Enums: always include `*_UNSPECIFIED = 0` with `(mcp.options.v1.enum_value) = { hidden: true }`

### File layout
One service per file. File named after the domain block: `diagnostics.proto`, `modules.proto`, etc.

## Testing

### Mock pattern
Function fields, nil = no-op:
```go
type mockClient struct {
    listNodesFunc func(ctx context.Context) ([]corev1.Node, error)
    // ... 36 function fields total ...
}

func (m *mockClient) ListNodes(ctx context.Context) ([]corev1.Node, error) {
    if m.listNodesFunc != nil {
        return m.listNodesFunc(ctx)
    }
    return nil, nil
}
```

### Test helper naming
```go
func makeNode(name string, ready bool) corev1.Node { ... }
func makeNodeGroup(name string, ready, total int) unstructured.Unstructured { ... }
func makeModuleConfig(name string, enabled bool) unstructured.Unstructured { ... }
func makeRelease(name, version, phase string) unstructured.Unstructured { ... }
```

### Compile-time interface check
```go
var _ k8s.Client = (*mockClient)(nil)
```
Always in `mock_client_test.go`.

### Polling test expectations
Tests for `AddWorkerNode`, `WaitNodeReady`, and `DrainNode` use a real 30s clock — expected to be slow (~30s each). The full suite is 134 unit tests, total ~3 min.
