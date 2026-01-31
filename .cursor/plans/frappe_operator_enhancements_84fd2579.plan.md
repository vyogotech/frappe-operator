---
name: Frappe Operator Enhancements
overview: Implement production-readiness enhancements for Frappe Operator including observability (Prometheus metrics), resilience patterns (exponential backoff, circuit breakers), operational improvements (webhook validation, resource defaults), and enhanced maintainability using a hybrid Go/template approach for optimal type safety and maintainability.
todos:
  - id: metrics-implementation
    content: Implement Prometheus metrics package and integrate with controllers
    status: completed
  - id: exponential-backoff
    content: Enhance backoff package and integrate with job failure handling
    status: completed
  - id: webhook-validation
    content: Create admission webhooks for FrappeSite and FrappeBench validation
    status: completed
  - id: resource-defaults
    content: Add default resource limits for all components
    status: completed
  - id: resource-builders
    content: Create resource builder pattern to reduce code duplication in controllers
    status: completed
  - id: extract-scripts
    content: Extract embedded bash scripts to Go templates with go:embed
    status: completed
  - id: template-examples
    content: Create template-based example manifests for documentation
    status: completed
  - id: job-ttl-cleanup
    content: Add TTL configuration to all Job resources
    status: completed
  - id: circuit-breaker
    content: Implement circuit breaker for external database connections
    status: completed
  - id: upgrade-guide
    content: Create comprehensive operator upgrade documentation
    status: completed
  - id: monitoring-docs
    content: Document metrics, create Grafana dashboard, and alert rules
    status: completed
  - id: integration-tests
    content: Add integration tests for new resilience and validation features
    status: completed
isProject: false
---

# Frappe Operator Production Enhancement Plan (Hybrid Approach)

## Executive Summary

This plan implements critical production-readiness improvements using a **hybrid Go/template architecture** that balances type safety with maintainability:

**Architecture Strategy:**

- **Go Structs**: Keep for complex resources with dynamic logic (70% of codebase)
- **Go Templates**: Use for bash scripts, examples, and static configurations
- **Resource Builders**: Create factory pattern to reduce duplication
- **Type Safety**: Maintain compile-time validation for critical resources

**Key Improvements:**

1. **Observability**: Prometheus metrics for monitoring
2. **Resilience**: Exponential backoff and circuit breakers
3. **Operations**: Webhook validation and resource defaults
4. **Maintainability**: Builder pattern + template-based scripts

---

## Hybrid Architecture: Decision Matrix

### Use Go Structs (Current Approach) for:

✅ **Deployments** - Complex security contexts varying by platform
✅ **StatefulSets** - Dynamic Redis configuration  
✅ **Services** - Computed endpoint names
✅ **PVCs** - Storage class selection logic
✅ **All resources with >20% dynamic fields**

**Why:** Type safety, IDE support, refactoring safety

### Use Go Templates for:

✅ **Bash Scripts** - Init, backup, delete operations
✅ **Example Manifests** - User-customizable templates
✅ **Documentation** - Code snippets
✅ **Static Configs** - Redis args, environment files

**Why:** Syntax highlighting, linting, readability

### Use Builder Pattern for:

✅ **Reducing Code Duplication** - 60% less boilerplate
✅ **Resource Creation** - Fluent, chainable API
✅ **Test Fixtures** - Easy mock objects
✅ **Common Patterns** - Security contexts, volumes

**Why:** Maintainability without losing type safety

---

## Architecture Comparison


| Aspect            | Pure Go (Current)       | Pure Templates      | Hybrid (Recommended)  |
| ----------------- | ----------------------- | ------------------- | --------------------- |
| Type Safety       | ✅ Excellent             | ❌ Runtime only      | ✅ Best of both        |
| Maintainability   | ⚠️ Verbose (1835 lines) | ✅ Compact           | ✅ Builder reduces 60% |
| Performance       | ✅ Fast                  | ⚠️ Template parsing | ✅ Fast where needed   |
| Script Management | ❌ Inline strings        | ✅ Separate files    | ✅ Templates           |
| Testing           | ✅ Easy                  | ⚠️ Complex          | ✅ Unit test both      |
| Learning Curve    | ⚠️ Steep                | ✅ Familiar          | ✅ Moderate            |


