package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeSwiftMachineSpec defines the desired state of KubeSwiftMachine.
type KubeSwiftMachineSpec struct {
	// providerID identifies the SwiftGuest VM backing this machine, in the form
	// "kubeswift://<guest-namespace>/<guest-name>". The controller sets it once the
	// VM is provisioned; the Cluster API Machine controller copies it onto the
	// Node. Required by the infrastructure-provider contract.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// imageRef names the SwiftImage the VM boots from (disk boot).
	// +optional
	ImageRef string `json:"imageRef,omitempty"`

	// guestClassRef names the SwiftGuestClass that supplies the VM's CPU, memory,
	// and disk sizing.
	// +optional
	GuestClassRef string `json:"guestClassRef,omitempty"`

	// resources optionally overrides the CPU and memory from the guest class.
	// +optional
	Resources *MachineResources `json:"resources,omitempty"`

	// networkRef optionally attaches the VM's primary interface to a named network
	// (a NetworkAttachmentDefinition). Empty uses the default pod network.
	// +optional
	NetworkRef string `json:"networkRef,omitempty"`

	// failureDomain is the failure domain the VM is placed in. The controller
	// copies it from the owning Cluster API Machine's spec.failureDomain.
	// +optional
	FailureDomain *string `json:"failureDomain,omitempty"`
}

// MachineResources overrides the guest-class sizing for a single machine.
type MachineResources struct {
	// cpu is the number of vCPUs.
	// +optional
	// +kubebuilder:validation:Minimum=1
	CPU int32 `json:"cpu,omitempty"`

	// memoryMiB is the guest memory in MiB.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MemoryMiB int64 `json:"memoryMiB,omitempty"`
}

// KubeSwiftMachineStatus defines the observed state of KubeSwiftMachine.
type KubeSwiftMachineStatus struct {
	// ready denotes that the VM is provisioned and running. Required by the
	// Cluster API infrastructure-provider contract.
	// +optional
	Ready bool `json:"ready"`

	// addresses lists the addresses assigned to the VM.
	// +optional
	Addresses []MachineAddress `json:"addresses,omitempty"`

	// failureReason is a terminal, programmatic failure cause. When set, the
	// machine is not retried and requires manual intervention.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// failureMessage is a human-readable description of a terminal failure.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// conditions represents the observations of the KubeSwiftMachine's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=kubeswiftmachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=".status.ready",description="VM is provisioned and running"
// +kubebuilder:printcolumn:name="ProviderID",type=string,JSONPath=".spec.providerID",description="SwiftGuest provider ID"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// KubeSwiftMachine is the Schema for the kubeswiftmachines API. It is the
// infrastructure counterpart of a Cluster API Machine, backed by a SwiftGuest VM.
type KubeSwiftMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeSwiftMachineSpec   `json:"spec,omitempty"`
	Status KubeSwiftMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubeSwiftMachineList contains a list of KubeSwiftMachine.
type KubeSwiftMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeSwiftMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeSwiftMachine{}, &KubeSwiftMachineList{})
}
