// Package backend abstracts the KubeSwift resource that backs a KubeSwiftMachine.
//
// A KubeSwiftMachine's spec.backend selects a backend (today only SwiftGuest — see
// docs/spikes/01-sandbox-as-capi-node.md for why SwiftSandbox/SwiftSandboxPool are not
// node backends). Each backend creates, reconciles, and deletes its KubeSwift resource
// through the Kubernetes API — never by importing KubeSwift's Go packages, so this
// Apache-2.0 codebase stays clean against KubeSwift's AGPL-3.0 core — and maps the
// resource's state back onto the one Cluster API contract surface (providerID,
// initialization.provisioned, addresses).
package backend

import (
	"context"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

// Request carries everything a Backend needs to reconcile the resource backing a
// machine.
type Request struct {
	// Machine is the KubeSwiftMachine being reconciled.
	Machine *infrav1.KubeSwiftMachine
	// Cluster is the owning Cluster API Cluster.
	Cluster *clusterv1.Cluster
	// CAPIMachine is the owning Cluster API Machine.
	CAPIMachine *clusterv1.Machine
	// BootstrapData is the cloud-init produced by the bootstrap provider, read from
	// the Machine's bootstrap data secret.
	BootstrapData []byte
	// GuestNamespace is the namespace in which the backing KubeSwift resource is
	// created.
	GuestNamespace string
	// ProviderID is the provider ID the controller has assigned to this machine
	// (scheme "kubeswift://<namespace>/<name>"). The backend injects it into the
	// node via the bootstrap data so the Node registers with a matching ID.
	ProviderID string
}

// Result reports the outcome of a Backend reconcile.
type Result struct {
	// Provisioned is true once the backing resource is running and usable as a node.
	Provisioned bool
	// Addresses are the backing resource's node addresses.
	Addresses []clusterv1.MachineAddress
	// Requeue asks the caller to requeue, e.g. while waiting for the VM's IP.
	Requeue bool
}

// Backend reconciles the KubeSwift resource that backs a KubeSwiftMachine.
type Backend interface {
	// Type is the machine-backend discriminator this backend implements.
	Type() infrav1.MachineBackendType

	// Reconcile ensures the backing resource exists for the machine and reports its
	// provisioning state and addresses.
	Reconcile(ctx context.Context, req Request) (Result, error)

	// Delete tears down the backing resource. It returns done=false to signal the
	// caller to requeue until deletion completes.
	Delete(ctx context.Context, req Request) (done bool, err error)
}