---

## Phase 1: Observability & Resource Builders (Priority 1)

### 1.1 Create Resource Builder Pattern (NEW)

**Rationale**: Reduce 1835-line `frappebench_resources.go` by 60% while maintaining type safety

**Files to Create:**

- `pkg/resources/deployment_builder.go`
- `pkg/resources/service_builder.go`
- `pkg/resources/statefulset_builder.go`
- `pkg/resources/pvc_builder.go`
- `pkg/resources/builders_test.go`

**Implementation Example:**

```go
// pkg/resources/deployment_builder.go
package resources

type DeploymentBuilder struct {
    deployment *appsv1.Deployment
}

func NewDeployment(name, namespace string) *DeploymentBuilder {
    return &DeploymentBuilder{
        deployment: &appsv1.Deployment{
            ObjectMeta: metav1.ObjectMeta{
                Name:      name,
                Namespace: namespace,
            },
        },
    }
}

func (b *DeploymentBuilder) WithLabels(labels map[string]string) *DeploymentBuilder {
    b.deployment.Labels = labels
    b.deployment.Spec.Template.ObjectMeta.Labels = labels
    return b
}

func (b *DeploymentBuilder) WithReplicas(replicas int32) *DeploymentBuilder {
    b.deployment.Spec.Replicas = &replicas
    return b
}

func (b *DeploymentBuilder) WithContainer(container corev1.Container) *DeploymentBuilder {
    b.deployment.Spec.Template.Spec.Containers = append(
        b.deployment.Spec.Template.Spec.Containers, container,
    )
    return b
}

func (b *DeploymentBuilder) WithSecurityContext(ctx *corev1.PodSecurityContext) *DeploymentBuilder {
    b.deployment.Spec.Template.Spec.SecurityContext = ctx
    return b
}

func (b *DeploymentBuilder) WithVolume(volume corev1.Volume) *DeploymentBuilder {
    b.deployment.Spec.Template.Spec.Volumes = append(
        b.deployment.Spec.Template.Spec.Volumes, volume,
    )
    return b
}

func (b *DeploymentBuilder) Build() *appsv1.Deployment {
    return b.deployment
}
```

**Before (108 lines per deployment):**

```go
deploy = &appsv1.Deployment{
    ObjectMeta: metav1.ObjectMeta{
        Name:      deployName,
        Namespace: bench.Namespace,
        Labels:    r.benchLabels(bench),
    },
    Spec: appsv1.DeploymentSpec{
        Replicas: &replicas,
        // ... 95+ more lines
    },
}
```

**After (20 lines with builder):**

```go
deploy = resources.NewDeployment(deployName, bench.Namespace).
    WithLabels(r.benchLabels(bench)).
    WithReplicas(replicas).
    WithSelector(r.componentLabels(bench, "gunicorn")).
    WithSecurityContext(r.getPodSecurityContext(ctx, bench)).
    WithContainer(gunicornContainer()).
    WithVolume(sitesVolume(pvcName)).
    Build()
```

**Migration Plan:**

1. Create builders package
2. Add comprehensive tests (>90% coverage target)
3. Migrate one controller at a time (FrappeBench first)
4. Keep old code during validation period
5. Remove old code after 2-week production validation

### 1.2 Add Prometheus Metrics

**Files to Create:**

- `pkg/metrics/metrics.go`

**Files to Modify:**

- `[controllers/frappebench_controller.go](controllers/frappebench_controller.go)` - lines 74-233
- `[controllers/frappesite_controller.go](controllers/frappesite_controller.go)` - lines 70-475
- `[main.go](main.go)` - line 1

**Metrics to Track:**

```go
// Reconciliation performance
- frappe_operator_reconciliation_total{controller, result}
- frappe_operator_reconciliation_duration_seconds{controller}
- frappe_operator_reconciliation_errors_total{controller, error_type}

// Resource counts
- frappe_operator_benches_total{}
- frappe_operator_sites_total{phase}
- frappe_operator_jobs_running{job_type}

// Builder pattern metrics (NEW)
- frappe_operator_resource_builder_calls_total{resource_type}
```

