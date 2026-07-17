package v1alpha1

// InitializationStatus reports Cluster API contract initialization state for a
// KubeSwift infrastructure resource. Per the v1beta2 contract, the "core"
// controllers watch status.initialization.provisioned to orchestrate initial
// provisioning (it replaces the deprecated status.ready boolean).
type InitializationStatus struct {
	// provisioned is true when the infrastructure provider reports that the
	// resource's infrastructure is fully provisioned.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
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
