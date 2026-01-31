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
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
)

func TestSiteBackupReconciler_getBenchImage(t *testing.T) {
	r := &SiteBackupReconciler{}
	t.Run("ImageConfig override", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			Spec: vyogotechv1alpha1.FrappeBenchSpec{
				FrappeVersion: "15",
				ImageConfig: &vyogotechv1alpha1.ImageConfig{
					Repository: "myreg/erpnext",
					Tag:        "v15",
				},
			},
		}
		img := r.getBenchImage(bench)
		if img != "myreg/erpnext:v15" {
			t.Errorf("expected myreg/erpnext:v15, got %s", img)
		}
	})
	t.Run("Default with version", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			Spec: vyogotechv1alpha1.FrappeBenchSpec{FrappeVersion: "15"},
		}
		img := r.getBenchImage(bench)
		if img != "frappe/erpnext:15" {
			t.Errorf("expected frappe/erpnext:15, got %s", img)
		}
	})
}

func TestSiteBackupReconciler_getSitesPVCName(t *testing.T) {
	r := &SiteBackupReconciler{}
	bench := &vyogotechv1alpha1.FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: "my-bench"}}
	name := r.getSitesPVCName(bench)
	if name != "my-bench-sites" {
		t.Errorf("expected my-bench-sites, got %s", name)
	}
}

func TestSiteBackupReconciler_buildBackupArgs(t *testing.T) {
	r := &SiteBackupReconciler{}
	t.Run("minimal", func(t *testing.T) {
		sb := &vyogotechv1alpha1.SiteBackup{Spec: vyogotechv1alpha1.SiteBackupSpec{Site: "site1.local"}}
		args := r.buildBackupArgs(sb)
		if len(args) < 3 {
			t.Fatalf("expected at least --site site1.local backup, got %v", args)
		}
		if args[0] != "--site" || args[1] != "site1.local" || args[2] != "backup" {
			t.Errorf("expected --site site1.local backup, got %v", args)
		}
	})
	t.Run("with options", func(t *testing.T) {
		withFiles := true
		sb := &vyogotechv1alpha1.SiteBackup{
			Spec: vyogotechv1alpha1.SiteBackupSpec{
				Site:       "site2.local",
				WithFiles:  withFiles,
				Compress:   true,
				BackupPath: "/backups",
			},
		}
		args := r.buildBackupArgs(sb)
		foundWithFiles := false
		foundCompress := false
		for i, a := range args {
			if a == "--with-files" {
				foundWithFiles = true
			}
			if a == "--compress" {
				foundCompress = true
			}
			if a == "--backup-path" && i+1 < len(args) && args[i+1] == "/backups" {
				break
			}
		}
		if !foundWithFiles {
			t.Error("expected --with-files in args")
		}
		if !foundCompress {
			t.Error("expected --compress in args")
		}
	})
}

func TestSiteBackupReconciler_buildBackupJob(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	r := &SiteBackupReconciler{Scheme: scheme}
	siteBackup := &vyogotechv1alpha1.SiteBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "my-backup", Namespace: "default"},
		Spec:       vyogotechv1alpha1.SiteBackupSpec{Site: "site.local"},
	}
	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "default"},
		Spec:       vyogotechv1alpha1.FrappeBenchSpec{FrappeVersion: "15"},
	}
	job := r.buildBackupJob(siteBackup, bench)
	if job.Name != "my-backup-backup" || job.Namespace != "default" {
		t.Errorf("job name/ns: got %s/%s", job.Name, job.Namespace)
	}
	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatal("expected 1 container")
	}
	if job.Spec.Template.Spec.Containers[0].Command[0] != "bench" {
		t.Error("expected command bench")
	}
	if job.Spec.TTLSecondsAfterFinished == nil {
		t.Error("expected TTL on job")
	}
	if job.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "bench-sites" {
		t.Errorf("expected PVC bench-sites, got %s", job.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName)
	}
}

func TestSiteBackupReconciler_updateSiteBackupStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = vyogotechv1alpha1.AddToScheme(scheme)
	siteBackup := &vyogotechv1alpha1.SiteBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "sb", Namespace: "default"},
		Spec:       vyogotechv1alpha1.SiteBackupSpec{Site: "site.local"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteBackup).WithStatusSubresource(&vyogotechv1alpha1.SiteBackup{}).Build()
	r := &SiteBackupReconciler{Client: client}
	ctx := context.Background()
	err := r.updateSiteBackupStatus(ctx, siteBackup, "Running", "Backup in progress", "sb-backup")
	if err != nil {
		t.Fatalf("updateSiteBackupStatus: %v", err)
	}
	updated := &vyogotechv1alpha1.SiteBackup{}
	if err := client.Get(ctx, types.NamespacedName{Name: "sb", Namespace: "default"}, updated); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Status.Phase != "Running" || updated.Status.Message != "Backup in progress" || updated.Status.LastBackupJob != "sb-backup" {
		t.Errorf("status not updated: %+v", updated.Status)
	}
}

var _ = Describe("SiteBackup Controller", func() {
	var (
		ctx        context.Context
		reconciler *SiteBackupReconciler
		siteBackup *vyogotechv1alpha1.SiteBackup
		site       *vyogotechv1alpha1.FrappeSite
		bench      *vyogotechv1alpha1.FrappeBench
	)

	BeforeEach(func() {
		ctx = context.Background()

		bench = &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-bench",
				Namespace: "default",
			},
			Spec: vyogotechv1alpha1.FrappeBenchSpec{
				FrappeVersion: "15",
			},
		}

		site = &vyogotechv1alpha1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-site",
				Namespace: "default",
			},
			Spec: vyogotechv1alpha1.FrappeSiteSpec{
				SiteName: "test-site.local",
				BenchRef: &vyogotechv1alpha1.NamespacedName{
					Name:      bench.Name,
					Namespace: bench.Namespace,
				},
			},
		}

		siteBackup = &vyogotechv1alpha1.SiteBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-backup",
				Namespace: "default",
			},
			Spec: vyogotechv1alpha1.SiteBackupSpec{
				Site: "test-site.local",
			},
		}

		reconciler = &SiteBackupReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		// Create bench and site
		Expect(k8sClient.Create(ctx, bench)).To(Succeed())
		Expect(k8sClient.Create(ctx, site)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up
		k8sClient.Delete(ctx, siteBackup)
		k8sClient.Delete(ctx, site)
		k8sClient.Delete(ctx, bench)
	})

	Context("One-time backup", func() {
		BeforeEach(func() {
			siteBackup.Spec.Schedule = ""
		})

		It("should create a Job for one-time backup", func() {
			Expect(k8sClient.Create(ctx, siteBackup)).To(Succeed())

			req := ctrl.Request{}
			req.Namespace = siteBackup.Namespace
			req.Name = siteBackup.Name
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			job := &batchv1.Job{}
			jobKey := ctrl.Request{}
			jobKey.Namespace = siteBackup.Namespace
			jobKey.Name = siteBackup.Name + "-backup"
			Eventually(func() error {
				return k8sClient.Get(ctx, jobKey.NamespacedName, job)
			}, "10s", "1s").Should(Succeed())

			Expect(job.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"bench"}))
			Expect(job.Spec.Template.Spec.Containers[0].Args).To(ContainElements("--site", "test-site.local", "backup"))
			Expect(job.Spec.TTLSecondsAfterFinished).NotTo(BeNil())
			Expect(*job.Spec.TTLSecondsAfterFinished).To(Equal(resources.DefaultJobTTL))
		})
	})
})
