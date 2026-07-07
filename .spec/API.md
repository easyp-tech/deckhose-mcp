<!-- generated: 2026-05-12, updated: 2026-07-07, template: api.md -->
# API Reference — Deckhouse Harness

## 1. Overview

**Protocol:** Model Context Protocol (MCP). Tool definitions are generated from protobuf via `protoc-gen-mcp` (v0.5.0); all tools use ProtoJSON encoding (field names are camelCase in JSON).
**Transport (dual):**
- **stdio** (default) — newline-delimited JSON-RPC on stdin/stdout. Used by local MCP clients (Claude Desktop, Cursor, `mcp` CLI). Logs go to stderr.
- **SSE (HTTP)** — enabled via `TRANSPORT=sse`, `LISTEN_ADDR`, or `-listen :8080`. Uses `mcp.NewSSEHandler` + `http.Server`; multiple clients may connect concurrently.

**Auth:** None at the MCP level — Kubernetes RBAC enforces access at the Pod/ServiceAccount level (SSE in-cluster) or via the local `KUBECONFIG` identity (stdio).
**Base URL (SSE mode):** `http://<service>:8080` (in-cluster: `deckhouse-harness.d8-system.svc.cluster.local:8080`)

**Tool count:** **43 tools across 6 services** — Diagnostics 11, Modules 7, Releases 3, Nodes 13, Config 3, Sources 6.

## 2. Tool Naming

All tools use the namespace prefix `deckhouse_`:

```
{ServiceName}.{MethodName}  →  deckhouse_{MethodName}
DiagnosticsAPI.GetClusterStatus  →  deckhouse_GetClusterStatus
```

## 3. Tools Summary

| # | Tool | Block | Type | Description |
|---|------|-------|------|-------------|
| 1 | `deckhouse_GetClusterStatus` | Diagnostics | read | Aggregated cluster health summary |
| 2 | `deckhouse_ListNodes` | Diagnostics | read | Cluster nodes with group/status/role filters |
| 3 | `deckhouse_ListNodeGroups` | Diagnostics | read | All Deckhouse NodeGroups |
| 4 | `deckhouse_ListStaticInstances` | Diagnostics | read | StaticInstance resources with filters |
| 5 | `deckhouse_ListUnhealthyPods` | Diagnostics | read | Pods not Running/Succeeded |
| 6 | `deckhouse_GetNode` | Diagnostics | read | Detailed node info + conditions + events |
| 7 | `deckhouse_GetNodeGroup` | Diagnostics | read | Single NodeGroup detail + member nodes |
| 8 | `deckhouse_GetDeckhouseLogs` | Diagnostics | read | Deckhouse controller pod logs |
| 9 | `deckhouse_GetNodeEvents` | Diagnostics | read | Kubernetes Events for a node |
| 10 | `deckhouse_GetStaticInstance` | Diagnostics | read | Single StaticInstance detail |
| 11 | `deckhouse_GetPodLogs` | Diagnostics | read | Logs for a specific pod/container |
| 12 | `deckhouse_ListModuleConfigs` | Modules | read | ModuleConfig objects with enabled filter |
| 13 | `deckhouse_GetModuleConfig` | Modules | read | Single ModuleConfig detail |
| 14 | `deckhouse_EnableModule` | Modules | write (idempotent) | Enable a module |
| 15 | `deckhouse_DisableModule` | Modules | write (idempotent) | Disable a module |
| 16 | `deckhouse_ListModules` | Modules | read | Runtime Module resources |
| 17 | `deckhouse_UpdateModuleSettings` | Modules | write | JSON Merge Patch on module settings |
| 18 | `deckhouse_SetModuleMaintenance` | Modules | write (idempotent) | Toggle module maintenance mode |
| 19 | `deckhouse_ListDeckhouseReleases` | Releases | read | DeckhouseRelease objects with phase filter |
| 20 | `deckhouse_GetDeckhouseRelease` | Releases | read | Single release detail + requirements |
| 21 | `deckhouse_ApproveRelease` | Releases | write (idempotent) | Approve a pending release |
| 22 | `deckhouse_CreateSSHCredentials` | Nodes | write | Create SSHCredentials resource |
| 23 | `deckhouse_DeleteSSHCredentials` | Nodes | write | Delete SSHCredentials resource |
| 24 | `deckhouse_CreateStaticInstance` | Nodes | write | Create StaticInstance resource |
| 25 | `deckhouse_DeleteStaticInstance` | Nodes | write | Delete StaticInstance resource |
| 26 | `deckhouse_AddWorkerNode` | Nodes | write (composite) | SSHCredentials → StaticInstance → wait |
| 27 | `deckhouse_RemoveNode` | Nodes | write (composite) | Cordon + evict + delete StaticInstance |
| 28 | `deckhouse_CreateNodeGroup` | Nodes | write | Create a NodeGroup |
| 29 | `deckhouse_DeleteNodeGroup` | Nodes | write | Delete a NodeGroup |
| 30 | `deckhouse_WaitNodeReady` | Nodes | read (polling) | Poll StaticInstance until Running |
| 31 | `deckhouse_CordonNode` | Nodes | write (idempotent) | Mark node unschedulable |
| 32 | `deckhouse_UncordonNode` | Nodes | write (idempotent) | Mark node schedulable |
| 33 | `deckhouse_DrainNode` | Nodes | write (composite) | Cordon + PDB-aware eviction loop |
| 34 | `deckhouse_CreateNodeGroupConfiguration` | Nodes | write | Bash script bound to NodeGroups |
| 35 | `deckhouse_GetClusterConfiguration` | Config | read | Read ClusterConfiguration YAML |
| 36 | `deckhouse_GetStaticClusterConfiguration` | Config | read | Read StaticClusterConfiguration YAML |
| 37 | `deckhouse_UpdateKubernetesVersion` | Config | write (destructive) | Patch kubernetesVersion |
| 38 | `deckhouse_ListModuleSources` | Sources | read | ModuleSource (OCI registry) resources |
| 39 | `deckhouse_CreateModuleSource` | Sources | write | Create a ModuleSource |
| 40 | `deckhouse_DeleteModuleSource` | Sources | write (destructive) | Delete a ModuleSource (optional force) |
| 41 | `deckhouse_ListModuleUpdatePolicies` | Sources | read | ModuleUpdatePolicy resources |
| 42 | `deckhouse_CreateModuleUpdatePolicy` | Sources | write | Create a ModuleUpdatePolicy |
| 43 | `deckhouse_ListModuleReleases` | Sources | read | ModuleRelease versions for a module |

