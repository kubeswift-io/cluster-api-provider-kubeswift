package backend

import (
	"fmt"
	"strings"
)

// renderBootstrapUserData combines the Cluster API bootstrap cloud-init with a small
// cloud-config that sets the kubelet --provider-id, as a cloud-init multipart MIME
// document. Both parts are text/cloud-config; the provider-id part carries a merge_how
// directive so cloud-init appends (rather than replaces) the bootstrap's write_files
// list. The result makes the node's kubelet register with the provider ID this
// controller assigned, which the Cluster API Machine controller requires to bind the
// Machine to its Node.
//
// NOTE: the provider-id drop-in is validated end-to-end only on a real Cluster API +
// KubeSwift management cluster (see docs/design/capi-kubeswift-architecture.md, Risks).
// This rendering is unit-tested; assumes the bootstrap data is cloud-config (the
// kubeadm default).
func renderBootstrapUserData(bootstrap []byte, providerID string) string {
	const boundary = "==kubeswift-capi=="

	providerIDPart := fmt.Sprintf(`#cloud-config
merge_how:
  - name: list
    settings: [append]
  - name: dict
    settings: [no_replace, recurse_list]
write_files:
  - path: /etc/systemd/system/kubelet.service.d/20-kubeswift-provider-id.conf
    owner: root:root
    permissions: "0644"
    content: |
      [Service]
      Environment="KUBELET_EXTRA_ARGS=--provider-id=%s"
`, providerID)

	var b strings.Builder
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%q\n", boundary)
	b.WriteString("MIME-Version: 1.0\n\n")

	writePart := func(body string) {
		b.WriteString("--" + boundary + "\n")
		b.WriteString("Content-Type: text/cloud-config; charset=\"us-ascii\"\n")
		b.WriteString("MIME-Version: 1.0\n\n")
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}

	writePart(providerIDPart)
	writePart(string(bootstrap))
	b.WriteString("--" + boundary + "--\n")
	return b.String()
}
