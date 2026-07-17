package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeSwiftClusterSpec defines the desired state of KubeSwiftCluster.
type KubeSwiftClusterSpec struct {
	// controlPlaneEndpoint is the endpoint used to reach the workload cluster's
	// Kubernetes API server. KubeSwift does not provision a load balancer, so this
	// is supplied by the operator (a control-plane VIP or an external load
	// balancer) and must be set before the cluster reports Ready.
	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// guestNamespace is the namespace in which the SwiftGuest VMs backing this
	// cluster's machines are created. Defaults to the KubeSwiftCluster's namespace.
	// +optional
	GuestNamespace string `json:"guestNamespace,omitempty"`
}

// KubeSwiftClusterStatus defines the observed state of KubeSwiftCluster.
type KubeSwiftClusterStatus struct {
	// ready denotes that the cluster infrastructure is ready. Required by the
	// Cluster API infrastructure-provider contract.
	// +optional
	Ready bool `json:"ready"`

	// conditions represents the observations of the KubeSwiftCluster's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=kubeswiftclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=".status.ready",description="Cluster infrastructure is ready"
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
