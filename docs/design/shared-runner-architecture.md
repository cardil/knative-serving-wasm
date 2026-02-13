# WASM Shared Runners Architecture

> **Status:** Proposal  
> **Authors:** knative-serving-wasm maintainers  
> **Target:** v1alpha1

## Executive Summary

Today each `WasmModule` creates a dedicated Knative Service with its own runner
pod. The cold start path is: schedule pod → pull runner image → download WASM
module from OCI → compile WASM → serve. This puts WASM startup on par with
(or worse than) regular containers, negating WASM's key advantage: tiny modules.

**Insight:** Runner images are ~50-100 MB; WASM modules are ~100 KB-2 MB.
A pool of long-lived runners can host many modules, with intelligent placement
and on-demand loading in milliseconds — no pod scheduling needed.

### Startup Comparison

| Approach | State | What happens | Latency |
|---|---|---|---|
| Knative container | cold | Schedule pod → pull image → start process | ~3-10 s |
| Knative container | warm | Reuse running pod | <10 ms |
| WASM PoC | cold | Schedule pod → pull runner image → download .wasm → compile | ~2-5 s |
| WASM PoC | warm | Reuse running pod with compiled module | <10 ms |
| WASM shared runner | cold | Download .wasm into running pod → compile | <100 ms |
| WASM shared runner | warm | Route to in-memory module | <10 ms |

The shared runner pool **bypasses the Kubernetes scheduler entirely** for WASM
module scaling. Runner pods are already running — modules are loaded/unloaded at
runtime via the runner's control API, not by creating new pods. No scheduling, no
image pulling, no container startup. Only a lightweight OCI fetch of a
sub-megabyte module. This is the architectural advantage WASM was designed for.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         Kubernetes Cluster                      │
│                                                                 │
│  ┌──────────────┐      ┌─────────────────────────────────────┐  │
│  │  Controller  │      │       Default Runner Pool           │  │
│  │              │      │  ┌─────────┐ ┌─────────┐            │  │
│  │ - watches    │─────▶│  │Runner 1 │ │Runner 2 │ ...        │  │
│  │   WasmModule │      │  │ A, B, C │ │ D, E    │            │  │
│  │ - schedules  │      │  └─────────┘ └─────────┘            │  │
│  │   placement  │      └─────────────────────────────────────┘  │
│  └──────────────┘                                               │
│         │              ┌─────────────────────────────────────┐  │
│         └─────────────▶│       Named Runner: team-x          │  │
│                        │  ┌─────────┐                        │  │
│                        │  │ F, G    │  (isolated)            │  │
│                        │  └─────────┘                        │  │
│                        └─────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Runner Selection Model

Users specify `spec.runner` in WasmModule:

| Value | Behavior |
|---|---|
| `""` or `default` | Intelligent placement across the default runner pool |
| `<name>` | Dedicated runner for isolation/compliance/custom config |

**Default runner pool** — Multiple runner pods managed by the controller. Module
placement is determined by a scheduler that considers:
- Module size and declared memory limits
- Current runner capacity and load
- Historical telemetry (request patterns, memory usage)
- Co-location affinity (modules that call each other)
- Future: AI-based optimization for composition

The scheduler can rebalance modules across runners without user intervention.

**Named runners** — User-controlled, isolated runner instances for compliance,
custom configuration, or guaranteed performance isolation.

## Module Lifecycle

Modules progress through distinct states, with bytes storage split into two tiers:

```
┌──────────┐   ┌──────────┐   ┌────────┐   ┌────────┐   ┌──────────┐   ┌─────────┐
│ Unloaded │──▶│ Fetching │──▶│ Stored │──▶│ Loaded │──▶│ Compiled │──▶│ Running │
└──────────┘   └──────────┘   └────────┘   └────────┘   └──────────┘   └─────────┘
      ▲             │              │            │             │             │
      │             └──────────────┴────────────┴─────────────┴─────────────┘
      │                            │                             (eviction)
      │                            ▼
      │    (CR update)        ┌─────────┐
      └───────────────────────│  Error  │
                              └─────────┘
```

| State | Storage | Latency to serve | Survives restart |
|---|---|---|---|
| **Unloaded** | None | ~100+ ms (fetch + compile) | yes (metadata only) |
| **Fetching** | Downloading | N/A | no |
| **Stored** | Disk only | ~60-100 ms (read + compile) | yes |
| **Loaded** | Memory + disk | ~50-80 ms (compile) | no |
| **Compiled** | Machine code | ~1-5 ms (instantiate) | no |
| **Running** | Active instance | <1 ms | no |
| **Error** | Error details | N/A | yes |

### Eviction Strategy

Multi-tier eviction allows fine-grained memory/disk management:

1. **Running → Compiled**: Drop idle instances, keep compiled code
2. **Compiled → Loaded**: Drop machine code, keep bytes in memory
3. **Loaded → Stored**: Free RAM, bytes still on disk
4. **Stored → Unloaded**: Clear disk cache, refetch on next request

