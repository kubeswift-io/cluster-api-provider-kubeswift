# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Dependencies
- **Built against Cluster API v1.14.3** (from v1.13.6), with the versions that
  release pins: controller-runtime **v0.24.1** (from v0.23.3) and the Kubernetes
  libraries **v0.36.3** (from v0.35.9); envtest now runs Kubernetes 1.36. The
  contract is unchanged (v1beta2). Cluster API v1.14 moved its API types into their
  own module, `sigs.k8s.io/cluster-api/api`, now required at `v1.14.3`; the import
  paths are the same.
- The API package registers its types with apimachinery's scheme builder instead
  of controller-runtime's `scheme.Builder`, deprecated in v0.24. `AddToScheme` is
  unchanged; the exported `SchemeBuilder` is gone.

 — 2026-10-11

Requires **KubeSwift v0.13.12 or later** (v0.15.0+ recommended), and KubeSwift's
CRDs installed before the provider starts.

### Changed
- **The bootstrap cloud-init is no longer copied into the SwiftSeedProfile.** A
  machine's SwiftSeedProfile now references the Cluster API bootstrap data secret
  through `spec.userDataFrom.secretKeyRef` instead of carrying the cloud-init —
  kubeadm join token and all — inline in `spec.userData`, where anyone able to read
  SwiftSeedProfiles could see it and it was stored unencrypted. Profiles created by
  an earlier release are converted in place on the next reconcile. Running guests
  are not restarted: KubeSwift renders the seed only when it creates a launcher pod,
  and the secret holds the same bytes. `userDataFrom` works from KubeSwift v0.13.12;
  an older KubeSwift's CRD rejects the profile with `spec.userData: Required value`.
- **A nat guest's machine address is its launcher pod IP.** Every nat launcher
  hands its guest an address from the same pod-private range (192.168.99.10–20), so
  KubeSwiftMachines reported duplicate `InternalIP`s that nothing outside the
  launcher could reach. When KubeSwift marks `status.network.primaryIPScope: Pod`
  (v0.15.0+), the machine now reports `status.network.podIP` — unique, and where the
  guest's declared ports are DNAT'd. A `Network`-scope address (a bridge/NAD
  `networkRef`) is reported as before, and so is every address on a KubeSwift
  without the field. In the multi-node flavor this is the `mgmt` interface's pod IP;
  the Node's own `InternalIP` still comes from kubelet `--node-ip`.
- **The machine controller watches SwiftGuests.** A guest's status changes on its
  own — it gets an address, waits on a reference, moves to a new launcher pod on a
  restart or migration — and the provider used to notice only on its next poll or
  never, once provisioned. Addresses and readiness now follow the guest. This makes
  the SwiftGuest CRD a startup dependency of the provider.

### Added
- **A machine says why its VM is waiting for a reference.** Since KubeSwift v0.14.x
  a guest whose image, class, seed profile, kernel or data disk is missing or not
  ready stays `Pending` instead of going `Failed` — so a typo in `imageRef` used to
  read as an endless "Waiting for the backing VM". The KubeSwiftMachine's `Ready`
  condition now carries reason `VMWaitingForReferences` with the guest's `Resolved`
  message, e.g. `SwiftImage not found: ubuntu-nobel`.
- **Compatibility table** in the README (provider ↔ Cluster API contract ↔
  KubeSwift).

