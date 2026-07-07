<!-- generated: 2026-07-07, template: development.md -->
# Tools & Commands — Deckhouse Harness

## 0. Dev Environment Setup

**Prerequisites:**

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.26+ | `brew install go` |
| go-task | latest | `brew install go-task` |
| easyp | latest | `brew install easyp-tech/tap/easyp` |
| Docker | latest | [docker.com](https://docs.docker.com/get-docker/) |
| Kind | latest | `brew install kind` (for integration tests) |

**First Run:**
```bash
git clone https://github.com/easyp-tech/deckhouse-harness
cd deckhouse-harness
easyp mod download    # download proto dependencies
task generate         # generate *.pb.go + *.mcp.go
task build            # verify build
task test             # run unit tests
```

The server runs inside a Kubernetes cluster. For local testing, use Kind + integration tasks.

## 1. Overview

All commands are run via `go-task` (`task`). Run `task --list` for a full list. Proto generation uses `easyp`.

## 2. Quick Reference

| Action | Command |
|--------|---------|
| Generate code | `task generate` |
| Lint proto | `task lint` |
| Build binary | `task build` |
| Unit tests | `task test` |
| Docker build | `task docker:build` |
| Load into Kind | `task docker:load` |
| Integration tests | `task integration` |

## 3. Detailed Commands

### Code Generation

```bash
task generate
# Equivalent to:
easyp mod download   # fetch proto deps declared in easyp.yaml
easyp generate       # generate *.pb.go + *.mcp.go from *.proto
```

**What it generates:**
- `proto/deckhouse/v1/*.pb.go` — protobuf message types
- `proto/deckhouse/v1/*.mcp.go` — MCP tool handler interfaces + `Register*Tools()` functions

**Config:** `easyp.yaml` — plugins, managed mode, output paths.

Run after **any** change to a `.proto` file.

### Linting

```bash
task lint
# Equivalent to:
easyp lint
```

Lints all `.proto` files against rules in `easyp.yaml`. Run before committing proto changes.

### Building

```bash
task build
# Equivalent to:
go build ./cmd/deckhouse-harness
```

Output: `./deckhouse-harness` binary (or `./deckhouse-harness.exe` on Windows).

### Testing

```bash
task test
# Equivalent to:
go test ./...
```

Runs all 134 unit tests. Total time ~3 min due to polling tests `AddWorkerNode`, `WaitNodeReady`, and `DrainNode`, which use a real 30s clock (each ~30s).

To run a specific test:
```bash
go test ./internal/handler/ -run TestGetClusterStatus -v
```

### Docker

```bash
task docker:build
# docker build -t deckhouse-harness:local .

task docker:load
# kind load docker-image deckhouse-harness:local --name d8
```

Multi-stage Dockerfile: `golang:1.26` builder → `distroless/static-debian12` runtime.

### Integration Tests

```bash
task integration
# Equivalent to: setup → test → teardown

task integration:setup     # create Kind cluster, load CRDs + fixtures
task integration:test      # run test.sh against the cluster
task integration:teardown  # delete Kind cluster
```

Integration tests require Docker and Kind installed.

## 4. Code Generation Details

| Command | Config file | Generates |
|---------|-------------|-----------|
| `easyp mod download` | `easyp.yaml` → `deps` section | proto deps in vendor/proto |
| `easyp generate` | `easyp.yaml` → `generate` section | `*.pb.go`, `*.mcp.go` |

Proto source files: `proto/deckhouse/v1/*.proto`  
Generated output: same directory (`proto/deckhouse/v1/`)

## 5. CI/CD Cheatsheet

Simulate CI locally:
```bash
easyp lint             # proto lint (must pass)
task generate          # ensure generated code is up to date
git diff --exit-code   # fail if generated files differ (generation check)
task build             # build must succeed
task test              # all tests must pass
task docker:build      # image must build
```

## 6. Tool Installation

| Tool | Install |
|------|---------|
| `go-task` | `brew install go-task` |
| `easyp` | `brew install easyp-tech/tap/easyp` |
| `kind` | `brew install kind` |
| `protoc-gen-mcp` | installed automatically via `easyp generate` (managed mode) |
