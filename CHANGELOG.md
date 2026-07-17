# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Repository scaffold for the Cluster API infrastructure provider (kubebuilder).
- CRDs (`v1alpha1`, group `infrastructure.cluster.x-k8s.io`): `KubeSwiftCluster`,
  `KubeSwiftClusterTemplate`, `KubeSwiftMachine`, `KubeSwiftMachineTemplate`, with
  the Cluster API v1beta2 infrastructure-provider contract fields (control-plane
  endpoint, `providerID`, `status.initialization.provisioned`, addresses, conditions).
- Architecture design doc (`docs/design/capi-kubeswift-architecture.md`).
- Apache-2.0 license; AI tooling (`.claude/`, `CLAUDE.md`) and design docs are
  tracked in-repo (a departure from the KubeSwift core repo).

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
- Controllers are stubs. The provider does not yet reconcile machines into VMs.
- Go file license headers are not yet applied (follow-up).
