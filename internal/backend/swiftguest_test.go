package backend

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

func testRenderRequest(exposure *ControlPlaneExposure, networkRef string) (Request, *infrav1.SwiftGuestBackend) {
	req := Request{
		Machine:              &infrav1.KubeSwiftMachine{ObjectMeta: metav1.ObjectMeta{Name: "cp-0", Namespace: "ns"}},
		Cluster:              &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "demo"}},
		GuestNamespace:       "ns",
		ControlPlaneExposure: exposure,
	}
	cfg := &infrav1.SwiftGuestBackend{ImageRef: "img", GuestClassRef: "class", NetworkRef: networkRef}
	return req, cfg
}

func TestRenderSwiftGuest_ControlPlaneExposure(t *testing.T) {
	req, cfg := testRenderRequest(&ControlPlaneExposure{PoolLabel: "demo-cp", Port: 6443}, "")
	g := renderSwiftGuest(req, cfg)

	if got := g.GetLabels()[infrav1.ControlPlanePoolLabelKey]; got != "demo-cp" {
		t.Fatalf("pool label = %q, want demo-cp", got)
	}
	if binding, _, _ := unstructured.NestedString(g.Object, "spec", "network", "binding"); binding != "nat" {
		t.Fatalf("network.binding = %q, want nat", binding)
	}
	ports, _, _ := unstructured.NestedSlice(g.Object, "spec", "network", "ports")
	if len(ports) != 1 {
		t.Fatalf("want 1 exposed port, got %d", len(ports))
	}
	p, ok := ports[0].(map[string]interface{})
	if !ok {
		t.Fatalf("port entry is %T, want map", ports[0])
	}
	if p["port"] != int64(6443) || p["targetPort"] != int64(6443) {
		t.Fatalf("port/targetPort = %v/%v, want 6443/6443", p["port"], p["targetPort"])
	}
}

// TestReconcile_ExposureWithNetworkRefIsRejected checks the guard fires before any
// client use, so a nil-client backend is fine here.
func TestReconcile_ExposureWithNetworkRefIsRejected(t *testing.T) {
	b := &SwiftGuestBackend{}
	req := Request{
		Machine: &infrav1.KubeSwiftMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-0", Namespace: "ns"},
			Spec: infrav1.KubeSwiftMachineSpec{
				Backend: infrav1.MachineBackend{
					Type:       infrav1.SwiftGuestBackendType,
					SwiftGuest: &infrav1.SwiftGuestBackend{ImageRef: "img", GuestClassRef: "class", NetworkRef: "nad"},
				},
			},
		},
		ControlPlaneExposure: &ControlPlaneExposure{PoolLabel: "demo-cp", Port: 6443},
		GuestNamespace:       "ns",
	}
	if _, err := b.Reconcile(context.Background(), req); err == nil {
		t.Fatal("expected an error when control-plane exposure is combined with a networkRef")
	}
}

func TestRenderSwiftGuest_NoExposure(t *testing.T) {
	req, cfg := testRenderRequest(nil, "")
	g := renderSwiftGuest(req, cfg)

	if _, found, _ := unstructured.NestedMap(g.Object, "spec", "network"); found {
		t.Fatal("spec.network should be absent without control-plane exposure")
	}
	if _, ok := g.GetLabels()[infrav1.ControlPlanePoolLabelKey]; ok {
		t.Fatal("pool label should be absent without control-plane exposure")
	}
}

// TestRenderSwiftGuest_NodeNetworkRef verifies the multi-node shape: a nat primary
// (management + the reachable endpoint) plus a secondary routable node-datapath
// interface, composing with the Service-backed nat endpoint (no rejection).
func TestRenderSwiftGuest_NodeNetworkRef(t *testing.T) {
	req, cfg := testRenderRequest(&ControlPlaneExposure{PoolLabel: "demo-cp", Port: 6443}, "")
	cfg.NodeNetworkRef = "sec-net"
	g := renderSwiftGuest(req, cfg)

	ifaces, _, _ := unstructured.NestedSlice(g.Object, "spec", "interfaces")
	if len(ifaces) != 2 {
		t.Fatalf("want 2 interfaces (nat primary + secondary node network), got %d", len(ifaces))
	}
	primary, _ := ifaces[0].(map[string]interface{})
	if _, hasRef := primary["networkRef"]; hasRef {
		t.Fatal("the primary interface must be node-local nat (no networkRef)")
	}
	secondary, _ := ifaces[1].(map[string]interface{})
	ref, _ := secondary["networkRef"].(map[string]interface{})
	if ref["name"] != "sec-net" {
		t.Fatalf("secondary interface networkRef = %v, want sec-net", ref["name"])
	}
	// The nat endpoint still applies (node-network is a secondary, not the primary).
	if binding, _, _ := unstructured.NestedString(g.Object, "spec", "network", "binding"); binding != "nat" {
		t.Fatalf("network.binding = %q, want nat (endpoint coexists with the node network)", binding)
	}
}

// TestRenderSwiftGuest_StorageClassName verifies the root-disk StorageClass override
// lands on spec.storage.storageClassName, and that leaving it empty omits the block
// (so KubeSwift inherits the source SwiftImage's class).
func TestRenderSwiftGuest_StorageClassName(t *testing.T) {
	req, cfg := testRenderRequest(nil, "")
	cfg.StorageClassName = "longhorn-r1"
	g := renderSwiftGuest(req, cfg)
	if sc, _, _ := unstructured.NestedString(g.Object, "spec", "storage", "storageClassName"); sc != "longhorn-r1" {
		t.Fatalf("spec.storage.storageClassName = %q, want longhorn-r1", sc)
	}
	// SwiftGuest validation requires accessMode present whenever spec.storage is set.
	if am, _, _ := unstructured.NestedString(g.Object, "spec", "storage", "accessMode"); am != "ReadWriteOnce" {
		t.Fatalf("spec.storage.accessMode = %q, want ReadWriteOnce", am)
	}

	req, cfg = testRenderRequest(nil, "")
	g = renderSwiftGuest(req, cfg)
	if _, found, _ := unstructured.NestedMap(g.Object, "spec", "storage"); found {
		t.Fatal("spec.storage must be omitted when storageClassName is empty (inherit image class)")
	}
}
