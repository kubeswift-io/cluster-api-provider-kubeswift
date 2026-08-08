package backend

import (
	"strings"
	"testing"

	infrav1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

func TestRenderSwiftGuest_NodeName(t *testing.T) {
	req, cfg := testRenderRequest(nil, "")
	cfg.NodeName = "boba"
	g := renderSwiftGuest(req, cfg)

	spec := g.Object["spec"].(map[string]interface{})
	if spec["nodeName"] != "boba" {
		t.Errorf("spec.nodeName = %v, want boba", spec["nodeName"])
	}
}

func TestRenderSwiftGuest_NodeNameOmittedWhenUnset(t *testing.T) {
	// An existing machine must render byte-identically to before this field existed.
	req, cfg := testRenderRequest(nil, "")
	g := renderSwiftGuest(req, cfg)

	spec := g.Object["spec"].(map[string]interface{})
	if _, set := spec["nodeName"]; set {
		t.Errorf("spec.nodeName present without being asked for: %v", spec["nodeName"])
	}
}

// The combination below fails SILENTLY without this guard: nodeName bypasses the
// scheduler, and a DRA claim is allocated by the scheduler, so the VM would come up
// with an unallocated claim and no device.
func TestValidatePlacement(t *testing.T) {
	cases := map[string]struct {
		cfg     infrav1.SwiftGuestBackend
		wantErr string
	}{
		"neither":       {infrav1.SwiftGuestBackend{}, ""},
		"node only":     {infrav1.SwiftGuestBackend{NodeName: "boba"}, ""},
		"gpu only":      {infrav1.SwiftGuestBackend{GPU: &infrav1.SwiftGuestGPU{ResourceClaimTemplateName: "t"}}, ""},
		"node plus DRA": {infrav1.SwiftGuestBackend{NodeName: "boba", GPU: &infrav1.SwiftGuestGPU{ResourceClaimTemplateName: "t"}}, "cannot be combined"},
		"node plus native": {infrav1.SwiftGuestBackend{NodeName: "boba",
			GPU: &infrav1.SwiftGuestGPU{GPUProfileRef: "p"}}, "cannot be combined"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := c.cfg
			err := validatePlacement(&cfg)
			switch {
			case c.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.wantErr != "" && err == nil:
				t.Fatalf("want error containing %q, got nil", c.wantErr)
			case c.wantErr != "" && !strings.Contains(err.Error(), c.wantErr):
				t.Fatalf("error %q does not contain %q", err, c.wantErr)
			}
		})
	}
}
