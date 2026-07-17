package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeSwiftMachineTemplateSpec defines the desired state of KubeSwiftMachineTemplate.
type KubeSwiftMachineTemplateSpec struct {
	// template is the KubeSwiftMachine that MachineDeployments and ClusterClass
	// stamp out.
	Template KubeSwiftMachineTemplateResource `json:"template"`
}

// KubeSwiftMachineTemplateResource is the templated KubeSwiftMachine body.
type KubeSwiftMachineTemplateResource struct {
	// metadata is the labels and annotations to stamp onto the created object.
	// +optional
	ObjectMeta ObjectMeta `json:"metadata,omitempty"`

	// spec is the KubeSwiftMachineSpec to stamp out.
	Spec KubeSwiftMachineSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeswiftmachinetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// KubeSwiftMachineTemplate is the Schema for the kubeswiftmachinetemplates API.
type KubeSwiftMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KubeSwiftMachineTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// KubeSwiftMachineTemplateList contains a list of KubeSwiftMachineTemplate.
type KubeSwiftMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeSwiftMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeSwiftMachineTemplate{}, &KubeSwiftMachineTemplateList{})
}
