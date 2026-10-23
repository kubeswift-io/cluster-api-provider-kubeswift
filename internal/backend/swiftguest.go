package backend

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

// KubeSwift API kinds the SwiftGuest backend talks to via the unstructured client,
// so this Apache-2.0 repo never imports KubeSwift's AGPL Go packages.
var (
	swiftGuestGVK = schema.GroupVersionKind{
		Group: "swift.kubeswift.io", Version: "v1alpha1", Kind: "SwiftGuest",
	}
	swiftSeedProfileGVK = schema.GroupVersionKind{
		Group: "seed.kubeswift.io", Version: "v1alpha1", Kind: "SwiftSeedProfile",
	}
)

// MachineLabelKey labels each KubeSwift resource the SwiftGuest backend creates with
// the name of the KubeSwiftMachine it backs (in the same namespace).
const MachineLabelKey = "infrastructure.cluster.x-k8s.io/kubeswiftmachine"

// NewSwiftGuestObject returns an empty SwiftGuest, e.g. for a controller to watch.
func NewSwiftGuestObject() *unstructured.Unstructured { return newUnstructured(swiftGuestGVK) }

// SwiftGuestBackend backs a KubeSwiftMachine with a SwiftGuest VM.
type SwiftGuestBackend struct {
	Client client.Client
}

// NewSwiftGuestBackend returns a SwiftGuest backend using the given client.
func NewSwiftGuestBackend(c client.Client) *SwiftGuestBackend {
	return &SwiftGuestBackend{Client: c}
}

// Type implements Backend.
func (b *SwiftGuestBackend) Type() infrav1.MachineBackendType {
	return infrav1.SwiftGuestBackendType
}

func seedName(machineName string) string { return machineName + "-seed" }

// validatePlacement rejects the one combination that fails silently: a pinned node
// plus a GPU. nodeName bypasses the scheduler, and a DRA claim is allocated by the
// scheduler — so the pair yields a VM whose claim is never allocated and which
// therefore never gets its device. (A native gpuProfileRef is placed by KubeSwift's
// allocator, which nodeName would have to match by luck; KubeSwift's own webhook
// then rejects the mismatch.) Either way the answer is the same: a GPU machine is
// already placed by where its device is.
func validatePlacement(cfg *infrav1.SwiftGuestBackend) error {
	if cfg.NodeName != "" && cfg.GPU != nil {
		return fmt.Errorf("spec.backend.swiftGuest: nodeName cannot be combined with gpu — " +
			"a GPU machine is placed by where its device is, and pinning the pod bypasses " +
			"the scheduler that allocates the claim")
	}
	return nil
}

// validateGPU enforces one allocation backend. KubeSwift rejects a guest that
// names both, and a VFIO device backs exactly one running VM — so a shared claim
// across machines is a configuration that cannot work, not a preference.
func validateGPU(gpu *infrav1.SwiftGuestGPU) error {
	if gpu == nil {
		return nil
	}
	set := 0
	for _, v := range []string{gpu.ResourceClaimTemplateName, gpu.ResourceClaimName, gpu.GPUProfileRef} {
		if v != "" {
			set++
		}
	}
	if set != 1 {
		return fmt.Errorf("spec.backend.swiftGuest.gpu: set exactly one of " +
			"resourceClaimTemplateName, resourceClaimName or gpuProfileRef")
	}
	if gpu.Tier != "" && gpu.Tier != infrav1.GPUTierPCIe {
		return fmt.Errorf("spec.backend.swiftGuest.gpu.tier %q: only %s is supported "+
			"(hgx tiers need QEMU and a host Fabric Manager)", gpu.Tier, infrav1.GPUTierPCIe)
	}
	return nil
}

