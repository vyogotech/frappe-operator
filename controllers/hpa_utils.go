package controllers

import (
	"context"
	"fmt"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *FrappeBenchReconciler) ensureHPA(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, deployName string, config *vyogotechv1alpha1.HPAConfig) error {
	logger := log.FromContext(ctx)

	// If HPA is disabled or config is nil, ensure it's deleted
	if config == nil || !config.Enabled {
		return r.deleteHPAIfExists(ctx, bench, deployName)
	}

	hpaName := deployName // Use same name as deployment
	hpa := &unstructured.Unstructured{}
	hpa.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling",
		Version: "v2",
		Kind:    "HorizontalPodAutoscaler",
	})
	hpa.SetName(hpaName)
	hpa.SetNamespace(bench.Namespace)

	// Build spec
	if err := r.applyHPASpec(hpa, deployName, config); err != nil {
		return err
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(bench, hpa, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference for HPA: %w", err)
	}

	// Create or Update
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(hpa.GroupVersionKind())
	err := r.Get(ctx, types.NamespacedName{Name: hpaName, Namespace: bench.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating HPA", "hpa", hpaName)
			return r.Create(ctx, hpa)
		}
		return err
	}

	// Update existing
	hpa.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("Updating HPA", "hpa", hpaName)
	return r.Update(ctx, hpa)
}

// deleteHPAIfExists deletes the HPA if it exists
func (r *FrappeBenchReconciler) deleteHPAIfExists(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, deployName string) error {
	logger := log.FromContext(ctx)
	hpaName := deployName

	hpa := &unstructured.Unstructured{}
	hpa.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling",
		Version: "v2",
		Kind:    "HorizontalPodAutoscaler",
	})

	err := r.Get(ctx, types.NamespacedName{Name: hpaName, Namespace: bench.Namespace}, hpa)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	logger.Info("Deleting HPA", "hpa", hpaName)
	return r.Delete(ctx, hpa)
}

// applyHPASpec populates the HPA spec
func (r *FrappeBenchReconciler) applyHPASpec(hpa *unstructured.Unstructured, deployName string, config *vyogotechv1alpha1.HPAConfig) error {
	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       deployName,
		},
		"minReplicas": int64(1),
		"maxReplicas": int64(config.MaxReplicas),
	}

	if config.MinReplicas != nil {
		spec["minReplicas"] = int64(*config.MinReplicas)
	}

	metrics := []interface{}{}

	if config.TargetCPUUtilizationPercentage != nil {
		metrics = append(metrics, map[string]interface{}{
			"type": "Resource",
			"resource": map[string]interface{}{
				"name": "cpu",
				"target": map[string]interface{}{
					"type":               "Utilization",
					"averageUtilization": int64(*config.TargetCPUUtilizationPercentage),
				},
			},
		})
	}

	if config.TargetMemoryUtilizationPercentage != nil {
		metrics = append(metrics, map[string]interface{}{
			"type": "Resource",
			"resource": map[string]interface{}{
				"name": "memory",
				"target": map[string]interface{}{
					"type":               "Utilization",
					"averageUtilization": int64(*config.TargetMemoryUtilizationPercentage),
				},
			},
		})
	}

	if len(metrics) > 0 {
		spec["metrics"] = metrics
	}

	return unstructured.SetNestedField(hpa.Object, spec, "spec")
}
