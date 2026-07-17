# cluster-api-provider-kubeswift — Architecture

> Status: design anchor for the initial provider. Written alongside the scaffold.
> The CRDs exist and compile; the controllers described here are the implementation
> plan, not yet built.

## 1. Purpose

Provide a [Cluster API](https://cluster-api.sigs.k8s.io/) (CAPI) **infrastructure
provider** so that Kubernetes clusters can be declared with the standard CAPI
resources (`Cluster`, `MachineDeployment`, `KubeadmControlPlane`, ...) and have
their machines materialised as [KubeSwift](https://github.com/kubeswift-io/kubeswift)
`SwiftGuest` virtual machines.

This makes KubeSwift a first-class CAPI substrate: the same declarative,
GitOps-friendly cluster lifecycle that operators use for AWS/Azure/vSphere, backed
by KubeSwift VMs on their own hardware.

## 2. Where this fits in Cluster API

CAPI splits a cluster into cooperating providers:

| Provider | Example | Responsibility |
|----------|---------|----------------|
| Core | `cluster-api` | orchestrates the lifecycle |
| Bootstrap | Kubeadm (CABPK) | turns a machine into a node (cloud-init) |
| Control plane | KubeadmControlPlane (KCP) | manages control-plane machines |
| **Infrastructure** | **this provider** | provisions the machines + cluster infra |

This provider implements only the **infrastructure** contract. Bootstrap stays
Kubeadm; control plane stays KCP. We provide the VMs and the cluster-level
infrastructure state.

## 3. Topology

The model mirrors the Docker provider (CAPD), which runs workload nodes as
containers on the management cluster. Here they are VMs:

```
management cluster
  core Cluster API  +  Kubeadm bootstrap  +  KubeadmControlPlane
  cluster-api-provider-kubeswift (this)
  KubeSwift (swiftletd + Cloud Hypervisor)     <- must be installed here

  Cluster ─────────── KubeSwiftCluster           control-plane endpoint, readiness
  Machine ─────────── KubeSwiftMachine ── SwiftGuest VM   (a workload node)
  Machine ─────────── KubeSwiftMachine ── SwiftGuest VM
  ...
```

The `SwiftGuest` VMs boot on the management cluster's KubeSwift nodes and together
form the **workload cluster**. The workload cluster's API server is reached through
`KubeSwiftCluster.spec.controlPlaneEndpoint`.

## 4. License-clean boundary

The provider is **Apache-2.0**; KubeSwift is **AGPL-3.0**. To keep the two
codebases license-clean, the provider **never imports KubeSwift Go packages**. It
talks to KubeSwift only through the Kubernetes API:

- `SwiftGuest` is created/read/deleted as `unstructured.Unstructured` (or a minimal
  locally-defined typed struct) at GVK `swift.kubeswift.io/v1alpha1`, kind
  `SwiftGuest`, via the controller-runtime client.
- Any field shape the provider needs (e.g. `status.network.primaryIP`,
  `status.phase`) is referenced by JSON path, not by importing the KubeSwift type.

This is also why the provider CRDs redefine small helpers (`APIEndpoint`,
`MachineAddress`, `ObjectMeta`) locally rather than importing CAPI's `v1beta*`
types into the API package. The controllers may import CAPI's Go module (Apache-2.0)
for the `Cluster`/`Machine` owner objects and `util` helpers.

## 5. CRDs and the contract

All in group `infrastructure.cluster.x-k8s.io`, version `v1alpha1`.

### KubeSwiftCluster (InfraCluster)
- `spec.controlPlaneEndpoint {host, port}` — how to reach the workload API server.
  KubeSwift does not provision a load balancer, so in v0 the operator supplies this
  (a control-plane VIP or an external LB). Later phases can automate it (kube-vip).
- `spec.guestNamespace` — namespace for the backing `SwiftGuest` VMs (defaults to
  the KubeSwiftCluster namespace).
- `status.ready` — set true once the endpoint is known and cluster infra is up.
- `status.conditions`.

### KubeSwiftMachine (InfraMachine)
- `spec.providerID` — `kubeswift://<guest-namespace>/<guest-name>`. Set by the
  controller once the VM exists. The CAPI Machine controller matches it to a Node.
- `spec.imageRef` / `spec.guestClassRef` / `spec.resources` / `spec.networkRef` —
  the VM shape, rendered into a `SwiftGuest`.
- `status.ready` — true once the VM is running.
- `status.addresses` — VM addresses, surfaced to the Machine.
- `status.failureReason` / `status.failureMessage` — terminal failure surface.

### KubeSwiftClusterTemplate / KubeSwiftMachineTemplate
- `spec.template.{metadata,spec}` — templated bodies for ClusterClass and
  MachineDeployment. No status.

## 6. Reconcile flows (implementation plan)

### KubeSwiftMachine controller (the core of the provider)
1. Fetch the owning CAPI `Machine` (via owner ref). If absent or `paused`, requeue.
2. Wait for: the owning `Cluster` infra ready, and
   `Machine.spec.bootstrap.dataSecretName` populated (bootstrap provider produced
   cloud-init).
3. Read the bootstrap Secret (cloud-init user-data).
4. Render a `SwiftGuest` (unstructured) into `guestNamespace`:
   - `imageRef` / guest class / resources / network from the KubeSwiftMachine spec.
   - cloud-init user-data delivered as a NoCloud seed (via a `SwiftSeedProfile` or
     inline seed), so the node runs Kubeadm and joins.
   - set `--provider-id=kubeswift://<ns>/<name>` in the kubelet args **through the
     bootstrap data** so the Node registers with a providerID matching this
     KubeSwiftMachine (see Risks).
   - owner ref → KubeSwiftMachine; label `cluster.x-k8s.io/cluster-name`.
5. Watch the `SwiftGuest`. When it reports Running with an IP:
   - set `spec.providerID`, `status.addresses`, `status.ready=true`, conditions.
6. Finalizer: on delete, delete the `SwiftGuest`, then remove the finalizer.

### KubeSwiftCluster controller
1. Fetch the owning `Cluster`. If `paused`, requeue.
2. When `spec.controlPlaneEndpoint` is set (operator-provided in v0), set
   `status.ready=true` and the `Cluster`-visible condition.
3. Finalizer for any future cluster-scoped infra (none in v0).

## 7. Bootstrap and providerID (the hard part)

- **Bootstrap delivery:** the Kubeadm bootstrap provider writes cloud-init to a
  Secret. The provider injects that into the VM as NoCloud user-data. KubeSwift's
  existing cloud-init / `SwiftSeedProfile` path is the delivery mechanism.
- **providerID match:** CAPI links `InfraMachine.spec.providerID` to a workload
  `Node.spec.providerID`. For the match to happen, the node's kubelet must start
  with `--provider-id=kubeswift://<ns>/<name>`. We compute that ID at render time
  and inject it via the bootstrap config (KubeadmConfig `kubeletExtraArgs`, or a
  cloud-init file). No in-cluster cloud-provider is required for whole-VM nodes.

## 8. Phases

- **P0 — scaffold (this).** CRDs + contract fields, compiling manager, docs, agents.
- **P1 — single-node happy path.** KubeSwiftMachine → SwiftGuest → providerID +
  ready; static operator-provided control-plane endpoint; one control-plane node
  joins; `KubeSwiftCluster` ready. Manual `clusterctl`/kubectl walkthrough.
- **P2 — lifecycle.** Finalizers, conditions, delete/GC, `paused`, failure surfaces.
- **P3 — workers + templates.** MachineDeployment via KubeSwiftMachineTemplate;
  scale up/down; node deletion.
- **P4 — ClusterClass.** KubeSwiftClusterTemplate; managed topologies.
- **P5 — control-plane endpoint automation.** kube-vip (or equivalent) so the
  operator no longer hand-provisions the VIP.
- **P6 — e2e + clusterctl packaging.** `metadata.yaml` contract pinned to the target
  CAPI version, `clusterctl generate` templates, CAPI test-framework e2e.
- **Later — flavors.** GPU machines (SwiftGPU), telco/NFV machines (SR-IOV,
  vhost-user), Windows nodes.

## 9. Risks and open questions

1. **Control-plane endpoint chicken-and-egg.** Kubeadm needs the endpoint before the
   first control-plane node boots. v0 requires the operator to provide a VIP up
   front; P5 automates it. This is the same problem every infra provider solves.
2. **providerID / Node match.** The bootstrap must set kubelet `--provider-id` to
   the value the provider computes. If they diverge, the Machine never becomes a
   Node. Pin this in P1.
3. **CAPI module version.** The controllers will import the CAPI Go module for
   `Cluster`/`Machine`. CAPI v1.11 reorganised its API packages; pin the exact
   import path and contract version (`metadata.yaml`) when P1 wires the controllers.
4. **KubeSwift required on the management cluster.** The VMs run where KubeSwift
   runs. Document this prerequisite; do not assume a separate infra cluster in v0.
5. **SwiftGuest status shape coupling.** The provider reads `SwiftGuest` status by
   JSON path (license-clean). Those paths are a coupling point; isolate them in one
   small mapping function so a KubeSwift status change is a one-file fix.
6. **Network / IP reachability.** The workload API server endpoint and node IPs must
   be reachable from the management cluster's CAPI controllers. Works cleanly with
   the KubeSwift OVN-Kubernetes / multi-node L2 datapaths; document the constraint.

## 10. Non-goals (v0)

- No load-balancer provisioning (operator supplies the endpoint).
- No MachineHealthCheck remediation specifics beyond the standard contract.
- No GPU/telco machine flavors yet (planned, see Phases).
- No managed KubeSwift install — KubeSwift is a prerequisite on the mgmt cluster.