## 4. Tools Reference

Each tool is generated from an RPC in `proto/deckhouse/v1/`. Inputs/outputs below show illustrative ProtoJSON; the authoritative schema is the `.proto` (see §7). Optional fields are marked; omit them to accept defaults.

### Block A — Diagnostics (11 tools, read-only)

All diagnostics tools have `read_only_hint: true`.

---

#### `deckhouse_GetClusterStatus`

Aggregated cluster health: node readiness counts, per-NodeGroup health, ModuleConfigs with errors, DeckhouseReleases in Pending phase, total unhealthy pod count, and the deployed Deckhouse version.

**Input:** `{}` (empty)

**Output:**
```json
{
  "nodes": { "total": 4, "ready": 3, "notReady": 1 },
  "nodeGroups": [ { "name": "worker", "ready": 2, "total": 3 } ],
  "erroredModules": ["my-module"],
  "pendingReleases": [ { "name": "v1.74.15", "version": "v1.74.15" } ],
  "unhealthyPodsCount": 2,
  "deckhouseVersion": "v1.74.15"
}
```

---

#### `deckhouse_ListNodes`

Lists cluster nodes with optional filtering.

**Input (all optional):**
```json
{ "nodeGroup": "worker", "status": "NODE_STATUS_FILTER_READY", "role": "worker" }
```
`status` enum: `NODE_STATUS_FILTER_READY`, `NODE_STATUS_FILTER_NOT_READY`, `NODE_STATUS_FILTER_ALL`.

**Output:**
```json
{
  "nodes": [
    {
      "name": "worker-01", "status": "Ready", "role": "worker",
      "internalIp": "192.168.1.10", "osImage": "Ubuntu 22.04.4 LTS",
      "kubeletVersion": "v1.31.6", "age": "3d", "nodeGroup": "worker"
    }
  ]
}
```

