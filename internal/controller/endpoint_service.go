package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

// controlPlaneServiceName is the name of the Service the provider mints to front a
// cluster's control-plane guests when endpoint.mode=Service.
func controlPlaneServiceName(ksc *infrav1.KubeSwiftCluster) string {
	return ksc.Name + "-cp"
}

// reconcileServiceEndpoint mints (or reconciles) the endpoint Service, reads its
// address, and adopts it as spec.controlPlaneEndpoint once available.
func (r *KubeSwiftClusterReconciler) reconcileServiceEndpoint(ctx context.Context, ksc *infrav1.KubeSwiftCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	svc, err := r.ensureControlPlaneService(ctx, ksc, cluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("ensuring control-plane Service: %w", err)
	}

	host, ready := controlPlaneServiceAddress(svc)
	if !ready {
		// A ClusterIP is assigned at creation; a LoadBalancer address may lag. The
		// Owns(&Service) watch re-reconciles when the address lands.
		markClusterNotProvisioned(ksc, infrav1.WaitingForEndpointServiceReason,
			fmt.Sprintf("Waiting for Service %s/%s to get an address", svc.Namespace, svc.Name))
		return ctrl.Result{}, nil
	}

	ksc.Spec.ControlPlaneEndpoint = &clusterv1.APIEndpoint{Host: host, Port: controlPlaneServicePort(ksc)}
	markClusterProvisioned(ksc)
	return ctrl.Result{}, nil
}

// ensureControlPlaneService creates the endpoint Service if absent, or reconciles the
// fields the provider owns (type, selector, ports, configured annotations) while
// preserving apiserver-assigned fields (clusterIP, nodePorts).
func (r *KubeSwiftClusterReconciler) ensureControlPlaneService(ctx context.Context, ksc *infrav1.KubeSwiftCluster, cluster *clusterv1.Cluster) (*corev1.Service, error) {
	desired := r.buildControlPlaneService(ksc, cluster)
	if err := controllerutil.SetControllerReference(ksc, desired, r.Scheme); err != nil {
		return nil, err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return nil, err
		}
		return desired, nil
	}
	if err != nil {
		return nil, err
	}

	updated := existing.DeepCopy()
	updated.Spec.Type = desired.Spec.Type
	updated.Spec.Selector = desired.Spec.Selector
	updated.Spec.Ports = mergeServicePorts(existing.Spec.Ports, desired.Spec.Ports)
	if desired.Spec.Type == corev1.ServiceTypeLoadBalancer {
		updated.Spec.LoadBalancerClass = desired.Spec.LoadBalancerClass
	}
	// Merge our configured annotations without removing foreign ones (e.g. those an LB
	// controller writes back).
	if len(desired.Annotations) > 0 {
		if updated.Annotations == nil {
			updated.Annotations = map[string]string{}
		}
		for k, v := range desired.Annotations {
			updated.Annotations[k] = v
		}
	}

	if apiequality.Semantic.DeepEqual(existing.Spec.Type, updated.Spec.Type) &&
		apiequality.Semantic.DeepEqual(existing.Spec.Selector, updated.Spec.Selector) &&
		apiequality.Semantic.DeepEqual(existing.Spec.Ports, updated.Spec.Ports) &&
		apiequality.Semantic.DeepEqual(existing.Spec.LoadBalancerClass, updated.Spec.LoadBalancerClass) &&
		apiequality.Semantic.DeepEqual(existing.Annotations, updated.Annotations) {
		return existing, nil
	}
	if err := r.Update(ctx, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// buildControlPlaneService renders the desired endpoint Service.
func (r *KubeSwiftClusterReconciler) buildControlPlaneService(ksc *infrav1.KubeSwiftCluster, cluster *clusterv1.Cluster) *corev1.Service {
	port := controlPlaneServicePort(ksc)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controlPlaneServiceName(ksc),
			Namespace: ksc.Namespace,
			Labels:    map[string]string{clusterv1.ClusterNameLabel: cluster.Name},
		},
		Spec: corev1.ServiceSpec{
			Type: controlPlaneServiceType(ksc),
			// Select the control-plane guests' launcher pods (the provider stamps this
			// label on each control-plane SwiftGuest; KubeSwift propagates it to the pod).
			Selector: map[string]string{
				infrav1.ControlPlanePoolLabelKey: infrav1.ControlPlaneServiceSelectorValue(cluster.Name),
			},
			Ports: []corev1.ServicePort{{
				Name:       infrav1.APIServerPortName,
				Port:       port,
				TargetPort: intstr.FromInt32(port), // pod-side port; KubeSwift's DNAT forwards it to the VM
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	if cfg := serviceCfg(ksc); cfg != nil {
		if len(cfg.Annotations) > 0 {
			svc.Annotations = cfg.Annotations
		}
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer && cfg.LoadBalancerClass != nil {
			svc.Spec.LoadBalancerClass = cfg.LoadBalancerClass
		}
	}
	return svc
}

// deleteControlPlaneService removes the endpoint Service (no-op if absent or if the
// cluster never used Service mode).
func (r *KubeSwiftClusterReconciler) deleteControlPlaneService(ctx context.Context, ksc *infrav1.KubeSwiftCluster) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: controlPlaneServiceName(ksc), Namespace: ksc.Namespace}}
	if err := r.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting control-plane Service: %w", err)
	}
	return nil
}

// controlPlaneServiceAddress returns the Service's reachable address and whether it is
// ready: the ClusterIP for a ClusterIP Service, or the first LoadBalancer ingress
// address once assigned.
func controlPlaneServiceAddress(svc *corev1.Service) (string, bool) {
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				return ing.IP, true
			}
			if ing.Hostname != "" {
				return ing.Hostname, true
			}
		}
		return "", false
	}
	if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != corev1.ClusterIPNone {
		return svc.Spec.ClusterIP, true
	}
	return "", false
}

// mergeServicePorts keeps desired ports but carries over any NodePort the apiserver
// assigned to a port with the same name.
func mergeServicePorts(existing, desired []corev1.ServicePort) []corev1.ServicePort {
	byName := map[string]int32{}
	for _, p := range existing {
		if p.NodePort != 0 {
			byName[p.Name] = p.NodePort
		}
	}
	out := make([]corev1.ServicePort, len(desired))
	copy(out, desired)
	for i := range out {
		if np, ok := byName[out[i].Name]; ok && out[i].NodePort == 0 {
			out[i].NodePort = np
		}
	}
	return out
}

func serviceCfg(ksc *infrav1.KubeSwiftCluster) *infrav1.ControlPlaneServiceSpec {
	if ksc.Spec.Endpoint == nil {
		return nil
	}
	return ksc.Spec.Endpoint.Service
}

func controlPlaneServicePort(ksc *infrav1.KubeSwiftCluster) int32 {
	if cfg := serviceCfg(ksc); cfg != nil && cfg.Port != 0 {
		return cfg.Port
	}
	return defaultAPIServerPort
}

func controlPlaneServiceType(ksc *infrav1.KubeSwiftCluster) corev1.ServiceType {
	if cfg := serviceCfg(ksc); cfg != nil && cfg.Type != "" {
		return corev1.ServiceType(cfg.Type)
	}
	return corev1.ServiceTypeClusterIP
}
