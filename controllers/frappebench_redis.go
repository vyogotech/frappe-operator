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

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureRedis ensures the Redis StatefulSet and Service exist
func (r *FrappeBenchReconciler) ensureRedis(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	// Create redis-cache and redis-queue services (socketio not needed for v15+)
	if err := r.ensureRedisService(ctx, bench, "redis-cache"); err != nil {
		return err
	}
	if err := r.ensureRedisService(ctx, bench, "redis-queue"); err != nil {
		return err
	}
	if err := r.ensureRedisStatefulSet(ctx, bench, "redis-cache"); err != nil {
		return err
	}
	return r.ensureRedisStatefulSet(ctx, bench, "redis-queue")
}

func (r *FrappeBenchReconciler) ensureRedisService(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, serviceType string) error {
	logger := log.FromContext(ctx)

	svcName := fmt.Sprintf("%s-%s", bench.Name, serviceType)
	svc := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: bench.Namespace}, svc)
	if err == nil {
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Redis Service", "service", svcName, "type", serviceType)

	svc, err = resources.NewServiceBuilder(svcName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
		WithSelector(r.componentLabels(bench, fmt.Sprintf("redis-%s", serviceType))).
		WithPort("redis", 6379, 6379).
		WithOwner(bench, r.Scheme).
		Build()
	if err != nil {
		return err
	}

	return r.Create(ctx, svc)
}

func (r *FrappeBenchReconciler) ensureRedisStatefulSet(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, role string) error {
	logger := log.FromContext(ctx)

	stsName := fmt.Sprintf("%s-%s", bench.Name, role)
	sts := &appsv1.StatefulSet{}

	err := r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: bench.Namespace}, sts)
	existing := err == nil
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if !existing {
		logger.Info("Creating Redis StatefulSet", "statefulset", stsName)
	}

	replicas := int32(1)
	redisImage := r.getRedisImage(bench)

	container := resources.NewContainerBuilder("redis", redisImage).
		WithCommand("redis-server").
		WithArgs("--save", "", "--appendonly", "no", "--stop-writes-on-bgsave-error", "no").
		WithPort("redis", 6379).
		WithResources(r.getRedisResources(bench)).
		WithSecurityContext(r.getRedisContainerSecurityContext(bench)).
		Build()

	newSts, err := resources.NewStatefulSetBuilder(stsName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
		WithSelector(r.componentLabels(bench, fmt.Sprintf("redis-%s", role))).
		WithServiceName(stsName).
		WithReplicas(replicas).
		WithPodSecurityContext(r.getRedisPodSecurityContext(bench)).
		WithContainer(container).
		WithOwner(bench, r.Scheme).
		Build()
	if err != nil {
		return err
	}

	if !existing {
		return r.Create(ctx, newSts)
	}

	// Update existing StatefulSet - Metadata and Template are mutable
	sts.Labels = newSts.Labels
	sts.Spec.Replicas = newSts.Spec.Replicas
	sts.Spec.Template = newSts.Spec.Template
	// Do NOT overwrite Spec.Selector as it is immutable
	return r.Update(ctx, sts)
}

func (r *FrappeBenchReconciler) getRedisAddress(bench *vyogotechv1alpha1.FrappeBench) string {
	return fmt.Sprintf("%s-redis-cache:6379", bench.Name)
}
