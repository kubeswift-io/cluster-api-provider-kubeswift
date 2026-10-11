package controller

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kubeswift-io/cluster-api-provider-kubeswift/internal/backend"
)

func TestSwiftGuestToKubeSwiftMachine(t *testing.T) {
	g := backend.NewSwiftGuestObject()
	g.SetNamespace("ns")
	g.SetName("w-0")
	g.SetLabels(map[string]string{backend.MachineLabelKey: "w-0"})

	reqs := swiftGuestToKubeSwiftMachine(context.Background(), g)
	if len(reqs) != 1 || reqs[0].NamespacedName != (types.NamespacedName{Namespace: "ns", Name: "w-0"}) {
		t.Fatalf("requests = %v, want ns/w-0", reqs)
	}

	g.SetLabels(nil)
	if reqs := swiftGuestToKubeSwiftMachine(context.Background(), g); len(reqs) != 0 {
		t.Fatalf("a SwiftGuest without the machine label should map to nothing, got %v", reqs)
	}
}
