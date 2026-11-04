package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1alpha1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

// The CRD's CEL rules on spec.backend.swiftGuest reject a bad interface list at
// admission, for KubeSwiftMachines and KubeSwiftMachineTemplates alike. These specs
// run against envtest's real apiserver, which also proves the rules fit the CEL
// cost budget (the CRDs would fail to install otherwise).
var _ = Describe("SwiftGuest interfaces admission", func() {
	ctx := context.Background()

	backend := func(mutate func(*infrastructurev1alpha1.SwiftGuestBackend)) *infrastructurev1alpha1.SwiftGuestBackend {
		b := &infrastructurev1alpha1.SwiftGuestBackend{ImageRef: "ubuntu-noble", GuestClassRef: "capi-worker"}
		mutate(b)
		return b
	}
	machineSpec := func(b *infrastructurev1alpha1.SwiftGuestBackend) infrastructurev1alpha1.KubeSwiftMachineSpec {
		return infrastructurev1alpha1.KubeSwiftMachineSpec{
			Backend: infrastructurev1alpha1.MachineBackend{
				Type:       infrastructurev1alpha1.SwiftGuestBackendType,
				SwiftGuest: b,
			},
		}
	}

	n := 0
	createMachine := func(b *infrastructurev1alpha1.SwiftGuestBackend) error {
		n++
		m := &infrastructurev1alpha1.KubeSwiftMachine{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ifaces-%d", n), Namespace: "default"},
			Spec:       machineSpec(b),
		}
		err := k8sClient.Create(ctx, m)
		if err == nil {
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, m)).To(Succeed()) })
		}
		return err
	}
	createTemplate := func(b *infrastructurev1alpha1.SwiftGuestBackend) error {
		n++
		t := &infrastructurev1alpha1.KubeSwiftMachineTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ifaces-tmpl-%d", n), Namespace: "default"},
			Spec: infrastructurev1alpha1.KubeSwiftMachineTemplateSpec{
				Template: infrastructurev1alpha1.KubeSwiftMachineTemplateResource{
					// clusterv1.ObjectMeta requires at least one property when present.
					ObjectMeta: clusterv1.ObjectMeta{Labels: map[string]string{"test": "interfaces"}},
					Spec:       machineSpec(b),
				},
			},
		}
		err := k8sClient.Create(ctx, t)
		if err == nil {
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, t)).To(Succeed()) })
		}
		return err
	}

	expectRejected := func(err error, message string) {
		GinkgoHelper()
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "want an Invalid error, got %v", err)
		Expect(err.Error()).To(ContainSubstring(message))
	}

	DescribeTable("accepts",
		func(mutate func(*infrastructurev1alpha1.SwiftGuestBackend)) {
			Expect(createMachine(backend(mutate))).To(Succeed())
			Expect(createTemplate(backend(mutate))).To(Succeed())
		},
		Entry("no interfaces", func(*infrastructurev1alpha1.SwiftGuestBackend) {}),
		Entry("legacy networkRef", func(b *infrastructurev1alpha1.SwiftGuestBackend) { b.NetworkRef = "nad" }),
		Entry("legacy nodeNetworkRef", func(b *infrastructurev1alpha1.SwiftGuestBackend) { b.NodeNetworkRef = "nad" }),
		Entry("primary on a NAD with secondaries", func(b *infrastructurev1alpha1.SwiftGuestBackend) {
			b.Interfaces = []infrastructurev1alpha1.SwiftGuestInterface{
				{Name: "mgmt", Primary: true, NetworkRef: "mgmt-net"},
				{Name: "storage", NetworkRef: "storage-net"},
			}
		}),
		Entry("node-local primary with a secondary", func(b *infrastructurev1alpha1.SwiftGuestBackend) {
			b.Interfaces = []infrastructurev1alpha1.SwiftGuestInterface{
				{Name: "mgmt", Primary: true},
				{Name: "node", NetworkRef: "node-net"},
			}
		}),
	)

	DescribeTable("rejects",
		func(mutate func(*infrastructurev1alpha1.SwiftGuestBackend), message string) {
			expectRejected(createMachine(backend(mutate)), message)
			expectRejected(createTemplate(backend(mutate)), message)
		},
		Entry("interfaces with networkRef", func(b *infrastructurev1alpha1.SwiftGuestBackend) {
			b.NetworkRef = "nad"
			b.Interfaces = []infrastructurev1alpha1.SwiftGuestInterface{{Name: "mgmt", Primary: true, NetworkRef: "mgmt-net"}}
		}, "interfaces cannot be combined with networkRef or nodeNetworkRef"),
		Entry("interfaces with nodeNetworkRef", func(b *infrastructurev1alpha1.SwiftGuestBackend) {
			b.NodeNetworkRef = "nad"
			b.Interfaces = []infrastructurev1alpha1.SwiftGuestInterface{{Name: "mgmt", Primary: true}}
		}, "interfaces cannot be combined with networkRef or nodeNetworkRef"),
		Entry("no primary", func(b *infrastructurev1alpha1.SwiftGuestBackend) {
			b.Interfaces = []infrastructurev1alpha1.SwiftGuestInterface{{Name: "mgmt", NetworkRef: "mgmt-net"}}
		}, "exactly one interface must set primary"),
		Entry("two primaries", func(b *infrastructurev1alpha1.SwiftGuestBackend) {
			b.Interfaces = []infrastructurev1alpha1.SwiftGuestInterface{
				{Name: "mgmt", Primary: true, NetworkRef: "mgmt-net"},
				{Name: "storage", Primary: true, NetworkRef: "storage-net"},
			}
		}, "exactly one interface must set primary"),
		Entry("secondary without networkRef", func(b *infrastructurev1alpha1.SwiftGuestBackend) {
			b.Interfaces = []infrastructurev1alpha1.SwiftGuestInterface{
				{Name: "mgmt", Primary: true, NetworkRef: "mgmt-net"},
				{Name: "storage"},
			}
		}, "a secondary interface must set networkRef"),
		Entry("duplicate interface names", func(b *infrastructurev1alpha1.SwiftGuestBackend) {
			b.Interfaces = []infrastructurev1alpha1.SwiftGuestInterface{
				{Name: "mgmt", Primary: true, NetworkRef: "mgmt-net"},
				{Name: "mgmt", NetworkRef: "storage-net"},
			}
		}, "Duplicate value"),
	)
})
