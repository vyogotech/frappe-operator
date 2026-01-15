package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

var _ = Describe("SiteBackup E2E", func() {
	ctx := context.Background()
	var k8sClient client.Client

	BeforeEach(func() {
		// Get the k8s client from the test environment
		k8sClient = testClient
	})

	Context("Basic SiteBackup functionality", func() {
		It("should create and manage a SiteBackup resource", func() {
			// Create a basic FrappeBench first
			bench := &vyogotechv1alpha1.FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-bench",
					Namespace: "default",
				},
				Spec: vyogotechv1alpha1.FrappeBenchSpec{
					FrappeVersion: "version-15",
				},
			}

			Expect(k8sClient.Create(ctx, bench)).To(Succeed())

			// Create a basic FrappeSite
			site := &vyogotechv1alpha1.FrappeSite{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-site",
					Namespace: "default",
				},
				Spec: vyogotechv1alpha1.FrappeSiteSpec{
					SiteName: "e2e.example.com",
					BenchRef: &vyogotechv1alpha1.NamespacedName{
						Name: bench.Name,
					},
				},
			}

			Expect(k8sClient.Create(ctx, site)).To(Succeed())

			// Create a SiteBackup
			backup := &vyogotechv1alpha1.SiteBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-backup",
					Namespace: "default",
				},
				Spec: vyogotechv1alpha1.SiteBackupSpec{
					Site:      "e2e.example.com",
					WithFiles: true,
					Compress:  true,
				},
			}

			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			// Wait for backup job to be created
			Eventually(func() error {
				job := &batchv1.Job{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-backup-backup",
					Namespace: "default",
				}, job)
			}, time.Minute*2, time.Second*5).Should(Succeed())

			// Verify backup job was created with correct arguments
			job := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "e2e-backup-backup",
				Namespace: "default",
			}, job)).To(Succeed())

			// Check that the job has the expected command
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.Command).To(ContainElement("sh"))

			// The args should contain bench backup command with options
			argsStr := fmt.Sprintf("%v", container.Args)
			Expect(argsStr).To(ContainSubstring("bench"))
			Expect(argsStr).To(ContainSubstring("--site e2e.example.com"))
			Expect(argsStr).To(ContainSubstring("--with-files"))
			Expect(argsStr).To(ContainSubstring("--compress"))

			// Clean up
			k8sClient.Delete(ctx, backup)
			k8sClient.Delete(ctx, site)
			k8sClient.Delete(ctx, bench)
		})

		It("should create scheduled backups", func() {
			// Create bench and site first
			bench := &vyogotechv1alpha1.FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-scheduled-bench",
					Namespace: "default",
				},
				Spec: vyogotechv1alpha1.FrappeBenchSpec{
					FrappeVersion: "version-15",
				},
			}
			Expect(k8sClient.Create(ctx, bench)).To(Succeed())

			site := &vyogotechv1alpha1.FrappeSite{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-scheduled-site",
					Namespace: "default",
				},
				Spec: vyogotechv1alpha1.FrappeSiteSpec{
					SiteName: "scheduled.example.com",
					BenchRef: &vyogotechv1alpha1.NamespacedName{
						Name: bench.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, site)).To(Succeed())

			// Create scheduled backup
			backup := &vyogotechv1alpha1.SiteBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-scheduled-backup",
					Namespace: "default",
				},
				Spec: vyogotechv1alpha1.SiteBackupSpec{
					Site:     "scheduled.example.com",
					Schedule: "0 2 * * *", // Daily at 2 AM
					Compress: true,
				},
			}

			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			// Wait for CronJob to be created
			Eventually(func() error {
				cronJob := &batchv1.CronJob{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-scheduled-backup-backup",
					Namespace: "default",
				}, cronJob)
			}, time.Minute*2, time.Second*5).Should(Succeed())

			// Verify CronJob was created with correct schedule
			cronJob := &batchv1.CronJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "e2e-scheduled-backup-backup",
				Namespace: "default",
			}, cronJob)).To(Succeed())

			Expect(cronJob.Spec.Schedule).To(Equal("0 2 * * *"))

			// Clean up
			k8sClient.Delete(ctx, backup)
			k8sClient.Delete(ctx, site)
			k8sClient.Delete(ctx, bench)
		})
	})
})