---

#### `deckhouse_ListNodeGroups`

Lists all Deckhouse NodeGroups with status and conditions.

**Input:** `{}`

**Output:**
```json
{
  "nodeGroups": [
    {
      "name": "worker", "nodeType": "Static",
      "ready": 2, "total": 3, "upToDate": 3,
      "conditions": [ { "type": "Ready", "status": "True", "message": "" } ],
      "error": ""
    }
  ]
}
```
> If the `node-manager` module is disabled, the `nodegroups` CRD is not registered and the tool returns an actionable error: `CRD deckhouse.io/v1/nodegroups not registered (is node-manager module enabled?)`.

---

#### `deckhouse_ListStaticInstances`

Lists StaticInstance resources with optional filters.

**Input (all optional):**
```json
{ "nodeGroup": "worker", "phase": "STATIC_INSTANCE_PHASE_RUNNING" }
```
`phase` enum: `..._PENDING`, `..._BOOTSTRAPPING`, `..._RUNNING`, `..._CLEANING`, `..._ERROR`.

**Output:**
```json
{
  "instances": [
    { "name": "worker-01", "address": "10.0.0.5", "phase": "Running",
      "nodeRef": "worker-01", "lastUpdateTime": "2026-07-01T12:00:00Z" }
  ]
}
```

---

#### `deckhouse_ListUnhealthyPods`

Lists pods not in Running/Succeeded phase (Pending, Failed, CrashLoopBackOff, etc.).

**Input (all optional):**
```json
{ "namespace": "d8-system", "excludeCompleted": true }
```
Omit `namespace` to search all namespaces.

**Output:**
```json
{
  "pods": [
    { "name": "my-pod-abc", "namespace": "default", "status": "CrashLoopBackOff",
      "reason": "OOMKilled", "restartCount": 5, "age": "2h" }
  ]
}
```

---

#### `deckhouse_GetNode`

Detailed info for a single node: conditions, allocatable/capacity, IP, kubelet version, optional StaticInstance phase, and last 10 events.

**Input:**
```json
{ "name": "worker-01" }
```

**Output:**
```json
{
  "node": { "name": "worker-01", "status": "Ready", "role": "worker", "nodeGroup": "worker" },
  "conditions": [ { "type": "Ready", "status": "True", "message": "kubelet is posting ready status" } ],
  "allocatable": { "cpu": "4", "memory": "8Gi", "pods": "110" },
  "capacity": { "cpu": "4", "memory": "8Gi", "pods": "110" },
  "staticInstancePhase": "Running",
  "events": [ { "reason": "NodeReady", "message": "...", "type": "Normal", "lastTime": "...", "count": 1 } ]
}
```

---

#### `deckhouse_GetNodeGroup`

Full spec/status of a NodeGroup plus the names of member nodes (via label `node.deckhouse.io/group`).

**Input:**
```json
{ "name": "worker" }
```

**Output:**
```json
{
  "name": "worker", "nodeType": "Static", "ready": 2, "total": 3, "upToDate": 3,
  "statusMessage": "", "nodeNames": ["worker-01", "worker-02", "worker-03"]
}
```

---

#### `deckhouse_GetDeckhouseLogs`

Logs of the Deckhouse controller pod in `d8-system`.

**Input (all optional):**
```json
{ "tail": 100, "since": "30m", "grep": "error" }
```
`tail` default 100; `since` accepts durations like `30m`, `1h`; `grep` is a case-sensitive substring filter.

**Output:**
```json
{ "logs": "line1\nline2\n..." }
```

---

#### `deckhouse_GetNodeEvents`

Kubernetes Events whose `involvedObject.name` matches the node (most recent, limited to 10).

**Input:**
```json
{ "name": "worker-01" }
```

**Output:**
```json
{
  "events": [
    { "reason": "NodeNotReady", "message": "Node worker-01 status is now: NodeNotReady",
      "type": "Warning", "lastTime": "2026-07-01T12:00:00Z", "count": 3 }
  ]
}
```

---

#### `deckhouse_GetStaticInstance`

Detailed info for a single StaticInstance.

