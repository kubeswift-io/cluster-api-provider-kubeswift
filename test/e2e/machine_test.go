package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubeswift-io/cluster-api-provider-kubeswift/test/utils"
)

// The machine specs drive one worker Machine through the provider against real
// Cluster API controllers. KubeSwift's controller is not running (Kind cannot run its
// VMs), so the specs stand in for it: they write the SwiftGuest status KubeSwift would
// — waiting on a reference, then Running with a nat (Pod-scope) address — and check
// what the provider makes of it.
const (
	workloadNamespace = "capks-e2e-workload"
	clusterName       = "demo"
	machineName       = "demo-w0"
	bootstrapSecret   = "demo-w0-bootstrap"
	seedProfileName   = machineName + "-seed"

	// natPrimaryIP is the pod-private address every nat guest leases; podIP is where
	// the guest is actually reachable, and what the machine must report.
	natPrimaryIP = "192.168.99.10"
	launcherPod  = "10.244.0.99"
)

var workloadObjects = fmt.Sprintf(`
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: KubeSwiftCluster
    name: %[1]s
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: KubeSwiftCluster
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  controlPlaneEndpoint:
    host: 10.0.0.1
    port: 6443
---
apiVersion: v1
kind: Secret
metadata:
  name: %[4]s
  namespace: %[2]s
  labels:
    cluster.x-k8s.io/cluster-name: %[1]s
type: cluster.x-k8s.io/secret
stringData:
  format: cloud-config
  value: |
    #cloud-config
    # stands in for the kubeadm join cloud-init
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: KubeSwiftMachine
metadata:
  name: %[3]s
  namespace: %[2]s
  labels:
    cluster.x-k8s.io/cluster-name: %[1]s
spec:
  backend:
    type: SwiftGuest
    swiftGuest:
      imageRef: ubuntu-noble
      guestClassRef: capi-worker
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: Machine
metadata:
  name: %[3]s
  namespace: %[2]s
  labels:
    cluster.x-k8s.io/cluster-name: %[1]s
spec:
  clusterName: %[1]s
  bootstrap:
    dataSecretName: %[4]s
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: KubeSwiftMachine
    name: %[3]s
`, clusterName, workloadNamespace, machineName, bootstrapSecret)

