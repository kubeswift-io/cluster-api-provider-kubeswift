package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// KubeSwiftClusterSpec defines the desired state of KubeSwiftCluster.
type KubeSwiftClusterSpec struct {
	// controlPlaneEndpoint is the endpoint used to reach the workload cluster's
	// Kubernetes API server. It is the resolved endpoint: with endpoint.mode=External
	// the operator supplies it (a control-plane VIP or external load balancer); with
	// endpoint.mode=Service or RoutableIP the provider provisions and writes it. Once
	// it is set, the controller reports the cluster provisioned and the core Cluster
	// controller surfaces it onto Cluster.spec.controlPlaneEndpoint.
	// +optional
	ControlPlaneEndpoint *clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// endpoint configures how the provider establishes the control-plane endpoint.
	// When unset (or mode=External), the operator supplies controlPlaneEndpoint and the
	// provider provisions nothing.
	// +optional
	Endpoint *ControlPlaneEndpointSpec `json:"endpoint,omitempty"`

	// guestNamespace is the namespace in which the SwiftGuest VMs backing this
	// cluster's machines are created. Defaults to the KubeSwiftCluster's namespace.
	// +optional
	GuestNamespace string `json:"guestNamespace,omitempty"`
}

// Control-plane endpoint provisioning modes.
const (
	// EndpointModeExternal: the operator supplies spec.controlPlaneEndpoint (a VIP or
	// an external load balancer); the provider provisions nothing. Default.
	EndpointModeExternal = "External"
	// EndpointModeService: the provider mints a Kubernetes Service in the management
	// cluster fronting the control-plane guests on the API-server port and reports its
	// address as the endpoint. CNI-agnostic — works without OVN.
	EndpointModeService = "Service"
	// EndpointModeRoutableIP: the provider uses the control-plane guest's
	// cluster-routable IP (OVN-Kubernetes) as the endpoint.
	EndpointModeRoutableIP = "RoutableIP"
)

// ControlPlanePoolLabelKey is the KubeSwift guest label the provider stamps on
// control-plane SwiftGuests so its Service can select their launcher pods. It is the
// only guest-settable label KubeSwift propagates from a SwiftGuest to its launcher pod
// (mirrors KubeSwift's own pool-selector label). Coupling to this key is deliberate and
// isolated here.
const ControlPlanePoolLabelKey = "swift.kubeswift.io/pool"

// APIServerPortName is the name shared by the control-plane guest's exposed API-server
// port and the endpoint Service's port; keeping them equal keeps the two in step.
const APIServerPortName = "apiserver"

// ControlPlaneServiceSelectorValue is the ControlPlanePoolLabelKey value stamped on a
// cluster's control-plane guests (and selected by that cluster's endpoint Service).
func ControlPlaneServiceSelectorValue(clusterName string) string {
	return clusterName + "-cp"
}

// ControlPlaneEndpointSpec configures how the provider establishes the control-plane
// endpoint.
type ControlPlaneEndpointSpec struct {
	// mode selects the endpoint strategy:
	//   External — the operator supplies spec.controlPlaneEndpoint; the provider
	//              provisions nothing.
	//   Service  — the provider mints a Kubernetes Service in the management cluster
	//              fronting the control-plane guests on the API-server port, and
	//              reports its address as the endpoint. CNI-agnostic (no OVN needed).
	// (RoutableIP — use the control-plane guest's OVN-routable IP — is planned; see
	// docs/design/control-plane-endpoint.md.)
	// +kubebuilder:validation:Enum=External;Service
	// +kubebuilder:default=External
	Mode string `json:"mode,omitempty"`

	// service configures the Service-backed endpoint. Used when mode=Service.
	// +optional
	Service *ControlPlaneServiceSpec `json:"service,omitempty"`
}

// ControlPlaneServiceSpec configures the Service the provider mints to front the
// control-plane guests when endpoint.mode=Service.
type ControlPlaneServiceSpec struct {
	// type is the Service type fronting the control-plane guests. ClusterIP is reachable
	// from the management cluster (the CAPI controllers and worker VMs) and is enough for
	// a CAPI-managed cluster to form; LoadBalancer additionally exposes the endpoint
	// outside the management cluster (e.g. for operator kubectl access).
	// +kubebuilder:validation:Enum=ClusterIP;LoadBalancer
	// +kubebuilder:default=ClusterIP
	Type string `json:"type,omitempty"`

	// port is the API-server port fronted by the Service.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=6443
	Port int32 `json:"port,omitempty"`

	// annotations are copied onto the Service (e.g. a MetalLB address pool or an
	// external-dns hostname). Chiefly useful with type=LoadBalancer.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// loadBalancerClass selects a specific load-balancer implementation. Only valid with
	// type=LoadBalancer.
	// +optional
	LoadBalancerClass *string `json:"loadBalancerClass,omitempty"`
}

// KubeSwiftClusterStatus defines the observed state of KubeSwiftCluster.
type KubeSwiftClusterStatus struct {
	// initialization reports the KubeSwiftCluster initialization state.
	// Part of the Cluster API infrastructure-provider contract (v1beta2).
	// +optional
	Initialization InitializationStatus `json:"initialization,omitempty"`

	// conditions represents the observations of the KubeSwiftCluster's state.
	// A condition of type "Ready" is mirrored into the Cluster's
	// InfrastructureReady condition.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=kubeswiftclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Provisioned",type=boolean,JSONPath=".status.initialization.provisioned",description="Cluster infrastructure is provisioned"
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=".spec.controlPlaneEndpoint.host",description="Control plane endpoint host"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// KubeSwiftCluster is the Schema for the kubeswiftclusters API. It is the
// infrastructure counterpart of a Cluster API Cluster, backed by KubeSwift.
type KubeSwiftCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeSwiftClusterSpec   `json:"spec,omitempty"`
	Status KubeSwiftClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubeSwiftClusterList contains a list of KubeSwiftCluster.
type KubeSwiftClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeSwiftCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeSwiftCluster{}, &KubeSwiftClusterList{})
}
