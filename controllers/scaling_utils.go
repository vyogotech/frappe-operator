package controllers

import (
	"context"
	"fmt"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureAutoscaler handles unified scaling logic (HPA or KEDA)
func (r *FrappeBenchReconciler) ensureAutoscaler(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, deployName string, config *vyogotechv1alpha1.ScalingConfig) error {
	// 1. If config is nil or disabled, cleanup everything
	if config == nil || !config.Enabled {
		if err := r.deleteHPAIfExists(ctx, bench, deployName); err != nil {
			return err
		}
		return r.deleteScaledObjectIfExists(ctx, bench, deployName)
	}

	// 2. Determine Scaling Mode
	// Prefer KEDA if Triggers are defined OR if KEDA is available and user wants advanced features
	kedaAvailable := r.isKEDAAvailable(ctx)
	useKEDA := kedaAvailable && (len(config.Triggers) > 0 || config.PollingInterval != nil || config.CooldownPeriod != nil)

	if useKEDA {
		// Clean up HPA if it exists to avoid conflicts
		if err := r.deleteHPAIfExists(ctx, bench, deployName); err != nil {
			return err
		}
		return r.ensureScaledObjectGeneric(ctx, bench, deployName, config)
	}

	// Fallback to HPA (CPU/Memory only)
	// Clean up ScaledObject if it exists
	if err := r.deleteScaledObjectIfExists(ctx, bench, deployName); err != nil {
		return err
	}
	return r.ensureHPA(ctx, bench, deployName, config)
}

// ensureHPA ensures a HorizontalPodAutoscaler exists for a deployment
func (r *FrappeBenchReconciler) ensureHPA(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, deployName string, config *vyogotechv1alpha1.ScalingConfig) error {
	logger := log.FromContext(ctx)

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
func (r *FrappeBenchReconciler) applyHPASpec(hpa *unstructured.Unstructured, deployName string, config *vyogotechv1alpha1.ScalingConfig) error {
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
	} else {
		return fmt.Errorf("no metrics defined for HPA")
	}

	return unstructured.SetNestedField(hpa.Object, spec, "spec")
}

// ensureScaledObjectGeneric creates/updates a generic KEDA ScaledObject
func (r *FrappeBenchReconciler) ensureScaledObjectGeneric(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, deployName string, config *vyogotechv1alpha1.ScalingConfig) error {
	logger := log.FromContext(ctx)

	scaledObjectName := deployName
	scaledObject := &unstructured.Unstructured{}
	scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})
	scaledObject.SetName(scaledObjectName)
	scaledObject.SetNamespace(bench.Namespace)
	if scaledObject.Object == nil {
		scaledObject.Object = make(map[string]interface{})
	}
	// Re-apply GVK just in case
	scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})
	scaledObject.SetLabels(r.componentLabels(bench, deployName))

	// Triggers
	triggers := []interface{}{}
	for _, t := range config.Triggers {
		metadata := map[string]interface{}{}
		for k, v := range t.Metadata {
			metadata[k] = v
		}

		triggerMap := map[string]interface{}{
			"type":     t.Type,
			"metadata": metadata,
		}

		if t.AuthenticationRef != nil && *t.AuthenticationRef != "" {
			triggerMap["authenticationRef"] = map[string]interface{}{
				"name": *t.AuthenticationRef,
			}
		}

		triggers = append(triggers, triggerMap)
	}

	// Add System Triggers (CPU/Memory) if configured
	if config.TargetCPUUtilizationPercentage != nil {
		triggers = append(triggers, map[string]interface{}{
			"type": "cpu",
			"metadata": map[string]interface{}{
				"type":  "Utilization",
				"value": fmt.Sprintf("%d", *config.TargetCPUUtilizationPercentage),
			},
		})
	}

	if config.TargetMemoryUtilizationPercentage != nil {
		triggers = append(triggers, map[string]interface{}{
			"type": "memory",
			"metadata": map[string]interface{}{
				"type":  "Utilization",
				"value": fmt.Sprintf("%d", *config.TargetMemoryUtilizationPercentage),
			},
		})
	}

	if len(triggers) == 0 {
		return fmt.Errorf("no triggers defined for ScaledObject")
	}

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       deployName,
		},
		"minReplicaCount": int64(1),
		"maxReplicaCount": int64(config.MaxReplicas),
		"triggers":        triggers,
	}

	if config.MinReplicas != nil {
		spec["minReplicaCount"] = int64(*config.MinReplicas)
	}
	if config.CooldownPeriod != nil {
		spec["cooldownPeriod"] = int64(*config.CooldownPeriod)
	}
	if config.PollingInterval != nil {
		spec["pollingInterval"] = int64(*config.PollingInterval)
	}

	if err := unstructured.SetNestedField(scaledObject.Object, spec, "spec"); err != nil {
		return fmt.Errorf("failed to set ScaledObject spec: %w", err)
	}

	if err := controllerutil.SetControllerReference(bench, scaledObject, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(scaledObject.GroupVersionKind())
	err := r.Get(ctx, types.NamespacedName{Name: scaledObjectName, Namespace: bench.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating ScaledObject", "name", scaledObjectName)
			return r.Create(ctx, scaledObject)
		}
		return err
	}

	scaledObject.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("Updating ScaledObject", "name", scaledObjectName)
	return r.Update(ctx, scaledObject)
}

// deleteScaledObjectIfExists deletes a ScaledObject if it exists (Generic version)
func (r *FrappeBenchReconciler) deleteScaledObjectIfExistsGeneric(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, deployName string) error {
	logger := log.FromContext(ctx)
	scaledObjectName := deployName

	scaledObject := &unstructured.Unstructured{}
	scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})

	err := r.Get(ctx, types.NamespacedName{Name: scaledObjectName, Namespace: bench.Namespace}, scaledObject)
	if err != nil {
		if meta.IsNoMatchError(err) || errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	logger.Info("Deleting ScaledObject", "name", scaledObjectName)
	return r.Delete(ctx, scaledObject)
}
