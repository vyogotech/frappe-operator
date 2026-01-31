/*
Copyright 2023 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use it except in compliance with the License.
*/

package v1alpha1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Ensure shared_types helpers are tested; ResourceRequirements uses corev1 for ResourceList

func TestDefaultComponentResources(t *testing.T) {
	cr := DefaultComponentResources()
	if cr.Gunicorn == nil {
		t.Fatal("DefaultComponentResources() Gunicorn should not be nil")
	}
	if cr.Gunicorn.Requests.Cpu().Cmp(resource.MustParse("100m")) != 0 {
		t.Errorf("Gunicorn CPU request expected 100m, got %s", cr.Gunicorn.Requests.Cpu().String())
	}
	if cr.Nginx == nil || cr.Scheduler == nil || cr.Socketio == nil {
		t.Error("DefaultComponentResources() should populate Nginx, Scheduler, Socketio")
	}
	if cr.WorkerDefault == nil || cr.WorkerLong == nil || cr.WorkerShort == nil {
		t.Error("DefaultComponentResources() should populate Worker*")
	}
}

func TestProductionComponentResources(t *testing.T) {
	cr := ProductionComponentResources()
	if cr.Gunicorn == nil {
		t.Fatal("ProductionComponentResources() Gunicorn should not be nil")
	}
	if cr.Gunicorn.Requests.Cpu().Cmp(resource.MustParse("500m")) != 0 {
		t.Errorf("Production Gunicorn CPU request expected 500m, got %s", cr.Gunicorn.Requests.Cpu().String())
	}
	if cr.Gunicorn.Limits.Memory().Cmp(resource.MustParse("4Gi")) != 0 {
		t.Errorf("Production Gunicorn memory limit expected 4Gi, got %s", cr.Gunicorn.Limits.Memory().String())
	}
}

func TestComponentResources_MergeWithDefaults(t *testing.T) {
	defaults := DefaultComponentResources()
	empty := ComponentResources{}
	merged := empty.MergeWithDefaults(defaults)
	if merged.Gunicorn == nil || merged.Gunicorn.Requests.Cpu().Cmp(resource.MustParse("100m")) != 0 {
		t.Error("MergeWithDefaults with empty should copy defaults")
	}

	// Override one component
	custom := ComponentResources{
		Gunicorn: &ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		},
	}
	merged2 := custom.MergeWithDefaults(defaults)
	if merged2.Gunicorn.Requests.Cpu().Cmp(resource.MustParse("500m")) != 0 {
		t.Errorf("MergeWithDefaults should prefer custom Gunicorn, got CPU %s", merged2.Gunicorn.Requests.Cpu().String())
	}
	if merged2.Nginx == nil || merged2.Nginx.Requests.Cpu().Cmp(resource.MustParse("50m")) != 0 {
		t.Error("MergeWithDefaults should keep default Nginx when not overridden")
	}
}

func TestMustParseQuantity(t *testing.T) {
	q := MustParseQuantity("100m")
	if q.Cmp(resource.MustParse("100m")) != 0 {
		t.Errorf("MustParseQuantity(100m) = %s", q.String())
	}
	q = MustParseQuantity("1Gi")
	if q.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Errorf("MustParseQuantity(1Gi) = %s", q.String())
	}
}