**Input:**
```json
{ "name": "worker-01" }
```

**Output:**
```json
{
  "name": "worker-01", "address": "10.0.0.5", "phase": "Running",
  "credentialsRef": "worker-01-ssh", "nodeRef": "worker-01",
  "labels": { "node-role.deckhouse.io/worker": "" }
}
```

---

#### `deckhouse_GetPodLogs`

Logs from a specific pod and optional container.

**Input:**
```json
{ "namespace": "d8-system", "pod": "deckhouse-7d8c8f5b4d-abcde", "container": "deckhouse", "tail": 200, "since": "1h" }
```
`container`, `tail`, `since` are optional. If `container` is omitted, the default (first) container is used.

**Output:**
```json
{ "logs": "line1\nline2\n..." }
```

---

### Block B — Modules (7 tools)

---

#### `deckhouse_ListModuleConfigs`

Lists ModuleConfig objects with optional `enabled` filter.

**Input (optional):** `{ "enabled": true }` — omit to list all.

**Output:**
```json
{
  "modules": [
    { "name": "cert-manager", "enabled": true, "version": "1",
      "source": "", "updatePolicy": "", "statusMessage": "Ready" }
  ]
}
```

---

#### `deckhouse_GetModuleConfig`

Full spec/status of a single ModuleConfig.

**Input:** `{ "name": "cert-manager" }`

**Output:**
```json
{ "name": "cert-manager", "enabled": true, "version": 1,
  "settings": { "key": "value" }, "statusMessage": "Ready" }
```

---

#### `deckhouse_EnableModule`

Sets `spec.enabled=true` in the ModuleConfig. Idempotent (`idempotent_hint: true`).

**Input:** `{ "name": "cert-manager" }`

**Output:** `{ "success": true, "previousState": false }`

---

#### `deckhouse_DisableModule`

Sets `spec.enabled=false` in the ModuleConfig. Idempotent.

**Input:** `{ "name": "cert-manager" }`

**Output:** `{ "success": true, "previousState": true }`

---

#### `deckhouse_ListModules`

Lists runtime `Module` resources (distinct from ModuleConfig — runtime state, weight, source).

**Input:** `{}`

**Output:**
```json
{
  "modules": [
    { "name": "cert-manager", "weight": 30, "source": "", "state": "Enabled" }
  ]
}
```

---

#### `deckhouse_UpdateModuleSettings`

Deep-merges the provided settings into `ModuleConfig.spec.settings` using JSON Merge Patch (RFC 7396). Absent top-level keys are preserved; `null` values remove keys.

**Input:**
```json
{ "name": "cert-manager", "settings": { "logLevel": "Debug" } }
```

**Output:**
```json
{ "updated": true }
```
`updated` is `false` when the merged settings were byte-identical (no-op).

---

#### `deckhouse_SetModuleMaintenance`

Toggles `ModuleConfig.spec.maintenance`. When `enabled=true`, sets `spec.maintenance=NoResourceReconciliation`; when `false`, removes the field. Idempotent.

**Input:**
```json
{ "name": "cert-manager", "enabled": true }
```

**Output:**
```json
{ "maintenanceEnabled": true, "name": "cert-manager" }
```

---

### Block C — Releases (3 tools)

---

#### `deckhouse_ListDeckhouseReleases`

Lists DeckhouseRelease objects with optional phase filter.

**Input (optional):** `{ "phase": "DECKHOUSE_RELEASE_PHASE_PENDING" }`

**Output:**
```json
{
  "releases": [
    { "name": "v1.74.0", "version": "v1.74.0", "phase": "Pending",
      "approved": false, "transitionTime": "2026-05-01T12:00:00Z",
      "changelogLink": "https://deckhouse.ru/changelog/v1.74.0" }
  ]
}
```

---

#### `deckhouse_GetDeckhouseRelease`

Full spec/status of a single DeckhouseRelease with requirements.

**Input:** `{ "version": "v1.74.0" }`

**Output:**
```json
{
  "name": "v1.74.0", "version": "v1.74.0", "phase": "Pending", "approved": false,
  "transitionTime": "2026-05-01T12:00:00Z",
  "changelogLink": "https://deckhouse.ru/changelog/v1.74.0",
  "requirements": { "k8s": ">=1.28" }
}
```

