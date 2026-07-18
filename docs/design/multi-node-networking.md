# Multi-node workload clusters — networking design

> Status: **design + validated transport** (2026-07-18). Control-plane HA and worker
> pools need workload Node IPs that are unique *and* mutually routable. `mode: Service`
> (single-CP) does not provide that; this doc records why, the two network models that
> do, the endpoint models, and the validation plan across the two lab substrates.

## Problem

`spec.endpoint.mode: Service` puts each control-plane guest behind its launcher pod as
a `nat`-bound VM. That is correct for a **single** control-plane node, but it does not
scale to more nodes.

**Proven on dev (2026-07-18).** A 1-CP + 1-worker cluster: both Nodes register `Ready`
with *distinct* internal IPs (`192.168.99.10`, `192.168.99.16`) — so this is **not** an
IP-collision problem. But each `192.168.99.x` lives in an isolated per-launcher-pod
network namespace, so it is not routable between nodes. Result:

- `kubelet -> apiserver` works (one direction, via the endpoint) — hence `Ready`.
- `apiserver -> kubelet` fails: `Post https://192.168.99.16:10250/... : no route to host`.
- All node-to-node CNI traffic fails the same way.

So `exec` / `logs` / `port-forward` and every cross-node Pod flow are dead. A working
multi-node cluster needs Node IPs that are both unique and mutually reachable.

## Two reachability requirements (keep them separate)

1. **Control-plane endpoint** — workers (and the provider) reach the apiserver. A
   management-cluster Service already satisfies this: on a non-isolating primary CNI a
   `nat` guest's launcher pod *can* reach management ClusterIPs, which is why the worker
   above joined. This is the *easy* half.
2. **Node datapath** — `apiserver <-> every kubelet:10250` **and** node-to-node CNI
   overlay. This needs unique, mutually-routable Node IPs. This is the half `mode:
   Service` fails, and the half a LoadBalancer alone does **not** fix.

## Network models that satisfy #2

### Model A — pod-IP node addressing (no extra L2 substrate)

Advertise each Node's `InternalIP` as its **launcher pod IP** (already routable across
nodes by the management CNI), and reach the VM through the pod:

- The guest is `nat`-bound and cannot self-advertise the pod IP (it is on no guest
  interface). So bootstrap kubelet with `--cloud-provider=external` (kubelet then does
  not self-assign the wrong `192.168.99.x` and waits, carrying the
  `node.cloudprovider.kubernetes.io/uninitialized` taint).
- The provider — extending the Node-`providerID` patch it already performs — reports
  `status.addresses[InternalIP] = <launcher pod IP>` (read from `SwiftGuest.status.podRef`
  -> pod `status.podIP`) and clears the uninitialized taint. A small node-address
  reconciler, no separate component.
- Declare the node-datapath ports on every machine so KubeSwift installs the in-pod
  DNAT `podIP:port -> vmIP:port`: `10250/TCP` (kubelet) plus the workload CNI's overlay
  port (e.g. `8472/UDP` for a VXLAN overlay).

**Validated (transport).** A Pod on `boba` reached a CP guest's apiserver at the CP
**launcher pod IP** `10.244.125.191:6443` cross-node — `HTTP 200`, ~1 ms — over Calico's
**VXLAN** overlay (`ipipMode: Never`, `vxlanMode: Always`). VXLAN routing needs **no
ARP**, so it holds on L3-routed clouds (e.g. Hetzner) where ARP/L2 is unavailable.

**Open risk.** The single-TCP-service case is proven; the *workload CNI overlay* over
the DNAT'd port (bidirectional UDP-encap between guests that egress-MASQUERADE to their
pod IPs) is **not yet proven** and is the gating unknown for this model. If it does not
hold, fall back to Model B.

Pros: provider-only, no substrate, no ARP. Cons: the CNI-over-DNAT risk;
`--cloud-provider=external` lifecycle; the workload CNI's overlay port must be declared.

### Model B — routable-L2 binding (proven datapath)

Put guests on a cross-node L2 with `binding: bridge` + a `networkRef` NAD, so each guest
holds a **real routable IP** directly — Node IP is native, the workload CNI runs
normally (no DNAT games). This is the already-validated KubeSwift multi-node-L2 / primary
UDN datapath.

- **dev (no OVN):** a VXLAN-mesh bridge NAD across the worker nodes (Multus is present;
  no bridge NAD yet). VXLAN over routed L3 — Hetzner-safe, no ARP.
