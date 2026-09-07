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
	"strings"
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

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
)

func TestSiteBackupReconciler_getBenchImage(t *testing.T) {
	r := &SiteBackupReconciler{}
	t.Run("ImageConfig override", func(t *testing.T) {
		bench := &vyogotechv1.FrappeBench{
			Spec: vyogotechv1.FrappeBenchSpec{
				FrappeVersion: "15",
				ImageConfig: &vyogotechv1.ImageConfig{
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
		bench := &vyogotechv1.FrappeBench{
			Spec: vyogotechv1.FrappeBenchSpec{FrappeVersion: "15"},
		}
		img := r.getBenchImage(bench)
		if img != "frappe/erpnext:15" {
			t.Errorf("expected frappe/erpnext:15, got %s", img)
		}
	})
}

func TestSiteBackupReconciler_getSitesPVCName(t *testing.T) {
	r := &SiteBackupReconciler{}
	bench := &vyogotechv1.FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: "my-bench"}}
	name := r.getSitesPVCName(bench)
	if name != "my-bench-sites" {
		t.Errorf("expected my-bench-sites, got %s", name)
	}
}

func TestSiteBackupReconciler_buildBackupArgs(t *testing.T) {
	r := &SiteBackupReconciler{}
	t.Run("minimal", func(t *testing.T) {
		sb := &vyogotechv1.SiteBackup{Spec: vyogotechv1.SiteBackupSpec{Site: "site1.local"}}
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
		sb := &vyogotechv1.SiteBackup{
			Spec: vyogotechv1.SiteBackupSpec{
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
	t.Run("normalizes backup paths for bench (strip sites/, relocate to vyogo-backups)", func(t *testing.T) {
		// The console sends bench-root-relative paths ("sites/<site>/private/backups/<name>/..."),
		// but the operator must (1) strip the leading "sites/" because `bench backup
		// --backup-path*` resolves relative to the sites dir (otherwise paths double to
		// "sites/sites/..."), and (2) relocate the "private/backups/" segment to the
		// Frappe-safe "private/vyogo-backups/" dir (a per-backup subdir inside
		// private/backups makes Frappe's delete_temp_backups crash on the next backup).
		sb := &vyogotechv1.SiteBackup{
			Spec: vyogotechv1.SiteBackupSpec{
				Site:                   "site3.local",
				BackupPath:             "sites/site3.local/private/backups/bk1",
				BackupPathDB:           "sites/site3.local/private/backups/bk1/database.sql.gz",
				BackupPathConf:         "sites/site3.local/private/backups/bk1/site_config_backup.json",
				BackupPathFiles:        "sites/site3.local/private/backups/bk1/files.tar",
				BackupPathPrivateFiles: "sites/site3.local/private/backups/bk1/private-files.tar",
			},
		}
		args := r.buildBackupArgs(sb)
		want := map[string]string{
			"--backup-path":               "site3.local/private/vyogo-backups/bk1",
			"--backup-path-db":            "site3.local/private/vyogo-backups/bk1/database.sql.gz",
			"--backup-path-conf":          "site3.local/private/vyogo-backups/bk1/site_config_backup.json",
			"--backup-path-files":         "site3.local/private/vyogo-backups/bk1/files.tar",
			"--backup-path-private-files": "site3.local/private/vyogo-backups/bk1/private-files.tar",
		}
		for flag, expected := range want {
			found := false
			for i, a := range args {
				if a == flag {
					found = true
					if i+1 >= len(args) || args[i+1] != expected {
						t.Errorf("%s: expected %q, got %q", flag, expected, args[i+1])
					}
					if strings.HasPrefix(args[i+1], "sites/") {
						t.Errorf("%s: value still has leading sites/: %q", flag, args[i+1])
					}
					if strings.Contains(args[i+1], "private/backups/") {
						t.Errorf("%s: value not relocated off private/backups/: %q", flag, args[i+1])
					}
				}
			}
			if !found {
				t.Errorf("expected flag %s in args %v", flag, args)
			}
		}
	})
}

func TestSiteBackupReconciler_backupCommandAndArgs(t *testing.T) {
	r := &SiteBackupReconciler{}
	t.Run("no S3 runs bench directly", func(t *testing.T) {
		sb := &vyogotechv1.SiteBackup{Spec: vyogotechv1.SiteBackupSpec{Site: "s1.local"}}
		cmd, args := r.backupCommandAndArgs(sb)
		if len(cmd) != 1 || cmd[0] != "bench" {
			t.Fatalf("expected command [bench], got %v", cmd)
		}
		if len(args) < 3 || args[0] != "--site" {
			t.Errorf("expected bench backup args, got %v", args)
		}
	})
	t.Run("S3 wraps bench + upload script", func(t *testing.T) {
		sb := &vyogotechv1.SiteBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "bk1"},
			Spec: vyogotechv1.SiteBackupSpec{
				Site:         "s1.local",
				BackupPathDB: "sites/s1.local/private/backups/bk1/database.sql.gz",
				Storage: &vyogotechv1.BackupStorageConfig{
					Type: "s3",
					S3: &vyogotechv1.S3Config{
						Endpoint: "https://minio:9000", Bucket: "backups",
						AccessKeySecret: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "s3-creds"}, Key: "access"},
						SecretKeySecret: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "s3-creds"}, Key: "secret"},
					},
				},
			},
		}
		cmd, args := r.backupCommandAndArgs(sb)
		if len(cmd) != 2 || cmd[0] != "bash" || cmd[1] != "-c" {
			t.Fatalf("expected [bash -c], got %v", cmd)
		}
		if len(args) != 1 {
			t.Fatalf("expected one script arg, got %d", len(args))
		}
		script := args[0]
		for _, want := range []string{
			"bench ", "backup", "boto3", "upload_file",
			"/home/frappe/frappe-bench/sites/s1.local/private/vyogo-backups/bk1/database.sql.gz",
			"s1.local/bk1/database.sql.gz",
		} {
			if !strings.Contains(script, want) {
				t.Errorf("script missing %q\n---\n%s", want, script)
			}
		}
		// upload path must be relocated off private/backups
		if strings.Contains(script, "private/backups/bk1/database.sql.gz") {
			t.Errorf("upload local path not relocated to vyogo-backups:\n%s", script)
		}
		// S3 env should resolve keys from the secret
		env := backupS3Env(sb.Spec.Storage.S3)
		var gotBucket, gotAccessFrom bool
		for _, e := range env {
			if e.Name == "S3_BUCKET" && e.Value == "backups" {
				gotBucket = true
			}
			if e.Name == "S3_ACCESS_KEY" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil && e.ValueFrom.SecretKeyRef.Name == "s3-creds" {
				gotAccessFrom = true
			}
		}
		if !gotBucket || !gotAccessFrom {
			t.Errorf("S3 env incomplete: bucket=%v accessFromSecret=%v", gotBucket, gotAccessFrom)
		}
	})
}

