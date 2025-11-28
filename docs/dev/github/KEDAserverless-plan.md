# Frappe Operator: Serverless Worker Scaling Implementation Guide

## Project Overview

We are building a Kubernetes Operator for Frappe Framework that can deploy and manage Frappe applications with serverless worker scaling capabilities. The goal is to minimize infrastructure costs by scaling workers based on actual workload demand.

## Current State

You have developed a Frappe Operator that can deploy:

- Frappe web application pods
- Mariadb database pods
- Redis cache pods
- Worker pods (short, long, default types)

## Business Goal

Enable SaaS providers to run Frappe at minimal cost by:

- Scaling worker pods from 0 to N based on actual job queue depth
- Only paying for compute resources when jobs are being processed
- Achieving 60-80% cost reduction compared to always-on workers
- Supporting multi-tenant deployments efficiently

---

## Technical Context

### Frappe Worker Architecture

Frappe uses Redis-backed job queues for background processing:

- **Queue names**: `rq:queue:short`, `rq:queue:long`, `rq:queue:default`
- **Worker types**:
  - **short**: Quick tasks (emails, notifications, small operations) - typically complete in seconds
  - **long**: Time-intensive tasks (reports, imports, exports, heavy computations) - may take minutes
  - **default**: General purpose tasks and runs the Frappe scheduler (cron-like jobs)
- **Job flow**: Scheduler or user actions enqueue jobs → Workers pull from queues → Execute jobs

### Why KEDA

KEDA (Kubernetes Event-Driven Autoscaling) is the industry-standard solution for scaling workloads based on external metrics like queue depth. It:

- Monitors Redis queue lengths in real-time
- Scales Kubernetes Deployments from 0 to N replicas
- Integrates natively with Kubernetes HPA (Horizontal Pod Autoscaler)
- Supports scale-to-zero scenarios

---

## Implementation Goals

### Goal 1: Add KEDA Support to CRD

Extend the FrappeDeployment Custom Resource Definition to allow users to configure autoscaling behavior per worker type.

**Configuration requirements**:

- Enable/disable autoscaling per worker type
- Set minimum replicas (can be 0 for true serverless)
- Set maximum replicas (prevent runaway scaling)
- Configure polling interval (how often to check queue depth)
- Configure cooldown period (wait time before scaling down)
- Set queue length threshold (number of jobs that trigger scaling)

**Default configurations**:

- **Short workers**: minReplicas=0, maxReplicas=10, queueLength=5, cooldown=60s
- **Long workers**: minReplicas=0, maxReplicas=5, queueLength=1, cooldown=120s
- **Default workers**: autoscaling disabled, always keep 1 replica running (for scheduler)

### Goal 2: Generate KEDA ScaledObject Resources