Each tier has independent LRU tracking and configurable limits.

### Error Handling

Errors can occur at any stage:
- **Fetching**: Invalid image reference, auth failure, network error
- **Loaded → Compiled**: Invalid WASM, missing exports, compile failure
- **Running**: Runtime trap, fuel exhaustion, memory limit exceeded

**Error is a terminal state** — recovery requires user to update the WasmModule
CR (e.g., fix the image reference). On CR update, the controller resets state
to Unloaded and begins fresh reconciliation.

## Module Isolation

When multiple modules share a runner, each must be isolated from others:

### Volume Mounts

Volume handling spans three layers:

| Layer | Scope | Mutable at Runtime |
|---|---|---|
| K8s Volumes | Pod spec - storage sources | **NO** - requires pod recreation |
| K8s VolumeMounts | Runner filesystem paths | **NO** - requires pod recreation |
| WASI Preopens | Guest paths per module | **YES** - per-module config |

**Key insight**: [`builder.preopened_dir(host_path, guest_path, ...)`](runner/src/server.rs:203)
supports aliasing — the host path and guest path can differ.

#### Runtime-Stable Volume Strategy

To avoid pod recreation when deploying new modules:

```
┌───────────────────────────────────────────────────────────────────┐
│                     Three-Layer Volume Model                      │
├───────────────────────────────────────────────────────────────────┤
│  K8s Volume (pod spec)        │  PVC: shared-data                │
│  K8s VolumeMount (runner)     │  /wasm-volumes/shared-data       │
│  WASI Preopen (module-a)      │  host: /wasm-volumes/shared-data │
│                               │  guest: /data                    │
│  WASI Preopen (module-b)      │  host: /wasm-volumes/shared-data │
│                               │  guest: /storage                 │
└───────────────────────────────────────────────────────────────────┘
```

Runners mount volumes to prefixed paths (`/wasm-volumes/{volume-name}`).
Each module's preopen remaps to its expected guest path at runtime.

#### Volume Profile Matching

Runners are tagged with their "volume profile" — the set of mounted volumes.
The controller places modules on runners with compatible profiles:

| Module Needs | Runner Has | Result |
|---|---|---|
| (none) | (any) | **ALLOWED** - volumeless, fast placement |
| `pvc-A` | `pvc-A, pvc-B` | **ALLOWED** - required volume present |
| `pvc-A, pvc-B` | `pvc-A` | **NEW RUNNER** - missing volume |

#### Volume Access Isolation

Guest paths can be identical across modules (each has isolated WASI context).
The protection is against **unintentional shared volume access**.

**Per-volume opt-in**: We extend `corev1.Volume` with a wrapper type:

```go
type WasmVolume struct {
    corev1.Volume `json:",inline"`
    Shared bool `json:"shared,omitempty"`
}
```

**Conflict detection** - when two modules on the same runner reference the same volume:

| Module A | Module B | Result |
|---|---|---|
| `pvc-A` at `/data` | `pvc-B` at `/data` | **ALLOWED** - different volumes |
| `pvc-A` at `/mysql-data` | `pvc-A` at `/pgdata` | **REJECTED** - same volume, no opt-in |
| `pvc-A` + `shared: true` | `pvc-A` at `/pgdata` | **REJECTED** - both must opt-in |
| `pvc-A` + `shared: true` | `pvc-A` + `shared: true` | **ALLOWED** - mutual consent |

**Rule**: Two modules accessing the same volume must BOTH declare `shared: true`.

### Environment Variables

Each module has isolated environment variables. Variables are scoped to module
instances — no cross-module visibility.

### Network Permissions

Per-module network configuration (tcp.connect, udp.bind, etc.) is enforced via
the runner's socket permission checks. Modules cannot escalate permissions of
other modules on the same runner.

**Port binding validation**: Two modules on the same runner cannot bind to the
same port. The controller rejects CRs that would cause port conflicts.

### Resource Limits

Memory and CPU limits (fuel) are enforced per-module instance:
- Each WASM instance has its own `StoreLimits`
- Fuel consumption is tracked per-request
- One module exhausting limits does not affect others

**Capacity planning**: Module resource requests are summed and must not exceed
runner capacity. Runner pool sizing is configured via ConfigMap (not per-module
CRs), allowing cluster admins to control:
- Default runner pool size and resource allocation
- Named runner configurations
- Memory/CPU limits per runner pod

## Request Routing

Routing uses **Host header dispatch** — cleaner than path prefixing, no URL rewriting.

### K8s Service Model

Each WasmModule gets a dedicated K8s Service with unique DNS name:
- `module-a.default.svc.cluster.local` → shared runner pod
- `module-b.default.svc.cluster.local` → shared runner pod

All Services share the same `selector` pointing to runner pods:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: module-a
  namespace: default
spec:
  selector:
    wasm.knative.dev/runner: default  # Shared runner pool
  ports:
    - port: 80
      targetPort: 8080