---

#### `deckhouse_ApproveRelease`

Approves a pending release by patching the `release.deckhouse.io/approved` annotation to `"true"`. Idempotent.

**Input:** `{ "version": "v1.74.0" }`

**Output:** `{ "success": true, "previousApproved": false }`

---

### Block D — Nodes (13 tools, write)

Node write tools carry `destructive_hint: true` (or `read_only_hint: false`). Secrets (SSH private key, sudo password) are sent as **plain text** — base64 encoding happens inside the handler.

---

#### `deckhouse_CreateSSHCredentials`

Creates an SSHCredentials resource in `d8-system`.

**Input:**
```json
{
  "name": "worker-1-ssh", "user": "ubuntu",
  "privateKey": "-----BEGIN OPENSSH PRIVATE KEY-----\n...",
  "port": 22, "sshExtraArgs": "-o StrictHostKeyChecking=no", "sudoPassword": "secret"
}
```
`port`, `sshExtraArgs`, `sudoPassword` are optional.

**Output:** `{ "name": "worker-1-ssh" }`

---

#### `deckhouse_DeleteSSHCredentials`

Deletes an SSHCredentials resource by name. StaticInstances still referencing it via `credentialsRef` lose the ability to authenticate.

**Input:** `{ "name": "worker-1-ssh" }`

**Output:** `{ "deleted": true }`

---

#### `deckhouse_CreateStaticInstance`

Creates a StaticInstance bound to an SSHCredentials via `credentialsRef`.

**Input:**
```json
{
  "name": "worker-01", "address": "192.168.1.100",
  "credentialsRef": "worker-1-ssh", "labels": { "role": "worker" }
}
```

**Output:**
```json
{ "name": "worker-01", "address": "192.168.1.100",
  "credentialsRef": "worker-1-ssh", "labels": { "role": "worker" } }
```

---

#### `deckhouse_DeleteStaticInstance`

Deletes a StaticInstance by name; Deckhouse gracefully cleans up the node.

**Input:** `{ "name": "worker-01" }`

**Output:** `{ "success": true }`

---

#### `deckhouse_AddWorkerNode`

Composite: (1) create SSHCredentials, (2) create StaticInstance bound to the target NodeGroup, then (if `waitReady`, default true) poll every 30s until Running or timeout. If step 1 fails, step 2 is skipped; if step 2 fails, the SSHCredentials from step 1 still exist.

**Input:**
```json
{
  "address": "192.168.1.100", "sshUser": "ubuntu",
  "privateKey": "-----BEGIN OPENSSH PRIVATE KEY-----\n...",
  "nodeGroup": "worker", "nodeName": "worker-1",
  "sshPort": 22, "waitReady": true, "timeoutSeconds": 900
}
```
`nodeName` (default: derived from IP), `sshPort` (22), `waitReady` (true), `timeoutSeconds` (900) are optional.

**Output:**
```json
{
  "nodeName": "worker-1", "sshCredentialsName": "worker-1-ssh",
  "staticInstanceName": "worker-1", "phase": "Running",
  "elapsed": "2m30s", "timedOut": false
}
```

---

#### `deckhouse_RemoveNode`

Composite: cordon the node, evict all non-DaemonSet pods, then delete the associated StaticInstance (static nodes only).

**Input:**
```json
{ "name": "worker-01", "drain": true }
```
`drain` optional (default true).

**Output:** `{ "drained": true, "deleted": true }`

---

#### `deckhouse_CreateNodeGroup`

Creates a NodeGroup. In Deckhouse CE only `Static` nodeType is supported.

**Input:**
```json
{
  "name": "worker", "nodeType": "Static", "count": 3,
  "labels": { "node-role.kubernetes.io/worker": "" },
  "disruptions": "Automatic", "maxPodsPerNode": 110
}
```
`count`, `labels`, `disruptions`, `maxPodsPerNode` are optional.

**Output:** `{ "name": "worker", "nodeType": "Static", "count": 3 }`

---

#### `deckhouse_DeleteNodeGroup`

Deletes a NodeGroup by name. Joined nodes keep running kubelet but lose their NodeGroup affiliation.

