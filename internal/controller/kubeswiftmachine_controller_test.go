package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrastructurev1alpha1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

var _ = Describe("KubeSwiftMachine Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		kubeswiftmachine := &infrastructurev1alpha1.KubeSwiftMachine{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind KubeSwiftMachine")
			err := k8sClient.Get(ctx, typeNamespacedName, kubeswiftmachine)
			if err != nil && errors.IsNotFound(err) {
				resource := &infrastructurev1alpha1.KubeSwiftMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: infrastructurev1alpha1.KubeSwiftMachineSpec{
						Backend: infrastructurev1alpha1.MachineBackend{
							Type: infrastructurev1alpha1.SwiftGuestBackendType,
							SwiftGuest: &infrastructurev1alpha1.SwiftGuestBackend{
								ImageRef:      "ubuntu-noble",
								GuestClassRef: "capi-worker",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &infrastructurev1alpha1.KubeSwiftMachine{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance KubeSwiftMachine")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &KubeSwiftMachineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
