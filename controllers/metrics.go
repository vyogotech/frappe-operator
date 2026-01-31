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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ReconciliationDuration tracks how long reconciliation takes
	ReconciliationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "frappe_operator_reconciliation_duration_seconds",
			Help:    "Duration of reconciliation operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller", "result"},
	)

	// ReconciliationErrors counts reconciliation errors
	ReconciliationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "frappe_operator_reconciliation_errors_total",
			Help: "Total number of reconciliation errors",
		},
		[]string{"controller", "error_type"},
	)

	// JobStatus tracks job statuses (succeeded, failed, active)
	JobStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "frappe_operator_job_status",
			Help: "Current status of jobs (1=active, 2=succeeded, 3=failed)",
		},
		[]string{"controller", "namespace", "name", "status"},
	)

	// ResourceTotal tracks total resources managed
	ResourceTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "frappe_operator_resources_total",
			Help: "Total number of resources managed by the operator",
		},
		[]string{"controller", "namespace"},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		ReconciliationDuration,
		ReconciliationErrors,
		JobStatus,
		ResourceTotal,
	)
}
