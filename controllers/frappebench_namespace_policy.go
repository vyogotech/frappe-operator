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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// ensureNamespacePolicy maintains an optional ResourceQuota and LimitRange in the
// bench's namespace as a noisy-neighbor guardrail for the tenant. It is level-based:
// clearing spec.namespacePolicy (or any of its sub-sections) removes the corresponding
// operator-managed object on the next reconcile.
func (r *FrappeBenchReconciler) ensureNamespacePolicy(ctx context.Context, bench *vyogotechv1.FrappeBench) error {
	logger := log.FromContext(ctx)
	quotaName := fmt.Sprintf("%s-quota", bench.Name)
	limitsName := fmt.Sprintf("%s-limits", bench.Name)
	policy := bench.Spec.NamespacePolicy

	// --- ResourceQuota: total tenant caps for the namespace ---
	if policy != nil && len(policy.ResourceQuota) > 0 {
		rq := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: quotaName, Namespace: bench.Namespace}}
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, rq, func() error {
			rq.Spec.Hard = policy.ResourceQuota
			return controllerutil.SetControllerReference(bench, rq, r.Scheme)
		}); err != nil {
			return fmt.Errorf("reconcile ResourceQuota %s: %w", quotaName, err)
		}
		logger.V(1).Info("Reconciled namespace ResourceQuota", "name", quotaName)
	} else if err := r.deleteNamespaceObject(ctx, &corev1.ResourceQuota{}, quotaName, bench.Namespace); err != nil {
		return err
	}

	// --- LimitRange: per-container default/max so every pod is bounded ---
	if policy != nil && (len(policy.DefaultRequests) > 0 || len(policy.DefaultLimits) > 0 || len(policy.MaxLimits) > 0) {
		lr := &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: limitsName, Namespace: bench.Namespace}}
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, lr, func() error {
			item := corev1.LimitRangeItem{Type: corev1.LimitTypeContainer}
			if len(policy.DefaultRequests) > 0 {
				item.DefaultRequest = policy.DefaultRequests
			}
			if len(policy.DefaultLimits) > 0 {
				item.Default = policy.DefaultLimits
			}
			if len(policy.MaxLimits) > 0 {
				item.Max = policy.MaxLimits
			}
			lr.Spec.Limits = []corev1.LimitRangeItem{item}
			return controllerutil.SetControllerReference(bench, lr, r.Scheme)
		}); err != nil {
			return fmt.Errorf("reconcile LimitRange %s: %w", limitsName, err)
		}
		logger.V(1).Info("Reconciled namespace LimitRange", "name", limitsName)
	} else if err := r.deleteNamespaceObject(ctx, &corev1.LimitRange{}, limitsName, bench.Namespace); err != nil {
		return err
	}

	return nil
}

// deleteNamespaceObject best-effort deletes an operator-managed namespace object,
// ignoring not-found, so the reconcile converges when a policy section is cleared.
func (r *FrappeBenchReconciler) deleteNamespaceObject(ctx context.Context, obj client.Object, name, namespace string) error {
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
