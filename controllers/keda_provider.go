package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

type KEDAProvider struct {
	client client.Client
	scheme *runtime.Scheme
}

func (p *KEDAProvider) Name() string { return "keda" }

func (p *KEDAProvider) IsAvailable(ctx context.Context) bool {
	// Create a minimal unstructured list to check if the resource exists
	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})

	// Attempt to list - if this succeeds, KEDA is available
	err := p.client.List(ctx, list, client.Limit(1))
	return err == nil
}

func (p *KEDAProvider) Ensure(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, componentName string, deploymentName string, config *vyogotechv1alpha1.ComponentAutoscaling) error {
	logger := log.FromContext(ctx)

	scaledObjectName := fmt.Sprintf("%s-%s", bench.Name, componentName)

	if config.KEDA == nil {
		return fmt.Errorf("KEDA configuration missing for component %s", componentName)
	}

	// Build the ScaledObject using unstructured
	scaledObject := &unstructured.Unstructured{}
	scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})
	scaledObject.SetName(scaledObjectName)
	scaledObject.SetNamespace(bench.Namespace)

	// Note: componentLabels is a method of FrappeBenchReconciler.
	// For now, we'll manually set labels or wait until we have a helper.
	labels := map[string]string{
		"bench":     bench.Name,
		"component": componentName,
	}
	scaledObject.SetLabels(labels)

	// Build trigger based on KEDA config
	var trigger map[string]interface{}
	switch config.KEDA.Trigger {
	case "cpu":
		trigger = map[string]interface{}{
			"type":       "cpu",
			"metricType": "Utilization",
			"metadata": map[string]interface{}{
				"value": config.KEDA.TargetValue,
			},
		}
	case "memory":
		trigger = map[string]interface{}{
			"type":       "memory",
			"metricType": "Utilization",
			"metadata": map[string]interface{}{
				"value": config.KEDA.TargetValue,
			},
		}
	case "redis":
		metadata := map[string]interface{}{
			"listLength": config.KEDA.TargetValue,
		}
		for k, v := range config.KEDA.Metadata {
			metadata[k] = v
		}
		trigger = map[string]interface{}{
			"type":     "redis",
			"metadata": metadata,
		}
	default:
		return fmt.Errorf("unsupported KEDA trigger type: %s", config.KEDA.Trigger)
	}

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       deploymentName,
		},
		"minReplicaCount": int64(*config.MinReplicas),
		"maxReplicaCount": int64(*config.MaxReplicas),
		"cooldownPeriod":  int64(*config.CooldownPeriod),
		"pollingInterval": int64(*config.PollingInterval),
		"triggers": []interface{}{
			trigger,
		},
	}

	if err := unstructured.SetNestedField(scaledObject.Object, spec, "spec"); err != nil {
		return fmt.Errorf("failed to set ScaledObject spec: %w", err)
	}

	if err := controllerutil.SetControllerReference(bench, scaledObject, p.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(scaledObject.GroupVersionKind())
	err := p.client.Get(ctx, types.NamespacedName{Name: scaledObjectName, Namespace: bench.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating ScaledObject", "component", componentName, "name", scaledObjectName)
			return p.client.Create(ctx, scaledObject)
		}
		return err
	}

	scaledObject.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("Updating ScaledObject", "component", componentName, "name", scaledObjectName)
	return p.client.Update(ctx, scaledObject)
}

func (p *KEDAProvider) Delete(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, componentName string) error {
	logger := log.FromContext(ctx)
	scaledObjectName := fmt.Sprintf("%s-%s", bench.Name, componentName)

	scaledObject := &unstructured.Unstructured{}
	scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})

	err := p.client.Get(ctx, types.NamespacedName{Name: scaledObjectName, Namespace: bench.Namespace}, scaledObject)
	if err != nil {
		if meta.IsNoMatchError(err) || errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	logger.Info("Deleting ScaledObject", "component", componentName, "name", scaledObjectName)
	return p.client.Delete(ctx, scaledObject)
}
