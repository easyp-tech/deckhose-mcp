# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased] — dual transport (stdio + SSE)

### Added

- **Dual-mode server.** The binary now supports both transports:
  - **stdio** (default): `server.Run(ctx, &mcp.StdioTransport{})` — for local clients (Claude Desktop, etc.).
  - **SSE** (HTTP): activated by `TRANSPORT=sse` env var, `LISTEN_ADDR` env var, `-listen :8080` flag, or `-transport sse`. Uses `mcp.NewSSEHandler` + `http.Server` with graceful shutdown.
- CLI flags: `-listen <addr>` and `-transport (stdio|sse)`.
- Switched to `urfave/cli/v3` for flag + environment variable handling (`Sources: cli.EnvVars(...)`). `--help` now annotates env vars. Manual fallback code removed.
- `TRANSPORT` (and `MCP_TRANSPORT`) / `LISTEN_ADDR` continue to work; CLI flag wins.
- `serveSSE()` helper that reuses a single `*mcp.Server` for multiple concurrent SSE sessions.
- Constants: `defaultListenAddr=":8080"`, `shutdownTimeout=10s`, `readHeaderTimeout=5s`.
- **Restored**: `deploy/` manifests (`deployment.yaml`, `service.yaml`, `rbac.yaml`) for SSE in-cluster deployment in `d8-system` namespace.
- **Restored**: `Dockerfile` (multi-stage: `golang:1.26` → `gcr.io/distroless/static-debian12`).
- **Restored**: `docker:build` and `docker:load` tasks in `Taskfile.yml`.
- Integration tests now support both transport modes via `TRANSPORT` env var: `TRANSPORT=stdio` (default) uses FIFO helpers; `TRANSPORT=sse` uses curl-based SSE helpers with Docker image + Kind deployment + port-forward.

### Changed

- `main.go` now decides the transport at startup instead of hard-coding stdio.
- `serveSSE()` now uses `ReadHeaderTimeout=5s` and `shutdownTimeout=10s` (was 5s).
- Updated docs (README, AGENTS.md) to describe dual transport and in-cluster deployment.
- **Docs refresh.** Brought the reference docs in line with the code: refreshed the `.spec/` set (23 → 43 tools across 6 services, 70/38 → 134 unit tests, 17 → 36 `k8s.Client` methods, SSE-only → dual stdio+SSE transport, all 10 CRDs, full-43-tool RBAC table) — including a full rewrite of `.spec/API.md`; removed dangling `ROADMAP.md` links; corrected the `k8s.Client` method count in `AGENTS.md` (~40 → 36).
- **Rename to `deckhouse-harness`.** The Docker image and all Kubernetes resources (Service/Deployment/ServiceAccount `deckhouse-harness`, ClusterRole/Binding `d8:deckhouse-harness`) were renamed from `deckhouse-mcp` to match the binary/module, resolving the split naming (`Taskfile.yml` `IMAGE_NAME`, `deploy/*.yaml`, README/AGENTS references).
- `go.mod` already uses MCP SDK v1.6.0 (which includes maintained SSE support).
- Version bumped `0.3.0` → `0.3.1`.

### Fixed

- **`GetClusterStatus` resilience.** It no longer hard-fails when the `nodegroups` CRD is absent (a valid configuration — the optional `node-manager` module is disabled). Node-group collection now degrades to an empty list instead of erroring the whole status. Added the `k8s.IsCRDNotRegistered(err)` helper used for the detection.
- **Double-wrapped error messages.** The client layer no longer adds an operation prefix for `ListNodeGroups`, `ListStaticInstances` and `GetStaticInstance` — the handler owns the single prefix — so errors read `listing node groups: <cause>` instead of `listing node groups: listing node groups: <cause>`. The actionable "CRD not registered (is node-manager module enabled?)" hint is preserved.
- **`ListModuleConfigs` empty `version`.** `spec.version` is an integer in the CR but was read as a string (always empty). It is now read as int64 and rendered as a string, consistent with `GetModuleConfig`.
- Verified end-to-end against a live Deckhouse CE cluster (all 43 tools, incl. the module/release write tools through live admission webhooks) plus `go test ./...` (unit tests: `IsCRDNotRegistered`, `GetClusterStatus` degradation, `ListModuleConfigs` version).

