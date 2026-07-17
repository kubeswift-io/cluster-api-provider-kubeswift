# cluster-api-provider-kubeswift

A [Cluster API](https://cluster-api.sigs.k8s.io/) **infrastructure provider** that
reconciles workload-cluster machines into [KubeSwift](https://github.com/kubeswift-io/kubeswift)
`SwiftGuest` virtual machines. Model mirrors the Docker provider (CAPD): core
Cluster API and this provider run on a management cluster that has KubeSwift
installed; each `Machine` becomes a `SwiftGuest` VM that joins the workload cluster.

Read `docs/design/capi-kubeswift-architecture.md` before starting any work.

## Repository conventions (IMPORTANT — differ from the KubeSwift core repo)

- **AI tooling and design docs ARE committed here.** `.claude/` (agents), this
  `CLAUDE.md`, `docs/design/`, and `docs/spikes/` are tracked in git and pushed.
  (In the KubeSwift core repo they are gitignored-local; this repo is the opposite.)
- **License: Apache-2.0.** This is the Cluster API ecosystem norm and is a
  deliberate departure from KubeSwift's AGPL-3.0 core.
- Every commit is **signed off**: `git commit -s` as William Rizzo
  <william.rizzo@gmail.com>. No "Claude Code" / "Co-Authored-By" footer in commit
  messages (PR bodies may carry it).

## The license-clean boundary (do not break this)

The provider is Apache-2.0; KubeSwift is AGPL-3.0. To keep this codebase
license-clean, **never import KubeSwift's Go packages** (`github.com/kubeswift-io/kubeswift/...`).
Talk to KubeSwift only through the Kubernetes API:

- Use `sigs.k8s.io/controller-runtime` with `unstructured.Unstructured` (or a small
  locally-defined typed struct) against the `swift.kubeswift.io` group.
- The `SwiftGuest` GVK is `swift.kubeswift.io/v1alpha1`, kind `SwiftGuest`.
- If you need a typed field, define the minimal shape locally in this repo — do not
  vendor KubeSwift's types.

## Languages & structure

- **Go** (primary): CRD types, controllers, manager. kubebuilder v4 layout.
  - `api/v1alpha1/` — CRD Go types (`+kubebuilder:` markers).
  - `internal/controller/` — reconcilers.
  - `cmd/main.go` — manager entrypoint.
  - `config/` — CRD, RBAC, manager manifests (kustomize).
- **Rust**: only if a guest-side or host-side helper is later needed; none today.

## The Cluster API contract (what the CRDs must satisfy)

- **KubeSwiftCluster** (InfraCluster): `spec.controlPlaneEndpoint {host,port}`;
  `status.ready`. Owned by a `cluster.x-k8s.io` `Cluster`.
- **KubeSwiftMachine** (InfraMachine): `spec.providerID` (`kubeswift://<ns>/<name>`);
  `status.ready`, `status.addresses`, `status.failureReason/failureMessage`. Owned
  by a `cluster.x-k8s.io` `Machine`.
- **Templates** (`*Template`): wrap the spec in `spec.template.spec` for
  MachineDeployment / ClusterClass; no status.
- Honour the `cluster.x-k8s.io/paused` annotation and owner-ref wiring.

## Build & test

```bash
make generate      # deepcopy (controller-gen object)
make manifests     # CRDs + RBAC from +kubebuilder markers
make build         # compile the manager
make test          # envtest unit tests
make fmt vet       # gofmt + go vet
go build ./...     # quick compile check
```

After ANY change to Go types in `api/`: `make generate manifests`, then verify
`go build ./...` and `go vet ./...` are clean before committing.

## Agents

- `/staff-architect` — architecture, CRD/contract design, cross-cutting decisions.
- `/virtualization-architect` — the KubeSwift / SwiftGuest side: how a Machine maps
  to a VM, boot paths, networking, bootstrap/cloud-init delivery, providerID.
- `/golang-engineer` — all Go: controllers, kube clients, envtest, kubebuilder.
- `/rust-engineer` — any Rust work (guest/host helpers), cross-repo Rust context.
- `/security-engineer` — RBAC, controller privilege, tenant isolation (review only).
- `/tech-writer` — docs, design docs, runbooks, samples, CHANGELOG.

## Development principles

1. **Follow the Cluster API contract** — the CRD shapes and reconcile behaviours are
   specified upstream; match them, do not invent parallel conventions.
2. **License-clean boundary** — talk to KubeSwift via the API, never by Go import.
3. **No silent failures** — status/conditions must reflect real state; surface
   terminal failures via `failureReason`/`failureMessage`.
4. **Minimalism** — no dependency or abstraction that the contract does not require.
5. **Verified before merged** — `go build`, `go vet`, tests green; no speculative code.

## Background

KubeSwift itself lives at `../kubeswift` (separate repo, AGPL-3.0). Read its docs for
VM/SwiftGuest behaviour, but this repo does not depend on its code.
