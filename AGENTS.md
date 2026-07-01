# Deckhouse MCP Server

MCP server for managing [Deckhouse Kubernetes Platform](https://deckhouse.ru/docs) (Community Edition).
**Dual transport** — stdio (default) or SSE over HTTP. Stdio: local newline-delimited JSON. SSE: `LISTEN_ADDR=:8080` or `-listen :8080` starts `mcp.NewSSEHandler` + HTTP server.
Authenticates to Kubernetes via in-cluster config (inside a Pod) or `~/.kube/config` / `KUBECONFIG` (local).

## Tech Stack

- **Go 1.26** — primary language
- **Protobuf + protoc-gen-mcp v0.5.0** — proto-first MCP tool generation
- **MCP Go SDK v1.6.0** (`github.com/modelcontextprotocol/go-sdk`) — MCP server (stdio + SSE/Streamable HTTP transports)
- **easyp** — linting, code generation, proto dependency management
- **Kubernetes client-go v0.35.3** — typed client for core resources, dynamic client for CRDs
- **Deckhouse CRDs** — `NodeGroup` (v1), `StaticInstance` (v1alpha2), `SSHCredentials` (v1alpha2), `ModuleConfig` (v1alpha1), `DeckhouseRelease` (v1alpha1)
- **Module**: `github.com/easyp-tech/deckhouse-harness`

## Architecture

```
proto/                           # .proto files — single source of truth for all MCP tools
├── deckhouse/v1/                # services, messages, generated code
│   ├── diagnostics.proto        # Block A: DiagnosticsAPI (5 RPCs, read-only)
│   ├── diagnostics.pb.go        # generated: protobuf types
│   ├── diagnostics.mcp.go       # generated: MCP tool handler interface + registration
│   ├── modules.proto            # Block B: ModulesAPI (1 RPC)
│   ├── releases.proto           # Block C: ReleasesAPI (1 RPC)
│   ├── nodes.proto              # Block D: NodesAPI (3 RPCs, write)
│   ├── config.proto             # Block E: ConfigAPI (stub, no RPCs yet)
│   └── sources.proto            # Block F: SourcesAPI (stub, no RPCs yet)
cmd/
└── deckhouse-harness/
    └── main.go                  # Dual-mode (stdio default + SSE via TRANSPORT=sse / LISTEN_ADDR / -listen)
internal/
├── handler/                     # ToolHandler interface implementations
    └── k8s/
    └── client.go                # Client interface + typed/dynamic implementation
deploy/                          # Kubernetes manifests for SSE in-cluster deployment
├── deployment.yaml              # Deployment in d8-system, TRANSPORT=sse, containerPort 8080
├── service.yaml                 # ClusterIP Service, port 8080
└── rbac.yaml                    # ServiceAccount + ClusterRole + ClusterRoleBinding (all 43 tools)
Dockerfile                       # Multi-stage: golang:1.26 → gcr.io/distroless/static-debian12
Taskfile.yml                     # go-task: generate, lint, build, test, docker:build, docker:load
easyp.yaml                       # Proto deps, lint rules, codegen plugins
```

## Implementation Status

### Implemented (P0 — MVP, 10 handlers)

| Handler | Block | Proto RPC | Type |
|---------|-------|-----------|------|
| `GetClusterStatus` | A: Diagnostics | `DiagnosticsAPI.GetClusterStatus` | read-only |
| `ListNodes` | A: Diagnostics | `DiagnosticsAPI.ListNodes` | read-only |
| `ListNodeGroups` | A: Diagnostics | `DiagnosticsAPI.ListNodeGroups` | read-only |
| `ListStaticInstances` | A: Diagnostics | `DiagnosticsAPI.ListStaticInstances` | read-only |
| `ListUnhealthyPods` | A: Diagnostics | `DiagnosticsAPI.ListUnhealthyPods` | read-only |
| `ListModuleConfigs` | B: Modules | `ModulesAPI.ListModuleConfigs` | read-only |
| `ListDeckhouseReleases` | C: Releases | `ReleasesAPI.ListDeckhouseReleases` | read-only |
| `CreateSSHCredentials` | D: Nodes | `NodesAPI.CreateSSHCredentials` | write |
| `CreateStaticInstance` | D: Nodes | `NodesAPI.CreateStaticInstance` | write |
| `AddWorkerNode` | D: Nodes | `NodesAPI.AddWorkerNode` | write (composite) |

### Not Yet Implemented

Full spec: [mcp-handlers-full.md](mcp-handlers-full.md). Next priority: P1 → P2 → P3.

| Block | Purpose | P0 | P1 | P2 | P3 |
|-------|---------|----|----|----|----|
| A: Diagnostics | read-only cluster status, nodes, pods, logs | ✅ 4 | 2 | 4 | 1 |
| B: Modules | ModuleConfig CRUD, enable/disable | ✅ 1 | 3 | 2 | 1 |
| C: Releases | DeckhouseRelease, approve | ✅ 1 | 2 | — | — |
| D: Nodes | StaticInstance, SSHCredentials, drain/cordon, NodeGroup | ✅ 4 | 4 | 4 | 1 |
| E: Configuration | ClusterConfiguration, K8s version | — | 1 | 2 | — |
| F: Sources | ModuleSource, ModuleUpdatePolicy | — | — | 4 | 2 |

## Build & Generate

```bash
# Install easyp
brew install easyp-tech/tap/easyp

# Download proto dependencies
easyp mod download

# Lint proto files
easyp lint

# Generate *.pb.go + *.mcp.go
easyp generate

# Build
go build -o deckhouse-harness ./cmd/deckhouse-harness

# Test (38 tests, ~60s due to polling tests)
go test ./...

# All-in-one via Taskfile (go-task)
task generate        # easyp mod download && easyp generate
task lint            # easyp lint
task build           # go build -o deckhouse-harness ./cmd/deckhouse-harness
task test            # go test ./...
task docker:build    # docker build -t deckhouse-mcp:local .
task docker:load     # kind load docker-image deckhouse-mcp:local --name d8
task integration     # setup → test → teardown
```

## Conventions

### Proto

- Each block (A–F) is a separate `.proto` file with one `service`
- Service namespace = `deckhouse` (tool naming: `deckhouse_GetClusterStatus`)
- Read-only handlers: `annotations: { read_only_hint: true }`
- Write/destructive handlers: `annotations: { destructive_hint: true }` or `{ read_only_hint: false }`
- Required fields — singular without `optional`; filters — `optional`
- Enum zero-value `*_UNSPECIFIED = 0` hidden via `(mcp.options.v1.enum_value) = { hidden: true }`
- Generated interfaces: `DiagnosticsAPIToolHandler`, `ModulesAPIToolHandler`, `ReleasesAPIToolHandler`, `NodesAPIToolHandler`
- Registration: `pb.Register{Service}Tools(server *mcp.Server, impl handler, opts ...mcpruntime.RegisterOption) error`

### Go

- Handler package: `internal/handler/`
- Each handler file implements the generated `*ToolHandler` interface
- Constructor pattern: `New{Name}Handler(client k8s.Client) *{Name}Handler`
- K8s Client interface in `internal/k8s/client.go` — all K8s operations go through this interface, never directly
- Typed client for core resources (nodes, pods); dynamic client for Deckhouse CRDs (unstructured)
- Composite handler `AddWorkerNode`: SSHCredentials → StaticInstance → polling with 30s interval
- Secrets (SSH keys, sudo password) — base64-encode inside the handler, never accept base64 from the client
- Tests: standard Go `testing` package, mock `k8s.Client` with function fields, no external test frameworks

### K8s Client Interface

```go
type Client interface {
    ListNodes(ctx context.Context) ([]corev1.Node, error)
    ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error)
    ListNodeGroups(ctx context.Context) ([]unstructured.Unstructured, error)
    ListStaticInstances(ctx context.Context) ([]unstructured.Unstructured, error)
    GetStaticInstance(ctx context.Context, name string) (*unstructured.Unstructured, error)
    CreateStaticInstance(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
    ListModuleConfigs(ctx context.Context) ([]unstructured.Unstructured, error)
    ListDeckhouseReleases(ctx context.Context) ([]unstructured.Unstructured, error)
    CreateSSHCredentials(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
}
```

New methods should be added to this interface when implementing P1+ handlers.

### CRD GVR Constants

| CRD | Group | Version | Resource (plural) |
|-----|-------|---------|-------------------|
| NodeGroup | deckhouse.io | v1 | nodegroups |
| StaticInstance | deckhouse.io | v1alpha2 | staticinstances |
| SSHCredentials | deckhouse.io | v1alpha2 | sshcredentials |
| ModuleConfig | deckhouse.io | v1alpha1 | moduleconfigs |
| DeckhouseRelease | deckhouse.io | v1alpha1 | deckhouserelease |

### Server Entrypoint (`cmd/deckhouse-harness/main.go`)

- `loadKubeConfig()` — tries `rest.InClusterConfig()` first, falls back to `clientcmd` (`KUBECONFIG` env or `~/.kube/config`). Works for both modes: stdio local (in-cluster fails → kubeconfig) and SSE in-cluster (in-cluster succeeds).
- `configureLogger()` — builds `*slog.Logger` from `LOG_LEVEL` / `LOG_OUTPUT` / `LOG_FILE` env vars; logs never go to stdout (reserved for MCP protocol); default: stderr + INFO
- CLI: `urfave/cli/v3` (see `main()`): `--listen` / `--transport` flags + `Sources: cli.EnvVars("LISTEN_ADDR")` / `cli.EnvVars("TRANSPORT", "MCP_TRANSPORT")`.
- Transport selection (inside Action → `run(c *cli.Command)`):
  - `TRANSPORT=stdio` (default, including unset): `server.Run(ctx, &mcp.StdioTransport{})`
  - `TRANSPORT=sse` (or `LISTEN_ADDR` set): `serveSSE(...)` using `mcp.NewSSEHandler` + `http.Server` with graceful `Shutdown` (10s timeout, `ReadHeaderTimeout` 5s)
- `serveSSE(...)` reuses one `*mcp.Server` for N concurrent SSE sessions.
- `signal.NotifyContext(SIGINT, SIGTERM)` for graceful shutdown via context cancellation
- Constants: `defaultListenAddr=":8080"`, `shutdownTimeout=10s`, `readHeaderTimeout=5s`
- Handlers registered via generated `pb.Register{Service}Tools(server, handler)`

### RBAC

When `TRANSPORT=sse` and running inside a Kubernetes Pod (in-cluster config), the server needs a ServiceAccount with permissions for the resources it manages. The full RBAC manifests are in `deploy/rbac.yaml` (ServiceAccount + ClusterRole + ClusterRoleBinding). The required permissions cover all 43 tools:

- **read**: `nodes`, `pods`, `events` (core); `nodegroups`, `staticinstances`, `moduleconfigs`, `deckhousereleases`, `modules`, `modulesources`, `moduleupdatepolicies`, `modulereleases` (deckhouse.io CRDs)
- **write**: `staticinstances`, `sshcredentials` (create); `nodes` (update/patch for cordon/uncordon); `pods/eviction` (create for drain); `moduleconfigs` (update/patch); `deckhousereleases` (patch for approve); `nodegroups` (create/delete); `modulesources`, `moduleupdatepolicies` (create); `nodegroupconfigurations` (create); `secrets/d8-cluster-configuration` (get/update)

### Error Handling

- Kubernetes API errors → wrap with `fmt.Errorf("operation: %w", err)` → proxied as MCP tool error
- Timeout in polling handlers → return last known state + `timedOut: true`
- Missing resource → `not found` error, no panic
- Error on step 1 of composite handler → abort remaining steps, report what was already created

### Testing

- 38 unit tests across 5 test files in `internal/handler/`
- Mock `k8s.Client` with function fields (no external mock library)
- `AddWorkerNode` polling tests use mock with `time.Sleep` — each takes ~30s
- Total test time: ~60s

## Skills (`.agents/skills/`)

Three agent skills are installed in the project. Each is auto-invoked by keyword match.

### protobuf-expert-skill

Protocol Buffers expert with deep EasyP CLI knowledge.

- **When**: writing/reviewing `.proto` files, configuring `easyp.yaml`, choosing lint rules, setting up codegen plugins, managing proto deps, detecting breaking changes, debugging easyp errors, protobuf style guide
- **SKILL.md**: `.agents/skills/protobuf-expert-skill/SKILL.md`
- **References** (`.agents/skills/protobuf-expert-skill/references/`):

| File | Topic |
|------|-------|
| `cli-commands.md` | All `easyp` CLI commands and flags |
| `config-reference.md` | Full `easyp.yaml` schema |
| `lint-rules.md` | 42+ lint rules with descriptions |
| `breaking-checks.md` | Breaking change detection rules |
| `installation.md` | Install methods (brew, go install, binary) |
| `migration-from-buf.md` | Migrate from `buf.build` to EasyP |
| `ci-cd-integration.md` | CI/CD setup (GitHub Actions, GitLab) |
| `protobuf-best-practices.md` | Proto API design best practices |
| `troubleshooting.md` | Common errors and fixes |

- **Assets** (`.agents/skills/protobuf-expert-skill/assets/`): starter `easyp.yaml` configs — `easyp-minimal.yaml`, `easyp-strict.yaml`, `easyp-go-grpc.yaml`

### protoc-gen-mcp-skill

Build MCP servers from protobuf definitions using `protoc-gen-mcp` and EasyP.

- **When**: creating MCP server, generating MCP tools from proto, building proto-first MCP server in Go, adding MCP annotations to services, implementing MCP tool handlers, ProtoJSON-based MCP tools
- **Keywords**: `protoc-gen-mcp`, `mcp proto`, `proto mcp server`, `easyp mcp`
- **SKILL.md**: `.agents/skills/protoc-gen-mcp-skill/SKILL.md`
- **References** (`.agents/skills/protoc-gen-mcp-skill/references/`):

| File | Topic |
|------|-------|
| `options-reference.md` | All MCP proto annotation options (`mcp.options.v1.*`) |
| `schema-mapping.md` | Proto type → JSON Schema mapping rules |

### spec-driven-dev

6-phase spec-driven development pipeline with human approval gates.

- **When**: structured feature development, spec-first approach, "add feature X", "new feature", "implement", "build"
- **Pipeline**: `Explore → [APPROVE] → Requirements → [APPROVE] → Design → [APPROVE] → Task Plan → [APPROVE] → Implementation → [APPROVE] → Review → [APPROVE] → Done`
- **SKILL.md**: `.agents/skills/spec-driven-dev/SKILL.md`
- **State script**: `sh .agents/skills/spec-driven-dev/scripts/pipeline.sh [--feature <name>] <command>`
  - `status` — current phase & progress
  - `init [--branch] <name>` — start new feature pipeline (optionally create git branch)
  - `artifact [path]` — register phase output
  - `approve` — advance to next phase (only after user approval)
  - `task T-N` — mark implementation task done
  - `abandon [feature]` — abandon the current (or named) feature pipeline
  - `history` — list all features and their status
  - `revisions [phase]` — view revision history for a phase
  - `config-check` — validate `.spec/config.yaml` keys and types
  - `docs-check` — check project documentation freshness
  - `inject <phase> <path>` — inject a pre-written artifact and skip to that phase
- **Multi-feature**: add `--feature <name>` before any command when multiple pipelines are active
- **Project config**: `.spec/config.yaml` — optional, supports `context`, `rules.<phase>`, `test_skill`, `test_reference`, `docs_dir`, `doc_freshness_days`, `auto_branch`, `branch_prefix`
- **Templates** (`.agents/skills/spec-driven-dev/templates/`):

| File | Phase |
|------|-------|
| `explore.md` | Phase 1: Exploration & research |
| `requirements.md` | Phase 2: Formal requirements (WHEN/SHALL) |
| `design.md` | Phase 3: Architecture & design, ADRs |
| `task-plan.md` | Phase 4: TDD implementation plan |
| `implementation.md` | Phase 5: Implementation report |
| `review.md` | Phase 6: Code review |

- **Reference docs** (`.agents/skills/spec-driven-dev/templates/reference/`): `antipatterns.md`, `correctness-properties-examples.md`, `review-reference.md`, `task-types.md`
- **Doc templates** (`.agents/skills/spec-driven-dev/templates/docs/`): 14 templates for project documentation maintenance (API, auth, core, database, deployment, etc.)
- **Artifacts output**: `.spec/features/<feature-name>/` — one file per phase

### Agents

| Name | Type | Purpose |
|------|------|---------|
| `Explore` | Subagent | Fast read-only codebase exploration and Q&A. Safe to call in parallel. Specify thoroughness: quick, medium, thorough |

## Key References

- [Deckhouse docs](https://deckhouse.ru/docs)
- [Deckhouse GitHub](https://github.com/deckhouse/deckhouse)
- [protoc-gen-mcp](https://github.com/easyp-tech/protoc-gen-mcp)
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [MCP Spec](https://spec.modelcontextprotocol.io)
- [SDD Artifacts](.spec/features/deckhouse-harness-mvp/) — explore, requirements, design, task-plan, implementation, review
