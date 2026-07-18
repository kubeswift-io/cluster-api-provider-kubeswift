package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

func serviceModeCluster(svcType corev1.ServiceType, port int32, lbClass *string) *infrav1.KubeSwiftCluster {
	return &infrav1.KubeSwiftCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "ns"},
		Spec: infrav1.KubeSwiftClusterSpec{
			Endpoint: &infrav1.ControlPlaneEndpointSpec{
				Mode:    infrav1.EndpointModeService,
				Service: &infrav1.ControlPlaneServiceSpec{Type: string(svcType), Port: port, LoadBalancerClass: lbClass},
			},
		},
	}
}

func TestBuildControlPlaneService_ClusterIP(t *testing.T) {
	r := &KubeSwiftClusterReconciler{}
	ksc := serviceModeCluster(corev1.ServiceTypeClusterIP, 6443, nil)
	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "ns"}}

	svc := r.buildControlPlaneService(ksc, cluster)

	if svc.Name != "demo-cp" || svc.Namespace != "ns" {
		t.Fatalf("name/ns = %s/%s, want demo-cp/ns", svc.Name, svc.Namespace)
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("type = %s, want ClusterIP", svc.Spec.Type)
	}
	if got := svc.Spec.Selector[infrav1.ControlPlanePoolLabelKey]; got != "demo-cp" {
		t.Fatalf("selector[%s] = %q, want demo-cp", infrav1.ControlPlanePoolLabelKey, got)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 6443 || svc.Spec.Ports[0].TargetPort.IntValue() != 6443 {
		t.Fatalf("ports = %+v, want a single 6443->6443", svc.Spec.Ports)
	}
}

func TestBuildControlPlaneService_LoadBalancerDefaultsAndAnnotations(t *testing.T) {
	r := &KubeSwiftClusterReconciler{}
	lbc := "metallb"
	ksc := serviceModeCluster(corev1.ServiceTypeLoadBalancer, 0, &lbc) // port 0 -> default 6443
	ksc.Spec.Endpoint.Service.Annotations = map[string]string{"metallb.universe.tf/address-pool": "cp"}
	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "demo"}}

	svc := r.buildControlPlaneService(ksc, cluster)

	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("type = %s, want LoadBalancer", svc.Spec.Type)
	}
	if svc.Spec.LoadBalancerClass == nil || *svc.Spec.LoadBalancerClass != "metallb" {
		t.Fatalf("loadBalancerClass = %v, want metallb", svc.Spec.LoadBalancerClass)
	}
	if svc.Spec.Ports[0].Port != 6443 {
		t.Fatalf("default port = %d, want 6443", svc.Spec.Ports[0].Port)
	}
	if svc.Annotations["metallb.universe.tf/address-pool"] != "cp" {
		t.Fatalf("annotations not copied: %v", svc.Annotations)
	}
}

func TestControlPlaneServiceAddress(t *testing.T) {
	clusterIP := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.5"}}
	if got, ok := controlPlaneServiceAddress(clusterIP); !ok || got != "10.0.0.5" {
		t.Fatalf("clusterIP addr = %q/%v, want 10.0.0.5/true", got, ok)
	}

	headless := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: corev1.ClusterIPNone}}
	if _, ok := controlPlaneServiceAddress(headless); ok {
		t.Fatal("headless ClusterIP should not be ready")
	}

	lbPending := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}
	if _, ok := controlPlaneServiceAddress(lbPending); ok {
		t.Fatal("LoadBalancer with no ingress should not be ready")
	}

	lbReady := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}
	lbReady.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "203.0.113.4"}}
	if got, ok := controlPlaneServiceAddress(lbReady); !ok || got != "203.0.113.4" {
		t.Fatalf("LoadBalancer addr = %q/%v, want 203.0.113.4/true", got, ok)
	}
}

func TestEndpointModeDefaultsToExternal(t *testing.T) {
	if got := endpointMode(&infrav1.KubeSwiftCluster{}); got != infrav1.EndpointModeExternal {
		t.Fatalf("default mode = %q, want External", got)
	}
	ksc := serviceModeCluster(corev1.ServiceTypeClusterIP, 6443, nil)
	if got := endpointMode(ksc); got != infrav1.EndpointModeService {
		t.Fatalf("mode = %q, want Service", got)
	}
}

func TestMergeServicePorts_PreservesNodePort(t *testing.T) {
	existing := []corev1.ServicePort{{Name: "apiserver", Port: 6443, NodePort: 30443}}
	desired := []corev1.ServicePort{{Name: "apiserver", Port: 6443}}

	out := mergeServicePorts(existing, desired)

	if len(out) != 1 || out[0].NodePort != 30443 {
		t.Fatalf("merged ports = %+v, want nodePort 30443 preserved", out)
	}
}