**Input:** `{ "name": "worker" }`

**Output:** `{ "deleted": true }`

---

#### `deckhouse_WaitNodeReady`

Polls `StaticInstance.status.currentStatus.phase` until `Running` or timeout.

**Input:**
```json
{ "name": "worker-01", "timeoutSeconds": 900, "intervalSeconds": 30 }
```
`timeoutSeconds` (900) and `intervalSeconds` (30) are optional.

**Output:** `{ "phase": "Running", "elapsed": "45s", "timedOut": false }`

---

#### `deckhouse_CordonNode`

Sets `spec.unschedulable=true`. Idempotent.

**Input:** `{ "name": "worker-01" }`

**Output:** `{ "previousState": false }`

---

#### `deckhouse_UncordonNode`

Sets `spec.unschedulable=false`. Idempotent.

**Input:** `{ "name": "worker-01" }`

**Output:** `{ "previousState": true }`

---

#### `deckhouse_DrainNode`

Composite: cordon the node, then iteratively evict every non-DaemonSet, non-mirror pod via the Eviction API (`policy/v1`), respecting PodDisruptionBudgets. Polls every 30s until all evictable pods are gone or timeout. The node stays cordoned after the call.

**Input:**
```json
{ "name": "worker-01", "timeoutSeconds": 300 }
```
`timeoutSeconds` optional (default 300).

**Output:**
```json
{
  "cordoned": true, "evictedCount": 12,
  "failedPods": ["default/web-7f8d-abc"], "timedOut": false, "elapsed": "1m10s"
}
```

---

#### `deckhouse_CreateNodeGroupConfiguration`

Creates a NodeGroupConfiguration — a bash script that runs on every node in the targeted NodeGroups (kubelet tuning, kernel modules, labels/taints, etc.).

**Input:**
```json
{
  "name": "kubelet-tuning",
  "content": "#!/bin/bash\nset -e\necho ok",
  "nodeGroups": ["worker", "master"],
  "weight": 100
}
```
`weight` optional (default 100; lower runs earlier).

**Output:** `{ "created": true, "name": "kubelet-tuning" }`

---

### Block E — Config (3 tools)

---

#### `deckhouse_GetClusterConfiguration`

Reads ClusterConfiguration from the `d8-cluster-configuration` Secret in `kube-system`. Returns raw YAML.

**Input:** `{}`

**Output:**
```json
{ "configuration": "apiVersion: deckhouse.io/v1\nkind: ClusterConfiguration\n..." }
```

---

#### `deckhouse_GetStaticClusterConfiguration`

Reads StaticClusterConfiguration YAML (key `static-cluster-configuration.yaml` of the same Secret). Errors if the key is absent.

**Input:** `{}`

**Output:**
```json
{ "configuration": "apiVersion: deckhouse.io/v1\nkind: StaticClusterConfiguration\n..." }
```

---

#### `deckhouse_UpdateKubernetesVersion`

Patches the `kubernetesVersion` field inside the ClusterConfiguration YAML (read-modify-write, up to 3 retries on conflict). Deckhouse then upgrades/downgrades control plane and nodes. `destructive_hint: true`.

**Input:**
```json
{ "version": "1.29" }
```
`version` is `MAJOR.MINOR` (pattern `^[0-9]+\.[0-9]+$`); must be supported by the installed Deckhouse release.

**Output:**
```json
{ "updated": true, "previousVersion": "1.28" }
```

---

### Block F — Sources (6 tools)

---

#### `deckhouse_ListModuleSources`

Lists ModuleSource resources (OCI registries from which Deckhouse pulls modules).

**Input:** `{}`

**Output:**
```json
{
  "sources": [
    { "name": "deckhouse", "registry": "registry.deckhouse.io/deckhouse/ce/modules",
      "status": "synced 2026-07-01T12:00:00Z" }
  ]
}
```

---

#### `deckhouse_CreateModuleSource`

Creates a ModuleSource pointing at an OCI registry. Already-exists error if the name is taken.

**Input:**
```json
{ "name": "custom-modules", "registry": "registry.example.com/modules" }
```

**Output:** `{ "created": true, "name": "custom-modules" }`

---

