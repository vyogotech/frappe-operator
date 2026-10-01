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
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// FrappeBenchPoolReconciler reconciles a FrappeBenchPool object
type FrappeBenchPoolReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenchpools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenchpools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenchpools/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile manages pool scaling, ensuring warm shards and capacity are available
func (r *FrappeBenchPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("frappebenchpool", req.NamespacedName)

	pool := &vyogotechv1.FrappeBenchPool{}
	if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	policy := r.effectivePolicy(pool)

	// List FrappeBenches in the pool namespace
	var benchList vyogotechv1.FrappeBenchList
	if err := r.List(ctx, &benchList, client.InNamespace(pool.Namespace)); err != nil {
		logger.Error(err, "Failed to list FrappeBenches in namespace")
		return ctrl.Result{}, err
	}

	// Filter benches that belong to or match this pool
	matchingBenches := r.filterMatchingBenches(pool, policy.ShardPrefix, benchList.Items)

	// List FrappeSites to compute occupancy per bench
	var siteList vyogotechv1.FrappeSiteList
	if err := r.List(ctx, &siteList, client.InNamespace(pool.Namespace)); err != nil {
		logger.Error(err, "Failed to list FrappeSites in namespace")
		return ctrl.Result{}, err
	}

	siteCounts := make(map[string]int32)
	for _, site := range siteList.Items {
		if site.Spec.BenchRef != nil && site.Spec.BenchRef.Name != "" {
			siteCounts[site.Spec.BenchRef.Name]++
		}
	}

	// Build shard information & compute capacity statistics
	var readyShards int32
	var totalActiveSites int32
	var emptyReadyShards int32
	var inFlightShards int32
	shardInfos := make([]vyogotechv1.BenchShardInfo, 0, len(matchingBenches))

	for _, bench := range matchingBenches {
		count := siteCounts[bench.Name]
		totalActiveSites += count
		isReady := bench.Status.Phase == "Ready"
		if isReady {
			readyShards++
			if count == 0 {
				emptyReadyShards++
			}
		} else if bench.Status.Phase != "Failed" {
			inFlightShards++
		}

		atCapacity := count >= policy.MaxSitesPerShard
		shardInfos = append(shardInfos, vyogotechv1.BenchShardInfo{
			Name:       bench.Name,
			Phase:      bench.Status.Phase,
			SiteCount:  count,
			Capacity:   policy.MaxSitesPerShard,
			AtCapacity: atCapacity,
		})
	}

	// Sort shards alphabetically by name
	sort.Slice(shardInfos, func(i, j int) bool {
		return shardInfos[i].Name < shardInfos[j].Name
	})

	totalShards := int32(len(matchingBenches))

	// Determine if scaling is needed
	needsScale, reason := r.shouldScaleUp(totalShards, emptyReadyShards, matchingBenches, siteCounts, policy)

	if needsScale {
		if totalShards >= policy.MaxShards {
			logger.Info("Scale up desired but pool has reached MaxShards ceiling", "total", totalShards, "max", policy.MaxShards)
			r.setCondition(pool, "ScalingBlocked", metav1.ConditionTrue, "MaxShardsReached",
				fmt.Sprintf("Pool reached maximum allowed shards (%d)", policy.MaxShards))
		} else if inFlightShards > 0 {
			logger.Info("Scale up desired but a shard is currently initializing; waiting for readiness", "inFlight", inFlightShards)
			// Requeue in 15 seconds to check again once in-flight shard finishes
			return r.updateStatusAndReturn(ctx, pool, totalShards, readyShards, totalActiveSites, shardInfos, ctrl.Result{RequeueAfter: 15 * time.Second})
		} else if pool.Status.LastScaleTime != nil && time.Since(pool.Status.LastScaleTime.Time) < 2*time.Minute {
			waitRemaining := 2*time.Minute - time.Since(pool.Status.LastScaleTime.Time)
			logger.Info("Scale up in cooldown period", "remaining", waitRemaining)
			return r.updateStatusAndReturn(ctx, pool, totalShards, readyShards, totalActiveSites, shardInfos, ctrl.Result{RequeueAfter: waitRemaining})
		} else {
			// Execute scale up
			nextShardName := r.computeNextShardName(matchingBenches, policy.ShardPrefix)
			logger.Info("Scaling up FrappeBenchPool", "reason", reason, "newShard", nextShardName)

			if err := r.createShard(ctx, pool, nextShardName); err != nil {
				logger.Error(err, "Failed to create next shard bench", "shard", nextShardName)
				r.Recorder.Eventf(pool, corev1.EventTypeWarning, "ScaleUpFailed", "Failed to create shard %s: %v", nextShardName, err)
				return ctrl.Result{}, err
			}

			r.Recorder.Eventf(pool, corev1.EventTypeNormal, "ScaledUp", "Created new bench shard %s (reason: %s)", nextShardName, reason)
			pool.Status.LastScaleTime = &metav1.Time{Time: time.Now()}
			r.setCondition(pool, "Scaling", metav1.ConditionTrue, "ShardCreated", fmt.Sprintf("Provisioned shard %s", nextShardName))

			// Recheck shortly after creating shard
			return r.updateStatusAndReturn(ctx, pool, totalShards+1, readyShards, totalActiveSites, shardInfos, ctrl.Result{RequeueAfter: 20 * time.Second})
		}
	} else {
		r.setCondition(pool, "Scaling", metav1.ConditionFalse, "Optimal", "Pool capacity is within configured boundaries")
	}

	if readyShards > 0 {
		r.setCondition(pool, "Ready", metav1.ConditionTrue, "ShardsReady", fmt.Sprintf("%d/%d shards ready", readyShards, totalShards))
	} else {
		r.setCondition(pool, "Ready", metav1.ConditionFalse, "NoShardsReady", "No shards are currently in Ready state")
	}

	return r.updateStatusAndReturn(ctx, pool, totalShards, readyShards, totalActiveSites, shardInfos, ctrl.Result{RequeueAfter: 2 * time.Minute})
}

