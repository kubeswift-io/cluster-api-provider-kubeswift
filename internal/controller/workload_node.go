package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureNodeProviderID sets the workload Node's spec.providerID to the value this
// controller assigned, so the core Machine controller can bind the Machine to its Node.
//
// KubeSwift is agnostic to providerID and the kubelet registers the Node without one, so
// the provider sets it here by patching the Node directly — the reliable, standard
// pattern (the Cluster API Docker provider does the same). Patching is done over the
// workload kubeconfig the control-plane provider publishes as "<cluster>-kubeconfig".
//
// It returns patched=false (no error) when the control plane is not reachable yet or the
// Node has not registered, so the caller requeues. This only works with the provider
// running in the management cluster (it must reach the workload API endpoint).
func (r *KubeSwiftMachineReconciler) ensureNodeProviderID(
	ctx context.Context, cluster *clusterv1.Cluster, nodeName, providerID string,
) (bool, error) {
	log := logf.FromContext(ctx)

	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name + "-kubeconfig"}
	if err := r.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil // control plane not up yet; the kubeconfig secret is absent
		}
		return false, fmt.Errorf("reading workload kubeconfig secret %s: %w", key, err)
	}
	data, ok := secret.Data["value"]
	if !ok {
		return false, fmt.Errorf("workload kubeconfig secret %s has no 'value' key", key)
	}

	restCfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return false, fmt.Errorf("parsing workload kubeconfig %s: %w", key, err)
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return false, fmt.Errorf("building workload client: %w", err)
	}

	node, err := cs.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		// Node not registered yet, or the endpoint is briefly unreachable while the
		// control plane comes up — requeue quietly rather than error out.
		log.V(1).Info("workload Node not reachable yet; will retry", "node", nodeName, "err", err.Error())
		return false, nil
	}
	if node.Spec.ProviderID == providerID {
		return true, nil
	}
	if node.Spec.ProviderID != "" {
		// A different providerID is already set (immutable) — nothing to do.
		log.Info("workload Node already has a different providerID; leaving it",
			"node", nodeName, "existing", node.Spec.ProviderID, "wanted", providerID)
		return true, nil
	}

	patch := []byte(fmt.Sprintf(`{"spec":{"providerID":%q}}`, providerID))
	if _, err := cs.CoreV1().Nodes().Patch(ctx, nodeName, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return false, fmt.Errorf("patching Node %s providerID: %w", nodeName, err)
	}
	log.Info("patched workload Node providerID", "node", nodeName, "providerID", providerID)
	return true, nil
}
