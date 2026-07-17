package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// KubeSwiftClusterTemplateSpec defines the desired state of KubeSwiftClusterTemplate.
type KubeSwiftClusterTemplateSpec struct {
	// template is the KubeSwiftCluster that ClusterClass stamps out.
	Template KubeSwiftClusterTemplateResource `json:"template"`
}

// KubeSwiftClusterTemplateResource is the templated KubeSwiftCluster body.
type KubeSwiftClusterTemplateResource struct {
	// metadata is the labels and annotations to stamp onto the created object.
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the KubeSwiftClusterSpec to stamp out.
	Spec KubeSwiftClusterSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:path=kubeswiftclustertemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// KubeSwiftClusterTemplate is the Schema for the kubeswiftclustertemplates API.
type KubeSwiftClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KubeSwiftClusterTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// KubeSwiftClusterTemplateList contains a list of KubeSwiftClusterTemplate.
type KubeSwiftClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeSwiftClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeSwiftClusterTemplate{}, &KubeSwiftClusterTemplateList{})
}
