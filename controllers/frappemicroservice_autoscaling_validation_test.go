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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// CEL rules for spec.deployment.autoscaling, run against the envtest API server.
var _ = Describe("FrappeMicroservice autoscaling validation", func() {
	var (
		ctx context.Context
		n   int
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	newMS := func(mutate func(ms *vyogotechv1.FrappeMicroservice)) *vyogotechv1.FrappeMicroservice {
		n++
		ms := &vyogotechv1.FrappeMicroservice{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ms-autoscale-%d", n), Namespace: "default"},
			Spec: vyogotechv1.FrappeMicroserviceSpec{
				SiteRef:    vyogotechv1.LocalSiteReference{Name: "acme-site"},
				Image:      vyogotechv1.ImageConfig{Repository: "ghcr.io/acme/orders", Tag: "1.0.0"},
				Deployment: &vyogotechv1.MicroserviceDeploymentSpec{},
			},
		}
		if mutate != nil {
			mutate(ms)
		}
		return ms
	}

	requests := func(cpu, memory string) *vyogotechv1.ResourceRequirements {
		rl := corev1.ResourceList{}
		if cpu != "" {
			rl[corev1.ResourceCPU] = resource.MustParse(cpu)
		}
		if memory != "" {
			rl[corev1.ResourceMemory] = resource.MustParse(memory)
		}
		return &vyogotechv1.ResourceRequirements{Requests: rl}
	}

	autoscaling := func(a *vyogotechv1.MicroserviceAutoscaling) func(ms *vyogotechv1.FrappeMicroservice) {
		return func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Deployment.Autoscaling = a }
	}

	redisKEDA := func() *vyogotechv1.MicroserviceKEDA {
		return &vyogotechv1.MicroserviceKEDA{Triggers: []vyogotechv1.MicroserviceKEDATrigger{{
			Type: "redis", Metadata: map[string]string{"address": "redis-queue:6379", "listName": "rq:queue:default", "listLength": "50"},
		}}}
	}

	expectRejected := func(ms *vyogotechv1.FrappeMicroservice, fragment string) {
		err := k8sClient.Create(ctx, ms)
		Expect(err).To(HaveOccurred(), "expected the API server to reject %+v", ms.Spec.Deployment.Autoscaling)
		Expect(err.Error()).To(ContainSubstring(fragment))
	}

	It("accepts CPU and memory targets with matching requests", func() {
		ms := newMS(func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Resources = requests("200m", "256Mi")
			autoscaling(&vyogotechv1.MicroserviceAutoscaling{
				MinReplicas: int32Ptr(2), MaxReplicas: 10,
				TargetCPUUtilizationPercentage: int32Ptr(70), TargetMemoryUtilizationPercentage: int32Ptr(80),
			})(ms)
		})
		Expect(k8sClient.Create(ctx, ms)).To(Succeed())
	})

	It("accepts a KEDA ScaledObject that scales to zero", func() {
		Expect(k8sClient.Create(ctx, newMS(autoscaling(&vyogotechv1.MicroserviceAutoscaling{
			MinReplicas: int32Ptr(0), MaxReplicas: 20, Keda: redisKEDA(),
		})))).To(Succeed())
	})

	It("accepts KEDA without resource requests", func() {
		Expect(k8sClient.Create(ctx, newMS(autoscaling(&vyogotechv1.MicroserviceAutoscaling{
			MaxReplicas: 20, Keda: redisKEDA(),
		})))).To(Succeed())
	})

	It("rejects autoscaling on a Job", func() {
		ms := newMS(func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Type = vyogotechv1.MicroserviceTypeJob
			ms.Spec.Command = []string{"python", "-m", "task"}
			ms.Spec.Resources = requests("200m", "")
			autoscaling(&vyogotechv1.MicroserviceAutoscaling{MaxReplicas: 3, TargetCPUUtilizationPercentage: int32Ptr(70)})(ms)
		})
		expectRejected(ms, "deployment is only valid when type is Deployment")
	})

	It("rejects minReplicas above maxReplicas", func() {
		ms := newMS(func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Resources = requests("200m", "")
			autoscaling(&vyogotechv1.MicroserviceAutoscaling{MinReplicas: int32Ptr(5), MaxReplicas: 3, TargetCPUUtilizationPercentage: int32Ptr(70)})(ms)
		})
		expectRejected(ms, "minReplicas must not exceed maxReplicas")
	})

	It("rejects minReplicas 0 for an HPA", func() {
		ms := newMS(func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Resources = requests("200m", "")
			autoscaling(&vyogotechv1.MicroserviceAutoscaling{MinReplicas: int32Ptr(0), MaxReplicas: 3, TargetCPUUtilizationPercentage: int32Ptr(70)})(ms)
		})
		expectRejected(ms, "minReplicas must be at least 1 unless keda is set")
	})

	It("rejects autoscaling without any metric or trigger", func() {
		expectRejected(newMS(autoscaling(&vyogotechv1.MicroserviceAutoscaling{MaxReplicas: 3})),
			"set a CPU or memory utilization target, or keda with at least one trigger")
	})

	It("rejects keda without triggers", func() {
		expectRejected(newMS(autoscaling(&vyogotechv1.MicroserviceAutoscaling{
			MaxReplicas: 3, Keda: &vyogotechv1.MicroserviceKEDA{},
		})), "triggers")
	})

	It("rejects keda combined with a CPU target", func() {
		ms := newMS(func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Resources = requests("200m", "")
			autoscaling(&vyogotechv1.MicroserviceAutoscaling{MaxReplicas: 3, TargetCPUUtilizationPercentage: int32Ptr(70), Keda: redisKEDA()})(ms)
		})
		expectRejected(ms, "keda cannot be combined with CPU or memory targets")
	})

	It("rejects a CPU target without resources.requests.cpu", func() {
		ms := newMS(func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Resources = requests("", "256Mi")
			autoscaling(&vyogotechv1.MicroserviceAutoscaling{MaxReplicas: 3, TargetCPUUtilizationPercentage: int32Ptr(70)})(ms)
		})
		expectRejected(ms, "targetCPUUtilizationPercentage requires resources.requests.cpu")
	})

	It("rejects a CPU target without any resources", func() {
		expectRejected(newMS(autoscaling(&vyogotechv1.MicroserviceAutoscaling{MaxReplicas: 3, TargetCPUUtilizationPercentage: int32Ptr(70)})),
			"targetCPUUtilizationPercentage requires resources.requests.cpu")
	})

	It("rejects a memory target without resources.requests.memory", func() {
		ms := newMS(func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Resources = requests("200m", "")
			autoscaling(&vyogotechv1.MicroserviceAutoscaling{MaxReplicas: 3, TargetMemoryUtilizationPercentage: int32Ptr(80)})(ms)
		})
		expectRejected(ms, "targetMemoryUtilizationPercentage requires resources.requests.memory")
	})

	It("rejects maxReplicas below 1", func() {
		ms := newMS(func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Resources = requests("200m", "")
			autoscaling(&vyogotechv1.MicroserviceAutoscaling{MaxReplicas: 0, TargetCPUUtilizationPercentage: int32Ptr(70)})(ms)
		})
		expectRejected(ms, "maxReplicas")
	})
})