The operator should create Kubernetes ScaledObject resources (KEDA's CRD) for each worker type when autoscaling is enabled.

**ScaledObject requirements**:

- Reference the correct worker Deployment
- Configure Redis trigger with proper queue name
- Use the Redis service created by the operator
- Set appropriate scaling parameters from CRD spec
- Include proper labels for tracking and management

**Redis trigger configuration**:

- Address: Point to the Redis service deployed by operator
- List name: `rq:queue:{workerType}` (e.g., `rq:queue:short`)
- List length threshold: From CRD spec
- Database index: 0 (Frappe uses Redis DB 0 for queues)

### Goal 3: Modify Worker Deployment Logic

Update how the operator creates worker Deployments to be compatible with KEDA.

**Key changes needed**:

- When autoscaling is enabled, set initial replicas to minReplicas value
- When autoscaling is disabled, use the static replicas count
- Ensure Deployments don't have HPA configured (KEDA creates its own HPA)
- Add annotations to indicate KEDA management

### Goal 4: Reconciliation Logic

Update the operator's reconciliation loop to manage KEDA resources.

**Reconciliation requirements**:

- Create ScaledObject resources when autoscaling is enabled
- Delete ScaledObject resources when autoscaling is disabled
- Update ScaledObject resources when configuration changes
- Handle ScaledObject ownership (set controller reference)
- Ensure idempotency (don't recreate unchanged resources)

### Goal 5: Default Worker Strategy

Implement the recommended production deployment pattern:

**Pattern**:

- 1 default worker always running (handles scheduler, quick response for urgent tasks)
- Short workers fully serverless (0 to N based on queue depth)
- Long workers fully serverless (0 to N based on queue depth)

**Rationale**:

- Scheduler must run continuously for cron-like jobs
- One small always-on pod costs ~$10/month but ensures system reliability
- Scaled workers only cost money when jobs are queued
- Typical cost: $30-100/month vs $200-400/month for always-on workers

---

## Resource Specifications

### Short Workers (Quick Tasks)

- **CPU**: 500m request, 1000m limit
- **Memory**: 1Gi request, 2Gi limit
- **Scaling**: 0-10 replicas
- **Trigger threshold**: 5 jobs in queue
- **Cooldown**: 60 seconds
- **Use case**: Email notifications, small DB operations, API calls

### Long Workers (Heavy Tasks)

- **CPU**: 2000m request, 4000m limit
- **Memory**: 4Gi request, 8Gi limit
- **Scaling**: 0-5 replicas
- **Trigger threshold**: 1 job in queue (scale immediately)
- **Cooldown**: 120 seconds
- **Use case**: Large imports/exports, complex reports, bulk operations

### Default Workers (Scheduler + General)

- **CPU**: 200m request, 500m limit
- **Memory**: 512Mi request, 1Gi limit
- **Scaling**: Static 1 replica (no autoscaling)
- **Use case**: Running Frappe scheduler, handling general queue

---

## Storage Considerations

The implementation must support ReadWriteMany (RWX) storage for Frappe file uploads that need to be shared across pods.

**Recommended storage solutions**:

- OpenEBS (lightweight, works well with k3s)
- Longhorn (built for k3s, includes HA)
- NFS provisioner (simple, lower performance)
- Rook-Ceph (enterprise, complex)

**Note**: The operator should assume a StorageClass with RWX capability exists in the cluster.

---

## Expected Behavior After Implementation

### Scenario 1: No Jobs in Queue

- Short workers: 0 pods running
- Long workers: 0 pods running
- Default worker: 1 pod running
- **Cost**: Minimal (~$10-20/month for baseline)

### Scenario 2: User Triggers Export (Long Job)

1. Job enqueued to `rq:queue:long`
2. KEDA detects queue length = 1
3. Scales long worker deployment 0→1
4. Pod starts (~30-60 seconds cold start)
5. Worker picks up job and processes
6. Job completes, queue empty
7. After 120s cooldown, KEDA scales back to 0

### Scenario 3: Bulk Email Send (Many Short Jobs)

1. 50 emails enqueued to `rq:queue:short`
2. KEDA detects queue length = 50 (threshold: 5)
3. Scales short workers 0→10 (max replicas)
4. Workers process emails in parallel
5. Queue drains
6. After 60s cooldown, scales back to 0

### Scenario 4: Scheduled Daily Report (Scheduler)

1. Default worker running scheduler detects cron trigger
2. Scheduler enqueues job to `rq:queue:long`
3. KEDA scales long worker 0→1
4. Long worker processes report
5. Long worker scales back to 0 after cooldown
6. Default worker continues running (always-on)

---

## Success Criteria

### Functional Requirements

✅ Users can enable/disable autoscaling per worker type via CRD  
✅ KEDA ScaledObjects are created automatically when autoscaling enabled  
✅ Workers scale from 0 to maxReplicas based on Redis queue depth  
✅ Workers scale back to minReplicas after cooldown period  
✅ Default worker remains running for scheduler  
✅ Multiple Frappe sites can run independently with their own scaling

### Performance Requirements

✅ Cold start time: < 60 seconds (pod scheduling + startup)  
✅ Scaling decision latency: < 30 seconds (KEDA polling + K8s scheduling)  
✅ No job loss during scaling events  
✅ Workers properly terminate gracefully when scaling down

### Cost Requirements

✅ Achieve 60-80% cost reduction vs always-on workers  
✅ Support 100 tenants at <$2/tenant/month infrastructure cost  
✅ Scale efficiently for bursty workloads

---

## Testing Scenarios

### Test 1: Basic Scaling

1. Deploy Frappe site with autoscaling enabled
2. Verify all workers start at minReplicas
3. Enqueue 10 jobs to short queue
4. Verify short workers scale up
5. Wait for jobs to complete
6. Verify workers scale back down after cooldown

### Test 2: Scale to Zero

1. Ensure no jobs in any queue
2. Wait for cooldown period
3. Verify short and long workers scale to 0 pods
4. Verify default worker remains at 1 pod

### Test 3: Rapid Scaling

1. Enqueue 100 jobs quickly
2. Verify workers scale to maxReplicas
3. Verify no jobs are lost
4. Verify workers distribute load

### Test 4: Configuration Changes

1. Update worker maxReplicas in CRD
2. Verify ScaledObject is updated
3. Verify new max is respected during scaling

### Test 5: Multi-Tenant

1. Deploy 3 different Frappe sites
2. Enqueue jobs to site A only
3. Verify only site A workers scale
4. Verify sites B and C remain at minReplicas

---

## Dependencies and Prerequisites

### Required Components

- KEDA must be installed in the cluster (version 2.x)
- Redis must be deployed and accessible
- StorageClass with RWX support must exist
- Kubernetes 1.19+ for ScaledObject API compatibility

### Operator Dependencies

- Add KEDA Go client library
- Import KEDA CRD types
- Update operator RBAC to manage ScaledObjects

---

## Migration Path for Existing Deployments

### Phase 1: Add Configuration

- Update CRD with autoscaling fields
- Provide sensible defaults (autoscaling disabled initially)
- Existing deployments continue working unchanged

### Phase 2: Enable Autoscaling

- Users opt-in by setting `autoscaling.enabled: true`
- Operator creates ScaledObjects for opted-in workers
- Both modes (static and autoscaled) work simultaneously

### Phase 3: Gradual Rollout

- Test with small subset of sites
- Monitor metrics and costs
- Adjust defaults based on real-world usage
- Enable by default for new deployments

---

## Documentation Requirements

### User Documentation Needed

- How to enable autoscaling in CRD
- Explanation of each configuration parameter
- Cost comparison: autoscaled vs static workers
- Troubleshooting guide for scaling issues
- Best practices for setting thresholds

### Operator Developer Documentation

- Architecture overview of KEDA integration
- How reconciliation handles ScaledObjects
- Debugging KEDA scaling decisions
- Testing autoscaling locally

---

## Future Enhancements (Out of Scope for Initial Implementation)

### Advanced Features to Consider Later

- Custom metrics (CPU/memory-based scaling in addition to queue depth)
- Predictive scaling based on historical patterns
- Per-tenant resource quotas
- Prometheus metrics for scaling events
- Dashboard for monitoring worker utilization
- Cost tracking per tenant
- Automatic queue length threshold tuning
- Support for additional queue backends beyond Redis
