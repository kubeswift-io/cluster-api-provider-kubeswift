package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
	"github.com/kubeswift-io/cluster-api-provider-kubeswift/internal/backend"
)

const (
	machineRequeueAfter = 15 * time.Second
	// defaultAPIServerPort is the control-plane API-server port used when the endpoint
	// Service config does not override it.
	defaultAPIServerPort = 6443
)

// KubeSwiftMachineReconciler reconciles a KubeSwiftMachine object.
type KubeSwiftMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubeswiftmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubeswiftmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubeswiftmachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubeswiftclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=swift.kubeswift.io,resources=swiftguests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=seed.kubeswift.io,resources=swiftseedprofiles,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives a KubeSwiftMachine to the Cluster API contract state by dispatching
// to the selected backend (SwiftGuest) and surfacing providerID, addresses, and
// status.initialization.provisioned once the backing VM is running.
func (r *KubeSwiftMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := logf.FromContext(ctx)

	ksm := &infrav1.KubeSwiftMachine{}
	if err := r.Get(ctx, req.NamespacedName, ksm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Owning Cluster API Machine (set by the Machine controller from spec.infrastructureRef).
	machine, err := util.GetOwnerMachine(ctx, r.Client, ksm.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for the owning Machine to be set via OwnerReferences")
		return ctrl.Result{}, nil
	}

	// Owning Cluster (via the cluster-name label).
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, ksm.ObjectMeta)
	if err != nil {
		log.Info("Waiting for the owning Cluster label to be set")
		return ctrl.Result{}, nil //nolint:nilerr // wait for the label; a Cluster event re-triggers
	}

	if annotations.IsPaused(cluster, ksm) {
		log.Info("Reconciliation is paused for this KubeSwiftMachine")
		return ctrl.Result{}, nil
	}

	b, err := backendFor(ksm.Spec.Backend.Type, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	patchHelper, err := patch.NewHelper(ksm, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, ksm); err != nil && rerr == nil {
			rerr = err
		}
	}()

	if !ksm.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, ksm, machine, cluster, b)
	}
	return r.reconcileNormal(ctx, ksm, machine, cluster, b)
}

