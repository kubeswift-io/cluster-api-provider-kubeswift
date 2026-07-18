# cluster-api-provider-kubeswift

A [Cluster API](https://cluster-api.sigs.k8s.io/) infrastructure provider that
runs workload-cluster machines as [KubeSwift](https://github.com/kubeswift-io/kubeswift)
virtual machines. Core Cluster API and this provider run on a management cluster
that has KubeSwift installed; each `Machine` is reconciled into a `SwiftGuest` VM,
the same way the Docker provider (CAPD) backs machines with containers.

> Status: **alpha.** Works end-to-end: a `Cluster` (plain or ClusterClass topology)
> reconciles into KubeSwift VMs, the nodes join, and the provider provisions the
> control-plane endpoint — including mode `Service`, which needs no OVN or operator
> VIP. The `v1alpha1` API may still change.

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

## Install (clusterctl)

Add the provider to `~/.cluster-api/clusterctl.yaml`:

```yaml
providers:
  - name: kubeswift
    url: https://github.com/kubeswift-io/cluster-api-provider-kubeswift/releases/latest/infrastructure-components.yaml
    type: InfrastructureProvider
```

On a management cluster that already runs KubeSwift and cert-manager:

```sh
clusterctl init --infrastructure kubeswift
```

Generate a workload cluster. `mode: Service` (the default in the shipped template)
makes the provider mint the control-plane endpoint as a Service, so no OVN or VIP is
required:

```sh
clusterctl generate cluster my-cluster \
  --infrastructure kubeswift \
  --kubernetes-version v1.34.0 \
  --control-plane-machine-count 1 \
  --worker-machine-count 2 | kubectl apply -f -
```

Prerequisites on the management cluster: KubeSwift, cert-manager, a Ready `SwiftImage`
(the template defaults to `ubuntu-noble`), and cluster-scoped `SwiftGuestClass`es
(`capi-controlplane`, `capi-worker`). Install a CNI on the workload cluster once its
control plane is up. Workload pod/service CIDRs must not overlap the management
cluster's or the guest NAT range — see [`templates/cluster-template.yaml`](templates/cluster-template.yaml).

## Build

```sh
make generate manifests   # deepcopy + CRDs from the Go types
make build                # compile the manager
make test                 # unit tests (envtest)
make build-installer      # render dist/install.yaml (the release components manifest)
```

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
