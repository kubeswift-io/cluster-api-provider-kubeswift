package backend

import (
	"context"
	"testing"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

func guestWithNetwork(network map[string]interface{}) *unstructured.Unstructured {
	g := newUnstructured(swiftGuestGVK)
	g.Object["status"] = map[string]interface{}{"phase": "Running", "network": network}
	return g
}

func TestGuestAddresses(t *testing.T) {
	cases := []struct {
		name    string
		network map[string]interface{}
		want    string // "" means no address
	}{
		{
			name:    "nat guest reports its launcher pod IP, not the pod-private primaryIP",
			network: map[string]interface{}{"primaryIP": "192.168.99.10", "primaryIPScope": "Pod", "podIP": "10.244.1.7"},
			want:    "10.244.1.7",
		},
		{
			name:    "nat guest without a pod IP yet has no address",
			network: map[string]interface{}{"primaryIP": "192.168.99.10", "primaryIPScope": "Pod"},
		},
		{
			name:    "Network-scope primaryIP is reachable and reported",
			network: map[string]interface{}{"primaryIP": "10.10.0.5", "primaryIPScope": "Network", "podIP": "10.244.1.7"},
			want:    "10.10.0.5",
		},
		{
			name:    "KubeSwift before v0.15.0 (no scope) keeps primaryIP",
			network: map[string]interface{}{"primaryIP": "192.168.99.10"},
			want:    "192.168.99.10",
		},
		{
			name:    "no primaryIP means the VM has no address yet, even with a pod IP",
			network: map[string]interface{}{"podIP": "10.244.1.7"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := guestAddresses(guestWithNetwork(tc.network))
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no address, got %v", got)
				}
				return
			}
			want := []clusterv1.MachineAddress{{Type: clusterv1.MachineInternalIP, Address: tc.want}}
			if !apiequality.Semantic.DeepEqual(got, want) {
				t.Fatalf("addresses = %v, want %v", got, want)
			}
		})
	}
}

func TestUnresolvedMessage(t *testing.T) {
	g := newUnstructured(swiftGuestGVK)
	g.Object["status"] = map[string]interface{}{
		"phase": "Pending",
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "False", "reason": "NotRunning"},
			map[string]interface{}{"type": resolvedCondition, "status": "False", "reason": "ImageNotReady",
				"message": "SwiftImage not found: ubuntu-nobel"},
		},
	}
	msg, waiting := unresolvedMessage(g)
	if !waiting || msg != "SwiftImage not found: ubuntu-nobel" {
		t.Fatalf("unresolvedMessage = %q, %v; want the Resolved message, true", msg, waiting)
	}

	g.Object["status"].(map[string]interface{})["conditions"] = []interface{}{
		map[string]interface{}{"type": resolvedCondition, "status": "True"},
	}
	if _, waiting := unresolvedMessage(g); waiting {
		t.Fatal("a Resolved=True guest is not waiting for references")
	}
}

func seedRequest() Request {
	return Request{
		Machine:                 &infrav1.KubeSwiftMachine{ObjectMeta: metav1.ObjectMeta{Name: "w-0", Namespace: "ns"}},
		Cluster:                 &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "demo"}},
		GuestNamespace:          "ns",
		BootstrapDataSecretName: "w-0-bootstrap",
	}
}

func getSeedSpec(t *testing.T, b *SwiftGuestBackend) map[string]interface{} {
	t.Helper()
	seed := newUnstructured(swiftSeedProfileGVK)
	if err := b.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "w-0-seed"}, seed); err != nil {
		t.Fatalf("getting seed profile: %v", err)
	}
	spec, _, _ := unstructured.NestedMap(seed.Object, "spec")
	return spec
}

func TestEnsureSeedProfile_ReferencesBootstrapSecret(t *testing.T) {
	b := &SwiftGuestBackend{Client: fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()}
	if err := b.ensureSeedProfile(context.Background(), seedRequest()); err != nil {
		t.Fatalf("ensureSeedProfile: %v", err)
	}
	spec := getSeedSpec(t, b)
	if _, ok := spec["userData"]; ok {
		t.Fatal("the bootstrap cloud-init must not be inlined as spec.userData")
	}
	name, _, _ := unstructured.NestedString(spec, "userDataFrom", "secretKeyRef", "name")
	key, _, _ := unstructured.NestedString(spec, "userDataFrom", "secretKeyRef", "key")
	if name != "w-0-bootstrap" || key != BootstrapDataSecretKey {
		t.Fatalf("userDataFrom.secretKeyRef = %s/%s, want w-0-bootstrap/%s", name, key, BootstrapDataSecretKey)
	}
}

func TestEnsureSeedProfile_MigratesInlineUserData(t *testing.T) {
	legacy := newUnstructured(swiftSeedProfileGVK)
	legacy.SetNamespace("ns")
	legacy.SetName("w-0-seed")
	legacy.Object["spec"] = map[string]interface{}{
		"datasource": "NoCloud",
		"userData":   "#cloud-config\n# kubeadm join token here\n",
		"metaData":   "instance-id: w-0\nlocal-hostname: w-0\n",
	}
	b := &SwiftGuestBackend{Client: fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(legacy).Build()}

	if err := b.ensureSeedProfile(context.Background(), seedRequest()); err != nil {
		t.Fatalf("ensureSeedProfile: %v", err)
	}
	spec := getSeedSpec(t, b)
	if _, ok := spec["userData"]; ok {
		t.Fatal("inline userData should be removed from a pre-existing profile")
	}
	if name, _, _ := unstructured.NestedString(spec, "userDataFrom", "secretKeyRef", "name"); name != "w-0-bootstrap" {
		t.Fatalf("userDataFrom.secretKeyRef.name = %q, want w-0-bootstrap", name)
	}
	if md, _, _ := unstructured.NestedString(spec, "metaData"); md != "instance-id: w-0\nlocal-hostname: w-0\n" {
		t.Fatalf("metaData changed to %q", md)
	}
}