```

### Runner Dispatch

The runner extracts the Host header and routes to the matching module:

```
Client Request                        Runner Pod
      │                                   │
      │  Host: module-a.default.svc       │
      ├──────────────────────────────────►│
      │                                   │
      │                    ┌──────────────┴──────────────┐
      │                    │      Routing Table          │
      │                    │  module-a.* → module-a ctx  │
      │                    │  module-b.* → module-b ctx  │
      │                    └──────────────┬──────────────┘
      │                                   │
      │                                   ▼
      │                            Execute module-a
      │                            WASI context
```

### Lazy Loading on Request

Requests to modules in non-Running states trigger just-in-time loading:

| Current State | Action | Latency |
|---|---|---|
| Running | Direct dispatch | <1ms |
| Compiled | Instantiate | ~1ms |
| Stored (disk) | Load → Compile → Instantiate | ~10-50ms |
| Unloaded | Fetch → Store → Load → Compile → Instantiate | ~100-500ms |

## Trade-offs

### Benefits

| Aspect | 1:1 Model | Shared Runners |
|---|---|---|
| Cold start | 2-5 seconds (pod creation) | <100ms (module load) |
| Warm start | <10ms (brief window before scale-to-zero) | <10ms (compiled module cached) |
| Memory overhead | ~50-100MB per runner pod | Amortized across modules |
| K8s scheduler bypass | No | Yes - module placement at runtime |
| Module density | 1 per pod | 10-100+ per pod |

### Costs

| Aspect | Impact | Mitigation |
|---|---|---|
| Blast radius | Runner crash affects all modules | Health checks, graceful degradation |
| Volume changes | Pod recreation disrupts co-located modules | Volume profile matching |
| Isolation boundary | Process-level, not pod-level | WASI sandboxing, resource limits |
| Complexity | Multi-module state management | Well-defined state machine |
| Debugging | Shared logs across modules | Per-module log files (ConfigMap option) |
| Readiness model | K8s readiness is pod-level, not module-level | Module-level readiness in WasmModule status |
| Telemetry | Must aggregate pod + module metrics | Multi-layer telemetry collection |

**Telemetry layers**: The runner must expose both pod-level metrics (memory, CPU,
network) and per-module metrics (request count, latency, fuel consumption, errors).
Module telemetry must be isolated via labels/prefixes to prevent metric collisions:

```
wasm_module_requests_total{module="module-a", namespace="default"} 1234
wasm_module_requests_total{module="module-b", namespace="default"} 567
wasm_runner_memory_bytes{runner="default-pool-1"} 104857600
```

**Logging**: Per-module log files can be enabled via runner ConfigMap:
```yaml
data:
  logging.perModuleFiles: "true"  # Creates /var/log/wasm/{module-name}.log
```

**Readiness probe limitation**: K8s marks the runner pod as Ready once it starts.
New modules deployed to a running pod bypass K8s readiness probes entirely.
The controller must track per-module readiness via WasmModule status conditions:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: ModuleRunning
    - type: ModuleLoaded
      status: "True"
      reason: CompiledAndCached
```

Clients should check WasmModule status, not pod readiness.

### When to Use Named Runners

Named runners provide stronger isolation at the cost of density:

| Use Case | Runner Type |
|---|---|
| General workloads, microservices | Default pool |
| Compliance requirements (PCI, HIPAA) | Named, dedicated |
| Modules with specific volume needs | Named with volume profile |
| Resource-intensive modules | Named with higher limits |

## Scale-to-Zero

Shared runners change the scale-to-zero model:

| Model | Trigger | Wake Time |
|---|---|---|
| 1:1 (current) | Pod termination after idle | 2-5s (pod creation) |
| Shared | Module eviction after idle | <100ms (module reload) |

With shared runners, scale-to-zero becomes optional. Keeping `minScale: 1` for the
runner pool is often beneficial — the cost of one warm pod is amortized across
potentially hundreds of modules. Individual modules can still be evicted while the
runner remains warm, ready for instant reloads.

## Failure Recovery

| Failure Type | Detection | Recovery |
|---|---|---|
| Module panic | Caught by WASI runtime | Mark Error state, log, continue serving other modules |
| Runner crash | K8s liveness probe | Pod restart, reload all assigned modules |
| OOM | K8s OOMKilled | Pod restart, reload modules with LRU priority |
| Compile error | Caught during load | Mark Error state, reject requests to that module |

**Module restart**: Error state is terminal. To recover, the user must update the
WasmModule CR (fix image, config), triggering a new reconciliation cycle.

## Closing Thoughts

This architecture shifts WASM workload management from K8s pod orchestration
to in-process module orchestration. The key enabler is WASM's tiny footprint —
a 100-200KB module (typical for our examples) doesn't justify a 100MB pod.

By treating the runner as a multi-tenant runtime and modules as lightweight
tenants, we achieve the density of serverless with the control of containers.

This approach enables competing with cloud Lambda-like solutions in both
performance and footprint, while remaining fully open-source and tunable.
No vendor lock-in, no opaque runtime — just WASI modules on Kubernetes.
