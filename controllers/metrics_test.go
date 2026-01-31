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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMetricsRegistration(t *testing.T) {
	// Test that metrics are properly registered
	assert.NotNil(t, ReconciliationDuration)
	assert.NotNil(t, ReconciliationErrors)
	assert.NotNil(t, JobStatus)
	assert.NotNil(t, ResourceTotal)
}

func TestReconciliationDuration(t *testing.T) {
	// Reset metric
	ReconciliationDuration.Reset()

	// Record a duration
	timer := prometheus.NewTimer(ReconciliationDuration.WithLabelValues("frappesite", "success"))
	timer.ObserveDuration()

	// Verify metric was recorded
	count := testutil.CollectAndCount(ReconciliationDuration)
	assert.Greater(t, count, 0, "ReconciliationDuration should have recorded at least one observation")
}

func TestReconciliationErrors(t *testing.T) {
	// Reset metric
	ReconciliationErrors.Reset()

	// Increment error counter
	ReconciliationErrors.WithLabelValues("frappesite", "initialization_error").Inc()

	// Verify metric was incremented
	count := testutil.CollectAndCount(ReconciliationErrors)
	assert.Equal(t, 1, count, "ReconciliationErrors should have one metric family")

	// Verify the value
	value := testutil.ToFloat64(ReconciliationErrors.WithLabelValues("frappesite", "initialization_error"))
	assert.Equal(t, float64(1), value, "Error counter should be 1")
}

func TestJobStatus(t *testing.T) {
	// Reset metric
	JobStatus.Reset()

	// Set job status
	JobStatus.WithLabelValues("frappesite", "default", "test-job", "succeeded").Set(2)

	// Verify metric was set
	value := testutil.ToFloat64(JobStatus.WithLabelValues("frappesite", "default", "test-job", "succeeded"))
	assert.Equal(t, float64(2), value, "Job status should be 2 (succeeded)")
}

func TestResourceTotal(t *testing.T) {
	// Reset metric
	ResourceTotal.Reset()

	// Set resource count
	ResourceTotal.WithLabelValues("frappesite", "default").Set(5)

	// Verify metric was set
	value := testutil.ToFloat64(ResourceTotal.WithLabelValues("frappesite", "default"))
	assert.Equal(t, float64(5), value, "Resource total should be 5")
}
