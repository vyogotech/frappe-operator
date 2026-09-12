package controllers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func TestStorageClassSupportsRWX(t *testing.T) {
	// nil storage class -> false
	if storageClassSupportsRWX(nil) {
		t.Fatalf("expected false for nil StorageClass")
	}

	// with provisioners that imply RWX support
	scNFS := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "nfs-sc"}, Provisioner: "kubernetes.io/nfs"}
	if !storageClassSupportsRWX(scNFS) {
		t.Fatalf("expected RWX support for NFS provisioner")
	}

	scCeph := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "ceph-sc"}, Provisioner: "ceph.rbd"}
	if !storageClassSupportsRWX(scCeph) {
		t.Fatalf("expected RWX support for Ceph provisioner")
	}

	scAWS := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "aws-sc"}, Provisioner: "kubernetes.io/aws-ebs"}
	if storageClassSupportsRWX(scAWS) {
		t.Fatalf("did not expect RWX support for AWS EBS provisioner")
	}
}

func TestIsDefaultStorageClass(t *testing.T) {
	// nil storage class -> false
	if isDefaultStorageClass(nil) {
		t.Fatalf("expected false for nil StorageClass")
	}

	scDefault := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "def-sc", Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}}
	if !isDefaultStorageClass(scDefault) {
		t.Fatalf("expected true for default storage class annotation")
	}

	scBetaDefault := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "def-beta", Annotations: map[string]string{"storageclass.beta.kubernetes.io/is-default-class": "true"}}}
	if !isDefaultStorageClass(scBetaDefault) {
		t.Fatalf("expected true for beta default annotation")
	}

	scNoDefault := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "no-def", Annotations: map[string]string{}}}
	if isDefaultStorageClass(scNoDefault) {
		t.Fatalf("expected false when no default annotations present")
	}
}

func TestGetBenchStorageAccessMode(t *testing.T) {
	r := &FrappeBenchReconciler{}
	bench := &vyogotechv1.FrappeBench{}
	if mode := r.getBenchStorageAccessMode(bench); mode != corev1.ReadWriteMany {
		t.Fatalf("expected ReadWriteMany by default, got %v", mode)
	}

	bench.Annotations = map[string]string{"frappe.tech/storage-fallback": "true"}
	if mode := r.getBenchStorageAccessMode(bench); mode != corev1.ReadWriteOnce {
		t.Fatalf("expected ReadWriteOnce when storage-fallback is true, got %v", mode)
	}
}

func TestShouldFallbackStorage(t *testing.T) {
	bench := &vyogotechv1.FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: "bench1"}}
	pvc := &corev1.PersistentVolumeClaim{Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending}, ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"frappe.tech/requested-access": string(corev1.ReadWriteMany)}}}
	if !shouldFallbackStorage(pvc, bench) {
		t.Fatalf("expected fallback to be true for pending PVC with ReadWriteMany")
	}

	pvc.Status.Phase = corev1.ClaimBound
	if shouldFallbackStorage(pvc, bench) {
		t.Fatalf("expected fallback to be false when PVC not pending")
	}

	// When bench requests no-fallback, fallback should be false
	pvc.Status.Phase = corev1.ClaimPending
	bench.Annotations = map[string]string{"frappe.tech/storage-fallback": "true"}
	if shouldFallbackStorage(pvc, bench) {
		t.Fatalf("expected fallback to be false when bench already opts into fallback")
	}
}

func TestStorageClassSupportsRWXAnnotationOverride(t *testing.T) {
	// A driver the heuristic does not know (Longhorn) declares RWX explicitly.
	scLonghornRWX := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "longhorn-rwx", Annotations: map[string]string{AccessModeAnnotation: "ReadWriteMany"}},
		Provisioner: "driver.longhorn.io",
	}
	if !storageClassSupportsRWX(scLonghornRWX) {
		t.Fatalf("expected annotated longhorn class to be RWX")
	}
	// The same driver without the annotation stays on the heuristic (RWO).
	scLonghorn := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "longhorn"}, Provisioner: "driver.longhorn.io"}
	if storageClassSupportsRWX(scLonghorn) {
		t.Fatalf("expected unannotated longhorn class to be RWO")
	}
	// The annotation can also force RWO on an NFS-named class.
	scNFSForcedRWO := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "nfs-rwo", Annotations: map[string]string{AccessModeAnnotation: "ReadWriteOnce"}},
		Provisioner: "cluster.local/nfs-server-provisioner",
	}
	if storageClassSupportsRWX(scNFSForcedRWO) {
		t.Fatalf("expected annotation to force RWO")
	}
	// An unknown value falls back to the heuristic.
	scGarbage := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "nfs-garbage", Annotations: map[string]string{AccessModeAnnotation: "whatever"}},
		Provisioner: "nfs.csi.k8s.io",
	}
	if !storageClassSupportsRWX(scGarbage) {
		t.Fatalf("expected heuristic fallback for an invalid annotation value")
	}
}
