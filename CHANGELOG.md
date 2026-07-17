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
- Apache-2.0 license. Design and spike docs (`docs/design/`, `docs/spikes/`) are
  tracked in-repo; AI tooling (`.claude/`, `CLAUDE.md`, `.devcontainer/`) is
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
- Controllers are stubs. The provider does not yet reconcile machines into VMs.
- Go file license headers are not yet applied (follow-up).
