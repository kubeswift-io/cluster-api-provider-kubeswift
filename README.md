# cluster-api-provider-kubeswift

A [Cluster API](https://cluster-api.sigs.k8s.io/) infrastructure provider (**CAPKS**)
that runs workload-cluster machines as [KubeSwift](https://github.com/kubeswift-io/kubeswift)
virtual machines. Core Cluster API and this provider run on a management cluster
that has KubeSwift installed; each `Machine` is reconciled into a `SwiftGuest` VM,
the same way the Docker provider (CAPD) backs machines with containers.

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
  - name: kubeswift-io
    url: https://github.com/kubeswift-io/cluster-api-provider-kubeswift/releases/latest/infrastructure-components.yaml
    type: InfrastructureProvider
```

On a management cluster that already runs KubeSwift and cert-manager:

```sh
clusterctl init --infrastructure kubeswift-io
```

Generate a workload cluster. `mode: Service` (the default in the shipped template)
makes the provider mint the control-plane endpoint as a Service, so no OVN or VIP is
required:

```sh
clusterctl generate cluster my-cluster \
  --infrastructure kubeswift-io \
  --kubernetes-version v1.34.0 \
  --control-plane-machine-count 1 \
  --worker-machine-count 2 | kubectl apply -f -
```

Prerequisites on the management cluster: KubeSwift, cert-manager, a Ready `SwiftImage`
(the template defaults to `ubuntu-noble`), and cluster-scoped `SwiftGuestClass`es
(`capi-controlplane`, `capi-worker`). Install a CNI on the workload cluster once its
control plane is up. Workload pod/service CIDRs must not overlap the management
cluster's or the guest NAT range — see [`templates/cluster-template.yaml`](templates/cluster-template.yaml).

### Multi-node workload clusters

When workload nodes must reach each other across management-cluster nodes, use the
`multi-node` flavor. It attaches a secondary routable interface to every node
(`nodeNetworkRef`) and points kubelet `--node-ip` and the apiserver
`--advertise-address` at that address — without which every node advertises the same
node-local NAT IP and worker pods resolving `kubernetes.default` reach their own NAT:

```sh
KUBESWIFT_CONTROL_PLANE_NETWORK=sec-net KUBESWIFT_WORKER_NETWORK=sec-net \
clusterctl generate cluster my-cluster \
  --infrastructure kubeswift-io --flavor multi-node \
  --kubernetes-version v1.34.0 \
  --control-plane-machine-count 1 \
  --worker-machine-count 2 | kubectl apply -f -
```

Point both variables at the same `NetworkAttachmentDefinition` when one L2 segment
spans every node (an OVN-Kubernetes secondary UDN, say), or at different NADs when the
segment is assembled from per-node bridge NADs. Validated on OVN-Kubernetes and on
Calico with a VXLAN-mesh bridge NAD.

On a single-control-plane cluster, pods scheduled on the control plane can hit a
service hairpin reaching the apiserver — see
[single-control-plane-hairpin.md](docs/operations/single-control-plane-hairpin.md).

## Build

```sh
make generate manifests   # deepcopy + CRDs from the Go types
make build                # compile the manager
make test                 # unit tests (envtest)
make build-installer      # render dist/install.yaml (the release components manifest)
```

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
