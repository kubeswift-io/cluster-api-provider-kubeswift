# Single control plane: pods on the control-plane node can't reach the apiserver

On a workload cluster with **one** control-plane node, a pod scheduled **on that
node** may fail to reach the apiserver through the `kubernetes.default` ClusterIP.
Pods on worker nodes are unaffected.

## Symptom

A CNI or system pod pinned to the control plane crashloops or never starts:

```
dial tcp <service-cidr>.1:443: i/o timeout
```

Typically the control-plane CNI pod fails first and CoreDNS then sits in
`ContainerCreating` or `Pending`, because CoreDNS needs the pod network the CNI
never finished setting up. `kubectl` from outside the cluster still works — the
management-cluster path to the endpoint is a different route and is fine.

## Why

Inside the workload cluster a pod reaches the apiserver via the `kubernetes`
Service ClusterIP, which kube-proxy DNATs to the apiserver's
`--advertise-address:6443`. With a single control plane that address is on the
*same node* the pod is running on, so the connection hairpins: it leaves the pod,
gets DNAT'd back to the node it came from, and — unless the CNI bridge does hairpin
or the traffic is masqueraded — the reply is never associated with the original
connection and the SYN times out.

Multi-node clusters don't hit this for pods on workers: those reach the
control-plane apiserver across the node network, which is a normal routed path
with no hairpin.

## Fixes

Pick one.

**1. Add a worker and schedule the affected pods there.** The simplest option and
the one the multi-node template assumes. With `--worker-machine-count >= 1`, move
CoreDNS off the control plane:

```sh
kubectl -n kube-system patch deployment coredns --type merge \
  -p '{"spec":{"template":{"spec":{"nodeSelector":{"!node-role.kubernetes.io/control-plane":""}}}}}'
```

or cordon the control plane once workers are Ready so the scheduler places system
workloads on workers.

**2. Bypass the ClusterIP on the affected pods.** Point them straight at the node
address instead of the Service:

```yaml
env:
  - name: KUBERNETES_SERVICE_HOST
    value: "<control-plane node IP>"
  - name: KUBERNETES_SERVICE_PORT
    value: "6443"
```

Useful for a CNI DaemonSet that must run on the control plane before any pod
network exists.

**3. Make the hairpin work.** Either enable masquerading in kube-proxy:

```yaml
# kube-proxy ConfigMap
masqueradeAll: true
```

or enable hairpin mode on the workload CNI's bridge (the exact knob is
CNI-specific — for a bridge-plugin-based CNI it is `hairpinMode: true` in the CNI
conflist). This is the only option that keeps a genuinely single-node cluster
working unmodified, at the cost of masquerading more traffic.

## Scope

This is a property of kube-proxy plus the workload CNI, not of the KubeSwift
provider or of `endpoint.mode: Service` — the same hairpin appears on any
single-control-plane cluster whose CNI does not hairpin or masquerade. The
provider does not currently configure the workload CNI, so it cannot fix this on
your behalf; tracked in
[#8](https://github.com/kubeswift-io/cluster-api-provider-kubeswift/issues/8).