### Notes

- Stdio remains the default so existing local client configs are unaffected.
- When running in SSE mode, multiple MCP clients can connect simultaneously.
- **K8s auth: kubeconfig fallback.** `loadKubeConfig()` tries `rest.InClusterConfig()` first (in-cluster), then falls back to `clientcmd` (`KUBECONFIG` env or `~/.kube/config`).
- **Logging via env.** `LOG_LEVEL` (DEBUG/INFO/WARN/ERROR), `LOG_OUTPUT` (stderr/file/discard), `LOG_FILE` (path). Logs never go to stdout (reserved for MCP protocol).

### Infrastructure

- `protoc-gen-mcp` upgraded v0.3.1 → v0.5.0 (`easyp.yaml`, `go.mod`, `easyp.lock`)
- `cmd/deckhouse-harness/main.go` rewritten: `loadKubeConfig()`, `configureLogger()`, dual-mode `run()` with `serveSSE()`
- `Taskfile.yml`: `docker:build` and `docker:load` tasks restored; `build` outputs `./deckhouse-harness`; `IMAGE_NAME=deckhouse-harness` for Docker
- `tests/integration/setup.sh`: dual-mode — stdio builds local binary; SSE builds Docker image, loads into Kind, applies `deploy/`, starts port-forward
- `tests/integration/test.sh`: dual-mode helpers — stdio uses FIFO (`mcp_connect`/`mcp_disconnect`/`mcp_send`/`mcp_recv`); SSE uses curl (`curl -sN` for SSE stream, `curl -X POST` for requests); all 58 test cases unchanged
- `tests/integration/teardown.sh`: dual-mode cleanup — stdio removes binary; SSE kills port-forward, optionally deletes deployment
- `README.md` and `AGENTS.md` updated for dual transport, in-cluster deployment, and env-based logging

---

## [0.2.0-p3] — P3 — Edge Cases

4 new MCP handlers — module maintenance toggle, node group bootstrap scripts, module source cleanup, module release listing. Brings the tool catalog from 39 (P0+P1+P2) to **43 total**.

### Added

- `deckhouse_ListModuleReleases` (F6, read) — list `ModuleRelease` resources for a given `module_name` with optional `phase` filter; returns `name`, `version`, `phase`, `approved` flag
- `deckhouse_DeleteModuleSource` (F3, write) — delete `ModuleSource` CRD with safe-by-default pre-check (refuses deletion when any `ModuleRelease.metadata.labels[source]` matches); bypass via explicit `force=true`
- `deckhouse_CreateNodeGroupConfiguration` (D13, write) — create `NodeGroupConfiguration` CRD (a bash script bound to one or more `NodeGroups`); validates non-empty `content` and `node_groups`; default `weight=100`
- `deckhouse_SetModuleMaintenance` (B6, write, idempotent) — toggle `ModuleConfig.spec.maintenance`: when `enabled=true` sets `NoResourceReconciliation` (Deckhouse pauses enable/disable transitions while settings/version updates continue); when `enabled=false` removes the field via JSON merge patch `{"spec":{"maintenance":null}}`

### Infrastructure