var _ = Describe("KubeSwiftMachine", Ordered, func() {
	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	BeforeAll(func() {
		By("creating the workload namespace")
		_, err := utils.Run(exec.Command("kubectl", "create", "ns", workloadNamespace))
		Expect(err).NotTo(HaveOccurred())

		By("creating a Cluster, KubeSwiftCluster, bootstrap Secret, KubeSwiftMachine and Machine")
		Eventually(func() error {
			// Retried: Cluster API's and the provider's webhooks can take a moment to
			// answer after their Deployments report available.
			return kubectlApply(workloadObjects)
		}).Should(Succeed())
	})

	AfterAll(func() {
		By("deleting the workload cluster and namespace")
		_, _ = utils.Run(exec.Command("kubectl", "delete", "cluster", clusterName, "-n", workloadNamespace,
			"--ignore-not-found", "--timeout", "2m"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "ns", workloadNamespace,
			"--ignore-not-found", "--timeout", "2m"))
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		for _, kind := range []string{"cluster", "machine", "kubeswiftcluster", "kubeswiftmachine",
			"swiftguest", "swiftseedprofile"} {
			out, _ := utils.Run(exec.Command("kubectl", "get", kind, "-n", workloadNamespace, "-o", "yaml"))
			_, _ = fmt.Fprintf(GinkgoWriter, "--- %s:\n%s\n", kind, out)
		}
		logs, _ := utils.Run(exec.Command("kubectl", "logs", "deployment/"+controllerDeploymentName,
			"-n", namespace, "--tail", "200"))
		_, _ = fmt.Fprintf(GinkgoWriter, "--- provider logs:\n%s\n", logs)
	})

	It("creates a seed profile that references the bootstrap Secret, and the SwiftGuest", func() {
		Eventually(func(g Gomega) {
			g.Expect(jsonPath("swiftseedprofile", seedProfileName, "{.spec.userDataFrom.secretKeyRef.name}")).
				To(Equal(bootstrapSecret))
			g.Expect(jsonPath("swiftseedprofile", seedProfileName, "{.spec.userDataFrom.secretKeyRef.key}")).
				To(Equal("value"))
			// The bootstrap cloud-init (join token included) must not be copied in.
			g.Expect(jsonPath("swiftseedprofile", seedProfileName, "{.spec.userData}")).To(BeEmpty())
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(jsonPath("swiftguest", machineName, "{.spec.seedProfileRef.name}")).To(Equal(seedProfileName))
			g.Expect(jsonPath("swiftguest", machineName, "{.spec.imageRef.name}")).To(Equal("ubuntu-noble"))
			g.Expect(jsonPath("swiftguest", machineName,
				`{.metadata.labels.infrastructure\.cluster\.x-k8s\.io/kubeswiftmachine}`)).To(Equal(machineName))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(readyReason(g)).To(Equal("VMProvisioning"))
		}).Should(Succeed())
	})

	It("says why the VM is waiting when a reference is missing", func() {
		Expect(patchGuestStatus(`{"phase": "Pending", "conditions": [` +
			condition("Resolved", "False", "ImageNotFound", "SwiftImage not found: ubuntu-noble") + `]}`)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(readyReason(g)).To(Equal("VMWaitingForReferences"))
			g.Expect(jsonPath("kubeswiftmachine", machineName,
				`{.status.conditions[?(@.type=="Ready")].message}`)).To(ContainSubstring("SwiftImage not found: ubuntu-noble"))
		}).Should(Succeed())
	})

	It("provisions the machine at the launcher pod IP, not the nat guest's own address", func() {
		Expect(patchGuestStatus(fmt.Sprintf(`{"phase": "Running", "conditions": [%s],
			"network": {"primaryIP": %q, "primaryIPScope": "Pod", "podIP": %q}}`,
			condition("Resolved", "True", "Resolved", "all references resolved"), natPrimaryIP, launcherPod))).
			To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(jsonPath("kubeswiftmachine", machineName, "{.status.initialization.provisioned}")).To(Equal("true"))
			g.Expect(jsonPath("kubeswiftmachine", machineName, "{.status.addresses[*].address}")).To(Equal(launcherPod))
			g.Expect(jsonPath("kubeswiftmachine", machineName, "{.spec.providerID}")).
				To(Equal(fmt.Sprintf("kubeswift://%s/%s", workloadNamespace, machineName)))
			g.Expect(readyReason(g)).To(Equal("VMProvisioned"))
		}).Should(Succeed())

		By("checking Cluster API carried it onto the Machine")
		Eventually(func(g Gomega) {
			g.Expect(jsonPath("machine", machineName, "{.status.initialization.infrastructureProvisioned}")).
				To(Equal("true"))
			g.Expect(jsonPath("machine", machineName, "{.status.addresses[*].address}")).To(Equal(launcherPod))
		}).Should(Succeed())
	})

	It("deletes the SwiftGuest and seed profile with the Machine", func() {
		_, err := utils.Run(exec.Command("kubectl", "delete", "machine", machineName, "-n", workloadNamespace,
			"--wait=false"))
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			for _, kind := range []string{"swiftguest", "swiftseedprofile", "kubeswiftmachine", "machine"} {
				out, err := utils.Run(exec.Command("kubectl", "get", kind, "-n", workloadNamespace,
					"-o", "name"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(out)).To(BeEmpty(), "%s still present", kind)
			}
		}).Should(Succeed())
	})
})

// kubectlApply applies a multi-document manifest.
func kubectlApply(manifest string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	_, err := utils.Run(cmd)
	return err
}

// jsonPath reads one field of a workload-namespace object; "" when the object or the
// field is absent.
func jsonPath(kind, name, path string) string {
	out, err := utils.Run(exec.Command("kubectl", "get", kind, name, "-n", workloadNamespace,
		"-o", "jsonpath="+path))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// readyReason is the KubeSwiftMachine's Ready condition reason.
func readyReason(g Gomega) string {
	reason := jsonPath("kubeswiftmachine", machineName, `{.status.conditions[?(@.type=="Ready")].reason}`)
	g.Expect(reason).NotTo(BeEmpty(), "KubeSwiftMachine has no Ready condition yet")
	return reason
}

// patchGuestStatus writes SwiftGuest status the way KubeSwift's controller would.
func patchGuestStatus(status string) error {
	_, err := utils.Run(exec.Command("kubectl", "patch", "swiftguest", machineName, "-n", workloadNamespace,
		"--subresource=status", "--type=merge", "-p", `{"status": `+status+`}`))
	return err
}

// condition renders a metav1.Condition as JSON.
func condition(condType, status, reason, message string) string {
	return fmt.Sprintf(`{"type": %q, "status": %q, "reason": %q, "message": %q, "lastTransitionTime": %q}`,
		condType, status, reason, message, time.Now().UTC().Format(time.RFC3339))
}
