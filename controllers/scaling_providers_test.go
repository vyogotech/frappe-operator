package controllers

import (
	"context"
	"testing"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = vyogotechv1alpha1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	tests := []struct {
		name         string
		providerName string
		expectedName string
	}{
		{
			name:         "resolve keda",
			providerName: "keda",
			expectedName: "keda",
		},
		{
			name:         "resolve hpa",
			providerName: "hpa",
			expectedName: "hpa",
		},
		{
			name:         "resolve unknown defaults to hpa",
			providerName: "unknown",
			expectedName: "hpa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := resolveProvider(tt.providerName, client, scheme)
			if p.Name() != tt.expectedName {
				t.Errorf("resolveProvider() = %v, want %v", p.Name(), tt.expectedName)
			}
		})
	}
}

func TestKEDAProvider_IsAvailable(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = vyogotechv1alpha1.AddToScheme(scheme)

	// Since we use unstructured to check KEDA availability, the fake client
	// might return an error if the resource is not registered in the scheme.
	// We want it to returned an error so IsAvailable returns false.
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := &KEDAProvider{client: client}

	if p.IsAvailable(context.TODO()) {
		t.Error("Expected KEDA not to be available in fake client (resource not in scheme)")
	}
}

func TestHPAProvider_IsAvailable(t *testing.T) {
	p := &HPAProvider{}
	if !p.IsAvailable(context.TODO()) {
		t.Error("Expected HPA to always be available")
	}
}

func TestGetHPAMetricName(t *testing.T) {
	p := &HPAProvider{}
	tests := []struct {
		metric   string
		expected string
	}{
		{"memory", "memory"},
		{"cpu", "cpu"},
		{"unknown", "cpu"},
	}

	for _, tt := range tests {
		if name := p.getHPAMetricName(tt.metric); name != tt.expected {
			t.Errorf("getHPAMetricName(%s) = %s, want %s", tt.metric, name, tt.expected)
		}
	}
}
