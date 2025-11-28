<!-- 2b8d6906-e38f-4037-b948-a4d239e25f75 a11d18fd-03ba-4950-a394-a6fd5062a273 -->
# Serverless Worker Scaling Implementation (Loosely Coupled)

## Overview

Add KEDA-based autoscaling to FrappeBench workers using dynamic `Unstructured` resources. This avoids hard dependencies on the KEDA Go client while enabling full autoscaling capabilities.

## Implementation Steps

### 1. Extend FrappeBench CRD with Autoscaling Configuration

**File**: `api/v1alpha1/shared_types.go`

Add new `WorkerAutoscaling` type:

```go
// WorkerAutoscaling defines autoscaling configuration for a worker type
type WorkerAutoscaling struct {
    // Enabled controls whether autoscaling is active
    // +optional
    // +kubebuilder:default=true
    Enabled *bool `json:"enabled,omitempty"`
    
    // MinReplicas is the minimum number of replicas (can be 0 for serverless)
    // +optional
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:default=0
    MinReplicas *int32 `json:"minReplicas,omitempty"`
    
    // MaxReplicas is the maximum number of replicas
    // +optional
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:default=10
    MaxReplicas *int32 `json:"maxReplicas,omitempty"`
    
    // QueueLength is the number of jobs in queue that triggers scaling
    // +optional
    // +kubebuilder:default=5
    QueueLength *int32 `json:"queueLength,omitempty"`
    
    // CooldownPeriod is the wait time before scaling down (in seconds)
    // +optional
    // +kubebuilder:default=60
    CooldownPeriod *int32 `json:"cooldownPeriod,omitempty"`
    
    // PollingInterval is how often KEDA checks queue depth (in seconds)
    // +optional
    // +kubebuilder:default=30
    PollingInterval *int32 `json:"pollingInterval,omitempty"`
}
```

Add to `FrappeBenchSpec`:

```go
// WorkerAutoscalingConfig defines autoscaling for each worker type
// +optional
WorkerAutoscalingConfig *WorkerAutoscalingConfig `json:"workerAutoscalingConfig,omitempty"`
```

Add `WorkerAutoscalingConfig` type:

```go
// WorkerAutoscalingConfig defines autoscaling per worker type
type WorkerAutoscalingConfig struct {
    // Short worker autoscaling
    // +optional
    Short *WorkerAutoscaling `json:"short,omitempty"`
    
    // Long worker autoscaling
    // +optional
    Long *WorkerAutoscaling `json:"long,omitempty"`
    
    // Default worker autoscaling
    // +optional
    Default *WorkerAutoscaling `json:"default,omitempty"`
}
```

### 2. Add Helper Functions for Autoscaling Config

**File**: `controllers/frappebench_resources.go`

Implement `getWorkerAutoscalingConfig` and `getDefaultAutoscalingConfig` to return serverless-first defaults (Short/Long enabled, Default disabled).

### 3. Modify Worker Deployment Logic

**File**: `controllers/frappebench_resources.go`

Update `ensureWorkerDeployment` to:

1. Check autoscaling config.
2. If enabled, set `Spec.Replicas` to `MinReplicas`.
3. Add `keda.sh/managed-by: frappe-operator` annotation.

### 4. Implement Unstructured ScaledObject Creation

**File**: `controllers/frappebench_resources.go`

Add function to create/update ScaledObject using `unstructured.Unstructured`:

