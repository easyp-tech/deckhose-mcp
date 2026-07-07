<!-- generated: 2026-07-07, template: development.md -->
# Testing Conventions — Deckhouse Harness

## 1. Test Package Naming

Tests are in the **same package** as the code under test (`package handler`), giving access to unexported helpers:

```go
package handler  // internal/handler/diagnostics_test.go
```

## 2. Test File Structure

Each handler has a dedicated test file:

| File | Coverage |
|------|----------|
| `diagnostics_test.go` | 35 tests for `DiagnosticsHandler` |
| `modules_test.go` | 21 tests for `ModulesHandler` |
| `releases_test.go` | 7 tests for `ReleasesHandler` |
| `nodes_test.go` | 42 tests for `NodesHandler` |
| `config_test.go` | 9 tests for `ConfigHandler` |
| `sources_test.go` | 17 tests for `SourcesHandler` |
| `errors_test.go` | 3 tests for K8s error propagation |
| `mock_client_test.go` | `mockClient` definition (shared across all test files) |

**Total: 134 unit tests.**

## 3. Mock Client

All tests use `mockClient` — a struct with function fields, one per `k8s.Client`
method (36 function fields covering all 11 core + 25 CRD methods):

```go
// mock_client_test.go (abbreviated — 36 fields total)
type mockClient struct {
    listNodesFunc             func(ctx context.Context) ([]corev1.Node, error)
    listPodsFunc              func(ctx context.Context, namespace string) ([]corev1.Pod, error)
    listNodeGroupsFunc        func(ctx context.Context) ([]unstructured.Unstructured, error)
    listStaticInstancesFunc   func(ctx context.Context) ([]unstructured.Unstructured, error)
    getStaticInstanceFunc     func(ctx context.Context, name string) (*unstructured.Unstructured, error)
    createStaticInstanceFunc  func(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
    listModuleConfigsFunc     func(ctx context.Context) ([]unstructured.Unstructured, error)
    listDeckhouseReleasesFunc func(ctx context.Context) ([]unstructured.Unstructured, error)
    createSSHCredentialsFunc  func(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
    listModuleSourcesFunc     func(ctx context.Context) ([]unstructured.Unstructured, error)
    // ... one field per remaining k8s.Client method (36 total)
}
```

Nil function field = no-op (returns zero value, nil error). This means you only set the functions you care about:

```go
mc := &mockClient{
    listNodesFunc: func(_ context.Context) ([]corev1.Node, error) {
        return []corev1.Node{makeNode("node-1", true)}, nil
    },
}
h := NewDiagnosticsHandler(mc)
```

**Compile-time check:**
```go
var _ k8s.Client = (*mockClient)(nil)
```

**When adding a new K8s Client method:** add the corresponding function field to `mockClient` and implement the method using the same nil-check pattern.

## 4. Key Patterns

### Simple handler test (no K8s calls configured)

```go
func TestGetClusterStatus_Empty(t *testing.T) {
    h := NewDiagnosticsHandler(&mockClient{})
    resp, err := h.GetClusterStatus(context.Background(), &emptypb.Empty{})
    if err != nil {
        t.Fatal(err)
    }
    if resp.Nodes.Total != 0 {
        t.Errorf("expected total=0, got %d", resp.Nodes.Total)
    }
}
```

### Handler test with mock data

```go
func TestListNodes_Basic(t *testing.T) {
    mc := &mockClient{
        listNodesFunc: func(_ context.Context) ([]corev1.Node, error) {
            return []corev1.Node{
                makeNode("node-1", true),
                makeNode("node-2", false),
            }, nil
        },
    }
    h := NewDiagnosticsHandler(mc)
    resp, err := h.ListNodes(context.Background(), &pb.ListNodesRequest{})
    if err != nil {
        t.Fatal(err)
    }
    if len(resp.Nodes) != 2 {
        t.Errorf("expected 2 nodes, got %d", len(resp.Nodes))
    }
}
```

### Error propagation test

```go
func TestListNodes_K8sError(t *testing.T) {
    mc := &mockClient{
        listNodesFunc: func(_ context.Context) ([]corev1.Node, error) {
            return nil, errors.New("k8s error")
        },
    }
    h := NewDiagnosticsHandler(mc)
    _, err := h.ListNodes(context.Background(), &pb.ListNodesRequest{})
    if err == nil {
        t.Fatal("expected error, got nil")
    }
}
```

### Test helper functions

Short factory functions for test data — named `make{Type}(...)`:

```go
func makeNode(name string, ready bool) corev1.Node { ... }
func makeNodeGroup(name string, ready, total int64) unstructured.Unstructured { ... }
func makeStaticInstance(name, phase string) unstructured.Unstructured { ... }
func makeModuleConfig(name, status string, enabled bool) unstructured.Unstructured { ... }
func makeRelease(name, version, phase string) unstructured.Unstructured { ... }
```

## 5. Polling Tests

`AddWorkerNode`/`WaitNodeReady` tests use real `time.Sleep` via `pollInterval`
(30s clock). These are slow by design:

```go
// Each polling test takes ~30s (one poll cycle)
// Full suite runtime (134 tests): ~3 minutes, dominated by polling scenarios
```

Do not use `time.AfterFunc` or fake clocks — the current design trades test speed for simplicity.

## 6. Proto Request Construction

Use the generated proto request types directly:

```go
resp, err := h.CreateSSHCredentials(ctx, &pb.CreateSSHCredentialsRequest{
    Name:       "my-ssh-creds",
    User:       "ubuntu",
    PrivateKey: "-----BEGIN RSA PRIVATE KEY-----\n...",
})
```

Optional proto fields use pointer wrappers or `optional` fields as generated.

## 7. Running Tests

```bash
# All tests
go test ./...

# Handler tests only (skip slow polling with -short if needed)
go test ./internal/handler/...

# Specific test
go test ./internal/handler/ -run TestGetClusterStatus -v

# With race detector
go test -race ./...
```
