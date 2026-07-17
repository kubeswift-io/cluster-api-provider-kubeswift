package v1alpha1

// APIEndpoint represents a reachable Kubernetes API server endpoint.
type APIEndpoint struct {
	// host is the DNS name or IP address on which the API server is serving.
	// +optional
	Host string `json:"host,omitempty"`

	// port is the port on which the API server is serving.
	// +optional
	Port int32 `json:"port,omitempty"`
}

// MachineAddressType describes the type of a machine address.
type MachineAddressType string

// Machine address types.
const (
	MachineHostName    MachineAddressType = "Hostname"
	MachineInternalIP  MachineAddressType = "InternalIP"
	MachineExternalIP  MachineAddressType = "ExternalIP"
	MachineInternalDNS MachineAddressType = "InternalDNS"
	MachineExternalDNS MachineAddressType = "ExternalDNS"
)

// MachineAddress is an address surfaced from the backing VM.
type MachineAddress struct {
	// type is the machine address type: Hostname, InternalIP, ExternalIP,
	// InternalDNS, or ExternalDNS.
	Type MachineAddressType `json:"type"`

	// address is the machine address value.
	Address string `json:"address"`
}

// ObjectMeta is a minimal metadata subset (labels + annotations) stamped onto
// resources created from a template. It mirrors the Cluster API ObjectMeta so
// ClusterClass and MachineDeployment templating behave as operators expect,
// without this module depending on the Cluster API Go module.
type ObjectMeta struct {
	// labels is a map of string keys and values to attach to the created object.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations is a map of string keys and values to attach to the created object.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Finalizers owned by this provider.
const (
	// ClusterFinalizer lets the controller clean up cluster infrastructure before
	// the KubeSwiftCluster is removed from the API server.
	ClusterFinalizer = "kubeswiftcluster.infrastructure.cluster.x-k8s.io"

	// MachineFinalizer lets the controller delete the backing SwiftGuest VM before
	// the KubeSwiftMachine is removed from the API server.
	MachineFinalizer = "kubeswiftmachine.infrastructure.cluster.x-k8s.io"
)

// ProviderIDScheme is the scheme used for KubeSwiftMachine provider IDs:
// "kubeswift://<guest-namespace>/<guest-name>".
const ProviderIDScheme = "kubeswift"
