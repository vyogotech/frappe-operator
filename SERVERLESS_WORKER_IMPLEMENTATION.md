# Serverless Worker Scaling Implementation

This document describes the implementation of hybrid worker scaling for the Frappe Operator, which allows users to choose between KEDA-based autoscaling (including scale-to-zero) or static replica counts for each worker type.

## Overview

The implementation provides a **hybrid scaling approach** where users have full control over how each worker type scales:

1. **KEDA Autoscaling**: Workers can automatically scale based on Redis queue depth, including scale-to-zero when idle
2. **Static Replicas**: Workers can run with a fixed number of replicas
3. **Mixed Mode**: Different worker types can use different scaling strategies in the same bench

## Architecture

### Key Components

1. **CRD Updates** (`api/v1alpha1/`)

   - `WorkerAutoscaling`: Configuration for a single worker's scaling behavior
   - `WorkerAutoscalingConfig`: Per-worker-type configuration (short, long, default)
   - `WorkerScalingStatus`: Reports current scaling mode and replica counts
   - Backward compatible with deprecated `ComponentReplicas` fields

2. **Controller Logic** (`controllers/`)

   - Helper functions for configuration resolution and defaults
   - KEDA availability detection
   - ScaledObject creation/management using Unstructured resources
   - Worker deployment updates with scaling annotations
   - Status reporting

3. **RBAC Configuration** (`config/rbac/`)
   - Permissions to manage `keda.sh/scaledobjects`

## Implementation Details

### Worker Types

- **Short**: Quick tasks (e.g., email, notifications)
  - Default: Scale-to-zero, max 10 replicas, aggressive scaling
- **Long**: Heavy tasks (e.g., reports, imports)
  - Default: Scale-to-zero, max 5 replicas, conservative scaling
- **Default/Scheduler**: Scheduled tasks
  - Default: Static 1 replica (scheduler must always run)

### Configuration Resolution

The operator resolves worker configuration in this priority order:

1. `WorkerAutoscaling.{short|long|default}` - New hybrid configuration
2. `ComponentReplicas.worker{Short|Long|Default}` - Legacy configuration (converted to static)
3. Opinionated defaults based on worker type

### KEDA Integration

- **Loose Coupling**: No Go module dependency on KEDA
- **Graceful Fallback**: If KEDA is not installed, automatically uses static replicas
- **Unstructured Resources**: ScaledObjects created using `unstructured.Unstructured`
- **Redis Scaler**: Monitors Redis queue depth (e.g., `rq:queue:short`)

### ScaledObject Spec

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: {bench-name}-worker-{type}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {bench-name}-worker-{type}
  minReplicaCount: {minReplicas}
  maxReplicaCount: {maxReplicas}
  cooldownPeriod: {cooldownPeriod}
  pollingInterval: {pollingInterval}
  triggers:
    - type: redis
      metadata:
        address: {redis-host}:6379
        listName: rq:queue:{type}
        listLength: "{queueLength}"
        enableTLS: "false"
        databaseIndex: "0"
        activationListLength: "1"
```

### Status Reporting

The `WorkerScaling` status field reports per-worker information:

```yaml
status:
  workerScaling:
    short:
      mode: autoscaled # "autoscaled" or "static"
      currentReplicas: 3
      desiredReplicas: 5
      kedaManaged: true
    long:
      mode: static
      currentReplicas: 2
      desiredReplicas: 2
      kedaManaged: false
    default:
      mode: static
      currentReplicas: 1
      desiredReplicas: 1
      kedaManaged: false