```go
func (r *FrappeBenchReconciler) ensureScaledObject(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, workerType string) error {
    logger := log.FromContext(ctx)
    autoscaling := r.getWorkerAutoscalingConfig(bench, workerType)

    if !*autoscaling.Enabled {
        return r.deleteScaledObjectIfExists(ctx, bench, workerType)
    }

    scaledObjectName := fmt.Sprintf("%s-worker-%s", bench.Name, workerType)
    deploymentName := fmt.Sprintf("%s-worker-%s", bench.Name, workerType)
    queueName := fmt.Sprintf("rq:queue:%s", workerType)
    redisServiceName := fmt.Sprintf("%s-redis-queue", bench.Name)

    // Construct Unstructured object
    scaledObject := &unstructured.Unstructured{
        Object: map[string]interface{}{
            "apiVersion": "keda.sh/v1alpha1",
            "kind":       "ScaledObject",
            "metadata": map[string]interface{}{
                "name":      scaledObjectName,
                "namespace": bench.Namespace,
                "labels":    r.benchLabels(bench),
            },
            "spec": map[string]interface{}{
                "scaleTargetRef": map[string]interface{}{
                    "name": deploymentName,
                },
                "minReplicaCount": int64(*autoscaling.MinReplicas),
                "maxReplicaCount": int64(*autoscaling.MaxReplicas),
                "cooldownPeriod":  int64(*autoscaling.CooldownPeriod),
                "pollingInterval": int64(*autoscaling.PollingInterval),
                "triggers": []interface{}{
                    map[string]interface{}{
                        "type": "redis",
                        "metadata": map[string]string{
                            "address":       fmt.Sprintf("%s.%s.svc.cluster.local:6379", redisServiceName, bench.Namespace),
                            "listName":      queueName,
                            "listLength":    fmt.Sprintf("%d", *autoscaling.QueueLength),
                            "databaseIndex": "0",
                        },
                    },
                },
            },
        },
    }

    // Set owner reference
    if err := controllerutil.SetControllerReference(bench, scaledObject, r.Scheme); err != nil {
        return err
    }

    // Server-side apply or Create/Update logic
    // Using standard Create/Update logic with Unstructured works with the standard client
    found := &unstructured.Unstructured{}
    found.SetGroupVersionKind(schema.GroupVersionKind{
        Group:   "keda.sh",
        Version: "v1alpha1",
        Kind:    "ScaledObject",
    })
    
    err := r.Get(ctx, types.NamespacedName{Name: scaledObjectName, Namespace: bench.Namespace}, found)
    if err != nil {
        if errors.IsNotFound(err) {
            logger.Info("Creating ScaledObject", "name", scaledObjectName)
            return r.Create(ctx, scaledObject)
        }
        // If error is "no kind is registered for version", KEDA is not installed
        if meta.IsNoMatchError(err) {
             logger.Info("KEDA not installed, skipping ScaledObject creation")
             return nil
        }
        return err
    }

    // Update logic: Update spec fields if changed
    // For simplicity in this plan, we can just update the spec
    found.Object["spec"] = scaledObject.Object["spec"]
    return r.Update(ctx, found)
}
```

### 5. Update Reconciliation Loop

**File**: `controllers/frappebench_resources.go`

Update `ensureWorkers` to call `ensureScaledObject` for each worker type.

### 6. RBAC Configuration

**File**: `config/rbac/role.yaml` & `helm/frappe-operator/templates/rbac/clusterrole.yaml`

Add permissions for KEDA resources (even without Go types, the operator needs permission to access the API):

```yaml
- apiGroups: ["keda.sh"]
  resources: ["scaledobjects"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### 7. Documentation & Migration

**File**: `docs/SERVERLESS_WORKERS.md`

- Document the loose coupling approach (requires KEDA 2.x installed in cluster).
- Explain that if KEDA is missing, the operator will log a warning but proceed (static replicas only).

**Migration Notes**:

- Default behavior: Serverless enabled.
- If KEDA is not installed, it gracefully degrades to static replicas (minReplicas).

## Testing Plan

1.  **Unit Tests**: Mock client interactions with Unstructured objects.
2.  **Integration**:

    -   Install KEDA in minikube.
    -   Deploy bench.
    -   Verify ScaledObject created with correct fields.
    -   Uninstall KEDA -> Verify operator logs warning but doesn't crash.

## Dependencies

-   `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured` (Standard K8s lib)
-   No external KEDA Go modules.

### To-dos

- [ ] Clean up Kind cluster completely
- [ ] Compile operator with domain detection
- [ ] Deploy operator to clean cluster
- [ ] Test FrappeBench creation - ✅ ALL RESOURCES CREATED!
- [ ] Test FrappeSite creation - ✅ SITE READY + INGRESS CREATED!
- [ ] Test multiple sites - ✅ 2 sites running with separate configs!
- [ ] Update documentation
- [ ] Create release v1.0.0
- [ ] Fix RWX PVC issue for Kind testing - ✅ ALL PODS RUNNING
- [ ] Implement 2 separate Redis StatefulSets - ✅ COMPLETE!
- [ ] Integrate MariaDB Operator for database management
- [ ] Verify production entry points for all components
- [ ] Add validation for site name and domain matching
- [ ] Create comprehensive integration documentation
- [ ] Deploy and test with MariaDB Operator
- [ ] Verify end-to-end site functionality with database
- [ ] Extend FrappeBench CRD with Autoscaling Configuration in api/v1alpha1/shared_types.go
- [ ] Add Helper Functions for Autoscaling Config in controllers/frappebench_resources.go
- [ ] Modify Worker Deployment Logic in controllers/frappebench_resources.go
- [ ] Implement Unstructured ScaledObject Creation in controllers/frappebench_resources.go
- [ ] Update Reconciliation Loop in controllers/frappebench_resources.go
- [ ] Add RBAC Configuration for KEDA resources
- [ ] Update Documentation and Migration Notes
- [ ] Verify implementation with tests