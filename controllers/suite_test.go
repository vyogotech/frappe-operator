/*
Copyright 2023 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

// gate controller tests if etcd (envtest) binary is unavailable
var skipControllerTests bool

func init() {
	// Check for envtest binaries in multiple locations for better portability
	skipControllerTests = true

	// Priority 1: Check KUBEBUILDER_ASSETS environment variable
	if assets := os.Getenv("KUBEBUILDER_ASSETS"); assets != "" {
		etcdPath := filepath.Join(assets, "etcd")
		if _, err := os.Stat(etcdPath); err == nil {
			skipControllerTests = false
			return
		}
	}

	// Priority 2: Check project-local bin directory (standard for this project)
	cwd, _ := os.Getwd()
	// Navigate up from controllers if necessary, or check relative to root
	// In suite_test.go, Getwd() is normally the directory containing the file
	binPath := filepath.Join(cwd, "..", "bin", "k8s")
	
	// Try to find any etcd in bin/k8s subdirectories
	filepath.Walk(binPath, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Name() == "etcd" && !info.IsDir() {
			os.Setenv("KUBEBUILDER_ASSETS", filepath.Dir(path))
			skipControllerTests = false
			return filepath.SkipAll
		}
		return nil
	})
	if !skipControllerTests {
		return
	}

	// Priority 3: Check common installation paths
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
			skipControllerTests = false
			return
		}
	}
}

func TestAPIs(t *testing.T) {
	if skipControllerTests {
		t.Skip("Skipping controller tests: envtest control plane not available")
	}
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = vyogotechv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
