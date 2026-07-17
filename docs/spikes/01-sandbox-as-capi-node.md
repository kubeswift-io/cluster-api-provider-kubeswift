# Spike 01 — Can a SwiftSandbox / SwiftSandboxPool back a CAPI node?

> Question: the CAPI provider backs a `KubeSwiftMachine` with a KubeSwift VM. Can
> that VM be a **SwiftSandbox** or a **SwiftSandboxPool** checkout (not just a
> SwiftGuest), so operators can choose a sandbox-backed node? Investigated against the
> KubeSwift repo (read-only). File citations are into `../kubeswift`.

## Verdict

**NOT-FEASIBLE-AS-DESIGNED** for both, as a *persistent kubeadm node*. A SwiftSandbox
is a one-shot, ephemeral, exec-driven, NAT'd, ingress-denied microVM that powers off
when its single workload exits; a SwiftSandboxPool is a consume-and-replenish warm
buffer for that same one-shot model. Five independent hard blockers, any one
disqualifying:

| # | Blocker | Evidence (`../kubeswift`) |
|---|---------|---------------------------|
| 1 | **No cloud-init / seed — exec-only.** Workload is `command`/`args`/`env` on a config disk, or a single vsock exec. No NoCloud path to consume CAPI's kubeadm bootstrap Secret. | `api/sandbox/v1alpha1/swiftsandbox_types.go:46-62`; `internal/controller/swiftsandbox/pod.go:78-87`; `build/kernels/sandbox/rootfs-overlay/init:133-165`; `cmd/kubeswift-guest-agent/main.go:79-97` |
| 2 | **Ephemeral + one-shot.** RO OCI rootfs + tmpfs upper (writes lost on stop); bridge powers the VM **off** when the workload exits; pod `RestartPolicy: Never`. No reboot-and-rejoin. | `swiftsandbox_types.go:11-14`; `init:182-190,223-233`; `pod.go:454`; `docs/sandbox/overview.md:17-21` |
| 3 | **Non-routable + ingress-denied.** The IP is on a pod-internal bridge subnet, MASQUERADE'd behind the pod IP; every networked sandbox gets a deny-all-ingress NetworkPolicy (both `restricted` and `open`). apiserver↔kubelet (`:10250`) is impossible; `restricted` also blocks RFC1918 egress. | `pod.go:369-373`; `internal/controller/swiftsandbox/netpol.go:14-41`; `docs/sandbox/overview.md:110-123` |
| 4 | **Kernel can't host pods.** The sandbox kernel is monolithic `CONFIG_MODULES=n` with **no** `BRIDGE`/`VETH`/`TUN`/`NETFILTER`/`NF_CONNTRACK`/`NF_NAT`/`VXLAN`/`IP_VS`. CNI pod networking and kube-proxy are impossible, and nothing can be loaded at runtime. | `build/kernels/sandbox/configs/sandbox-linux.config`; `build/kernels/sandbox/README.md` |
| 5 | **No init/service supervision.** The bridge supervises a single foreground child; the guest-agent does one chroot+exec and returns. No systemd/containerd/kubelet co-supervision. | `init:216-233`; `cmd/kubeswift-guest-agent/main.go:79-97` |

**SwiftSandboxPool adds a sixth:** checkout is **consume-and-replenish** — the slot pod
is deleted the moment the injected workload reports complete/failed, and the pool boots
a fresh warm one. It is engineered for "inject a short workload, reclaim the slot," the
opposite of a long-lived node. `internal/controller/swiftsandbox/checkout.go:246-290`;
`docs/sandbox/warm-pool.md:20-24`.

## Why SwiftGuest is the right node backend

SwiftGuest has exactly the four things the sandbox lacks, and a node needs all four:
a **routable OVN pod-network IP** (L2-bridged tap0, not MASQUERADE), a **NoCloud
cloud-init seed** to consume CAPI's kubeadm blob (`SwiftSeedProfile`), a **persistent
disk**, and **reboot survival**. Backing a `KubeSwiftMachine` with a sandbox would mean
re-deriving all four on the sandbox substrate — i.e. rebuilding SwiftGuest.

## The genuinely valuable sandbox model (not a node)

Don't model a sandbox as a **Node**; model it as a **disposable compute unit** (a
pod-equivalent). The right integration surface is a **virtual-kubelet-style / managed-
runner** provider where a SwiftSandbox is a *pod*, not a node — no `kubeadm join`, no
CNI, no ingress: CI runners, agent/code-interpreter execution, per-job VMs (the
documented use case, `docs/sandbox/overview.md:23-31`), and **fast scale-from-zero GPU
inference** via warm GPU pools with a preloaded model (`swiftsandboxpool_types.go:71-80`,
`swiftsandbox_types.go:144-154`). That is a **separate component** from this CAPI
infrastructure provider, not a `KubeSwiftMachine` backend.

## What a PoC would need to prove (only if we ever revisit sandbox-as-node)

In dependency order — steps 1-2 alone require a bespoke node kernel and a rewrite of the
sandbox network posture, which is itself the proof that persistent nodes belong to
SwiftGuest:

1. A "node-sandbox" kernel profile adding bridge/veth/tun/netfilter/conntrack/nat/vxlan
   (or `CONFIG_MODULES=y` + modules initramfs); prove containerd + CNI + kube-proxy
   program the dataplane.
2. Replace MASQUERADE + deny-ingress with the SwiftGuest br0/tap0→OVN L2 path; prove
   `apiserver→:10250` works.
3. `kubeadm join` via config-disk exec, kubelet as a never-exiting foreground process
   (so the bridge does not power off the VM), join config on a `pvcRef` scratch disk.
4. `/var/lib/kubelet` + `/etc/kubernetes` on a `pvcRef` scratch disk; quantify the
   "no reboot" gap (VM stop = node gone, must re-join).
5. Only then, a pool-semantics change so a claimed slot is not torn down on workload exit.

## Recommendation

CAPI `KubeSwiftMachine` node backend = **SwiftGuest only**. Keep the machine API simple
(no speculative backend discriminator) until a second *feasible* node substrate exists.
Track sandbox-as-compute as a separate virtual-kubelet-style integration, out of this
provider.

## Decision

_(pending — see the P1 direction decision in the session)._
