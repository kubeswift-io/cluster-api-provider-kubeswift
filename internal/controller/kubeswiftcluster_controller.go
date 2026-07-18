package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
)

// KubeSwiftClusterReconciler reconciles a KubeSwiftCluster object.
type KubeSwiftClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubeswiftclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubeswiftclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubeswiftclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives a KubeSwiftCluster to the state the Cluster API contract expects:
// once a control-plane endpoint is known, it reports the cluster infrastructure
// provisioned. How the endpoint is established depends on spec.endpoint.mode:
// External (operator-supplied) or Service (the provider mints a Kubernetes Service
// fronting the control-plane guests — CNI-agnostic, works without OVN).
func (r *KubeSwiftClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := logf.FromContext(ctx)

	ksc := &infrav1.KubeSwiftCluster{}
	if err := r.Get(ctx, req.NamespacedName, ksc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Find the owning Cluster (set by the core Cluster controller from
	// Cluster.spec.infrastructureRef). Wait until it exists.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, ksc.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Waiting for the owning Cluster to be set via OwnerReferences")
		return ctrl.Result{}, nil
	}

	// Externally managed infrastructure is owned by an external system; do not touch it.
	if annotations.IsExternallyManaged(ksc) {
		log.Info("KubeSwiftCluster is externally managed, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Respect pause: leave the object untouched.
	if annotations.IsPaused(cluster, ksc) {
		log.Info("Reconciliation is paused for this KubeSwiftCluster")
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(ksc, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, ksc); err != nil && rerr == nil {
			rerr = err
		}
	}()

	if !ksc.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, ksc)
	}
	return r.reconcileNormal(ctx, ksc, cluster)
}

func (r *KubeSwiftClusterReconciler) reconcileNormal(ctx context.Context, ksc *infrav1.KubeSwiftCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	controllerutil.AddFinalizer(ksc, infrav1.ClusterFinalizer)

	// Service-backed endpoint: the provider mints a Service fronting the control-plane
	// guests and adopts its address as the endpoint. CNI-agnostic (no OVN needed).
	if endpointMode(ksc) == infrav1.EndpointModeService {
		return r.reconcileServiceEndpoint(ctx, ksc, cluster)
	}

	// External endpoint: wait for the operator (or the control-plane provider) to set
	// spec.controlPlaneEndpoint; once set, the core Cluster controller surfaces it onto
	// Cluster.spec.controlPlaneEndpoint.
	if ksc.Spec.ControlPlaneEndpoint == nil || ksc.Spec.ControlPlaneEndpoint.Host == "" {
		markClusterNotProvisioned(ksc, infrav1.WaitingForControlPlaneEndpointReason, "Waiting for spec.controlPlaneEndpoint to be set")
		return ctrl.Result{}, nil
	}
	markClusterProvisioned(ksc)
	return ctrl.Result{}, nil
}

func (r *KubeSwiftClusterReconciler) reconcileDelete(ctx context.Context, ksc *infrav1.KubeSwiftCluster) (ctrl.Result, error) {
	// The endpoint Service (Service mode) is owner-referenced to the KubeSwiftCluster and
	// garbage collected with it; delete it explicitly too so teardown does not depend on
	// GC ordering.
	if err := r.deleteControlPlaneService(ctx, ksc); err != nil {
		return ctrl.Result{}, err
	}
	controllerutil.RemoveFinalizer(ksc, infrav1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

// endpointMode returns the effective endpoint provisioning mode (default External).
func endpointMode(ksc *infrav1.KubeSwiftCluster) string {
	if ksc.Spec.Endpoint == nil || ksc.Spec.Endpoint.Mode == "" {
		return infrav1.EndpointModeExternal
	}
	return ksc.Spec.Endpoint.Mode
}

func markClusterProvisioned(ksc *infrav1.KubeSwiftCluster) {
	ksc.Status.Initialization.Provisioned = ptr.To(true)
	conditions.Set(ksc, metav1.Condition{
		Type:   infrav1.ReadyConditionType,
		Status: metav1.ConditionTrue,
		Reason: infrav1.ClusterProvisionedReason,
	})
}

func markClusterNotProvisioned(ksc *infrav1.KubeSwiftCluster, reason, message string) {
	ksc.Status.Initialization.Provisioned = ptr.To(false)
	conditions.Set(ksc, metav1.Condition{
		Type:    infrav1.ReadyConditionType,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeSwiftClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.KubeSwiftCluster{}).
		Owns(&corev1.Service{}). // re-reconcile when the endpoint Service gets an address
		Named("kubeswiftcluster").
		Complete(r)
}
