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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func TestWarnIfObjectStorage(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = vyogotechv1.AddToScheme(scheme)

	scWith := &vyogotechv1.SiteConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "t"},
		Spec: vyogotechv1.SiteConfigSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "site1"},
			ObjectStorage: &vyogotechv1.ObjectStorageConfig{
				Bucket: "b", EndpointURL: "http://minio:9000",
				CredentialsSecretRef: corev1.LocalObjectReference{Name: "s"},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(scWith).Build()
	ctx := context.Background()

	// --with-files on an object-storage site -> warning.
	t.Run("warns", func(t *testing.T) {
		rec := record.NewFakeRecorder(5)
		r := &SiteBackupReconciler{Client: cl, Scheme: scheme, Recorder: rec}
		r.warnIfObjectStorage(ctx, &vyogotechv1.SiteBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "bk", Namespace: "t"},
			Spec:       vyogotechv1.SiteBackupSpec{Site: "site1", WithFiles: true},
		})
		select {
		case ev := <-rec.Events:
			if !strings.Contains(ev, "FilesInObjectStorage") {
				t.Errorf("unexpected event: %s", ev)
			}
		default:
			t.Error("expected a FilesInObjectStorage warning")
		}
	})

	// No --with-files -> no warning.
	t.Run("no warning without files", func(t *testing.T) {
		rec := record.NewFakeRecorder(5)
		r := &SiteBackupReconciler{Client: cl, Scheme: scheme, Recorder: rec}
		r.warnIfObjectStorage(ctx, &vyogotechv1.SiteBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "bk2", Namespace: "t"},
			Spec:       vyogotechv1.SiteBackupSpec{Site: "site1", WithFiles: false},
		})
		select {
		case ev := <-rec.Events:
			t.Errorf("did not expect an event, got: %s", ev)
		default:
		}
	})

	// Different site (no matching object-storage SiteConfig) -> no warning.
	t.Run("no warning for other site", func(t *testing.T) {
		rec := record.NewFakeRecorder(5)
		r := &SiteBackupReconciler{Client: cl, Scheme: scheme, Recorder: rec}
		r.warnIfObjectStorage(ctx, &vyogotechv1.SiteBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "bk3", Namespace: "t"},
			Spec:       vyogotechv1.SiteBackupSpec{Site: "other", WithFiles: true},
		})
		select {
		case ev := <-rec.Events:
			t.Errorf("did not expect an event, got: %s", ev)
		default:
		}
	})
}