```

## Files Modified

### API Types

- `api/v1alpha1/shared_types.go`

  - Added `WorkerAutoscaling` struct (7 fields)
  - Added `WorkerAutoscalingConfig` struct
  - Deprecated worker replica fields in `ComponentReplicas`

- `api/v1alpha1/frappebench_types.go`
  - Added `WorkerAutoscaling` field to `FrappeBenchSpec`
  - Added `WorkerScalingStatus` struct
  - Added `WorkerScaling` map to `FrappeBenchStatus`

### Controllers

- `controllers/frappebench_resources.go`

  - `getWorkerAutoscalingConfig()`: Config resolution with legacy fallback
  - `getDefaultAutoscalingConfig()`: Opinionated defaults per worker type
  - `fillAutoscalingDefaults()`: Fill missing config fields
  - `getWorkerReplicaCount()`: Determine replicas based on mode
  - `isKEDAAvailable()`: Check if KEDA CRDs are installed
  - `ensureScaledObject()`: Create/update KEDA ScaledObject
  - `deleteScaledObjectIfExists()`: Clean up ScaledObject
  - `getRedisAddress()`: Get Redis connection string
  - `ensureWorkers()`: Updated to support hybrid scaling
  - `ensureWorkerDeployment()`: Updated with scaling annotations

- `controllers/frappebench_controller.go`
  - `updateWorkerScalingStatus()`: Update status with scaling info
  - Updated reconciliation loop to call ScaledObject management

### RBAC

- `config/rbac/role.yaml`
  - Added permissions for `keda.sh` API group
  - Added `scaledobjects`, `scaledobjects/finalizers`, `scaledobjects/status`

### Documentation

- `examples/worker-autoscaling.yaml`
  - Multiple examples: KEDA, static, legacy, mixed mode

## Configuration Examples

### KEDA Autoscaling (Scale-to-Zero)

```yaml
workerAutoscaling:
  short:
    enabled: true
    minReplicas: 0
    maxReplicas: 10
    queueLength: 5
    cooldownPeriod: 60
    pollingInterval: 15
```

### Static Replicas

```yaml
workerAutoscaling:
  short:
    enabled: false
    staticReplicas: 2
```

### Mixed Mode

```yaml
workerAutoscaling:
  short:
    enabled: true # Autoscale
    minReplicas: 0
    maxReplicas: 10
  long:
    enabled: false # Static
    staticReplicas: 2
  default:
    enabled: false # Static
    staticReplicas: 1
```

### Legacy (Backward Compatible)

```yaml
componentReplicas:
  workerShort: 2
  workerLong: 1
  workerDefault: 1
```

## Default Values

If no configuration is provided, the operator uses these defaults:

| Worker Type | Enabled | Min | Max | Queue Length | Cooldown | Polling |
| ----------- | ------- | --- | --- | ------------ | -------- | ------- |
| short       | true    | 0   | 10  | 5            | 60s      | 15s     |
| long        | true    | 0   | 5   | 2            | 300s     | 30s     |
| default     | false   | -   | -   | -            | -        | -       |

Default worker always uses `staticReplicas: 1` since the scheduler must run continuously.

## Behavior

### With KEDA Installed

- Workers configured with `enabled: true` will autoscale based on queue depth
- ScaledObjects are created/updated for autoscaled workers
- Deployments are annotated with `keda.sh/managed-by: keda`
- Operator does not update replica count (KEDA controls it)

### Without KEDA

- All workers use `staticReplicas` regardless of `enabled` setting
- No ScaledObjects are created
- Deployments are annotated with `frappe.io/scaling-mode: static`
- Operator manages replica count directly

### Mode Transitions

- **KEDA Disabled → Enabled**: ScaledObject created, operator stops managing replicas
- **KEDA Enabled → Disabled**: ScaledObject deleted, operator resumes managing replicas
- **KEDA Uninstalled**: Automatic fallback to static replicas

## Annotations

Deployments are annotated to indicate scaling mode:

```yaml
metadata:
  annotations:
    frappe.io/scaling-mode: "autoscaled" # or "static"
    keda.sh/managed-by: "keda" # only when autoscaled
```

## Migration Path

1. **Existing Users**: No changes required
   - Legacy `ComponentReplicas` continues to work
   - Automatically converted to static replicas
2. **New Features**: Opt-in
   - Users can add `WorkerAutoscaling` to enable KEDA
   - Can migrate gradually (one worker type at a time)

## Testing Checklist

- [x] Deploy bench with KEDA autoscaling enabled
- [x] Verify ScaledObjects are created
- [x] Verify workers scale to zero when idle
- [x] Verify workers scale up when jobs added
- [x] Deploy bench with static replicas (long and default workers)
- [x] Verify fixed replica counts maintained
- [x] Deploy bench with mixed mode
- [x] Verify some workers autoscale, others static
- [ ] Deploy bench with legacy ComponentReplicas
- [ ] Verify backward compatibility
- [ ] Uninstall KEDA during operation
- [ ] Verify graceful fallback to static
- [x] Check status reporting accuracy

## Test Results (Minikube - 28 Nov 2025)

### Environment
- **Cluster**: Minikube on M1 Mac (ARM)
- **Kubernetes**: v1.31
- **KEDA**: v2.16.1
- **Operator Image**: localhost/frappe-operator:dev

### Test Scenario: Mixed Mode (Hybrid Scaling)
```yaml
workerAutoscaling:
  short:
    enabled: true
    minReplicas: 0
    maxReplicas: 3
    queueLength: 2
    cooldownPeriod: 30
    pollingInterval: 10
  long:
    enabled: false
    staticReplicas: 1
  default:
    enabled: false
    staticReplicas: 1
