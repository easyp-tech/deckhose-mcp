<!-- generated: 2026-07-07, template: bootstrap.md -->
# Agent Rules — Deckhouse Harness

Mandatory rules for AI agents working on this project.

## Code Style

- Follow standard Go conventions (Effective Go, `gofmt`)
- No `init()` functions
- No global mutable state outside `var` GVR constants in `internal/k8s/client.go`
- Prefer explicit over implicit; no magic
- Keep functions small and focused; extract helpers for repeated unstructured field access

## Naming Conventions

- Handler structs: `{Service}Handler` (e.g., `DiagnosticsHandler`, `NodesHandler`, `ConfigHandler`)
- Constructor: `New{Name}Handler(client k8s.Client) *{Name}Handler`
- K8s client methods: verb + noun (e.g., `ListNodes`, `GetNodeGroup`, `CreateSSHCredentials`, `PatchDeckhouseRelease`)
- Proto-generated interfaces: `{Service}APIToolHandler` — never rename, never embed partial
- GVR constants: `{Resource}GVR` (e.g., `NodeGroupGVR`, `StaticInstanceGVR`)
- Test helpers: `make{TypeName}(...)` (e.g., `makeNode`, `makeNodeGroup`, `makeModuleConfig`, `makeRelease`)

## Error Handling

- Wrap all K8s API errors: `fmt.Errorf("listing nodes: %w", err)`
- Never panic on missing fields in unstructured resources — use the `unstructuredNested*` helpers
- On composite handler failure (e.g., `AddWorkerNode`, `RemoveNode`): abort remaining steps, report what was already created/done
- Polling timeout: return last known state + `timedOut: true` field
- Missing resource → return `not found` wrapped error, never nil + nil

## Testing

- Use standard `testing` package only — no testify, gomock, or other frameworks
- Mock `k8s.Client` via `mockClient` struct in `mock_client_test.go` (36 function fields, nil = no-op)
- Test files are in the same package (`package handler`)
- Table-driven tests via `map[string]struct{ ... }` or named `[]struct` slices
- Polling tests (`AddWorkerNode`, `WaitNodeReady`, `DrainNode`) use a real 30s clock — expected to be slow (~30s each)
- Always add `var _ k8s.Client = (*mockClient)(nil)` compile-time check when adding methods to mock
- Total: 134 unit tests, ~3 min runtime

## Dependencies

- Never add external test frameworks (testify, gomock, etc.)
- Add K8s client methods to `internal/k8s/client.go` `Client` interface before using them
- Do not call K8s API directly in handlers — always go through `k8s.Client`
- New proto deps: add to `easyp.yaml` and run `task generate`

## Formatting

- Run `gofmt` / `goimports` before commit
- Proto files: run `easyp lint` before commit
- `task generate` must succeed without errors after any proto change
- Secrets (SSH keys, sudo passwords): base64-encode inside handler, never accept pre-encoded input from client

## Proto Conventions

- Each proto service block = one `.proto` file
- Required fields: singular (no `optional`) — optional/filter fields: `optional`
- Read-only tools: `annotations: { read_only_hint: true }`
- Destructive tools: `annotations: { destructive_hint: true }`
- Idempotent write tools: `annotations: { read_only_hint: false, idempotent_hint: true }`
- Enum zero-value `*_UNSPECIFIED = 0` always hidden via `(mcp.options.v1.enum_value) = { hidden: true }`
- Service namespace: `"deckhouse"` (tool names become `deckhouse_MethodName`)

## RBAC

- After adding any K8s operation, update `deploy/rbac.yaml` with the minimum required permissions
- Use least-privilege: add only the specific verbs needed (get/list/create/update/patch/delete)
- Never add wildcard verbs (`*`) or wildcard resources
- Current RBAC in `deploy/rbac.yaml` covers all 43 tools (P0–P3) — expand `deploy/rbac.yaml` with least-privilege permissions for the resources each new tool touches

## Handler Registration

- Handlers are auto-registered via generated `pb.Register{Service}Tools()` — 6 calls in `main.go`
- Do NOT manually register tools — always go through the generated registration function
- Currently registered: `DiagnosticsAPI`, `ModulesAPI`, `ReleasesAPI`, `NodesAPI`, `ConfigAPI`, `SourcesAPI` (43 tools across 6 services)
