<!-- generated: 2026-07-07, template: errors.md -->
# Error Handling — Deckhouse Harness

## 1. Error Architecture

```
┌─────────────────────────────────────────────────────┐
│  MCP Tool Layer (*.mcp.go)                           │
│  Propagates error as MCP CallToolResult (isError)    │
├─────────────────────────────────────────────────────┤
│  Handler Layer (internal/handler/)                   │
│  Wraps K8s errors with operation context             │
│  Returns domain-meaningful errors                    │
├─────────────────────────────────────────────────────┤
│  K8s Client Layer (internal/k8s/)                   │
│  Returns raw client-go errors                        │
├─────────────────────────────────────────────────────┤
│  Kubernetes API                                      │
│  API server errors, network errors, not found, etc.  │
└─────────────────────────────────────────────────────┘
```

Errors flow upward. Each handler layer adds context via `fmt.Errorf`. The MCP layer proxies the final error to the AI client as a human-readable string.

## 2. Error Wrapping Convention

All K8s API errors are wrapped with the operation that failed:

```go
// Pattern: "gerund noun: %w"
nodes, err := h.client.ListNodes(ctx)
if err != nil {
    return nil, fmt.Errorf("listing nodes: %w", err)
}

nodeGroups, err := h.client.ListNodeGroups(ctx)
if err != nil {
    return nil, fmt.Errorf("listing node groups: %w", err)
}
```

**Naming convention:** `"<verb>ing <resource>: %w"` — always lowercase, gerund form, colon space `%w`.

Examples:
- `"listing nodes: ..."` — `ListNodes`
- `"listing node groups: ..."` — `ListNodeGroups`
- `"creating static instance: ..."` — `CreateStaticInstance`
- `"creating ssh credentials: ..."` — `CreateSSHCredentials`
- `"getting static instance: ..."` — `GetStaticInstance`

## 3. Validation Errors

Input validation is done at handler entry before any K8s call:

```go
func (h *NodesHandler) CreateSSHCredentials(ctx context.Context, req *pb.CreateSSHCredentialsRequest) (*pb.CreateSSHCredentialsResponse, error) {
    if req.PrivateKey == "" {
        return nil, fmt.Errorf("privateKey is required")
    }
    // ...
}
```

No custom error types — plain `fmt.Errorf` with a descriptive message.

## 4. Composite Handler Errors

For `AddWorkerNode` (multi-step): if step N fails, the error message includes what was already created:

```go
// Step 1 success, step 2 fails:
return nil, fmt.Errorf(
    "creating static instance (sshCredentials %q already created): %w",
    sshCredsName, err,
)
```

This allows the AI agent to inform the user about partially created resources.

## 5. Polling Timeout

Polling handlers (`AddWorkerNode`, future `WaitNodeReady`) do not return an error on timeout. Instead, they return the last observed state with `timedOut: true`:

```go
// Timeout reached — not an error, but a status signal
return &pb.AddWorkerNodeResponse{
    SshCredentialsName: sshCredsName,
    StaticInstanceName: siName,
    Phase:              lastPhase,
    TimedOut:           true,
}, nil
```

The AI agent can decide whether to retry or report the situation to the user.

## 6. Unstructured Field Missing

When reading CRD fields from `unstructured.Unstructured`, missing fields are **not errors**. Use the helper pattern:

```go
phase, found, err := unstructuredNestedString(obj.Object, "status", "currentStatus", "phase")
if err != nil {
    return nil, fmt.Errorf("reading static instance phase: %w", err)
}
if !found {
    phase = ""  // treat as zero value, not an error
}
```

Never panic on missing unstructured fields.

## 7. Actionable Errors from Raw K8s API Errors

Where a raw client-go error is opaque, the client layer rewrites it into an actionable message the AI agent can act on. `ListNodeGroups` and `ListStaticInstances` in `internal/k8s/client.go` special-case the "CRD not registered" API error (returned when an optional module is disabled) into a message naming the GVR and the module to enable:

```go
// When the deckhouse.io/v1/nodegroups CRD is not served by the API:
return nil, fmt.Errorf(
    "CRD deckhouse.io/v1/nodegroups not registered (is node-manager module enabled?)",
)
```

This turns a generic "the server could not find the requested resource" into a hint that the optional `node-manager` module needs enabling. Integration tests that hit these tools skip (exit 77) when the CRD is absent rather than failing.

**Single-wrap rule.** The client adds *only* the actionable hint (or, for the generic case, returns the error unprefixed); the calling handler adds the operation prefix exactly once. So a missing-CRD failure reads `listing node groups: CRD deckhouse.io/v1/nodegroups not registered (is node-manager module enabled?): …` — not the doubled `listing node groups: listing node groups: …` the two layers used to produce.

**Graceful degradation.** `k8s.IsCRDNotRegistered(err)` reports whether an error is this missing-CRD case (via `kerrors.IsNotFound` or the discovery message). Aggregate handlers use it to stay useful when an optional module is off: `GetClusterStatus` collects node-group status through this check and returns an empty `nodeGroups` list — plus the still-valid node counts, unhealthy-pod count and Deckhouse version — instead of failing the whole call.

## 8. Error Propagation to MCP Client

The MCP Go SDK converts returned Go errors into MCP `CallToolResult` with `isError: true`. The `content` is the `.Error()` string of the returned error, which includes the full wrapped chain:

```
"listing nodes: dial tcp: connection refused"
"listing node groups: the server could not find the requested resource"
"privateKey is required"
```

No structured error codes are exposed at the MCP protocol level. The AI agent reads the plain text to understand the problem.
