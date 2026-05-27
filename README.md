# Pronto

A custom Kubernetes scheduler that uses **Functional Principal Component Analysis (FPCA)** to predict node load and make smarter pod placement decisions.

The default Kubernetes scheduler places pods based on declared resource *requests*, which do not reflect actual runtime utilisation. Pronto addresses this by continuously monitoring real CPU and memory usage on each node, distilling it into a compact *job signal* via FPCA, and routing new pods to the node with the most available capacity.

> **Note:** This repository is the original prototype implementation. A refactored version built on the official Kubernetes Scheduling Framework is available at [pronto-framework](https://github.com/LucaChot/pronto-framework).

---

## Architecture

Pronto is composed of three components:

```
┌─────────────────────────────────────────────────────────────┐
│                      Kubernetes Cluster                      │
│                                                              │
│  Control Plane                                               │
│  ┌─────────────────────┐                                     │
│  │   Central Scheduler  │◄── watches for unscheduled pods    │
│  │                      │    binds pods to nodes via K8s API │
│  │  nodeSignals[]       │                                     │
│  └──────────┬───────────┘                                    │
│             │ gRPC (RequestPod)                              │
│             │                                                │
│  Worker Nodes (DaemonSet)                                    │
│  ┌───────────────────┐   ┌───────────────────┐              │
│  │  Remote Scheduler │   │  Remote Scheduler │  ...         │
│  │                   │   │                   │              │
│  │  MetricsCollector │   │  MetricsCollector │              │
│  │  (CPU + RAM/1s)   │   │  (CPU + RAM/1s)   │              │
│  │       │           │   │       │           │              │
│  │  FPCAAgent        │   │  FPCAAgent        │              │
│  │  (local update)   │   │  (local update)   │              │
│  │       │           │   │       │           │              │
│  └───────┼───────────┘   └───────┼───────────┘              │
│          │                       │                           │
│          └──────────┬────────────┘                           │
│                     │ gRPC (RequestAggMerge)                 │
│          ┌──────────▼────────────┐                           │
│          │      Aggregator       │                           │
│          │  (global FPCA merge)  │                           │
│          └───────────────────────┘                           │
└─────────────────────────────────────────────────────────────┘
```

### Central Scheduler
Runs on the control plane as a single `Deployment`. It watches for pods that request the `pronto` scheduler, maintains a per-node job signal updated by remote schedulers, and binds each pod to the node with the lowest signal (most available capacity) using the Kubernetes API directly.

### Remote Scheduler (DaemonSet)
Runs on every worker node. Every second it:
1. Collects CPU and RAM utilisation via `gopsutil`
2. Feeds the measurements into a local **FPCA** update, producing a low-rank embedding `(U, Σ)` of the node's resource usage over time
3. Requests a global merge from the Aggregator to align all nodes to a shared embedding space
4. Computes a scalar **job signal** from the current measurement and the global embedding
5. If the signal is below a threshold (node has capacity), calls `RequestPod` on the Central Scheduler to advertise availability

### Aggregator
Runs as a single `Deployment`. It maintains a global `U·Σ` matrix and merges incoming local estimates from remote schedulers using a concatenation + truncated SVD approach (`AggMerge`). This ensures all nodes compute their signals relative to the same shared subspace, making signals comparable across the cluster.

---

## Algorithm: FPCA-based Job Signal

Each node's resource usage is modelled as a multivariate time series of CPU and RAM samples collected over a sliding window. FPCA decomposes this into a low-rank basis `U` (principal components) and singular values `Σ`.

The **job signal** for a node is computed as:

```
signal = sum(|y^T · U| · Σ)
```

where `y` is the current measurement vector. A lower signal indicates more available capacity. The Central Scheduler always places the next pod on the node with the minimum signal.

The distributed aggregation step (`AggMerge`) merges local `(U, Σ)` estimates across nodes so that the basis vectors are globally consistent, improving placement quality under heterogeneous workloads.

---

## Repository Structure

```
cmd/
  central_sched/   # Central scheduler binary
  remote_sched/    # Remote scheduler binary (runs on each worker node)
  aggregator/      # Aggregator binary
src/
  central/         # Central scheduler logic and K8s API interactions
  remote/          # Remote scheduler loop and job signal computation
  fpca/            # FPCA agent (local updates + aggregator client)
  metrics/         # CPU and RAM collection via gopsutil
  matrix/          # FPCA math: SVD, Merge, AggMerge, Rank
  aggregate/       # Aggregator server
  message/         # Protobuf/gRPC definitions
deploy/            # Kubernetes manifests
docker/            # Dockerfiles
```

---

## Building

Each component is built separately using the `SCHED` variable:

```bash
# Build the central scheduler
make compile SCHED=CTL

# Build the remote scheduler
make compile SCHED=RMT

# Build the aggregator
make compile SCHED=AGG
```

To build and push Docker images (requires Docker login):

```bash
make SCHED=CTL
make SCHED=RMT
make SCHED=AGG
```

To regenerate the protobuf bindings:

```bash
make msg
```

---

## Deployment

The manifests in `deploy/` assume a multi-node Kubernetes cluster. Apply them in order:

```bash
kubectl apply -f deploy/namespace.yaml
kubectl apply -f deploy/aggregator-service.yaml
kubectl apply -f deploy/aggregator.yaml
kubectl apply -f deploy/central-sched-service.yaml
kubectl apply -f deploy/central-sched.yaml
kubectl apply -f deploy/remote-sched-service.yaml
kubectl apply -f deploy/remote-sched.yaml
```

To schedule a pod with Pronto, set `spec.schedulerName: pronto` in the pod manifest.

---

## Dependencies

| Dependency | Purpose |
|---|---|
| `k8s.io/client-go` | Kubernetes API interactions |
| `google.golang.org/grpc` | Inter-component communication |
| `gonum.org/v1/gonum` | Matrix operations (SVD, dense/diagonal matrices) |
| `github.com/shirou/gopsutil` | CPU and RAM metrics collection |
| `github.com/sirupsen/logrus` | Structured logging |
