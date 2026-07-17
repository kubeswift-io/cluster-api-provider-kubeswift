package v1alpha1

import (
	"context"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/cluster-api/util/topology"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

// SetupKubeSwiftMachineTemplateWebhookWithManager registers the webhook for
// KubeSwiftMachineTemplate in the manager.
func SetupKubeSwiftMachineTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1alpha1.KubeSwiftMachineTemplate{}).
		WithValidator(&KubeSwiftMachineTemplateCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-kubeswiftmachinetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=kubeswiftmachinetemplates,verbs=create;update,versions=v1alpha1,name=vkubeswiftmachinetemplate-v1alpha1.kb.io,admissionReviewVersions=v1

// KubeSwiftMachineTemplateCustomValidator validates KubeSwiftMachineTemplate resources.
// It enforces that spec.template.spec is immutable — so a template change forces a
// rollout rather than mutating in place — except when the Cluster API topology
// controller performs a server-side-apply dry-run (how ClusterClass diffs templates),
// where the check is skipped.
type KubeSwiftMachineTemplateCustomValidator struct{}

// ValidateCreate implements the typed webhook validator.
func (v *KubeSwiftMachineTemplateCustomValidator) ValidateCreate(_ context.Context, _ *infrastructurev1alpha1.KubeSwiftMachineTemplate) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate enforces spec.template.spec immutability outside topology dry-runs.
func (v *KubeSwiftMachineTemplateCustomValidator) ValidateUpdate(ctx context.Context, oldTemplate, newTemplate *infrastructurev1alpha1.KubeSwiftMachineTemplate) (admission.Warnings, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an admission.Request in the context: %v", err))
	}

	if !topology.IsDryRunRequest(req, newTemplate) &&
		!reflect.DeepEqual(oldTemplate.Spec.Template.Spec, newTemplate.Spec.Template.Spec) {
		return nil, apierrors.NewInvalid(
			infrastructurev1alpha1.GroupVersion.WithKind("KubeSwiftMachineTemplate").GroupKind(),
			newTemplate.Name,
			field.ErrorList{
				field.Invalid(
					field.NewPath("spec", "template", "spec"),
					newTemplate.Spec.Template.Spec,
					"KubeSwiftMachineTemplate spec.template.spec is immutable",
				),
			},
		)
	}
	return nil, nil
}

// ValidateDelete implements the typed webhook validator.
func (v *KubeSwiftMachineTemplateCustomValidator) ValidateDelete(_ context.Context, _ *infrastructurev1alpha1.KubeSwiftMachineTemplate) (admission.Warnings, error) {
	return nil, nil
}