// effectivePolicy normalizes default values for ShardingPolicy
func (r *FrappeBenchPoolReconciler) effectivePolicy(pool *vyogotechv1.FrappeBenchPool) vyogotechv1.ShardingPolicy {
	p := pool.Spec.ShardingPolicy
	if p.MinShards < 1 {
		p.MinShards = 1
	}
	if p.MaxShards < p.MinShards {
		p.MaxShards = p.MinShards
	}
	if p.MaxSitesPerShard < 1 {
		p.MaxSitesPerShard = 25
	}
	if p.WatermarkPercent < 10 || p.WatermarkPercent > 100 {
		p.WatermarkPercent = 80
	}
	if p.BufferWarmShards < 0 {
		p.BufferWarmShards = 1
	}
	if p.ShardPrefix == "" {
		p.ShardPrefix = fmt.Sprintf("standard-v%s", pool.Spec.FrappeVersion)
	}
	return p
}

// filterMatchingBenches returns benches managed by or matching this pool
func (r *FrappeBenchPoolReconciler) filterMatchingBenches(pool *vyogotechv1.FrappeBenchPool, prefix string, benches []vyogotechv1.FrappeBench) []vyogotechv1.FrappeBench {
	var matching []vyogotechv1.FrappeBench
	for _, b := range benches {
		// Matches if owner is this pool
		isOwned := metav1.IsControlledBy(&b, pool)
		// Or matches pool label
		hasPoolLabel := b.Labels != nil && b.Labels["vyogo.tech/pool"] == pool.Name
		// Or matches shard prefix naming convention (allows adopting existing shards)
		hasPrefix := strings.HasPrefix(b.Name, prefix+"-")

		if isOwned || hasPoolLabel || hasPrefix {
			matching = append(matching, b)
		}
	}
	return matching
}

