package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubeswift-io/cluster-api-provider-kubeswift/test/utils"
)

// Run the suite with `make test-e2e`, which creates the Kind cluster and sets these.
var (
	// kindCluster is the Kind cluster the suite runs in; the suite refuses any other
	// kubectl context.
	kindCluster = envOr("KIND_CLUSTER", "capks-test-e2e")
	// clusterctl is the clusterctl binary used to install Cluster API.
	clusterctl = envOr("CLUSTERCTL", "clusterctl")
	// capiVersion is the Cluster API release installed: the one this provider is
	// built against.
	capiVersion = os.Getenv("CAPI_VERSION")
	// kubeSwiftVersion is the KubeSwift release whose CRDs are installed.
	kubeSwiftVersion = envOr("KUBESWIFT_VERSION", "v0.22.0")

	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/cluster-api-provider-kubeswift:v0.0.1"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// TestE2E runs the end-to-end (e2e) test suite. It runs in a disposable Kind cluster
// with Cluster API and KubeSwift's CRDs installed (not KubeSwift's controller: Kind
// cannot run VMs, so the specs write SwiftGuest status the way KubeSwift would), builds
// and loads the manager image, and deploys the provider.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting cluster-api-provider-kubeswift integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("checking the suite is pointed at its own Kind cluster")
	Expect(utils.RequireKindContext(kindCluster)).To(Succeed())
	Expect(capiVersion).NotTo(BeEmpty(), "CAPI_VERSION is not set; run the suite with `make test-e2e`")

	By("building the manager(Operator) image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	By("installing Cluster API " + capiVersion + " (and cert-manager) with clusterctl")
	Expect(utils.InstallClusterAPI(clusterctl, capiVersion)).To(Succeed(), "Failed to install Cluster API")

	// Before the provider starts: it watches SwiftGuests, so without the CRD its
	// manager cannot start.
	By("installing the KubeSwift " + kubeSwiftVersion + " CRDs")
	Expect(utils.InstallKubeSwiftCRDs(kubeSwiftVersion)).To(Succeed(), "Failed to install the KubeSwift CRDs")

	By("creating manager namespace")
	cmd = exec.Command("kubectl", "create", "ns", namespace)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

	By("labeling the namespace to enforce the restricted security policy")
	cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
		"pod-security.kubernetes.io/enforce=restricted")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

	By("deploying the controller-manager")
	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

	By("waiting for the controller-manager to be available")
	cmd = exec.Command("kubectl", "rollout", "status", "deployment/"+controllerDeploymentName,
		"-n", namespace, "--timeout", "3m")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "The controller-manager did not become available")
})

// AfterSuite removes the provider; Cluster API, cert-manager and the KubeSwift CRDs go
// with the Kind cluster (`make cleanup-test-e2e`).
var _ = AfterSuite(func() {
	By("undeploying the controller-manager")
	_, _ = utils.Run(exec.Command("make", "undeploy", "ignore-not-found=true"))

	By("removing manager namespace")
	_, _ = utils.Run(exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found"))
})