### Dependencies
- Go toolchain pinned to **1.26.9** (`toolchain go1.26.9`); the image builder is
  pinned to `golang:1.26.9` instead of floating on `golang:1.27` (#54).
- Kubernetes libraries **v0.35.9** (#50); gomega v1.44.0, OpenTelemetry v1.47.0,
  `golang.org/x/{sys,sync,tools}` and other minor/patch updates (#53).

### Upgrade
- **From v0.2.x:** upgrade in place (same `capks-system` namespace, same v1beta2
  contract). Upgrade KubeSwift to v0.13.12+ first, or new machines' seed profiles
  are rejected. Then either upgrade every provider for the contract:

  ```bash
  clusterctl upgrade plan
  clusterctl upgrade apply --contract v1beta2
  ```

  or only this one — `upgrade apply` takes `namespace/provider:version`, not the
  bare `kubeswift-io` that `init` takes:

  ```bash
  clusterctl upgrade apply --infrastructure capks-system/kubeswift-io:v0.3.0
  ```

  Both need the `kubeswift-io` entry in `~/.cluster-api/clusterctl.yaml` (see the
  README), as `init` did.
- On the first reconcile after the upgrade, existing machines' SwiftSeedProfiles
  drop their inline `userData`, and nat machines' reported address changes from
  `192.168.99.x` to their launcher pod IP. Neither restarts a VM.

## [v0.2.1] — 2026-08-15

### Changed
- **The provider now installs into `capks-system`**, not
  `cluster-api-provider-kubeswift-system` (#23). Every upstream Cluster API
  provider names itself by its acronym — core is `capi-system`, AWS `capa-`,
  Azure `capz-` — and the full repository name gave a 38-character namespace plus
  object names like `cluster-api-provider-kubeswift-kubeswiftmachinetemplate-editor-role`.
  Resources are now `capks-controller-manager`, `capks-webhook-service`,
  `capks-metrics-service`. The repository name still appears in the
  `app.kubernetes.io/name` label.

  The clusterctl provider label (`cluster.x-k8s.io/provider:
  infrastructure-kubeswift-io`) is **unchanged**, so clusterctl still finds,
  upgrades and deletes provider objects the same way.

  **Breaking for an existing install.** clusterctl records the namespace a
  provider was installed into and will not move it across namespaces, so an
  in-place `clusterctl upgrade` from v0.2.0 will not relocate the provider:

  ```bash
  clusterctl delete --infrastructure kubeswift-io
  clusterctl init   --infrastructure kubeswift-io
  ```

  The old namespace survives the delete and can be removed once empty.

## [v0.2.0] — 2026-08-08

### Added
- **Whole-GPU passthrough for a machine** — `KubeSwiftMachine.spec.backend.swiftGuest.gpu`
  takes either a DRA claim (`resourceClaimTemplateName` / `resourceClaimName`, with
  `requestName`, `tier`, `hugepages`) or a native `gpuProfileRef`, making a machine a
  GPU-capable Kubernetes worker. KubeSwift owns the physical device — it binds it to
  vfio-pci and passes it into the VM; what shares or consumes it *inside* the guest is
  the workload cluster's business and nothing to do with this provider. Only `tier: pcie`
  is supported: the HGX tiers need QEMU plus a host Fabric Manager, and KubeSwift rejects
  `hgx-full` at allocation.
- **Static machine placement** — `KubeSwiftMachine.spec.backend.swiftGuest.nodeName`
  pins a machine's VM to a named node in the infrastructure cluster, for placements that
  are not the scheduler's to choose: the host with the hardware, a licence bound to a
  machine, storage only one node serves, or a node-local network the VM must share.
  KubeSwift writes `pod.spec.nodeName` on the launcher pod, so a bad fit is rejected by
  the kubelet in seconds rather than sitting Pending. It cannot be combined with `gpu`:
  pinning bypasses the scheduler, a DRA claim is allocated *by* the scheduler, and a GPU
  machine is already placed by where its device is.

Both fields are optional and inert when unset, so an existing machine renders exactly
the SwiftGuest it rendered before.

### Fixed
- **The e2e suite never went green**, for two independent reasons, and so was no signal
  at all. `InstallCertManager` waited for the webhook *Deployment* to be Available, which
  does not mean its CA has been injected — the next apply failed with
  `x509: certificate signed by unknown authority`, taking down `[BeforeAll]` before a
  single spec ran. It now waits until the webhook actually answers a server-side dry-run.
  And the metrics spec looked for a Service name kustomize does not generate. The
  metrics ClusterRoleBinding is also created idempotently, so one failure no longer
  poisons every later run with `already exists`. `make test-e2e` now passes 4 of 4.

## [v0.1.2] — 2026-07-20

### Changed
- **clusterctl provider name is now `kubeswift-io`** (was `kubeswift`). The
  `github.com/kubeswift` GitHub org is owned by another entity, and the clusterctl
  provider-naming rule requires a provider outside `kubernetes-sigs` to prefix its
  name with the GitHub org it is developed under — here `kubeswift-io`. Install with
  `clusterctl init --infrastructure kubeswift-io` and generate with
  `--infrastructure kubeswift-io`; the components now carry
  `cluster.x-k8s.io/provider: infrastructure-kubeswift-io`. The API group, CRDs,
  container image, and namespace are unchanged. Anyone who installed `v0.1.1` under
  the old name should `clusterctl delete kubeswift` and reinstall under `kubeswift-io`.

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
