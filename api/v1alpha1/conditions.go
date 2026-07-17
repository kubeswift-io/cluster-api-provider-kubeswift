package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReadyConditionType is the condition that Cluster API mirrors into the owning
// Cluster's / Machine's InfrastructureReady condition. It surfaces the state of the
// infrastructure resource across its whole lifecycle (provisioning, ready, deletion).
const ReadyConditionType = "Ready"

// Condition reasons used by the KubeSwiftCluster and KubeSwiftMachine controllers.
const (
	// WaitingForControlPlaneEndpointReason: the cluster has no control-plane
	// endpoint yet (KubeSwift does not provision one; the operator or control-plane
	// provider must supply it).
	WaitingForControlPlaneEndpointReason = "WaitingForControlPlaneEndpoint"
	// ClusterProvisionedReason: cluster infrastructure is ready.
	ClusterProvisionedReason = "Provisioned"

	// WaitingForClusterInfrastructureReason: the owning Cluster's infrastructure is
	// not ready yet.
	WaitingForClusterInfrastructureReason = "WaitingForClusterInfrastructure"
	// WaitingForBootstrapDataReason: the Machine has no bootstrap data secret yet.
	WaitingForBootstrapDataReason = "WaitingForBootstrapData"
	// VMProvisioningReason: the backing VM is being created or is not yet running.
	VMProvisioningReason = "VMProvisioning"
	// VMProvisionedReason: the backing VM is running with an address.
	VMProvisionedReason = "VMProvisioned"
	// VMFailedReason: the backing VM reported a failed state.
	VMFailedReason = "VMFailed"
)

// GetConditions implements the Cluster API conditions.Getter interface.
func (c *KubeSwiftCluster) GetConditions() []metav1.Condition { return c.Status.Conditions }

// SetConditions implements the Cluster API conditions.Setter interface.
func (c *KubeSwiftCluster) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}

// GetConditions implements the Cluster API conditions.Getter interface.
func (m *KubeSwiftMachine) GetConditions() []metav1.Condition { return m.Status.Conditions }

// SetConditions implements the Cluster API conditions.Setter interface.
func (m *KubeSwiftMachine) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}
