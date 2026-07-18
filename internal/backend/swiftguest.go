package backend

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// Reconcile ensures the SwiftSeedProfile + SwiftGuest exist and reports the VM state.
func (b *SwiftGuestBackend) Reconcile(ctx context.Context, req Request) (Result, error) {
	cfg := req.Machine.Spec.Backend.SwiftGuest
	if cfg == nil {
		return Result{}, fmt.Errorf("spec.backend.swiftGuest must be set when backend.type is %q", infrav1.SwiftGuestBackendType)
	}
	if cfg.ImageRef == "" || cfg.GuestClassRef == "" {
		return Result{}, fmt.Errorf("spec.backend.swiftGuest.imageRef and guestClassRef are required")
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
	primaryIP, _, _ := unstructured.NestedString(guest.Object, "status", "network", "primaryIP")

	switch {
	case phase == "Failed":
		return Result{}, fmt.Errorf("SwiftGuest %s/%s reported phase Failed", req.GuestNamespace, req.Machine.Name)
	case phase != "Running" || primaryIP == "":
		// VM not running yet, or its IP not yet discovered — requeue.
		return Result{Requeue: true}, nil
	default:
		return Result{
			Provisioned: true,
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: primaryIP},
			},
		}, nil
	}
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

// ensureSeedProfile creates the NoCloud SwiftSeedProfile (cloud-init) if absent.
func (b *SwiftGuestBackend) ensureSeedProfile(ctx context.Context, req Request) error {
	name := seedName(req.Machine.Name)
	existing := newUnstructured(swiftSeedProfileGVK)
	err := b.Client.Get(ctx, types.NamespacedName{Namespace: req.GuestNamespace, Name: name}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	seed := newUnstructured(swiftSeedProfileGVK)
	seed.SetNamespace(req.GuestNamespace)
	seed.SetName(name)
	setKubeSwiftLabels(seed, req)
	seed.Object["spec"] = map[string]interface{}{
		"datasource": "NoCloud",
		// The Cluster API bootstrap cloud-init is used verbatim. The kubelet provider-id
		// is NOT injected here — the controller patches the workload Node's providerID
		// directly (reliable; see internal/controller/workload_node.go), which supersedes
		// the fragile cloud-init drop-in path.
		"userData": string(req.BootstrapData),
		"metaData": fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", req.Machine.Name, req.Machine.Name),
	}
	return b.Client.Create(ctx, seed)
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
	if cfg.NetworkRef != "" {
		spec["interfaces"] = []interface{}{
			map[string]interface{}{
				nameField:    "primary",
				"networkRef": localRef(cfg.NetworkRef),
			},
		}
	}
	if req.ControlPlaneExposure != nil {
		// Expose the API-server port via KubeSwift's in-pod DNAT (nat binding, no
		// per-guest "expose" — the KubeSwiftCluster mints one Service selecting all
		// control-plane guests). See docs/design/control-plane-endpoint.md.
		port := int64(req.ControlPlaneExposure.Port)
		spec["network"] = map[string]interface{}{
			"binding": "nat",
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
	labels["infrastructure.cluster.x-k8s.io/kubeswiftmachine"] = req.Machine.Name
	u.SetLabels(labels)
}
