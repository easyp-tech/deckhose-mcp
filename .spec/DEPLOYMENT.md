<!-- generated: 2026-07-07, template: deployment.md -->
# Deployment — Deckhouse Harness

## 1. Overview

Deployed as a **Kubernetes Pod** inside the Deckhouse-managed cluster it manages. No external cluster, no PaaS — in-cluster only.

The server supports dual transport: **stdio (default)** for local clients and **SSE (HTTP)** for in-cluster serving. The `deploy/` manifests target the in-cluster SSE mode — the Deployment sets `TRANSPORT=sse` and exposes port 8080; stdio is used when running locally.

```
git push
  → CI (lint + test + build image)
    → push image to registry
      → kubectl apply -f deploy/
        → Pod in d8-system namespace
          → readiness probe (tcpSocket :8080)
```

## 2. Environments

| Environment | Host | Purpose |
|-------------|------|---------|
| Local (Kind) | `localhost` | Development + integration tests |
| Production | In-cluster, `d8-system` NS | Live, managing Deckhouse cluster |

No staging environment. Local testing uses Kind (`task integration`).

## 3. Docker

### Dockerfile (multi-stage)

```dockerfile
FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /deckhouse-harness ./cmd/deckhouse-harness

FROM gcr.io/distroless/static-debian12
COPY --from=builder /deckhouse-harness /deckhouse-harness
USER nonroot:nonroot
ENTRYPOINT ["/deckhouse-harness"]
```

- **Builder**: `golang:1.26` — dependencies cached in a separate layer
- **Runtime**: `distroless/static-debian12` — minimal, no shell
- **CGO disabled** — fully static binary
- **Runs as nonroot** — security best practice
- **No exposed ports in Dockerfile** — port configured in K8s Service/Deployment

### Useful commands

```bash
task docker:build           # docker build -t deckhouse-harness:local .
task docker:load            # kind load docker-image deckhouse-harness:local --name d8
docker run --rm deckhouse-harness:local  # quick sanity check (will fail without K8s)
```

## 4. Kubernetes Manifests (`deploy/`)

### Deployment (`deployment.yaml`)

| Field | Value |
|-------|-------|
| Namespace | `d8-system` |
| Replicas | 1 |
| ServiceAccount | `deckhouse-harness` |
| Image | `deckhouse-harness:local` (override for production) |
| Container port | `8080` (HTTP/SSE) |
| CPU request/limit | `50m` / `200m` |
| Memory request/limit | `64Mi` / `128Mi` |

**Liveness probe:** `tcpSocket :8080`, delay 5s, period 30s  
**Readiness probe:** `tcpSocket :8080`, delay 3s, period 10s

### Service (`service.yaml`)

ClusterIP service on port 8080. Internal DNS:  
`deckhouse-harness.d8-system.svc.cluster.local:8080`

### RBAC (`rbac.yaml`)

ServiceAccount `deckhouse-harness` in `d8-system` with `ClusterRole`.

**Current permissions (all 43 tools, P0–P3):**

| Resource | APIGroup | Verbs |
|----------|----------|-------|
| `nodes`, `pods`, `events` | `""` (core) | `get`, `list` |
| `pods/log` | `""` (core) | `get`, `list` |
| `nodes` | `""` (core) | `update`, `patch` (cordon/uncordon/drain) |
| `pods/eviction` | `""` (core) | `create` (drain) |
| `secrets` (named: `d8-cluster-configuration`) | `""` (core) | `get`, `update` |
| `nodegroups`, `staticinstances`, `moduleconfigs`, `deckhousereleases`, `modules`, `modulesources`, `moduleupdatepolicies`, `modulereleases` | `deckhouse.io` | `get`, `list` |
| `staticinstances`, `sshcredentials`, `nodegroups`, `modulesources`, `moduleupdatepolicies`, `nodegroupconfigurations` | `deckhouse.io` | `create` |
| `staticinstances`, `sshcredentials`, `nodegroups`, `modulesources` | `deckhouse.io` | `delete` |
| `moduleconfigs` | `deckhouse.io` | `update`, `patch` |
| `deckhousereleases` | `deckhouse.io` | `patch` |

**Expanding RBAC for new handlers:**

When adding new handlers, expand `deploy/rbac.yaml` with least-privilege permissions for the resources each new tool touches. Always add only the specific verbs needed (no wildcards).

## 5. Rollout Strategy

Kubernetes default rolling update (1 replica — effectively recreate):
- `maxSurge: 25%`, `maxUnavailable: 25%` (default)
- 30s graceful shutdown: `httpServer.Shutdown(ctx)` with 30s timeout

Since the server is stateless (no persistent state, no DB), rollouts are safe at any time.

## 6. Health Checks

| Probe | Type | Port | Timing |
|-------|------|------|--------|
| Liveness | `tcpSocket` | 8080 | delay 5s, period 30s |
| Readiness | `tcpSocket` | 8080 | delay 3s, period 10s |

No HTTP healthcheck endpoint — TCP socket check is sufficient for an MCP SSE server.

The server is considered ready as soon as the TCP port is open (immediately after `ListenAndServe`).

## 7. Rollback Procedure

```bash
# 1. Identify issue (logs)
kubectl -n d8-system logs -l app=deckhouse-harness --tail=100

# 2. Rollback deployment
kubectl -n d8-system rollout undo deployment/deckhouse-harness

# 3. Verify
kubectl -n d8-system rollout status deployment/deckhouse-harness
kubectl -n d8-system get pods -l app=deckhouse-harness
```

## 8. Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TRANSPORT` | `stdio` | Transport mode: `stdio` (local) or `sse` (in-cluster; set by the Deployment) |
| `LISTEN_ADDR` | `:8080` | HTTP listen address for the SSE server |
