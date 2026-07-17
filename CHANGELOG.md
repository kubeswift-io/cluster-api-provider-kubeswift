# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Repository scaffold for the Cluster API infrastructure provider (kubebuilder).
- CRDs (`v1alpha1`, group `infrastructure.cluster.x-k8s.io`): `KubeSwiftCluster`,
  `KubeSwiftClusterTemplate`, `KubeSwiftMachine`, `KubeSwiftMachineTemplate`, with
  the Cluster API infrastructure-provider contract fields (control-plane endpoint,
  `providerID`, `ready`, addresses, conditions).
- Architecture design doc (`docs/design/capi-kubeswift-architecture.md`).
- Apache-2.0 license; AI tooling (`.claude/`, `CLAUDE.md`) and design docs are
  tracked in-repo (a departure from the KubeSwift core repo).

### Notes
- Controllers are stubs. The provider does not yet reconcile machines into VMs.
- Go file license headers are not yet applied (follow-up).
