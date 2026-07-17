package backend

import (
	"strings"
	"testing"
)

func TestRenderBootstrapUserData(t *testing.T) {
	bootstrap := "#cloud-config\nruncmd:\n  - [ kubeadm, join ]\n"
	providerID := "kubeswift://ns/machine-0"

	out := renderBootstrapUserData([]byte(bootstrap), providerID)

	for _, want := range []string{
		`Content-Type: multipart/mixed; boundary="==kubeswift-capi=="`,
		"--provider-id=" + providerID,
		"20-kubeswift-provider-id.conf",
		"merge_how:",
		"runcmd:\n  - [ kubeadm, join ]", // bootstrap data included verbatim
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered user-data missing %q\n---\n%s", want, out)
		}
	}

	if got := strings.Count(out, "Content-Type: text/cloud-config"); got != 2 {
		t.Errorf("expected 2 cloud-config MIME parts, got %d", got)
	}
	if !strings.HasSuffix(out, "--==kubeswift-capi==--\n") {
		t.Errorf("expected a closing MIME boundary; got:\n%s", out)
	}
}
