# cluster-api-provider-kubeswift

A [Cluster API](https://cluster-api.sigs.k8s.io/) infrastructure provider that
runs workload-cluster machines as [KubeSwift](https://github.com/kubeswift-io/kubeswift)
virtual machines. Core Cluster API and this provider run on a management cluster
that has KubeSwift installed; each `Machine` is reconciled into a `SwiftGuest` VM,
the same way the Docker provider (CAPD) backs machines with containers.

> Status: **pre-alpha scaffold.** The CRDs and their shape exist; the controllers
> are stubs. The `v1alpha1` API will change. Not usable yet.

## Custom Resources

| CRD | Cluster API role | Backs |
|-----|------------------|-------|
| `KubeSwiftCluster` | InfraCluster | cluster-wide infrastructure + control-plane endpoint |
| `KubeSwiftClusterTemplate` | InfraClusterTemplate | ClusterClass |
| `KubeSwiftMachine` | InfraMachine | one `SwiftGuest` VM |
| `KubeSwiftMachineTemplate` | InfraMachineTemplate | MachineDeployment / ClusterClass |

API group: `infrastructure.cluster.x-k8s.io`, version `v1alpha1`.

## How it fits together

```
management cluster (core Cluster API + bootstrap + control-plane + THIS provider + KubeSwift)
  Cluster ── KubeSwiftCluster            control-plane endpoint, readiness
  Machine ── KubeSwiftMachine ── SwiftGuest VM   (a workload-cluster node)
```

The provider is Apache-2.0 and talks to KubeSwift only through the Kubernetes API
(the `swift.kubeswift.io` CRDs), never by importing KubeSwift's Go packages. That
keeps this codebase license-clean against KubeSwift's AGPL-3.0 core. See
[`docs/design/capi-kubeswift-architecture.md`](docs/design/capi-kubeswift-architecture.md).

## Build

```sh
make generate manifests   # deepcopy + CRDs from the Go types
make build                # compile the manager
make test                 # unit tests (envtest)
```

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
