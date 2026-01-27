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
	"testing"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestFrappeBenchReconciler_getPodSecurityContext_Defaults tests default pod security context
func TestFrappeBenchReconciler_getPodSecurityContext_Defaults(t *testing.T) {
	r := &FrappeBenchReconciler{}

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bench",
			Namespace: "default",
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
		},
	}

	psc := r.getPodSecurityContext(context.TODO(), bench)

	// With new OpenShift compatibility changes, this should be nil if no env vars are set
	if psc != nil {
		t.Errorf("Expected nil PodSecurityContext (defer to platform), got %v", psc)
	}
}

// TestFrappeBenchReconciler_getPodSecurityContext_Override tests user override
func TestFrappeBenchReconciler_getPodSecurityContext_Override(t *testing.T) {
	r := &FrappeBenchReconciler{}

	customUser := int64(2000)
	customGroup := int64(2001)
	customFSGroup := int64(2002)

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bench",
			Namespace: "default",
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
			Security: &vyogotechv1alpha1.SecurityConfig{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsUser:  &customUser,
					RunAsGroup: &customGroup,
					FSGroup:    &customFSGroup,
				},
			},
		},
	}

	psc := r.getPodSecurityContext(context.TODO(), bench)

	if psc == nil {
		t.Fatal("Expected non-nil PodSecurityContext")
	}

	// Verify custom values are used
	if psc.RunAsUser == nil || *psc.RunAsUser != 2000 {
		t.Errorf("Expected RunAsUser=2000, got %v", psc.RunAsUser)
	}

	if psc.RunAsGroup == nil || *psc.RunAsGroup != 2001 {
		t.Errorf("Expected RunAsGroup=2001, got %v", psc.RunAsGroup)
	}

	if psc.FSGroup == nil || *psc.FSGroup != 2002 {
		t.Errorf("Expected FSGroup=2002, got %v", psc.FSGroup)
	}
}

// TestFrappeBenchReconciler_getContainerSecurityContext_Defaults tests default container security context
func TestFrappeBenchReconciler_getContainerSecurityContext_Defaults(t *testing.T) {
	r := &FrappeBenchReconciler{}

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bench",
			Namespace: "default",
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
		},
	}

	csc := r.getContainerSecurityContext(context.TODO(), bench)

	// With new OpenShift compatibility changes, this should be nil if no env vars are set
	if csc != nil {
		t.Errorf("Expected nil SecurityContext (defer to platform), got %v", csc)
	}
}

// TestFrappeBenchReconciler_getContainerSecurityContext_Override tests user override
func TestFrappeBenchReconciler_getContainerSecurityContext_Override(t *testing.T) {
	r := &FrappeBenchReconciler{}

	customUser := int64(3000)
	customGroup := int64(3001)
	allowPrivEsc := true

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bench",
			Namespace: "default",
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
			Security: &vyogotechv1alpha1.SecurityConfig{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:                &customUser,
					RunAsGroup:               &customGroup,
					AllowPrivilegeEscalation: &allowPrivEsc,
				},
			},
		},
	}

	csc := r.getContainerSecurityContext(context.TODO(), bench)

	if csc == nil {
		t.Fatal("Expected non-nil SecurityContext")
	}

	// Verify custom values are used
	if csc.RunAsUser == nil || *csc.RunAsUser != 3000 {
		t.Errorf("Expected RunAsUser=3000, got %v", csc.RunAsUser)
	}

	if csc.RunAsGroup == nil || *csc.RunAsGroup != 3001 {
		t.Errorf("Expected RunAsGroup=3001, got %v", csc.RunAsGroup)
	}

	if csc.AllowPrivilegeEscalation == nil || *csc.AllowPrivilegeEscalation != true {
		t.Errorf("Expected AllowPrivilegeEscalation=true, got %v", csc.AllowPrivilegeEscalation)
	}
}

// TestFrappeSiteReconciler_getPodSecurityContext_Defaults tests default pod security context for site controller
func TestFrappeSiteReconciler_getPodSecurityContext_Defaults(t *testing.T) {
	r := &FrappeSiteReconciler{}

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bench",
			Namespace: "default",
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
		},
	}

	psc := r.getPodSecurityContext(context.TODO(), bench)

	// With new OpenShift compatibility changes, this should be nil if no env vars are set
	if psc != nil {
		t.Errorf("Expected nil PodSecurityContext (defer to platform), got %v", psc)
	}
}

// TestFrappeSiteReconciler_getContainerSecurityContext_Defaults tests default container security context for site controller
func TestFrappeSiteReconciler_getContainerSecurityContext_Defaults(t *testing.T) {
	r := &FrappeSiteReconciler{}

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bench",
			Namespace: "default",
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
		},
	}

	csc := r.getContainerSecurityContext(context.TODO(), bench)

	// With new OpenShift compatibility changes, this should be nil if no env vars are set
	if csc != nil {
		t.Errorf("Expected nil SecurityContext (defer to platform), got %v", csc)
	}
}

// TestSecurityContext_NoRootUser tests that default security context doesn't run as root
func TestSecurityContext_NoRootUser(t *testing.T) {
	r := &FrappeBenchReconciler{}

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bench",
			Namespace: "default",
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
		},
	}

	psc := r.getPodSecurityContext(context.TODO(), bench)
	csc := r.getContainerSecurityContext(context.TODO(), bench)

	// Critical security check: ensure we never default to root USER (UID 0)
	// Note: GID 0 is intentionally allowed for OpenShift arbitrary UID support
	if psc != nil && psc.RunAsUser != nil && *psc.RunAsUser == 0 {
		t.Error("SECURITY ISSUE: PodSecurityContext defaults to root user (UID 0)")
	}

	// OpenShift uses GID 0 for arbitrary UID support - this is expected and safe
	// when combined with non-root UID

	if csc != nil && csc.RunAsUser != nil && *csc.RunAsUser == 0 {
		t.Error("SECURITY ISSUE: SecurityContext defaults to root user (UID 0)")
	}
}

// TestSecurityContext_PSPCompliance tests compliance with Pod Security Policy requirements
func TestSecurityContext_PSPCompliance(t *testing.T) {
	r := &FrappeBenchReconciler{}

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bench",
			Namespace: "default",
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
		},
	}

	psc := r.getPodSecurityContext(context.TODO(), bench)

	if psc == nil {
		// Nil is compliant by definition (defer to platform)
		return
	}

	// PSP requirement: if RunAsGroup is set, RunAsUser must also be set
	if psc.RunAsGroup != nil && psc.RunAsUser == nil {
		t.Error("PSP VIOLATION: RunAsGroup is set but RunAsUser is not")
	}

	// PSP requirement: if FSGroup is set, RunAsUser should be set
	if psc.FSGroup != nil && psc.RunAsUser == nil {
		t.Error("PSP VIOLATION: FSGroup is set but RunAsUser is not")
	}

	// All three should be consistently non-root
	if psc.RunAsUser != nil && psc.RunAsGroup != nil && psc.FSGroup != nil {
		if *psc.RunAsUser != *psc.RunAsGroup {
			t.Logf("INFO: RunAsUser (%d) and RunAsGroup (%d) are different - this is allowed but may cause permission issues", *psc.RunAsUser, *psc.RunAsGroup)
		}
	}
}
