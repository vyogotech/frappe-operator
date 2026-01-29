package e2e

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

var (
	cfg        *rest.Config
	testClient client.Client
	testEnv    *envtest.Environment
)

// gate E2E tests if etcd (envtest) binary is unavailable
var skipE2ETests bool

func init() {
	// Check for envtest binaries in multiple locations for better portability
	skipE2ETests = true

	// Priority 1: Check KUBEBUILDER_ASSETS environment variable
	if assets := os.Getenv("KUBEBUILDER_ASSETS"); assets != "" {
		etcdPath := filepath.Join(assets, "etcd")
		if _, err := os.Stat(etcdPath); err == nil {
			skipE2ETests = false
			return
		}
	}

	// Priority 2: Check common installation paths
	commonPaths := []string{
		"/usr/local/kubebuilder/bin/etcd",
		"/usr/bin/etcd",
		filepath.Join(os.Getenv("HOME"), "kubebuilder", "bin", "etcd"),
	}

	for _, path := range commonPaths {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			skipE2ETests = false
			return
		}
	}
}

func TestE2E(t *testing.T) {
	if skipE2ETests {
		t.Skip("Skipping E2E tests: envtest control plane not available")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{"../../config/crd/bases"},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    &[]bool{true}[0], // Use existing Kind cluster
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = vyogotechv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	testClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(testClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
