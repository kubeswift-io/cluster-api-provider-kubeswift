# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.1] — 2026-07-20

### Added
- **Multi-node cluster template** (`clusterctl generate cluster --flavor multi-node`):
  sets `backend.swiftGuest.nodeNetworkRef` on both machine templates and ships the
  bootstrap that makes it work — secondary-NIC DHCP bring-up (taking no default route
  and no DNS from it), kubelet `--node-ip` pinned to the routable secondary address,
  and on control-plane nodes the apiserver `--advertise-address` re-pointed at the
  same address. Previously these were hand-rolled per cluster. The node IP is
  discovered as the global IPv4 that is *not* on the default-route interface, so the
  template works for both a shared secondary UDN and per-node bridge NADs. Published
  as a release asset alongside the default flavor.
- **Operator note for the single-control-plane service hairpin**
  (`docs/operations/single-control-plane-hairpin.md`): the symptom, why the hairpin
  happens, and the three ways out (add a worker and move the pods, bypass the
  ClusterIP with `KUBERNETES_SERVICE_HOST`, or enable kube-proxy masquerade / CNI
  hairpin mode).

## [v0.1.0] — 2026-07-19

First release: a Cluster API infrastructure provider that backs `Machine`s with
KubeSwift `SwiftGuest` VMs. Installable via `clusterctl`.

### Added
- Repository scaffold for the Cluster API infrastructure provider (kubebuilder).
- CRDs (`v1alpha1`, group `infrastructure.cluster.x-k8s.io`): `KubeSwiftCluster`,
  `KubeSwiftClusterTemplate`, `KubeSwiftMachine`, `KubeSwiftMachineTemplate`, with
  the Cluster API v1beta2 infrastructure-provider contract fields (control-plane
  endpoint, `providerID`, `status.initialization.provisioned`, addresses, conditions).
- Architecture design doc (`docs/design/capi-kubeswift-architecture.md`).
- **KubeSwiftCluster + KubeSwiftMachine reconcilers** (v1beta2 contract): the cluster
  reports provisioned once a control-plane endpoint is set; the machine gates on
  cluster-infrastructure-ready + the bootstrap secret, creates a SwiftGuest +
  SwiftSeedProfile via the unstructured client (an internal `Backend` seam; SwiftGuest
  today) applying the bootstrap data verbatim, and surfaces `addresses` /
  `status.initialization.provisioned`.
- **Node providerID**: the machine controller sets each workload Node's `spec.providerID`
  by patching the Node through the cluster's kubeconfig (kubelet `--provider-id` is
  unreliable), the same pattern CAPD uses. The provider must run in the management
  cluster to reach the workload endpoint.
- **Control-plane endpoint modes** (`KubeSwiftCluster.spec.endpoint.mode`): `External`
  (operator supplies the endpoint) and `Service` — the provider mints a Service
  selecting the control-plane pool, so the endpoint (ClusterIP, or LoadBalancer via
  `service.type`) exists before kubeadm runs and no OVN or operator VIP is required.
  Validated end-to-end (a full workload cluster to CoreDNS-Ready) on Calico.
- **ClusterClass support**: a `KubeSwiftMachineTemplate` validating webhook enforcing
  `spec.template.spec` immutability with a topology dry-run carve-out
  (`topology.IsDryRunRequest`), plus sample manifests
  (`config/samples/capi-quickstart.yaml`, `config/samples/capi-service-endpoint.yaml`,
  `config/samples/clusterclass.yaml`).
- **Multi-node workload networking** (`backend.swiftGuest.nodeNetworkRef`): attaches a
  secondary routable interface (an OVN-Kubernetes secondary UDN, or a bridge / VXLAN-mesh
  NAD) that carries the node datapath — apiserver↔kubelet + the pod overlay — with unique
  cross-node-routable IPs, while the primary interface stays node-local NAT so the
  Service-backed control-plane endpoint remains reachable from the management cluster. The
  node's kubelet `--node-ip` and the apiserver `--advertise-address` are pointed at that
  interface (a bootstrap step; see `docs/design/multi-node-networking.md`). Validated
  end-to-end as 2-node clusters on OVN-Kubernetes and on Calico VXLAN.
- **Root-disk StorageClass override** (`backend.swiftGuest.storageClassName`): pins the
  SwiftGuest root-disk StorageClass (otherwise inherited from the source image), e.g. to
  fit a lower-replica class on constrained storage.
- **clusterctl packaging**: `metadata.yaml` (releaseSeries 0.1 → contract v1beta2), a
  `templates/cluster-template.yaml` for `clusterctl generate cluster` (mode `Service`,
  non-overlapping CIDRs), `clusterctl-settings.json`, and a tag-triggered release
  workflow that builds the manager image and publishes the `infrastructure-components.yaml`
  + `metadata.yaml` + template as release assets. `config/default` now wires cert-manager
  to issue the webhook serving cert and inject its CA into the webhook configuration, so
  `make deploy` / the components manifest stand up the webhook without manual certs.
- Apache-2.0 license. Design docs (`docs/design/`) are tracked in-repo; spike docs
  (`docs/spikes/`) and AI tooling (`.claude/`, `CLAUDE.md`, `.devcontainer/`) are
  gitignored and kept local (matching the KubeSwift core repo).
- CI: golangci-lint v2 (v1.63.4 cannot lint on `go 1.25`); the scaffolded Kind e2e
  workflow is manual-only until a real Cluster API + KubeSwift e2e harness exists.

### Changed
- Grounded against Cluster API **v1.13.4**: the CRDs target the **v1beta2**
  infrastructure-provider contract. `status.ready` / `failureReason` /
  `failureMessage` were replaced by `status.initialization.provisioned` +
  conditions; `providerID` is now `string`. Types import
  `sigs.k8s.io/cluster-api/api/core/v1beta2` (`clusterv1.APIEndpoint` /
  `MachineAddress` / `ObjectMeta`). Added the CRD label
  `cluster.x-k8s.io/v1beta2: v1alpha1` and set the clusterctl `metadata.yaml`
  contract to `v1beta2`. Toolchain moved to controller-runtime 0.23.3 + k8s libs
  v0.35 (Kubernetes 1.35).

### Notes
- A single-node control plane can hit a CNI service-hairpin (a pod on the CP node
  reaching the apiserver through the workload Service ClusterIP) that leaves CoreDNS
  pending; prefer a multi-node control plane, or set `KUBERNETES_SERVICE_HOST` to the
  node IP on the affected pods. Written up in
  `docs/operations/single-control-plane-hairpin.md`.
- Go file license headers are not yet applied (follow-up).
