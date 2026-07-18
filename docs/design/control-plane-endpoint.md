# Control-plane endpoint provisioning

How a KubeSwift-backed workload cluster gets a stable, reachable control-plane
endpoint. Cluster API needs `Cluster.spec.controlPlaneEndpoint` set **before** the
first control-plane node boots (kubeadm bakes it into certs and the kubeconfig), and
that endpoint must be reachable from the management cluster's CAPI controllers, from
the worker VMs, and — for `kubectl` from outside — from the operator.

KubeSwift runs the workload nodes as VMs on the management cluster. With the default
`nat` binding a VM lives behind its launcher pod's IP; a plain guest IP
(`192.168.99.x`) is pod-local and not routable cluster-wide. So the endpoint cannot
just be "the control-plane VM's IP" unless the CNI makes guest IPs routable (OVN).
This provider offers three modes so operators are not forced onto OVN.

## Modes

| `spec.endpoint.mode` | How the endpoint is established | Reachable from | Needs OVN |
|---|---|---|---|
| `External` (default) | Operator sets `spec.controlPlaneEndpoint` (external LB, kube-vip in-guest, DNS). Provider provisions nothing. | Wherever the operator's VIP is | No (manual) |
| `Service` | Provider mints one Kubernetes Service fronting the control-plane guests on the API-server port and adopts its address as the endpoint. | ClusterIP: mgmt cluster (CAPI controllers + worker VMs). LoadBalancer: also external. | **No** |
| _routable / OVN_ | Not a separate mode — use `Service` with `type: LoadBalancer`; the VIP (MetalLB / an OVN LB) is externally routable. See "Routable endpoint on OVN" below. | External (routable VIP) | needs an LB (MetalLB / OVN) |

## Mode: Service — the CNI-agnostic path

This is the "clusters without OVN" answer. It reuses KubeSwift's existing service
exposure (`SwiftGuest.spec.network`, per-port DNAT in the launcher pod) rather than
adding any runtime machinery.

**Mechanism**

1. For each control-plane machine (`cluster.x-k8s.io/control-plane` label) whose cluster
   is in `Service` mode, the SwiftGuest backend renders the guest with:
   - `spec.network.binding: nat` + `ports: [{name: apiserver, port: 6443, targetPort: 6443}]`
     — **no** `expose`, so KubeSwift installs the in-pod DNAT (`podIP:6443 → vm:6443`)
     and an apiserver readiness probe, but mints **no** per-guest Service.
   - the label `swift.kubeswift.io/pool: <cluster>-cp`. This is the one guest-settable
     label KubeSwift propagates from a SwiftGuest to its launcher pod, so the provider
     can select the control-plane pods with its own Service.
2. The `KubeSwiftCluster` controller mints one Service (`<name>-cp`) in the guests'
   namespace: selector `swift.kubeswift.io/pool=<cluster>-cp`, port `6443` → targetPort
   `6443` (the pod-side port the DNAT listens on), type `ClusterIP` or `LoadBalancer`.
3. It reads the Service address and writes `spec.controlPlaneEndpoint`. A `ClusterIP` is
   assigned at Service creation, so the endpoint is known **before** any control-plane
   guest boots — this resolves the kubeadm chicken-and-egg. A `LoadBalancer` address may
   lag; the controller owns the Service (`Owns`) and re-reconciles when it lands.
4. As control-plane guests boot, their launcher pods become endpoints of the Service
   (gated on the apiserver readiness probe), so traffic starts flowing to a live API
   server.

