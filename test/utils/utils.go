package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck,revive // ginkgo dot-import is idiomatic
)

const (
	prometheusOperatorVersion = "v0.77.1"
	prometheusOperatorURL     = "https://github.com/prometheus-operator/prometheus-operator/" +
		"releases/download/%s/bundle.yaml"

	// kubeSwiftCRDURLTmpl is a KubeSwift CRD at a release tag. KubeSwift's controller
	// is not installed — Kind cannot run its VMs — only the CRDs the provider creates
	// and watches.
	kubeSwiftCRDURLTmpl = "https://raw.githubusercontent.com/kubeswift-io/kubeswift/%s/config/crd/bases/%s.yaml"
)

// kubeSwiftCRDs are the KubeSwift CRDs the provider talks to.
var kubeSwiftCRDs = []string{
	"swift.kubeswift.io_swiftguests",
	"seed.kubeswift.io_swiftseedprofiles",
}

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %s\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed with error: (%v) %s", command, err, string(output))
	}

	return string(output), nil
}

// InstallPrometheusOperator installs the prometheus Operator to be used to export the enabled metrics.
func InstallPrometheusOperator() error {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "create", "-f", url)
	_, err := Run(cmd)
	return err
}

// UninstallPrometheusOperator uninstalls the prometheus
func UninstallPrometheusOperator() {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// IsPrometheusCRDsInstalled checks if any Prometheus CRDs are installed
// by verifying the existence of key CRDs related to Prometheus.
func IsPrometheusCRDsInstalled() bool {
	// List of common Prometheus CRDs
	prometheusCRDs := []string{
		"prometheuses.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
		"prometheusagents.monitoring.coreos.com",
	}

	cmd := exec.Command("kubectl", "get", "crds", "-o", "custom-columns=NAME:.metadata.name")
	output, err := Run(cmd)
	if err != nil {
		return false
	}
	crdList := GetNonEmptyLines(output)
	for _, crd := range prometheusCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// RequireKindContext fails unless kubectl's current context is the Kind cluster
// named cluster. The suite installs Cluster API and deploys and undeploys the
// provider, so pointed at a real management cluster it would replace what runs there.
func RequireKindContext(cluster string) error {
	out, err := Run(exec.Command("kubectl", "config", "current-context"))
	if err != nil {
		return err
	}
	if got, want := strings.TrimSpace(out), "kind-"+cluster; got != want {
		return fmt.Errorf("kubectl context is %q, want %q: run the suite with `make test-e2e`, "+
			"which points it at its own Kind cluster", got, want)
	}
	return nil
}

// InstallClusterAPI installs Cluster API — core, plus the kubeadm bootstrap and
// control-plane providers clusterctl adds by default — at version, waiting until they
// are available. clusterctl also installs cert-manager, and waits for its webhook to
// answer. A cluster that already has Cluster API is left as it is.
func InstallClusterAPI(clusterctl, version string) error {
	if _, err := Run(exec.Command("kubectl", "get", "crd", "clusters.cluster.x-k8s.io")); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Cluster API is already installed; skipping clusterctl init\n")
		return nil
	}
	cmd := exec.Command(clusterctl, "init",
		"--core", "cluster-api:"+version,
		"--bootstrap", "kubeadm:"+version,
		"--control-plane", "kubeadm:"+version,
		"--wait-providers",
	)
	_, err := Run(cmd)
	return err
}

// InstallKubeSwiftCRDs applies KubeSwift's SwiftGuest and SwiftSeedProfile CRDs at
// version and waits for them to be established. Server-side apply, because the
// SwiftGuest CRD is larger than the last-applied annotation client-side apply keeps.
func InstallKubeSwiftCRDs(version string) error {
	args := make([]string, 0, 3+2*len(kubeSwiftCRDs))
	args = append(args, "apply", "--server-side", "--force-conflicts")
	names := make([]string, 0, len(kubeSwiftCRDs))
	for _, crd := range kubeSwiftCRDs {
		args = append(args, "-f", fmt.Sprintf(kubeSwiftCRDURLTmpl, version, crd))
		group, plural, _ := strings.Cut(crd, "_")
		names = append(names, "crd/"+plural+"."+group)
	}
	if _, err := Run(exec.Command("kubectl", args...)); err != nil {
		return err
	}
	wait := append([]string{"wait", "--for", "condition=Established", "--timeout", "1m"}, names...)
	_, err := Run(exec.Command("kubectl", wait...))
	return err
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster
func LoadImageToKindClusterWithName(name string) error {
	cluster := "kind"
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	cmd := exec.Command("kind", kindOptions...)
	_, err := Run(cmd)
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %s to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		_, err := out.WriteString(strings.TrimPrefix(scanner.Text(), prefix))
		if err != nil {
			return err
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err := out.WriteString("\n"); err != nil {
			return err
		}
	}

	_, err = out.Write(content[idx+len(target):])
	if err != nil {
		return err
	}
	// false positive
	// nolint:gosec
	return os.WriteFile(filename, out.Bytes(), 0644)
}