func TestSiteBackupReconciler_buildBackupJob(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	r := &SiteBackupReconciler{Scheme: scheme}
	siteBackup := &vyogotechv1.SiteBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "my-backup", Namespace: "default"},
		Spec:       vyogotechv1.SiteBackupSpec{Site: "site.local"},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "default"},
		Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "15"},
	}
	job := r.buildBackupJob(context.Background(), siteBackup, bench)
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
	_ = vyogotechv1.AddToScheme(scheme)
	siteBackup := &vyogotechv1.SiteBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "sb", Namespace: "default"},
		Spec:       vyogotechv1.SiteBackupSpec{Site: "site.local"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteBackup).WithStatusSubresource(&vyogotechv1.SiteBackup{}).Build()
	r := &SiteBackupReconciler{Client: client}
	ctx := context.Background()
	err := r.updateSiteBackupStatus(ctx, siteBackup, "Running", "Backup in progress", "sb-backup")
	if err != nil {
		t.Fatalf("updateSiteBackupStatus: %v", err)
	}
	updated := &vyogotechv1.SiteBackup{}
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
		siteBackup *vyogotechv1.SiteBackup
		site       *vyogotechv1.FrappeSite
		bench      *vyogotechv1.FrappeBench
	)

	BeforeEach(func() {
		ctx = context.Background()

		bench = &vyogotechv1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-bench",
				Namespace: "default",
			},
			Spec: vyogotechv1.FrappeBenchSpec{
				FrappeVersion: "15",
			},
		}

		site = &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-site",
				Namespace: "default",
			},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: "test-site.local",
				BenchRef: &vyogotechv1.NamespacedName{
					Name:      bench.Name,
					Namespace: bench.Namespace,
				},
			},
		}

		siteBackup = &vyogotechv1.SiteBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-backup",
				Namespace: "default",
			},
			Spec: vyogotechv1.SiteBackupSpec{
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

		// Mock site status to Ready
		site.Status.Phase = vyogotechv1.FrappeSitePhaseReady
		Expect(k8sClient.Status().Update(ctx, site)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up
		_ = k8sClient.Delete(ctx, siteBackup)
		_ = k8sClient.Delete(ctx, site)
		_ = k8sClient.Delete(ctx, bench)
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
