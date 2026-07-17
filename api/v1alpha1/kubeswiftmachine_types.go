package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// MachineBackendType selects the KubeSwift resource that backs a KubeSwiftMachine.
// +kubebuilder:validation:Enum=SwiftGuest
type MachineBackendType string

const (
	// SwiftGuestBackendType backs the machine with a full SwiftGuest VM (disk boot,
	// cloud-init, routable IP, persistent disk) — the only KubeSwift substrate that
	// can be a persistent Kubernetes node. See docs/spikes/01-sandbox-as-capi-node.md
	// for why SwiftSandbox / SwiftSandboxPool are not valid node backends.
	SwiftGuestBackendType MachineBackendType = "SwiftGuest"
)

// KubeSwiftMachineSpec defines the desired state of KubeSwiftMachine.
type KubeSwiftMachineSpec struct {
	// providerID must match the provider ID on the Node backing this machine, in
	// the form "kubeswift://<guest-namespace>/<guest-name>". The controller sets it
	// once the backing resource is provisioned; the Cluster API Machine controller
	// surfaces it on the Machine. Cluster API infrastructure-provider contract field.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ProviderID string `json:"providerID,omitempty"`

	// backend selects and configures the KubeSwift resource that backs this machine.
	// The config matching backend.type must be set. Today only "SwiftGuest" is a
	// valid node backend; the discriminator exists so a future feasible substrate
	// can be added without an API break.
	Backend MachineBackend `json:"backend"`
}

// MachineBackend is a discriminated union selecting the KubeSwift resource that
// backs a KubeSwiftMachine.
type MachineBackend struct {
	// type is the backend kind. Only "SwiftGuest" is currently supported.
	// +kubebuilder:default=SwiftGuest
	Type MachineBackendType `json:"type"`

	// swiftGuest configures the SwiftGuest (full VM) backend. Required when
	// type is "SwiftGuest".
	// +optional
	SwiftGuest *SwiftGuestBackend `json:"swiftGuest,omitempty"`
}

// SwiftGuestBackend configures a machine backed by a SwiftGuest VM.
type SwiftGuestBackend struct {
	// imageRef names the SwiftImage the VM boots from (disk boot). It references a
	// SwiftImage by name in the guest namespace, which must be Ready before boot.
	// +optional
	ImageRef string `json:"imageRef,omitempty"`

	// guestClassRef names the cluster-scoped SwiftGuestClass that supplies the VM's
	// CPU, memory, and root-disk sizing (SwiftGuest carries no inline sizing).
	// +optional
	GuestClassRef string `json:"guestClassRef,omitempty"`

	// networkRef optionally attaches the VM's primary interface to a named network
	// (a NetworkAttachmentDefinition). Empty uses the default node-local network.
	// +optional
	NetworkRef string `json:"networkRef,omitempty"`
}

// KubeSwiftMachineStatus defines the observed state of KubeSwiftMachine.
type KubeSwiftMachineStatus struct {
	// initialization reports the KubeSwiftMachine initialization state.
	// Part of the Cluster API infrastructure-provider contract (v1beta2).
	// +optional
	Initialization InitializationStatus `json:"initialization,omitempty"`

	// addresses lists the addresses assigned to the VM, surfaced to the Machine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// failureDomain is the failure domain the VM was actually placed in, surfaced
	// to the Machine.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureDomain string `json:"failureDomain,omitempty"`

	// conditions represents the observations of the KubeSwiftMachine's state.
	// A condition of type "Ready" is mirrored into the Machine's InfrastructureReady
	// condition. Per the v1beta2 contract, terminal failures are surfaced via
	// conditions (there is no failureReason/failureMessage).
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=kubeswiftmachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Provisioned",type=boolean,JSONPath=".status.initialization.provisioned",description="VM is provisioned and running"
// +kubebuilder:printcolumn:name="Backend",type=string,JSONPath=".spec.backend.type",description="Machine backend"
// +kubebuilder:printcolumn:name="ProviderID",type=string,JSONPath=".spec.providerID",description="Provider ID"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// KubeSwiftMachine is the Schema for the kubeswiftmachines API. It is the
// infrastructure counterpart of a Cluster API Machine, backed by a KubeSwift VM.
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
