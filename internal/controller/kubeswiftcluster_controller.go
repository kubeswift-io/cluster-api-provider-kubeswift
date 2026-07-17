package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
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

// Reconcile drives a KubeSwiftCluster to the state the Cluster API contract expects:
// once a control-plane endpoint is known, it reports the cluster infrastructure
// provisioned. KubeSwift provisions no load balancer, so the endpoint is supplied by
// the operator (or control-plane provider) on spec.controlPlaneEndpoint.
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
		return r.reconcileDelete(ksc)
	}
	return r.reconcileNormal(ksc)
}

func (r *KubeSwiftClusterReconciler) reconcileNormal(ksc *infrav1.KubeSwiftCluster) (ctrl.Result, error) {
	controllerutil.AddFinalizer(ksc, infrav1.ClusterFinalizer)

	// KubeSwift does not provision a control-plane endpoint. Wait for the operator
	// (or the control-plane provider) to set spec.controlPlaneEndpoint; once set, the
	// core Cluster controller surfaces it onto Cluster.spec.controlPlaneEndpoint.
	if ksc.Spec.ControlPlaneEndpoint == nil || ksc.Spec.ControlPlaneEndpoint.Host == "" {
		ksc.Status.Initialization.Provisioned = ptr.To(false)
		conditions.Set(ksc, metav1.Condition{
			Type:    infrav1.ReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.WaitingForControlPlaneEndpointReason,
			Message: "Waiting for spec.controlPlaneEndpoint to be set",
		})
		return ctrl.Result{}, nil
	}

	ksc.Status.Initialization.Provisioned = ptr.To(true)
	conditions.Set(ksc, metav1.Condition{
		Type:   infrav1.ReadyConditionType,
		Status: metav1.ConditionTrue,
		Reason: infrav1.ClusterProvisionedReason,
	})
	return ctrl.Result{}, nil
}

func (r *KubeSwiftClusterReconciler) reconcileDelete(ksc *infrav1.KubeSwiftCluster) (ctrl.Result, error) {
	// KubeSwift provisions no cluster-scoped infrastructure, so there is nothing to
	// tear down. Release the finalizer.
	controllerutil.RemoveFinalizer(ksc, infrav1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeSwiftClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.KubeSwiftCluster{}).
		Named("kubeswiftcluster").
		Complete(r)
}