// Reconcile ensures the SwiftSeedProfile + SwiftGuest exist and reports the VM state.
func (b *SwiftGuestBackend) Reconcile(ctx context.Context, req Request) (Result, error) {
	cfg := req.Machine.Spec.Backend.SwiftGuest
	if cfg == nil {
		return Result{}, fmt.Errorf("spec.backend.swiftGuest must be set when backend.type is %q", infrav1.SwiftGuestBackendType)
	}
	if cfg.ImageRef == "" || cfg.GuestClassRef == "" {
		return Result{}, fmt.Errorf("spec.backend.swiftGuest.imageRef and guestClassRef are required")
	}
	if err := validateGPU(cfg.GPU); err != nil {
		return Result{}, err
	}
	if err := validatePlacement(cfg); err != nil {
		return Result{}, err
	}
	if req.ControlPlaneExposure != nil && cfg.NetworkRef != "" {
		// Service-backed endpoint provisioning exposes the API server over the pod
		// network (nat binding); a bridge/NAD networkRef is incompatible with it.
		return Result{}, fmt.Errorf("control-plane machine using Service-backed endpoint provisioning must not set spec.backend.swiftGuest.networkRef")
	}

	if err := b.ensureSeedProfile(ctx, req); err != nil {
		return Result{}, fmt.Errorf("ensuring SwiftSeedProfile: %w", err)
	}
	guest, err := b.ensureSwiftGuest(ctx, req, cfg)
	if err != nil {
		return Result{}, fmt.Errorf("ensuring SwiftGuest: %w", err)
	}

	phase, _, _ := unstructured.NestedString(guest.Object, "status", "phase")
	if phase == "Failed" {
		return Result{}, fmt.Errorf("SwiftGuest %s/%s reported phase Failed", req.GuestNamespace, req.Machine.Name)
	}
	addresses := guestAddresses(guest)
	if phase != "Running" || len(addresses) == 0 {
		// VM not running yet, or its address not yet discovered — requeue.
		res := Result{Requeue: true}
		if msg, waiting := unresolvedMessage(guest); waiting {
			res.WaitingReason = infrav1.VMWaitingForReferencesReason
			res.WaitingMessage = fmt.Sprintf("SwiftGuest %s/%s is waiting for its references: %s",
				req.GuestNamespace, req.Machine.Name, msg)
		}
		return res, nil
	}
	return Result{Provisioned: true, Addresses: addresses}, nil
}

// guestAddresses returns the address a running SwiftGuest is reachable at from
// outside its launcher pod, or nil until it has one. A nat guest's primaryIP is
// private to its launcher (every nat launcher hands out the same 192.168.99.x
// range), so it is replaced by the launcher pod's IP, where the guest's declared
// ports are DNAT'd and which is unique in the cluster. primaryIP is still what
// says the VM is up: it is set once the guest has leased its address. KubeSwift
// before v0.15.0 reports no primaryIPScope, and its primaryIP is used as-is.
func guestAddresses(guest *unstructured.Unstructured) []clusterv1.MachineAddress {
	primaryIP, _, _ := unstructured.NestedString(guest.Object, "status", "network", "primaryIP")
	if primaryIP == "" {
		return nil
	}
	address := primaryIP
	if scope, _, _ := unstructured.NestedString(guest.Object, "status", "network", "primaryIPScope"); scope == primaryIPScopePod {
		address, _, _ = unstructured.NestedString(guest.Object, "status", "network", "podIP")
		if address == "" {
			return nil
		}
	}
	return []clusterv1.MachineAddress{{Type: clusterv1.MachineInternalIP, Address: address}}
}

// unresolvedMessage reports whether the SwiftGuest is held back by its Resolved
// condition (a reference that is missing or not ready) and, if so, why.
func unresolvedMessage(guest *unstructured.Unstructured) (string, bool) {
	conds, _, _ := unstructured.NestedSlice(guest.Object, "status", "conditions")
	for _, c := range conds {
		cond, ok := c.(map[string]interface{})
		if !ok || cond["type"] != resolvedCondition || cond["status"] != string(metav1.ConditionFalse) {
			continue
		}
		msg, _ := cond["message"].(string)
		if msg == "" {
			msg, _ = cond["reason"].(string)
		}
		return msg, true
	}
	return "", false
}

// Delete removes the SwiftGuest, then (once it is gone) the SwiftSeedProfile. It
// returns done=false to signal the caller to requeue until deletion completes.
func (b *SwiftGuestBackend) Delete(ctx context.Context, req Request) (bool, error) {
	guest := newUnstructured(swiftGuestGVK)
	getErr := b.Client.Get(ctx, types.NamespacedName{Namespace: req.GuestNamespace, Name: req.Machine.Name}, guest)
	switch {
	case getErr == nil:
		if guest.GetDeletionTimestamp().IsZero() {
			if err := b.Client.Delete(ctx, guest); err != nil && !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("deleting SwiftGuest: %w", err)
			}
		}
		return false, nil // still terminating; requeue
	case !apierrors.IsNotFound(getErr):
		return false, getErr
	}

	seed := newUnstructured(swiftSeedProfileGVK)
	seed.SetNamespace(req.GuestNamespace)
	seed.SetName(seedName(req.Machine.Name))
	if err := b.Client.Delete(ctx, seed); err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("deleting SwiftSeedProfile: %w", err)
	}
	return true, nil
}