func (r *KubeSwiftMachineReconciler) reconcileNormal(
	ctx context.Context, ksm *infrav1.KubeSwiftMachine, machine *clusterv1.Machine,
	cluster *clusterv1.Cluster, b backend.Backend,
) (ctrl.Result, error) {
	controllerutil.AddFinalizer(ksm, infrav1.MachineFinalizer)

	// Wait for the cluster infrastructure to be provisioned.
	if !ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
		notReady(ksm, infrav1.WaitingForClusterInfrastructureReason, "Waiting for the cluster infrastructure to be ready")
		return ctrl.Result{}, nil
	}

	// Wait for the bootstrap provider to produce the cloud-init data secret.
	if machine.Spec.Bootstrap.DataSecretName == nil {
		notReady(ksm, infrav1.WaitingForBootstrapDataReason, "Waiting for the bootstrap data secret")
		return ctrl.Result{}, nil
	}

	bootstrapData, err := r.bootstrapData(ctx, machine)
	if err != nil {
		return ctrl.Result{}, err
	}

	guestNamespace := ksm.Namespace
	providerID := fmt.Sprintf("%s://%s/%s", infrav1.ProviderIDScheme, guestNamespace, ksm.Name)

	exposure, err := r.controlPlaneExposure(ctx, machine, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	result, err := b.Reconcile(ctx, backend.Request{
		Machine:              ksm,
		Cluster:              cluster,
		CAPIMachine:          machine,
		BootstrapData:        bootstrapData,
		GuestNamespace:       guestNamespace,
		ProviderID:           providerID,
		ControlPlaneExposure: exposure,
	})
	if err != nil {
		notReady(ksm, infrav1.VMFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	if !result.Provisioned {
		notReady(ksm, infrav1.VMProvisioningReason, "Waiting for the backing VM to be running with an address")
		return ctrl.Result{RequeueAfter: machineRequeueAfter}, nil
	}

	ksm.Spec.ProviderID = providerID
	ksm.Status.Addresses = result.Addresses
	ksm.Status.FailureDomain = machine.Spec.FailureDomain
	ksm.Status.Initialization.Provisioned = ptr.To(true)
	conditions.Set(ksm, metav1.Condition{
		Type:   infrav1.ReadyConditionType,
		Status: metav1.ConditionTrue,
		Reason: infrav1.VMProvisionedReason,
	})
	return ctrl.Result{}, nil
}

func (r *KubeSwiftMachineReconciler) reconcileDelete(
	ctx context.Context, ksm *infrav1.KubeSwiftMachine, machine *clusterv1.Machine,
	cluster *clusterv1.Cluster, b backend.Backend,
) (ctrl.Result, error) {
	done, err := b.Delete(ctx, backend.Request{
		Machine:        ksm,
		Cluster:        cluster,
		CAPIMachine:    machine,
		GuestNamespace: ksm.Namespace,
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if !done {
		return ctrl.Result{RequeueAfter: machineRequeueAfter}, nil
	}
	controllerutil.RemoveFinalizer(ksm, infrav1.MachineFinalizer)
	return ctrl.Result{}, nil
}

// bootstrapData reads the cloud-init from the Machine's bootstrap data secret.
func (r *KubeSwiftMachineReconciler) bootstrapData(ctx context.Context, machine *clusterv1.Machine) ([]byte, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: machine.Namespace, Name: *machine.Spec.Bootstrap.DataSecretName}
	if err := r.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("reading bootstrap data secret %s: %w", key, err)
	}
	data, ok := secret.Data["value"]
	if !ok {
		return nil, fmt.Errorf("bootstrap data secret %s has no 'value' key", key)
	}
	return data, nil
}

// controlPlaneExposure returns the API-server exposure to hand the backend for a
// control-plane machine whose cluster uses Service-backed endpoint provisioning
// (endpoint.mode=Service); nil for workers, other modes, or an unset endpoint block.
func (r *KubeSwiftMachineReconciler) controlPlaneExposure(
	ctx context.Context, machine *clusterv1.Machine, cluster *clusterv1.Cluster,
) (*backend.ControlPlaneExposure, error) {
	if !util.IsControlPlaneMachine(machine) || cluster.Spec.InfrastructureRef.Name == "" {
		return nil, nil
	}
	ksc := &infrav1.KubeSwiftCluster{}
	key := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Spec.InfrastructureRef.Name}
	if err := r.Get(ctx, key, ksc); err != nil {
		return nil, fmt.Errorf("reading KubeSwiftCluster %s: %w", key, err)
	}
	ep := ksc.Spec.Endpoint
	if ep == nil || ep.Mode != infrav1.EndpointModeService {
		return nil, nil
	}
	port := int32(defaultAPIServerPort)
	if ep.Service != nil && ep.Service.Port != 0 {
		port = ep.Service.Port
	}
	return &backend.ControlPlaneExposure{
		PoolLabel: infrav1.ControlPlaneServiceSelectorValue(cluster.Name),
		Port:      port,
	}, nil
}

func notReady(ksm *infrav1.KubeSwiftMachine, reason, message string) {
	ksm.Status.Initialization.Provisioned = ptr.To(false)
	conditions.Set(ksm, metav1.Condition{
		Type:    infrav1.ReadyConditionType,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}

func backendFor(t infrav1.MachineBackendType, c client.Client) (backend.Backend, error) {
	switch t {
	case infrav1.SwiftGuestBackendType:
		return backend.NewSwiftGuestBackend(c), nil
	default:
		return nil, fmt.Errorf("unsupported machine backend type %q", t)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeSwiftMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.KubeSwiftMachine{}).
		Named("kubeswiftmachine").
		Complete(r)
}