- Proto: +4 RPCs in `modules.proto`, `nodes.proto`, `sources.proto` (no new proto files)
- `k8s.Client` interface: +4 methods (`ListModuleReleases`, `DeleteModuleSource`, `CreateNodeGroupConfiguration`, `PatchModuleConfig`)
- 2 new GVR constants: `ModuleReleaseGVR`, `NodeGroupConfigurationGVR` (deckhouse.io/v1alpha1)
- `ListModuleReleases` accepts empty `moduleName` to list all (used by F3 source-based pre-check) — non-breaking change
- Integration CRDs: `modulereleases.deckhouse.io`, `nodegroupconfigurations.deckhouse.io` added to `tests/integration/crds.yaml`
- Maintenance-mode field name (`spec.maintenance` = `"NoResourceReconciliation"`) confirmed via Deckhouse public docs ([cr.html](https://deckhouse.io/products/kubernetes-platform/documentation/v1/cr.html), module-development docs)

### RBAC (least-privilege additions)

- Read: `modulereleases` (deckhouse.io)
- Write: `modulesources` delete; `nodegroupconfigurations` create; `moduleconfigs` patch (merged into existing rule alongside `update`)

### Tests

- 18 new unit tests (133 total, up from 115 in P2)
- Mock `k8s.Client` extended with 4 new function fields
- All previous P0/P1/P2 tests continue to pass
- **Bash integration suite extended to all 43 handlers** (58 cases — happy path + targeted error path; cleanup-before/after pattern). Final run on Kind + Deckhouse CE: 49 passed, 0 failed, 9 environment-skipped (`d8-cluster-configuration` secret, Deckhouse validating webhook unreachable, `NodeGroupConfiguration` CRD not in CE, single-node `DrainNode`)
- New `deckhouse_webhook_reachable` probe in `tests/integration/test.sh` so webhook-dependent tests skip cleanly instead of failing on infra issues

### Fixed

- **Critical**: `DeckhouseReleaseGVR.Resource` was `"deckhouserelease"` (singular) instead of the real CRD plural `"deckhousereleases"`. The dynamic client therefore returned `the server could not find the requested resource` for `ListDeckhouseReleases`, `GetDeckhouseRelease`, `ApproveRelease` and `GetClusterStatus` (which also queries releases). Unit tests didn't catch this — they mock `k8s.Client` and never resolve the GVR. Fixed in `internal/k8s/client.go`, `deploy/rbac.yaml`, `tests/integration/crds.yaml`, plus README/ROADMAP docs.
- **`CreateModuleUpdatePolicy` was unusable**: Deckhouse's validating webhook requires `spec.moduleReleaseSelector.labelSelector.matchLabels` (non-empty), but the proto/handler never set it. The handler now accepts a required `match_labels` map and rejects empty input with `errMatchLabelsRequired` before the API call. Proto field `match_labels = 3` added to `CreateModuleUpdatePolicyRequest`. Two new unit tests (`Happy` updated, `MissingMatchLabels` added) and matching integration coverage (`test_create_module_update_policy_missing_match_labels`).
- **`EventInfo.count` schema regression**: relaxed `minimum` from `1` to `0` in `proto/deckhouse/v1/diagnostics.proto`. Events emitted via `events.k8s.io/v1` can carry `count=0` on first occurrence; the previous constraint caused MCP output validation to fail for `GetNode` and `GetNodeEvents` whenever such events were present.
- **`test_create_module_update_policy` and `test_create_module_update_policy_already_exists`**: now guarded by `deckhouse_webhook_reachable`. The Deckhouse `update-policies` validating webhook intercepts both `create` and `delete` on `moduleupdatepolicies` — without it, leftover resources from previous runs cannot be cleaned up, and the `kubectl delete` cleanup helper inside the tests is also blocked. Skipping rather than failing matches the same pattern used for other webhook-dependent operations (enable/disable module, set module maintenance, approve release).

### CI

- Added `.github/workflows/ci.yml` with three independent jobs on `pull_request` and pushes to `main`:
  - `lint` — `easyp lint` over all `.proto` files
  - `test` — `go test ./...` (134 tests)
  - `build` — `go build ./cmd/deckhouse-harness`
- Concurrency group keyed on `github.ref` so superseded runs are cancelled. Permissions limited to `contents: read`. No integration job, no docker, no release automation in scope.

---

## [0.2.0-p2] — P2 — Advanced Management

16 new MCP handlers across 3 batches (read-only, writes, sources). Brings the tool catalog from 23 (P0+P1) to 39 total.

### Added

#### Batch 1 — Read-only diagnostics & module/node introspection (6 handlers)

- `deckhouse_GetNodeEvents` — list Kubernetes events scoped to a single node
- `deckhouse_GetPodLogs` — fetch container logs with `tail` and `since` parameters
- `deckhouse_GetStaticInstance` — get a single `StaticInstance` with labels and last-update time
- `deckhouse_ListModules` — list `Module` CRDs (status, weight, source)
- `deckhouse_CordonNode` — mark a node unschedulable; idempotent (returns `previousState`)
- `deckhouse_GetStaticClusterConfiguration` — read `static-cluster-configuration.yaml` from the `d8-cluster-configuration` Secret

#### Batch 2 — Write operations & cluster configuration (6 handlers)

- `deckhouse_UpdateModuleSettings` — RFC 7396 JSON Merge Patch on `ModuleConfig.spec.settings`; explicit `null` deletes keys
- `deckhouse_UncordonNode` — mark a node schedulable; idempotent skip if already schedulable
- `deckhouse_DrainNode` — composite: cordon → list non-DaemonSet/non-mirror pods → eviction loop with PDB awareness; 30s polling, default 300s timeout
- `deckhouse_DeleteSSHCredentials` — delete `SSHCredentials` CRD
- `deckhouse_DeleteNodeGroup` — delete `NodeGroup` CRD
- `deckhouse_UpdateKubernetesVersion` — patch `kubernetesVersion` in `d8-cluster-configuration` Secret with retry-on-conflict (up to 3 attempts), YAML round-trip via `sigs.k8s.io/yaml`

#### Batch 3 — Module sources & update policies (4 handlers)

- `deckhouse_ListModuleSources` — list `ModuleSource` CRDs with registry and status
- `deckhouse_CreateModuleSource` — create `ModuleSource` CRD with registry repo
- `deckhouse_ListModuleUpdatePolicies` — list `ModuleUpdatePolicy` CRDs with update mode
- `deckhouse_CreateModuleUpdatePolicy` — create `ModuleUpdatePolicy` CRD with update mode

#### Infrastructure

- New proto file `proto/deckhouse/v1/sources.proto` (`SourcesAPI` service)
- `k8s.Client` interface: +13 methods (`ListNodeEvents`, `GetPodLogs`, `GetSecret`, `GetModuleConfig`, `UpdateModuleConfig`, `GetNode`, `CordonNode`, `ListModules`, `UncordonNode`, `EvictPod`, `UpdateSecret`, `DeleteSSHCredentials`, `DeleteNodeGroup`, `ListModuleSources`, `CreateModuleSource`, `ListModuleUpdatePolicies`, `CreateModuleUpdatePolicy`)
- 2 new GVR constants: `ModuleSourceGVR`, `ModuleUpdatePolicyGVR` (deckhouse.io/v1alpha1)
- New handler file `internal/handler/sources.go` (`SourcesHandler`)
- Server registration: `pb.RegisterSourcesAPITools` (6th `Register*APITools` in `cmd/deckhouse-harness/main.go`)
- Integration CRDs: `modulesources.deckhouse.io`, `moduleupdatepolicies.deckhouse.io` in `tests/integration/crds.yaml`

#### RBAC (least-privilege expansion)

- Read: `events`, `pods/log`, `modules`, `modulesources`, `moduleupdatepolicies`
- Write: `secrets` update on `d8-cluster-configuration`; `nodes` update/patch; `pods/eviction` create; `moduleconfigs` update; `deckhouserelease` patch; `staticinstances`/`sshcredentials`/`nodegroups` delete; `nodegroups` create; `modulesources`/`moduleupdatepolicies` create

#### Tests

- 77 new unit tests (115 total, up from 38 in P0)
- Polling tests (`DrainNode_PDBBlocksThenSucceeds`, `DrainNode_Timeout`) — ~30s each
- Mock `k8s.Client` extended with 17 new function-fields

---

## [0.1.0] — 2026-04-13

Initial release — MVP (P0) feature set. MCP server for managing Deckhouse Kubernetes Platform (Community Edition) over SSE transport, deployed as a Pod in `d8-system`.

### Added

#### MCP Tools (10 handlers)

**Block A — Diagnostics (read-only)**

- `deckhouse_GetClusterStatus` — aggregated cluster status: node counts, NodeGroup readiness, errored modules, pending releases, unhealthy pod count, current DKP version
- `deckhouse_ListNodes` — list all cluster nodes with filters by NodeGroup, status, and role
- `deckhouse_ListNodeGroups` — list all NodeGroup resources with readiness and condition info
- `deckhouse_ListStaticInstances` — list StaticInstance resources with phase filtering
- `deckhouse_ListUnhealthyPods` — list pods not in Running/Completed state across any namespace

**Block B — Modules**

- `deckhouse_ListModuleConfigs` — list all ModuleConfig resources with enabled/disabled filter

**Block C — Releases**

- `deckhouse_ListDeckhouseReleases` — list DeckhouseRelease resources with phase filter

**Block D — Nodes (write)**

- `deckhouse_CreateSSHCredentials` — create SSHCredentials CRD (base64-encodes private key internally)
- `deckhouse_CreateStaticInstance` — create StaticInstance CRD with credential reference and labels
- `deckhouse_AddWorkerNode` — composite handler: creates SSHCredentials + StaticInstance, then polls until `Running` or timeout

#### Infrastructure

- **Proto-first design**: services and MCP tool schemas defined in `.proto` files, Go bindings generated via `protoc-gen-mcp` + `easyp`
- **Four proto services**: `DiagnosticsAPI`, `ModulesAPI`, `ReleasesAPI`, `NodesAPI` (10 RPCs total); `ConfigAPI` and `SourcesAPI` stubs for future phases
- **SSE transport**: HTTP server with `mcp.NewSSEHandler`, listens on `:8080` (configurable via `LISTEN_ADDR`)
- **In-cluster auth**: `rest.InClusterConfig()` + ServiceAccount `deckhouse-harness` in `d8-system`
- **k8s.Client interface** with typed client for core resources (`nodes`, `pods`) and dynamic client for Deckhouse CRDs (`NodeGroup`, `StaticInstance`, `SSHCredentials`, `ModuleConfig`, `DeckhouseRelease`)
- **Graceful shutdown**: `signal.NotifyContext(SIGINT, SIGTERM)` + `http.Server.Shutdown()`
- **Multi-stage Dockerfile**: `golang:1.26` build → `distroless` runtime image
- **Kubernetes manifests** in `deploy/`: `Deployment`, `Service`, `ServiceAccount`, `ClusterRole`, `ClusterRoleBinding`
- **RBAC** (least-privilege): read `nodes`, `pods`, `events`, `pods/log`, `nodegroups`, `staticinstances`, `moduleconfigs`, `deckhouserelease`; create `staticinstances`, `sshcredentials`
- **Taskfile** with tasks: `generate`, `lint`, `build`, `test`, `docker:build`, `docker:load`, `integration`
- **Integration test scaffolding**: `tests/integration/` with setup/teardown scripts, CRD fixtures

#### Tests

- 38 unit tests across 5 files in `internal/handler/`
- Mock `k8s.Client` using function fields (no external mock library)
- Coverage: `DiagnosticsHandler` (19 tests), `ModulesHandler` (3), `ReleasesHandler` (2), `NodesHandler` (11), error helpers (3)

[0.1.0]: https://github.com/easyp-tech/deckhouse-harness/releases/tag/v0.1.0