**Datapath** (per KubeSwift's service exposure): `Service(6443) → kube-proxy/CNI →
launcher pod containerPort 6443 → PREROUTING DNAT podIP:6443 → vm:6443 → kube-apiserver`.
Plain Service + pod IPs; no OVN dependency (validated on Calico).

**Reachability**

- *Mgmt-cluster CAPI controllers → workload apiserver*: a mgmt-cluster ClusterIP is
  reachable in-cluster. (This is exactly what a NAT'd guest IP was **not** — the reason
  node-join stalled on Calico before this mode.)
- *Worker VMs → control-plane endpoint*: a `nat`-bound VM reaches cluster ClusterIPs the
  way any pod-network client does (MASQUERADE out the pod IP + kube-proxy). No field to
  set. (Breaks only on eBPF/kube-proxy-free datapaths — out of scope.)
- *Operator `kubectl` from outside*: use `type: LoadBalancer` (MetalLB / cloud / an
  OVN LB). `ClusterIP` is enough for the cluster to form and be managed by CAPI, but is
  not externally reachable.

**Constraints**

- Requires `nat` binding. A control-plane machine in `Service` mode must not set a
  bridge/NAD `networkRef` — the backend rejects that combination loudly.
- The endpoint Service lives in the guests' namespace and is owner-referenced to the
  `KubeSwiftCluster` (garbage-collected with it; also deleted explicitly on teardown).
- The API-server serving cert covers the endpoint because CAPI's KubeadmControlPlane
  sets `ClusterConfiguration.controlPlaneEndpoint` from `Cluster.spec.controlPlaneEndpoint`,
  which kubeadm adds to the cert SANs.

## Mode: External

The universal escape hatch, unchanged from v0: the operator provisions a VIP (external
load balancer, `kube-vip` static pod delivered via the bootstrap config, DNS) and sets
`spec.controlPlaneEndpoint`. The controller reports the cluster provisioned once it is
set. CNI-agnostic, but manual.

## Routable endpoint on OVN

There is **no separate `RoutableIP` mode**, and no "endpoint = the control-plane
guest's own routable IP" mode, because KubeSwift cannot pre-reserve a routable IP
before the guest boots: a `SwiftGuest` interface has no IP field, and the OVN-K
`IPAMClaim` KubeSwift creates carries no requested IP (it persists whatever OVN
assigns — good for migration, but not operator-chosen). The routable IP is only known
*after* the pod attaches, which is too late for kubeadm (it needs the endpoint up
front). A guest-own-IP mode would require a KubeSwift core change (an IP-request field
+ a static IP on the IPAMClaim + OVN-K wiring).

Instead, the routable/external endpoint on OVN — or on any cluster — is `mode: Service`
with `type: LoadBalancer`: the VIP is assigned at Service creation (no chicken-and-egg),
externally routable, and the provider adopts it exactly like a ClusterIP. KubeSwift
mints a plain selector Service (no built-in OVN LB), so the VIP comes from an external
load-balancer controller — **MetalLB** on bare metal (or Cilium/Tailscale).

Validated on the ntx cluster (OVN-Kubernetes primary CNI + MetalLB): a `mode: Service`
/ `type: LoadBalancer` `KubeSwiftCluster` got the routable VIP `172.16.56.27` from
MetalLB, the provider adopted it as `spec.controlPlaneEndpoint`, core CAPI surfaced it
onto the `Cluster` (`Provisioned`), and the control-plane guest was rendered with the
pool label + `nat` 6443 exposure and selected by the Service.

## API

```yaml
spec:
  # Resolved endpoint. Operator-set for mode=External; provider-set for mode=Service.
  controlPlaneEndpoint: { host: <addr>, port: 6443 }
  endpoint:
    mode: Service          # External (default) | Service
    service:
      type: ClusterIP      # ClusterIP (default) | LoadBalancer
      port: 6443
      annotations: {}      # e.g. a MetalLB address pool (LoadBalancer)
      loadBalancerClass: <class>   # optional, LoadBalancer only
```

## Implementation

- API: `api/v1alpha1/kubeswiftcluster_types.go` (`ControlPlaneEndpointSpec`,
  `ControlPlaneServiceSpec`, `ControlPlanePoolLabelKey`, `ControlPlaneServiceSelectorValue`).
- Cluster controller: `internal/controller/kubeswiftcluster_controller.go` (mode
  dispatch) + `internal/controller/endpoint_service.go` (Service mint/GC + address).
- Machine controller: `internal/controller/kubeswiftmachine_controller.go`
  (`controlPlaneExposure`).
- Backend: `internal/backend/swiftguest.go` (`renderSwiftGuest` stamps the label + port).

## Status

`External` and `Service` (ClusterIP + LoadBalancer) are implemented and unit-tested.
The mechanism is cluster-validated: `Service`/ClusterIP on dev (Calico, no OVN) and
`Service`/LoadBalancer on ntx (OVN-Kubernetes + MetalLB, routable VIP). End-to-end
node-join (a joined, Ready Node behind the endpoint) additionally needs a
Kubernetes-preinstalled guest image.
