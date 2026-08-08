package backend

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	infrav1 "github.com/kubeswift-io/cluster-api-provider-kubeswift/api/v1alpha1"
)

func TestRenderSwiftGuest_GPU_DRATemplate(t *testing.T) {
	req, cfg := testRenderRequest(nil, "")
	cfg.GPU = &infrav1.SwiftGuestGPU{
		ResourceClaimTemplateName: "single-vfio-gpu",
		Tier:                      infrav1.GPUTierPCIe,
		Hugepages:                 "1Gi",
	}
	g := renderSwiftGuest(req, cfg)

	claim, found, _ := unstructured.NestedMap(g.Object, "spec", "gpuResourceClaim")
	if !found {
		t.Fatal("spec.gpuResourceClaim not rendered")
	}
	if claim["resourceClaimTemplateName"] != "single-vfio-gpu" || claim["tier"] != infrav1.GPUTierPCIe ||
		claim["hugepages"] != "1Gi" {
		t.Fatalf("spec.gpuResourceClaim = %v", claim)
	}
	// Naming both backends is what KubeSwift itself rejects.
	if _, found, _ := unstructured.NestedMap(g.Object, "spec", "gpuProfileRef"); found {
		t.Fatal("spec.gpuProfileRef must not be set alongside gpuResourceClaim")
	}
}

func TestRenderSwiftGuest_GPU_NativeProfile(t *testing.T) {
	req, cfg := testRenderRequest(nil, "")
	cfg.GPU = &infrav1.SwiftGuestGPU{GPUProfileRef: "single-pcie-gpu"}
	g := renderSwiftGuest(req, cfg)

	if name, _, _ := unstructured.NestedString(g.Object, "spec", "gpuProfileRef", "name"); name != "single-pcie-gpu" {
		t.Fatalf("spec.gpuProfileRef.name = %q, want single-pcie-gpu", name)
	}
	if _, found, _ := unstructured.NestedMap(g.Object, "spec", "gpuResourceClaim"); found {
		t.Fatal("spec.gpuResourceClaim must not be set alongside gpuProfileRef")
	}
}

func TestRenderSwiftGuest_GPU_OmittedWhenUnset(t *testing.T) {
	req, cfg := testRenderRequest(nil, "")
	g := renderSwiftGuest(req, cfg)
	for _, f := range []string{"gpuResourceClaim", "gpuProfileRef"} {
		if _, found, _ := unstructured.NestedMap(g.Object, "spec", f); found {
			t.Fatalf("spec.%s set on a machine that asked for no GPU", f)
		}
	}
}

func TestValidateGPU(t *testing.T) {
	cases := map[string]struct {
		gpu     *infrav1.SwiftGuestGPU
		wantErr string
	}{
		"none":     {nil, ""},
		"template": {&infrav1.SwiftGuestGPU{ResourceClaimTemplateName: "t"}, ""},
		"claim":    {&infrav1.SwiftGuestGPU{ResourceClaimName: "c"}, ""},
		"profile":  {&infrav1.SwiftGuestGPU{GPUProfileRef: "p"}, ""},
		// A VFIO device backs exactly one running VM, and KubeSwift rejects a guest
		// naming two backends — so both, or neither, cannot mean anything.
		"two backends": {&infrav1.SwiftGuestGPU{ResourceClaimTemplateName: "t", GPUProfileRef: "p"},
			"exactly one"},
		"empty": {&infrav1.SwiftGuestGPU{Tier: infrav1.GPUTierPCIe}, "exactly one"},
		"hgx tier": {&infrav1.SwiftGuestGPU{ResourceClaimTemplateName: "t", Tier: "hgx-shared"},
			"only pcie is supported"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateGPU(c.gpu)
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