- **ntx (OVN):** the OVN primary UDN (`udn-primary/udn-net`, `10.200.0.0/16`, `layer2`,
  `role: primary`) already gives guests unique routable IPs cross-node.

**Validated on ntx (2026-07-18):** a guest on a *primary* UDN needs **no** provider
datapath change — a plain SwiftGuest created **in the primary-UDN namespace** attaches
automatically (KubeSwift auto-detects the primary UDN; no `networkRef`/`binding`), and
holds a routable UDN IP reachable cross-node from that namespace. So the provider just
creates plain guests in that namespace under `mode: External`.

Pros: proven datapath; **zero** provider datapath logic on a primary UDN. Cons: needs
the L2 substrate stood up (trivial on ntx — the UDN exists; a one-time mesh setup on dev).

## Endpoint models

The endpoint must be reachable **from inside the workload L2** (the workers), which rules
out an in-tenant ARP/BGP VIP on an isolated network and, on Hetzner, **any** ARP-based
VIP (MetalLB L2, ARP-mode floating VIPs). Options:

- **Management-cluster LoadBalancer** (recommended, and the hard requirement operators
  should assume): a Service in the *management* cluster provides the CP IP that both the
  provider and the workers reach; on ntx MetalLB L2 supplies it (`172.16.56.27-29`,
  validated), on Hetzner-dev use a routed LB (hcloud CCM / BGP) or the management
  ClusterIP for in-fleet clusters. This mirrors a cloud-controller LoadBalancer proxy
  living in the management cluster.
- **CP's own routable IP** (Model B only): the CP guest's L2 IP — but it is unknown
  before boot, and Cluster API needs `controlPlaneEndpoint` set before kubeadm runs, so
  this needs a management-cluster VIP or a per-guest static IP the runtime cannot yet pin.
- **Not** an in-tenant `kube-vip`/MetalLB (ARP dropped by OVN port-security; and Hetzner
  has no L2). Ruled out.

**Validated on ntx (2026-07-18) — the primary-UDN endpoint is the hard part.** A real
Model A guest gets its node IP from the **UDN** (`10.200.0.20`), and the launcher pod is
dual-homed but its **default interface is `role: infrastructure-locked`** (KubeSwift-only;
the guest is not reachable there). Decisive reachability test to the guest's UDN IP `:22`:

| From | Result |
|---|---|
| a **UDN-namespace** pod (workers' path) | **REACHED** |
| a **default-network** pod (CAPI core + provider path) | **BLOCKED** |

So the CP guest's apiserver (on the UDN) is reachable by workers but **isolated from the
management controllers**. A plain management Service/LoadBalancer selecting the launcher
pod resolves to its *infrastructure-locked default IP* and does **not** reach the guest.
Consequences for the primary-UDN path:

- **The endpoint needs a bridge** — a small **dual-homed proxy** deployed in the guest's
  UDN namespace (a normal pod there holds both networks) that forwards `:6443` from its
  default IP to the CP guest's UDN IP, fronted by a management Service / MetalLB LB. That
  is a real per-cluster component (a management-cluster LoadBalancer proxy).

**Alternative that avoids both hard parts — nat default + *secondary* UDN.** Keep the
guest `nat`-bound on the default network (apiserver reachable at the pod IP via the
existing in-pod DNAT → management controllers reach it, no proxy) **and** attach a
*secondary* UDN via `networkRef` for a routable second interface. Set the node's
`--node-ip` to that secondary-UDN IP (it is a *local* interface IP, so kubelet accepts it —
no CCM), discovered at boot. This pairs the easy endpoint (nat DNAT) with the easy datapath
(routable UDN). Cost: a provider change to allow `networkRef` *together with* the nat
endpoint (today they are mutually exclusive), and a boot-time node-ip step.

## Plan

Two tracks, in parallel:

- **dev (no OVN):** Model A first (provider-only; smallest change; the transport is
  proven) — build the node-address reconciler + node-datapath port declaration +
  `--cloud-provider=external` template, and validate the CNI-over-DNAT overlay end to
  end. If the overlay does not hold, Model B (VXLAN-mesh bridge). Endpoint: management
  LoadBalancer / ClusterIP (no ARP).
- **ntx (OVN):** Model B on the primary UDN (routable IPs native). Endpoint: management
  MetalLB L2 LoadBalancer.

Both deliver control-plane HA (`>1` CP) and worker pools once #2 holds; the LoadBalancer
requirement covers #1 and HA.
