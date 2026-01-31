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
	"fmt"
	"strings"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureBenchStorage ensures the PVC for the bench exists
func (r *FrappeBenchReconciler) ensureBenchStorage(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	pvc := &corev1.PersistentVolumeClaim{}

	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: bench.Namespace}, pvc)
	if err == nil {
		logger.V(1).Info("PVC already exists", "pvc", pvcName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	sc, err := r.chooseStorageClass(ctx, bench)
	if err != nil {
		return err
	}

	accessMode, err := r.determineAccessMode(ctx, bench, sc)
	if err != nil {
		return err
	}

	return r.createBenchPVC(ctx, bench, accessMode, sc)
}

func (r *FrappeBenchReconciler) createBenchPVC(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, accessMode corev1.PersistentVolumeAccessMode, sc *storagev1.StorageClass) error {
	logger := log.FromContext(ctx)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	sizeStr := bench.Spec.StorageSize
	if sizeStr == "" {
		sizeStr = "10Gi"
	}
	storageSize := resource.MustParse(sizeStr)

	builder := resources.NewPVCBuilder(pvcName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
		WithAnnotations(map[string]string{
			"frappe.tech/requested-access": string(accessMode),
		}).
		WithAccessMode(accessMode).
		WithStorageRequest(storageSize)

	if sc != nil {
		builder.WithStorageClass(sc.Name).
			WithAnnotations(map[string]string{
				"frappe.tech/storage-class": sc.Name,
				"frappe.tech/provisioner":   sc.Provisioner,
			})
	}

	if accessMode == corev1.ReadWriteOnce {
		builder.WithAnnotations(map[string]string{"frappe.tech/fallback": "true"})
	}

	pvc, err := builder.WithOwner(bench, r.Scheme).Build()
	if err != nil {
		return err
	}

	logger.Info("Creating PVC for bench", "pvc", pvcName, "accessMode", accessMode)
	return r.Create(ctx, pvc)
}

func (r *FrappeBenchReconciler) chooseStorageClass(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) (*storagev1.StorageClass, error) {
	logger := log.FromContext(ctx)

	if bench.Spec.StorageClassName != "" {
		sc := &storagev1.StorageClass{}
		if err := r.Get(ctx, types.NamespacedName{Name: bench.Spec.StorageClassName}, sc); err != nil {
			if errors.IsNotFound(err) {
				return nil, fmt.Errorf("specified storage class '%s' not found in cluster. Available storage classes can be listed with 'kubectl get storageclass'", bench.Spec.StorageClassName)
			}
			return nil, fmt.Errorf("failed to get storage class '%s': %w", bench.Spec.StorageClassName, err)
		}

		// Validate that the storage class is ready for use
		if sc.Provisioner == "" {
			return nil, fmt.Errorf("storage class '%s' has no provisioner configured", bench.Spec.StorageClassName)
		}

		logger.Info("Using specified storage class", "storageClass", sc.Name, "provisioner", sc.Provisioner)
		return sc, nil
	}

	// Get all storage classes for selection
	list := &storagev1.StorageClassList{}
	if err := r.List(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to list storage classes: %w", err)
	}

	if len(list.Items) == 0 {
		return nil, fmt.Errorf("no storage classes available in cluster. Please create a storage class or specify storageClassName in bench spec")
	}

	// Try to find default storage class
	for _, sc := range list.Items {
		if isDefaultStorageClass(&sc) {
			logger.Info("Using default storage class", "storageClass", sc.Name, "provisioner", sc.Provisioner)
			return &sc, nil
		}
	}

	// No default found, use first available with warning
	sc := &list.Items[0]
	logger.Info("No default storage class found, using first available",
		"storageClass", sc.Name,
		"provisioner", sc.Provisioner,
		"recommendation", "Set a default storage class or specify storageClassName in bench spec")
	return sc, nil
}

func (r *FrappeBenchReconciler) determineAccessMode(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, sc *storagev1.StorageClass) (corev1.PersistentVolumeAccessMode, error) {
	logger := log.FromContext(ctx)

	if bench.Annotations != nil {
		if modeStr, ok := bench.Annotations["frappe.tech/storage-access-mode"]; ok {
			logger.V(1).Info("Using existing storage access mode from annotations", "mode", modeStr)
			return corev1.PersistentVolumeAccessMode(modeStr), nil
		}
	}

	mode := corev1.ReadWriteOnce
	if storageClassSupportsRWX(sc) {
		mode = corev1.ReadWriteMany
	}

	// Use patch instead of update to avoid race conditions
	patch := client.MergeFrom(bench.DeepCopy())
	if bench.Annotations == nil {
		bench.Annotations = make(map[string]string)
	}
	bench.Annotations["frappe.tech/storage-access-mode"] = string(mode)

	logger.Info("Setting storage access mode annotation", "mode", mode, "storageClass", sc.Name)
	if err := r.Patch(ctx, bench, patch); err != nil {
		logger.Error(err, "Failed to patch bench with storage access mode", "mode", mode)
		return corev1.ReadWriteOnce, err
	}
	return mode, nil
}

func storageClassSupportsRWX(sc *storagev1.StorageClass) bool {
	if sc == nil {
		return false
	}
	provisioner := strings.ToLower(sc.Provisioner)
	rwxProviders := []string{"nfs", "ceph", "gluster", "netapp", "azurefile", "filestore", "portworx"}
	for _, provider := range rwxProviders {
		if strings.Contains(provisioner, provider) {
			return true
		}
	}
	return false
}

func isDefaultStorageClass(sc *storagev1.StorageClass) bool {
	if sc == nil {
		return false
	}
	if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
		return true
	}
	if sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
		return true
	}
	return false
}

func (r *FrappeBenchReconciler) getBenchStorageAccessMode(bench *vyogotechv1alpha1.FrappeBench) corev1.PersistentVolumeAccessMode {
	if bench.Annotations != nil && bench.Annotations["frappe.tech/storage-fallback"] == "true" {
		return corev1.ReadWriteOnce
	}
	return corev1.ReadWriteMany
}

func (r *FrappeBenchReconciler) markStorageFallback(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	// Use patch instead of update to avoid race conditions
	patch := client.MergeFrom(bench.DeepCopy())
	if bench.Annotations == nil {
		bench.Annotations = make(map[string]string)
	}
	bench.Annotations["frappe.tech/storage-fallback"] = "true"

	logger.Info("Marking bench for storage fallback", "bench", bench.Name)
	return r.Patch(ctx, bench, patch)
}

func shouldFallbackStorage(pvc *corev1.PersistentVolumeClaim, bench *vyogotechv1alpha1.FrappeBench) bool {
	if pvc.Status.Phase != corev1.ClaimPending {
		return false
	}
	if pvc.Annotations["frappe.tech/requested-access"] != string(corev1.ReadWriteMany) {
		return false
	}
	if bench.Annotations != nil && bench.Annotations["frappe.tech/storage-fallback"] == "true" {
		return false
	}
	return true
}
