package controllers

import (
	"context"

	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// ScalingProvider defines the interface for scaling backend implementations
type ScalingProvider interface {
	// Ensure creates or updates scaling resources based on config
	Ensure(ctx context.Context, bench *vyogotechv1.FrappeBench, componentName string, deploymentName string, config *vyogotechv1.ComponentAutoscaling) error

	// Delete removes scaling resources for a component
	Delete(ctx context.Context, bench *vyogotechv1.FrappeBench, componentName string) error

	// IsAvailable checks if the provider is available in the cluster
	IsAvailable(ctx context.Context) bool

	// Name returns the provider name (keda, hpa, etc.)
	Name() string
}

// resolveProvider returns the appropriate ScalingProvider for the given name
func resolveProvider(providerName string, client client.Client, scheme *runtime.Scheme) ScalingProvider {
	switch providerName {
	case "keda":
		return &KEDAProvider{client: client, scheme: scheme}
	case "hpa":
		return &HPAProvider{client: client, scheme: scheme}
	default:
		return &HPAProvider{client: client, scheme: scheme} // Default to HPA (always available)
	}
}
