# Spikes

Time-boxed investigations that de-risk a design decision before implementation.
Unlike the KubeSwift core repo, spike write-ups **are committed here** (see
`CLAUDE.md`). One file per spike: the question, what was tried, the result, and the
go/no-go it informs.

Naming: `NN-short-slug.md` (e.g. `01-providerid-node-match.md`).

Candidate spikes for the initial provider (see
`../design/capi-kubeswift-architecture.md`, Risks):

- providerID → Node match: confirm kubelet `--provider-id` injection via the Kubeadm
  bootstrap data makes the CAPI Machine bind to the Node.
- Control-plane endpoint: kube-vip inside a SwiftGuest control-plane node vs an
  operator-provided VIP.
- SwiftGuest status read-back: the minimal JSON paths the provider needs
  (`status.phase`, `status.network.primaryIP`) as an unstructured read.