#### `deckhouse_DeleteModuleSource`

Deletes a ModuleSource. By default (`force=false`) fails when active ModuleReleases reference it; `force=true` bypasses the safety check. `destructive_hint: true`.

**Input:**
```json
{ "name": "custom-modules", "force": false }
```
`force` optional (default false).

**Output:**
```json
{ "deleted": true, "message": "ModuleSource custom-modules deleted" }
```

---

#### `deckhouse_ListModuleUpdatePolicies`

Lists ModuleUpdatePolicy resources controlling module auto-update behaviour.

**Input:** `{}`

**Output:**
```json
{ "policies": [ { "name": "auto", "updateMode": "Auto" } ] }
```

---

#### `deckhouse_CreateModuleUpdatePolicy`

Creates a ModuleUpdatePolicy. `matchLabels` is required by the Deckhouse webhook (binds the policy to matching ModuleReleases; common key `module`).

**Input:**
```json
{ "name": "auto", "updateMode": "Auto", "matchLabels": { "module": "console" } }
```

**Output:** `{ "created": true, "name": "auto" }`

---

#### `deckhouse_ListModuleReleases`

Lists ModuleRelease resources (available versions) for a module. `moduleName` is required; optional `phase` filter.

**Input:**
```json
{ "moduleName": "deckhouse", "phase": "Deployed" }
```

**Output:**
```json
{
  "releases": [
    { "name": "deckhouse-1.70.0", "module": "deckhouse", "version": "1.70.0",
      "source": "deckhouse", "phase": "Deployed", "approved": "true" }
  ]
}
```

---

## 5. Error Format

MCP tool errors are returned as a standard MCP `CallToolResult` with `isError: true`. The `content` text field holds a human-readable message from `fmt.Errorf` (no structured error codes):

```
"listing nodes: connection refused"
"privateKey is required"
"not found: StaticInstance worker-01"
"CRD deckhouse.io/v1/nodegroups not registered (is node-manager module enabled?)"
```

The last form is emitted by `ListNodeGroups` / `ListStaticInstances` when the corresponding CRD is not installed (e.g. the optional `node-manager` module is disabled) — turning a raw K8s API error into an actionable message.

## 6. Transport Details

**stdio (default):** newline-delimited JSON-RPC over stdin/stdout. `stdout` is reserved for the MCP protocol — logs go to `stderr`. This is the mode local clients (Claude Desktop, Cursor) launch.

**SSE (HTTP):** the client opens an SSE connection; the server multiplexes MCP messages over the stream via `mcp.NewSSEHandler`. A single `*mcp.Server` serves multiple concurrent sessions.

**Polling tools** (`AddWorkerNode`, `WaitNodeReady`, `DrainNode`) block for up to their timeout (default 15 min / 5 min) while polling Kubernetes. Clients must not time out the connection prematurely — polling handlers use a real 30s interval.

**Server timeouts (SSE):** `ReadHeaderTimeout: 5s`; graceful shutdown with a 10s timeout on `SIGINT`/`SIGTERM`. No per-request timeout — callers use context cancellation.

## 7. Proto / Schema

**`.proto` file locations:** `proto/deckhouse/v1/`

| File | Service | RPCs |
|------|---------|------|
| `diagnostics.proto` | `DiagnosticsAPI` | 11 |
| `modules.proto` | `ModulesAPI` | 7 |
| `releases.proto` | `ReleasesAPI` | 3 |
| `nodes.proto` | `NodesAPI` | 13 |
| `config.proto` | `ConfigAPI` | 3 |
| `sources.proto` | `SourcesAPI` | 6 |

**Code generation command:**
```bash
task generate  # easyp mod download && easyp generate
```

**Tool registration (in `cmd/deckhouse-harness/main.go`, generated `Register*Tools`):**
```go
pb.RegisterDiagnosticsAPITools(server, diagnosticsHandler)
pb.RegisterModulesAPITools(server, modulesHandler)
pb.RegisterReleasesAPITools(server, releasesHandler)
pb.RegisterNodesAPITools(server, nodesHandler)
pb.RegisterConfigAPITools(server, configHandler)
pb.RegisterSourcesAPITools(server, sourcesHandler)
```