```

### Observed Behavior

#### ✅ ScaledObject Creation
- ScaledObject `test-autoscaling-worker-short` created successfully
- Status: `READY: True, ACTIVE: False`
- Correctly configured Redis trigger targeting `rq:queue:short`
- Uses FQDN: `test-autoscaling-redis-queue.frappe-test.svc.cluster.local:6379`

#### ✅ Scale-to-Zero
- Short worker deployment started at 0/0 replicas
- No pods running when queue is empty
- KEDA correctly reports Active: False

#### ✅ Scale-Up
Timeline:
- **t=0s**: Queue empty, replicas=0
- **t=0s**: Added 5 jobs to queue
- **t=5s**: Scaled to 1 replica (first pod starting)
- **t=10s**: 1 pod ready
- **t=20s**: Scaled to 3 replicas (max reached)
- **t=25s**: All 3 pods ready

Scaling was responsive and reached maxReplicas (3) as expected based on queueLength threshold of 2.

#### ✅ Scale-Down
Timeline:
- **t=0s**: Queue cleared (DEL rq:queue:short)
- **t=0-30s**: Replicas remained at 3 (cooldown period)
- **t=35s**: Scaled to 0 replicas
- **t=35s+**: 0 pods running

Scale-down occurred ~5 seconds after the 30-second cooldown period, which is expected given the 10-second pollingInterval.

#### ✅ Static Workers
- Long worker: Maintained 1/1 replicas throughout test
- Default worker: Maintained 1/1 replicas throughout test
- No ScaledObjects created for static workers

#### ✅ Status Reporting
```yaml
status:
  workerScaling:
    short:
      mode: autoscaled
      currentReplicas: 0
      desiredReplicas: 0
      kedaManaged: true
    long:
      mode: static
      currentReplicas: 1
      desiredReplicas: 1
      kedaManaged: false
    default:
      mode: static
      currentReplicas: 1
      desiredReplicas: 1
      kedaManaged: false
```

All fields accurately reflected the actual state of the workers.

### Issues Encountered and Fixed

1. **Type Conversion Panic**: Initial deployment failed with "cannot deep copy int"
   - **Fix**: Changed `int()` to `int64()` for numeric fields in unstructured ScaledObject spec
   
2. **Slice Type Panic**: Second attempt failed with "cannot deep copy []map[string]interface{}"
   - **Fix**: Changed triggers to `[]interface{}` and metadata to `map[string]interface{}`

3. **Redis DNS Resolution**: KEDA couldn't resolve `{bench-name}-redis-queue`
   - **Fix**: Used fully qualified domain name (FQDN) for cross-namespace access

### Conclusions

✅ **All Core Features Working**:
- KEDA autoscaling with scale-to-zero
- Static replica management
- Mixed mode (hybrid scaling)
- Status reporting
- ScaledObject lifecycle management

✅ **Performance**: 
- Scale-up latency: ~5-10 seconds to first pod
- Scale-down respects cooldown period accurately
- Polling interval effective at 10 seconds

✅ **Reliability**:
- No crashes or reconciliation errors
- ScaledObjects properly owned by FrappeBench
- Graceful handling of empty queues

## Known Limitations

1. **Redis Address**: Currently assumes in-cluster Redis service
   - TODO: Read ConnectionSecretRef for external Redis
2. **KEDA Detection**: Simple CRD existence check

   - Could be enhanced with version checking

3. **Metrics**: No Prometheus metrics yet
   - Could add autoscaling events/metrics

## Future Enhancements

1. Support for external Redis (read ConnectionSecretRef)
2. Custom scaling metrics beyond queue depth
3. Per-worker scaling metrics in Prometheus
4. HPA support as alternative to KEDA
5. Worker-specific queue name overrides
6. Advanced KEDA features (multiple triggers, fallback, etc.)

## References

- KEDA Documentation: https://keda.sh/
- Redis Scaler: https://keda.sh/docs/latest/scalers/redis-lists/
- Frappe Worker Architecture: https://frappeframework.com/docs/user/en/basics/jobs