### 1.3 Add Metrics Documentation

**Files to Create:**

- `docs/monitoring.md` - Prometheus metrics reference
- `docs/grafana-dashboard.json` - Pre-built dashboard
- `docs/alert-rules.yaml` - Recommended alerts

---

## Phase 2: Template-Based Scripts (Priority 2)

### 2.1 Extract Bash Scripts to Go Templates

**Rationale**: Current 200+ line embedded bash strings are unmaintainable

**Files to Create:**

- `pkg/scripts/templates/bench_init.sh.tmpl`
- `pkg/scripts/templates/site_init.sh.tmpl`
- `pkg/scripts/templates/site_delete.sh.tmpl`
- `pkg/scripts/renderer.go`

**Files to Modify:**

- `[controllers/frappebench_controller.go](controllers/frappebench_controller.go)` - lines 482-606 (remove embedded script)
- `[controllers/frappesite_controller.go](controllers/frappesite_controller.go)` - lines 772-964 (remove embedded script)

**Template Example:**

```bash
# pkg/scripts/templates/bench_init.sh.tmpl
#!/bin/bash
set -e

# Setup user for OpenShift compatibility
if ! whoami &>/dev/null; then
  export USER=frappe
  export LOGNAME=frappe
  if [ -w /etc/passwd ]; then
    echo "frappe:x:$(id -u):0:frappe user:/home/frappe:/sbin/nologin" >> /etc/passwd
  fi
fi

cd /home/frappe/frappe-bench

# Create common_site_config.json
cat > sites/common_site_config.json <<EOF
{
  "redis_cache": "redis://{{.RedisCache}}:6379",
  "redis_queue": "redis://{{.RedisQueue}}:6379",
  "socketio_port": 9000
}
EOF

# Sync assets if available
if [ -d "/home/frappe/assets_cache" ]; then
    mkdir -p sites/assets
    cp -rn /home/frappe/assets_cache/* sites/assets/ || true
fi

echo "Bench configuration complete"
```

**Renderer:**

```go
// pkg/scripts/renderer.go
package scripts

import (
    "bytes"
    "embed"
    "text/template"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

type ScriptRenderer struct {
    templates *template.Template
}

func NewScriptRenderer() (*ScriptRenderer, error) {
    tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
    if err != nil {
        return nil, err
    }
    return &ScriptRenderer{templates: tmpl}, nil
}

func (r *ScriptRenderer) RenderBenchInit(data BenchInitData) (string, error) {
    var buf bytes.Buffer
    err := r.templates.ExecuteTemplate(&buf, "bench_init.sh.tmpl", data)
    return buf.String(), err
}

type BenchInitData struct {
    BenchName  string
    RedisCache string
    RedisQueue string
    SkipBuild  string
}
```

**Benefits:**

- ✅ Syntax highlighting in IDE
- ✅ Shellcheck validation in CI
- ✅ No escape character hell
- ✅ Can test scripts independently
- ✅ Easy to version and diff

### 2.2 Add Script Validation CI

**Files to Create:**

- `.github/workflows/validate-scripts.yml`

```yaml
name: Validate Scripts
on: [push, pull_request]

jobs:
  shellcheck:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: ShellCheck
        uses: ludeeus/action-shellcheck@master
        with:
          scandir: './pkg/scripts/templates'
          severity: warning
```

### 2.3 Create Template-Based Examples

**Files to Create:**

- `examples/templates/basic-bench.yaml.tmpl`
- `examples/templates/production-site.yaml.tmpl`
- `examples/templates/README.md`
- `examples/generate.go` - CLI tool

**Purpose**: Allow users to customize examples without editing operator code

---

## Phase 3: Resilience & Operations

### 3.1 Exponential Backoff

**Files to Modify:**

- `[pkg/backoff/backoff.go](pkg/backoff/backoff.go)` - Enhance existing
- `[controllers/frappebench_controller.go](controllers/frappebench_controller.go)` - Integrate

### 3.2 Webhook Validation

**Files to Create:**

- `api/v1alpha1/frappesite_webhook.go`
- `api/v1alpha1/frappebench_webhook.go`