// ensureSeedProfile creates the NoCloud SwiftSeedProfile (cloud-init) if absent, and
// moves one created before userDataFrom off its inline userData.
func (b *SwiftGuestBackend) ensureSeedProfile(ctx context.Context, req Request) error {
	name := seedName(req.Machine.Name)
	desired := seedProfileSpec(req)
	existing := newUnstructured(swiftSeedProfileGVK)
	err := b.Client.Get(ctx, types.NamespacedName{Namespace: req.GuestNamespace, Name: name}, existing)
	if err == nil {
		// A profile from an earlier release carries the bootstrap cloud-init — join
		// token included — inline in its spec. Point it at the secret instead. This
		// touches no running guest: KubeSwift renders the seed only when it creates a
		// launcher and does not watch profiles, and the secret holds the same bytes.
		userData, _, _ := unstructured.NestedString(existing.Object, "spec", "userData")
		userDataFrom, _, _ := unstructured.NestedMap(existing.Object, "spec", "userDataFrom")
		if userData == "" && apiequality.Semantic.DeepEqual(userDataFrom, desired["userDataFrom"]) {
			return nil
		}
		unstructured.RemoveNestedField(existing.Object, "spec", "userData")
		if err := unstructured.SetNestedField(existing.Object, desired["userDataFrom"], "spec", "userDataFrom"); err != nil {
			return err
		}
		return b.Client.Update(ctx, existing)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	seed := newUnstructured(swiftSeedProfileGVK)
	seed.SetNamespace(req.GuestNamespace)
	seed.SetName(name)
	setKubeSwiftLabels(seed, req)
	seed.Object["spec"] = desired
	return b.Client.Create(ctx, seed)
}

// seedProfileSpec renders a machine's SwiftSeedProfile spec (pure; no client).
func seedProfileSpec(req Request) map[string]interface{} {
	return map[string]interface{}{
		"datasource": "NoCloud",
		// The Cluster API bootstrap cloud-init is used verbatim, read by KubeSwift from
		// the bootstrap data secret (userDataFrom needs KubeSwift v0.13.12+) so the join
		// credentials it carries never sit in a SwiftSeedProfile spec. The kubelet
		// provider-id is NOT injected here — the controller patches the workload Node's
		// providerID directly (reliable; see internal/controller/workload_node.go),
		// which supersedes the fragile cloud-init drop-in path.
		"userDataFrom": map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				nameField: req.BootstrapDataSecretName,
				"key":     BootstrapDataSecretKey,
			},
		},
		"metaData": fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", req.Machine.Name, req.Machine.Name),
	}
}

