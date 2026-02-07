package controllers

import (
	"context"
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	corev1 "k8s.io/api/core/v1"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

type HPAProvider struct {
	client client.Client
	scheme *runtime.Scheme
}

func (p *HPAProvider) Name() string { return "hpa" }

func (p *HPAProvider) IsAvailable(ctx context.Context) bool {
	return true // HPA is built-in
}

func (p *HPAProvider) Ensure(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, componentName string, deploymentName string, config *vyogotechv1alpha1.ComponentAutoscaling) error {
	logger := log.FromContext(ctx)

	hpaName := fmt.Sprintf("%s-%s", bench.Name, componentName)

	if config.HPA == nil {
		return fmt.Errorf("HPA configuration missing for component %s", componentName)
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hpaName,
			Namespace: bench.Namespace,
			Labels: map[string]string{
				"bench":     bench.Name,
				"component": componentName,
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deploymentName,
			},
			MinReplicas: config.MinReplicas,
			MaxReplicas: *config.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceName(p.getHPAMetricName(config.HPA.Metric)),
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: config.HPA.TargetUtilization,
						},
					},
				},
			},
			Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
				ScaleUp: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: config.HPA.ScaleUpStabilization,
				},
				ScaleDown: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: config.HPA.ScaleDownStabilization,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, hpa, p.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &autoscalingv2.HorizontalPodAutoscaler{}
	err := p.client.Get(ctx, types.NamespacedName{Name: hpaName, Namespace: bench.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating HPA", "component", componentName, "name", hpaName)
			return p.client.Create(ctx, hpa)
		}
		return err
	}

	hpa.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("Updating HPA", "component", componentName, "name", hpaName)
	return p.client.Update(ctx, hpa)
}

func (p *HPAProvider) Delete(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, componentName string) error {
	logger := log.FromContext(ctx)
	hpaName := fmt.Sprintf("%s-%s", bench.Name, componentName)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	err := p.client.Get(ctx, types.NamespacedName{Name: hpaName, Namespace: bench.Namespace}, hpa)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	logger.Info("Deleting HPA", "component", componentName, "name", hpaName)
	return p.client.Delete(ctx, hpa)
}

func (p *HPAProvider) getHPAMetricName(metric string) string {
	if metric == "memory" {
		return "memory"
	}
	return "cpu"
}