### 3.3 Resource Defaults

**Files to Modify:**

- `[api/v1alpha1/shared_types.go](api/v1alpha1/shared_types.go)`
- Add `DefaultComponentResources()` function

### 3.4 Circuit Breaker

**Files to Create:**

- `pkg/circuitbreaker/circuitbreaker.go`
- `[controllers/database/external_provider.go](controllers/database/external_provider.go)` - Integrate

### 3.5 Job TTL Cleanup

**Files to Modify:**

- All job creation code to add `TTLSecondsAfterFinished: 3600`

---

## Phase 4: Documentation

### 4.1 Upgrade Guide

- `docs/upgrade-guide.md` - Version compatibility matrix

### 4.2 Disaster Recovery

- `docs/operations.md` - Enhanced backup/restore procedures

### 4.3 Runbook

- `docs/runbook.md` - Operational playbook

---

## Phase 5: Testing

### 5.1 Builder Pattern Tests

- `pkg/resources/builders_test.go` - Target: >90% coverage
- Test all builder methods
- Validate generated resources match expected structure

### 5.2 Template Tests

- `pkg/scripts/renderer_test.go`
- Test script rendering with various data
- Validate template syntax

### 5.3 Integration Tests

- Test builder-generated resources work in-cluster
- Validate template-rendered scripts execute correctly

### 5.4 Performance Tests (NEW)

- Compare builder pattern vs direct struct creation
- Measure template rendering overhead
- Test large-scale scenarios (100+ sites)

---

## Implementation Timeline

### Week 1: Foundation (Builders + Metrics)

- Day 1-2: Create builder package with tests
- Day 3-4: Implement Prometheus metrics
- Day 5: Refactor FrappeBench controller to use builders

### Week 2: Templates + Resilience

- Day 1-2: Extract bash scripts to templates
- Day 3: Add exponential backoff
- Day 4-5: Webhook validation

### Week 3: Operations + Migration

- Day 1-2: Resource defaults + circuit breaker
- Day 3-4: Complete builder migration (all controllers)
- Day 5: Job TTL cleanup

### Week 4: Documentation + Validation

- Day 1-2: Write comprehensive documentation
- Day 3-5: Testing and validation

### Week 5: Production Deployment

- Day 1-2: Deploy to dev/staging
- Day 3-5: Production rollout with monitoring

---

## Success Metrics

### Code Quality

- **60% reduction** in controller code duplication (via builders)
- **Zero** shellcheck warnings (via templates)
- **>90%** test coverage for builders
- **>70%** overall test coverage (from 44%)

### Maintainability

- **<30 minutes** to add new resource type (vs 2 hours currently)
- **<10 minutes** to modify scripts (no escaping)
- **50% less** lines per resource definition

### Operations

- **100%** of reconciliations tracked in metrics
- **90%** reduction in misconfiguration (webhooks)
- **<5 minutes** MTTR for transient failures (backoff)

### Performance

- **<1ms** overhead for builder pattern
- **<5ms** script template rendering
- Zero regression in reconciliation speed

---

## Migration Safety

### Backward Compatibility

✅ Builder pattern generates identical resources
✅ Template rendering is internal implementation detail
✅ No CRD changes required
✅ Users see no difference

### Validation Strategy

1. **Unit Tests**: Builders generate correct structures
2. **Integration Tests**: Resources work in-cluster
3. **Diff Tests**: Compare old vs new resource output
4. **Canary Deployment**: 10% of production traffic first
5. **Rollback Plan**: Keep old code for 2 weeks

### Risk Mitigation

- Builders tested with >90% coverage before migration
- Templates validated with shellcheck in CI
- Performance tests confirm no regression
- Gradual rollout with monitoring

---

## Conclusion

The hybrid approach provides the best of all worlds:

1. **Type Safety** (Go): Complex resources remain compile-time validated
2. **Maintainability** (Builders): 60% less code duplication
3. **Readability** (Templates): Scripts are properly highlighted and linted
4. **Performance**: No unnecessary overhead
5. **Flexibility**: Right tool for each task

This strategy transforms the operator from a maintenance burden into a sustainable, production-grade codebase.