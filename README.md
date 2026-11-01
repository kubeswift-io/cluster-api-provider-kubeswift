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

## Compatibility

| Provider | Cluster API contract | Built against Cluster API | KubeSwift |
|----------|----------------------|---------------------------|-----------|
| v0.4.x | v1beta2 | v1.14 | v0.13.12 or later; v0.15.0+ recommended |
| v0.3.x | v1beta2 | v1.13 | v0.13.12 or later; v0.15.0+ recommended |
| v0.2.x | v1beta2 | v1.13 | v0.13.x–v0.16.x |

The KubeSwift column for v0.3.x and v0.4.x is checked against the KubeSwift API up to
v0.22.0 (CRDs, admission and changelog; the SwiftGuest and SwiftSeedProfile APIs are
unchanged from v0.16.2 to v0.22.0), and the e2e suite runs against v0.22.0's CRDs; not
yet against a live KubeSwift controller.

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

Keep the `providers` entry in `clusterctl.yaml` after installing: kubeswift-io is not in
clusterctl's built-in provider list, so `clusterctl upgrade` also needs it to find new
releases (without it, both `plan` and `apply` fail with *failed to get configuration for
the InfrastructureProvider with name kubeswift-io*).

## Upgrade (clusterctl)

See what is installed and what each provider can move to:

```sh
clusterctl upgrade plan
```

Upgrade every provider to the latest release for the contract (what `plan` suggests):

```sh
clusterctl upgrade apply --contract v1beta2
```

Or upgrade only this provider. `upgrade apply` takes `namespace/provider:version`, not
the bare name `init` takes:

```sh
clusterctl upgrade apply --infrastructure capks-system/kubeswift-io:v0.4.1
```

Read the release's [CHANGELOG](CHANGELOG.md) entry first: it lists the minimum KubeSwift
version and anything that changes on upgrade.

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

Prerequisites on the management cluster: KubeSwift (v0.13.12 or later, v0.15.0+
recommended; install it before the provider, which watches its CRDs), cert-manager, a Ready `SwiftImage`
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

The e2e suite (`make test-e2e`) runs in its own Kind cluster, `capks-test-e2e`, through
its own kubeconfig (`bin/capks-test-e2e.kubeconfig`), so it never touches your current
kubectl context. It installs Cluster API with clusterctl and KubeSwift's CRDs — not
KubeSwift itself, so the suite plays KubeSwift's part by writing SwiftGuest status —
then deploys the provider and drives a Machine through it. Remove the cluster with
`make cleanup-test-e2e`. Needs Docker and Kind.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
