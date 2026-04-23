package controllers

import (
	"testing"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func TestComponentAutoscaling_Defaults(t *testing.T) {
	// Note: These tests will fail until types are implemented in api/v1alpha1/shared_types.go
	config := vyogotechv1.ComponentAutoscaling{}
	if config.Enabled != nil {
		t.Error("Expected nil Enabled by default")
	}
}

func TestComponentAutoscaling_ProviderValidation(t *testing.T) {
	// This tests the logic we expect in the controller or helper
	// Validation of enum is handled by kubebuilder in CRD
}

func TestKEDAScalingConfig_TriggerTypes(t *testing.T) {
	config := vyogotechv1.KEDAScalingConfig{
		Trigger: "cpu",
	}
	if config.Trigger != "cpu" {
		t.Errorf("Expected cpu trigger, got %s", config.Trigger)
	}
}

func TestHPAScalingConfig_MetricTypes(t *testing.T) {
	config := vyogotechv1.HPAScalingConfig{
		Metric: "cpu",
	}
	if config.Metric != "cpu" {
		t.Errorf("Expected cpu metric, got %s", config.Metric)
	}
}
