package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// KubeSwiftClusterSpec defines the desired state of KubeSwiftCluster.
type KubeSwiftClusterSpec struct {
	// controlPlaneEndpoint is the endpoint used to reach the workload cluster's
	// Kubernetes API server. KubeSwift does not provision a load balancer, so this
	// is supplied by the operator (a control-plane VIP or an external load
	// balancer) or by the control-plane provider. When it is set, the controller
	// reports the cluster provisioned.
	// +optional
	ControlPlaneEndpoint *clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// guestNamespace is the namespace in which the SwiftGuest VMs backing this
	// cluster's machines are created. Defaults to the KubeSwiftCluster's namespace.
	// +optional
	GuestNamespace string `json:"guestNamespace,omitempty"`
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