// shouldScaleUp determines if the pool needs another shard provisioned
func (r *FrappeBenchPoolReconciler) shouldScaleUp(totalShards, emptyReadyShards int32, matchingBenches []vyogotechv1.FrappeBench, siteCounts map[string]int32, policy vyogotechv1.ShardingPolicy) (bool, string) {
	if totalShards < policy.MinShards {
		return true, fmt.Sprintf("Current shard count (%d) is below minShards (%d)", totalShards, policy.MinShards)
	}

	if policy.BufferWarmShards > 0 && emptyReadyShards < policy.BufferWarmShards {
		return true, fmt.Sprintf("Empty warm shards (%d) is below bufferWarmShards (%d)", emptyReadyShards, policy.BufferWarmShards)
	}

	// Check if the most populated non-full shard has reached the watermark
	watermarkSites := int32((int64(policy.MaxSitesPerShard) * int64(policy.WatermarkPercent)) / 100)
	if watermarkSites < 1 {
		watermarkSites = 1
	}

	var maxOccupancyNonFull int32
	var maxOccupancyBench string
	for _, b := range matchingBenches {
		c := siteCounts[b.Name]
		if c < policy.MaxSitesPerShard && c > maxOccupancyNonFull {
			maxOccupancyNonFull = c
			maxOccupancyBench = b.Name
		}
	}

	if maxOccupancyNonFull >= watermarkSites {
		return true, fmt.Sprintf("Active shard %s has %d sites, reaching %d%% watermark threshold (%d sites)",
			maxOccupancyBench, maxOccupancyNonFull, policy.WatermarkPercent, watermarkSites)
	}

	return false, ""
}

// computeNextShardName generates the next available shard name formatted with %03d
func (r *FrappeBenchPoolReconciler) computeNextShardName(existing []vyogotechv1.FrappeBench, prefix string) string {
	maxIndex := 0
	prefixWithDash := prefix + "-"

	for _, b := range existing {
		if strings.HasPrefix(b.Name, prefixWithDash) {
			suffix := strings.TrimPrefix(b.Name, prefixWithDash)
			if idx, err := strconv.Atoi(suffix); err == nil && idx > maxIndex {
				maxIndex = idx
			}
		}
	}

	nextIndex := maxIndex + 1
	return fmt.Sprintf("%s-%03d", prefix, nextIndex)
}

// createShard constructs and applies a new FrappeBench child resource
func (r *FrappeBenchPoolReconciler) createShard(ctx context.Context, pool *vyogotechv1.FrappeBenchPool, shardName string) error {
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shardName,
			Namespace: pool.Namespace,
			Labels: map[string]string{
				"vyogo.tech/pool":           pool.Name,
				"vyogo.tech/bench-tier":     "standard",
				"vyogo.tech/frappe-version": pool.Spec.FrappeVersion,
			},
		},
		Spec: *pool.Spec.Template.DeepCopy(),
	}

	// Enforce that frappeVersion matches the pool
	bench.Spec.FrappeVersion = pool.Spec.FrappeVersion

	// Set ControllerReference so Kubernetes links the child to this pool
	if err := ctrl.SetControllerReference(pool, bench, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, bench)
}

// setCondition updates or adds a condition to the pool
func (r *FrappeBenchPoolReconciler) setCondition(pool *vyogotechv1.FrappeBenchPool, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: pool.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// updateStatusAndReturn updates status subresource and returns reconcile result
func (r *FrappeBenchPoolReconciler) updateStatusAndReturn(ctx context.Context, pool *vyogotechv1.FrappeBenchPool, total, ready, sites int32, shards []vyogotechv1.BenchShardInfo, result ctrl.Result) (ctrl.Result, error) {
	pool.Status.TotalShards = total
	pool.Status.ReadyShards = ready
	pool.Status.TotalActiveSites = sites
	pool.Status.Shards = shards
	pool.Status.ObservedGeneration = pool.Generation

	if err := r.Status().Update(ctx, pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FrappeBenchPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.FrappeBenchPool{}).
		Owns(&vyogotechv1.FrappeBench{}).
		Watches(
			&vyogotechv1.FrappeSite{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
				// When any FrappeSite changes, trigger reconcile on pools in the same namespace
				var poolList vyogotechv1.FrappeBenchPoolList
				if err := r.List(ctx, &poolList, client.InNamespace(obj.GetNamespace())); err != nil {
					return nil
				}
				requests := make([]ctrl.Request, 0, len(poolList.Items))
				for _, pool := range poolList.Items {
					requests = append(requests, ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      pool.Name,
							Namespace: pool.Namespace,
						},
					})
				}
				return requests
			}),
		).
		Complete(r)
}