// ensureSwiftGuest creates the SwiftGuest VM if absent and returns the current object.
func (b *SwiftGuestBackend) ensureSwiftGuest(ctx context.Context, req Request, cfg *infrav1.SwiftGuestBackend) (*unstructured.Unstructured, error) {
	existing := newUnstructured(swiftGuestGVK)
	err := b.Client.Get(ctx, types.NamespacedName{Namespace: req.GuestNamespace, Name: req.Machine.Name}, existing)
	if err == nil {
		return existing, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	guest := renderSwiftGuest(req, cfg)
	if err := b.Client.Create(ctx, guest); err != nil {
		return nil, err
	}
	return guest, nil
}

// renderSwiftGuest builds the SwiftGuest object for a machine (pure; no client). For a
// control-plane machine under Service-backed endpoint provisioning it also stamps the
// KubeSwift pool-selector label and a nat-bound DNAT exposure of the API-server port so
// the KubeSwiftCluster's Service can front it.
func renderSwiftGuest(req Request, cfg *infrav1.SwiftGuestBackend) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"imageRef":       localRef(cfg.ImageRef),
		"guestClassRef":  localRef(cfg.GuestClassRef),
		"seedProfileRef": localRef(seedName(req.Machine.Name)),
	}
	switch {
	case cfg.NetworkRef != "":
		// The named network is the guest's PRIMARY interface. KubeSwift hands the NAD's
		// IP and gateway to the guest only when primary is set explicitly.
		spec["interfaces"] = []interface{}{
			map[string]interface{}{
				nameField:    primaryInterfaceName,
				primaryField: true,
				"networkRef": localRef(cfg.NetworkRef),
			},
		}
	case cfg.NodeNetworkRef != "":
		// Multi-node: a node-local nat primary (management + the reachable control-plane
		// endpoint) plus a secondary routable interface carrying the node datapath
		// (apiserver<->kubelet + pod overlay). The bootstrap sets kubelet --node-ip to
		// the secondary interface's address.
		spec["interfaces"] = []interface{}{
			map[string]interface{}{nameField: "mgmt", primaryField: true},
			map[string]interface{}{nameField: "node", "networkRef": localRef(cfg.NodeNetworkRef)},
		}
	}
	if cfg.StorageClassName != "" {
		// Override the root-disk StorageClass (else KubeSwift inherits the source
		// SwiftImage's class). Lets a machine pin, e.g., a lower-replica class on a
		// cluster whose storage cannot hold the image class's replica count.
		// accessMode is set explicitly because SwiftGuest's validation requires it
		// present whenever spec.storage is set; ReadWriteOnce is the default and the
		// right mode for a CAPI node's root disk (workers are not live-migrated).
		spec["storage"] = map[string]interface{}{
			"storageClassName": cfg.StorageClassName,
			"accessMode":       "ReadWriteOnce",
		}
	}
	if cfg.NodeName != "" {
		// Static placement: KubeSwift pins the launcher pod by writing
		// pod.spec.nodeName directly, so the machine's VM lands on this host and
		// nowhere else. Direct binding also means a bad fit is rejected by the
		// kubelet in seconds rather than sitting Pending.
		spec["nodeName"] = cfg.NodeName
	}
	if gpu := cfg.GPU; gpu != nil {
		// Whole-device passthrough. KubeSwift owns the physical GPU: it binds the
		// device to vfio-pci and passes it into the VM. Whatever shares or consumes
		// it inside the guest is the workload cluster's business.
		switch {
		case gpu.GPUProfileRef != "":
			spec["gpuProfileRef"] = localRef(gpu.GPUProfileRef)
		default:
			claim := map[string]interface{}{}
			if gpu.ResourceClaimTemplateName != "" {
				claim["resourceClaimTemplateName"] = gpu.ResourceClaimTemplateName
			} else if gpu.ResourceClaimName != "" {
				claim["resourceClaimName"] = gpu.ResourceClaimName
			}
			if gpu.RequestName != "" {
				claim["requestName"] = gpu.RequestName
			}
			if gpu.Tier != "" {
				claim["tier"] = gpu.Tier
			}
			if gpu.Hugepages != "" {
				claim["hugepages"] = gpu.Hugepages
			}
			spec["gpuResourceClaim"] = claim
		}
	}
	if req.ControlPlaneExposure != nil {
		// Expose the API-server port via KubeSwift's in-pod DNAT (nat binding, no
		// per-guest "expose" — the KubeSwiftCluster mints one Service selecting all
		// control-plane guests). See docs/design/control-plane-endpoint.md.
		port := int64(req.ControlPlaneExposure.Port)
		spec["network"] = map[string]interface{}{
			"binding": natBinding,
			"ports": []interface{}{
				map[string]interface{}{
					nameField:    infrav1.APIServerPortName,
					"port":       port,
					"targetPort": port,
				},
			},
		}
	}

	guest := newUnstructured(swiftGuestGVK)
	guest.SetNamespace(req.GuestNamespace)
	guest.SetName(req.Machine.Name)
	setKubeSwiftLabels(guest, req)
	if req.ControlPlaneExposure != nil {
		labels := guest.GetLabels()
		labels[infrav1.ControlPlanePoolLabelKey] = req.ControlPlaneExposure.PoolLabel
		guest.SetLabels(labels)
	}
	guest.Object["spec"] = spec
	return guest
}

// nameField is the "name" key used throughout the unstructured SwiftGuest spec.
const nameField = "name"

// primaryField marks a SwiftGuest interface as the guest's primary NIC.
const primaryField = "primary"

// primaryInterfaceName is the interface name given to a networkRef-backed primary NIC.
const primaryInterfaceName = "primary"

// natBinding is the SwiftGuest network binding for a node-local (pod-network) NIC —
// the primary interface under Service-backed endpoint provisioning.
const natBinding = "nat"

// resolvedCondition is the SwiftGuest condition that says whether everything the
// guest references exists and is ready (KubeSwift waits on it rather than failing).
const resolvedCondition = "Resolved"

// primaryIPScopePod is the SwiftGuest status.network.primaryIPScope of an address
// private to the guest's launcher pod (KubeSwift v0.15.0+).
const primaryIPScopePod = "Pod"

// localRef renders a corev1.LocalObjectReference as an unstructured map (KubeSwift
// imageRef/guestClassRef/seedProfileRef are LocalObjectReferences — name only).
func localRef(name string) map[string]interface{} {
	return map[string]interface{}{nameField: name}
}

func newUnstructured(gvk schema.GroupVersionKind) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	return u
}

func setKubeSwiftLabels(u *unstructured.Unstructured, req Request) {
	labels := u.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	if req.Cluster != nil {
		labels[clusterv1.ClusterNameLabel] = req.Cluster.Name
	}
	labels[MachineLabelKey] = req.Machine.Name
	u.SetLabels(labels)
}
