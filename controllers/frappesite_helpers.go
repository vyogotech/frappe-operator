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
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// getImagePullSecrets retrieves the image pull secrets from the associated bench
func (r *FrappeSiteReconciler) getImagePullSecrets(bench *vyogotechv1alpha1.FrappeBench) []corev1.LocalObjectReference {
	if bench.Spec.ImageConfig != nil && len(bench.Spec.ImageConfig.PullSecrets) > 0 {
		secrets := make([]corev1.LocalObjectReference, len(bench.Spec.ImageConfig.PullSecrets))
		for i, s := range bench.Spec.ImageConfig.PullSecrets {
			secrets[i] = corev1.LocalObjectReference{Name: s.Name}
		}
		return secrets
	}
	return nil
}

// getImagePullPolicy retrieves the image pull policy from the associated bench
func (r *FrappeSiteReconciler) getImagePullPolicy(bench *vyogotechv1alpha1.FrappeBench) corev1.PullPolicy {
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.PullPolicy != "" {
		return bench.Spec.ImageConfig.PullPolicy
	}
	return corev1.PullPolicy("") // Leave empty so Kubernetes defaults apply
}
