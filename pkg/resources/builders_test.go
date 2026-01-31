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

package resources

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

func TestNewDeploymentBuilder(t *testing.T) {
	builder := NewDeploymentBuilder("test-deployment", "test-ns")
	if builder == nil {
		t.Fatal("expected non-nil builder")
	}
	if builder.deployment.Name != "test-deployment" {
		t.Errorf("expected name 'test-deployment', got '%s'", builder.deployment.Name)
	}
	if builder.deployment.Namespace != "test-ns" {
		t.Errorf("expected namespace 'test-ns', got '%s'", builder.deployment.Namespace)
	}
}

func TestDeploymentBuilderWithLabels(t *testing.T) {
	labels := map[string]string{
		"app":  "frappe",
		"tier": "backend",
	}
	selector := map[string]string{
		"app": "frappe",
	}
	d := NewDeploymentBuilder("test", "default").
		WithLabels(labels).
		WithSelector(selector).
		MustBuild()

	if d.Labels["app"] != "frappe" {
		t.Error("expected label 'app=frappe' on deployment")
	}
	if d.Spec.Selector.MatchLabels["app"] != "frappe" {
		t.Error("expected label 'app=frappe' on selector")
	}
	if d.Spec.Template.Labels["app"] != "frappe" {
		t.Error("expected label 'app=frappe' on pod template")
	}
}

func TestDeploymentBuilderWithReplicas(t *testing.T) {
	d := NewDeploymentBuilder("test", "default").
		WithReplicas(3).
		MustBuild()

	if *d.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %d", *d.Spec.Replicas)
	}
}

func TestDeploymentBuilderWithContainer(t *testing.T) {
	container := NewContainerBuilder("app", "nginx:latest").
		WithPort("http", 80).
		Build()

	d := NewDeploymentBuilder("test", "default").
		WithContainer(container).
		MustBuild()

	if len(d.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(d.Spec.Template.Spec.Containers))
	}
	if d.Spec.Template.Spec.Containers[0].Name != "app" {
		t.Errorf("expected container name 'app', got '%s'", d.Spec.Template.Spec.Containers[0].Name)
	}
}

func TestDeploymentBuilderWithPVCVolume(t *testing.T) {
	d := NewDeploymentBuilder("test", "default").
		WithPVCVolume("data", "my-pvc").
		MustBuild()

	if len(d.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(d.Spec.Template.Spec.Volumes))
	}
	vol := d.Spec.Template.Spec.Volumes[0]
	if vol.Name != "data" {
		t.Errorf("expected volume name 'data', got '%s'", vol.Name)
	}
	if vol.PersistentVolumeClaim.ClaimName != "my-pvc" {
		t.Errorf("expected PVC 'my-pvc', got '%s'", vol.PersistentVolumeClaim.ClaimName)
	}
}

func TestDeploymentBuilderWithSecurityContext(t *testing.T) {
	podSec := DefaultPodSecurityContext(1000, 1000)
	d := NewDeploymentBuilder("test", "default").
		WithPodSecurityContext(podSec).
		MustBuild()

	if d.Spec.Template.Spec.SecurityContext == nil {
		t.Fatal("expected pod security context")
	}
	if *d.Spec.Template.Spec.SecurityContext.RunAsUser != 1000 {
		t.Errorf("expected RunAsUser=1000, got %d", *d.Spec.Template.Spec.SecurityContext.RunAsUser)
	}
}

func TestNewServiceBuilder(t *testing.T) {
	builder := NewServiceBuilder("test-svc", "test-ns")
	if builder == nil {
		t.Fatal("expected non-nil builder")
	}
	if builder.service.Name != "test-svc" {
		t.Errorf("expected name 'test-svc', got '%s'", builder.service.Name)
	}
}

func TestServiceBuilderWithPort(t *testing.T) {
	s := NewServiceBuilder("test", "default").
		WithPort("http", 80, 8080).
		MustBuild()

	if len(s.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(s.Spec.Ports))
	}
	port := s.Spec.Ports[0]
	if port.Name != "http" {
		t.Errorf("expected port name 'http', got '%s'", port.Name)
	}
	if port.Port != 80 {
		t.Errorf("expected port 80, got %d", port.Port)
	}
}

func TestServiceBuilderTypes(t *testing.T) {
	// ClusterIP
	s := NewServiceBuilder("test", "default").AsClusterIP().MustBuild()
	if s.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("expected ClusterIP, got %s", s.Spec.Type)
	}

	// LoadBalancer
	s = NewServiceBuilder("test", "default").AsLoadBalancer().MustBuild()
	if s.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Errorf("expected LoadBalancer, got %s", s.Spec.Type)
	}

	// Headless
	s = NewServiceBuilder("test", "default").AsHeadless().MustBuild()
	if s.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Errorf("expected headless service (ClusterIP=None), got %s", s.Spec.ClusterIP)
	}
}

func TestNewJobBuilder(t *testing.T) {
	builder := NewJobBuilder("test-job", "test-ns")
	if builder == nil {
		t.Fatal("expected non-nil builder")
	}
	if builder.job.Name != "test-job" {
		t.Errorf("expected name 'test-job', got '%s'", builder.job.Name)
	}
	// Check default TTL
	if *builder.job.Spec.TTLSecondsAfterFinished != DefaultJobTTL {
		t.Errorf("expected default TTL %d, got %d", DefaultJobTTL, *builder.job.Spec.TTLSecondsAfterFinished)
	}
}

func TestJobBuilderWithTTL(t *testing.T) {
	j := NewJobBuilder("test", "default").
		WithTTL(7200).
		MustBuild()

	if *j.Spec.TTLSecondsAfterFinished != 7200 {
		t.Errorf("expected TTL 7200, got %d", *j.Spec.TTLSecondsAfterFinished)
	}
}

func TestJobBuilderWithBackoffLimit(t *testing.T) {
	j := NewJobBuilder("test", "default").
		WithBackoffLimit(5).
		MustBuild()

	if *j.Spec.BackoffLimit != 5 {
		t.Errorf("expected backoff limit 5, got %d", *j.Spec.BackoffLimit)
	}
}

func TestNewContainerBuilder(t *testing.T) {
	c := NewContainerBuilder("app", "nginx:latest").Build()

	if c.Name != "app" {
		t.Errorf("expected name 'app', got '%s'", c.Name)
	}
	if c.Image != "nginx:latest" {
		t.Errorf("expected image 'nginx:latest', got '%s'", c.Image)
	}
}

func TestContainerBuilderWithEnv(t *testing.T) {
	c := NewContainerBuilder("app", "nginx").
		WithEnv("PORT", "8080").
		WithEnv("DEBUG", "true").
		Build()

	if len(c.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(c.Env))
	}
	if c.Env[0].Name != "PORT" || c.Env[0].Value != "8080" {
		t.Errorf("expected env PORT=8080, got %s=%s", c.Env[0].Name, c.Env[0].Value)
	}
}

func TestContainerBuilderWithVolumeMount(t *testing.T) {
	c := NewContainerBuilder("app", "nginx").
		WithVolumeMount("data", "/data").
		WithVolumeMountSubPath("sites", "/home/frappe/sites", "frappe-sites").
		Build()

	if len(c.VolumeMounts) != 2 {
		t.Fatalf("expected 2 volume mounts, got %d", len(c.VolumeMounts))
	}
	if c.VolumeMounts[1].SubPath != "frappe-sites" {
		t.Errorf("expected subPath 'frappe-sites', got '%s'", c.VolumeMounts[1].SubPath)
	}
}

func TestContainerBuilderWithProbes(t *testing.T) {
	c := NewContainerBuilder("app", "nginx").
		WithHTTPReadinessProbe("/health", 8080, 10, 5).
		WithHTTPLivenessProbe("/health", 8080, 15, 10).
		Build()

	if c.ReadinessProbe == nil {
		t.Fatal("expected readiness probe")
	}
	if c.LivenessProbe == nil {
		t.Fatal("expected liveness probe")
	}
	if c.ReadinessProbe.HTTPGet.Path != "/health" {
		t.Errorf("expected readiness probe path '/health', got '%s'", c.ReadinessProbe.HTTPGet.Path)
	}
}

func TestStandardLabels(t *testing.T) {
	labels := StandardLabels("frappe", "gunicorn", "my-bench")

	if labels["app.kubernetes.io/name"] != "frappe" {
		t.Error("expected app.kubernetes.io/name=frappe")
	}
	if labels["app.kubernetes.io/component"] != "gunicorn" {
		t.Error("expected app.kubernetes.io/component=gunicorn")
	}
	if labels["app.kubernetes.io/instance"] != "my-bench" {
		t.Error("expected app.kubernetes.io/instance=my-bench")
	}
	if labels["app.kubernetes.io/managed-by"] != "frappe-operator" {
		t.Error("expected app.kubernetes.io/managed-by=frappe-operator")
	}
}

func TestMergeLabels(t *testing.T) {
	a := map[string]string{"app": "frappe", "tier": "backend"}
	b := map[string]string{"app": "erpnext", "version": "15"}

	merged := MergeLabels(a, b)

	if merged["app"] != "erpnext" {
		t.Errorf("expected app=erpnext (later map wins), got %s", merged["app"])
	}
	if merged["tier"] != "backend" {
		t.Error("expected tier=backend from first map")
	}
	if merged["version"] != "15" {
		t.Error("expected version=15 from second map")
	}
}

func TestResourceList(t *testing.T) {
	list := ResourceList("100m", "256Mi")

	if _, ok := list[corev1.ResourceCPU]; !ok {
		t.Error("expected CPU in resource list")
	}
	if _, ok := list[corev1.ResourceMemory]; !ok {
		t.Error("expected Memory in resource list")
	}
}

func TestResourceRequirements(t *testing.T) {
	req := ResourceRequirements("100m", "256Mi", "500m", "1Gi")

	if _, ok := req.Requests[corev1.ResourceCPU]; !ok {
		t.Error("expected CPU request")
	}
	if _, ok := req.Limits[corev1.ResourceMemory]; !ok {
		t.Error("expected memory limit")
	}
}

func TestJobBuilderWithContainerAndVolumes(t *testing.T) {
	c := NewContainerBuilder("init", "bench:latest").
		WithCommand("bash", "-c", "echo ok").
		WithVolumeMount("sites", "/sites").
		Build()
	j := NewJobBuilder("my-job", "default").
		WithLabels(map[string]string{"app": "frappe"}).
		WithContainer(c).
		WithPVCVolume("sites", "bench-sites").
		WithSecretVolume("secrets", "my-secret", nil).
		MustBuild()
	if len(j.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(j.Spec.Template.Spec.Containers))
	}
	if j.Spec.Template.Spec.Containers[0].Name != "init" {
		t.Errorf("container name expected init, got %s", j.Spec.Template.Spec.Containers[0].Name)
	}
	if len(j.Spec.Template.Spec.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(j.Spec.Template.Spec.Volumes))
	}
	if j.Labels["app"] != "frappe" {
		t.Errorf("expected label app=frappe, got %s", j.Labels["app"])
	}
}

func TestJobBuilderBuildWithOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	owner := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-site",
			Namespace: "default",
			UID:       "uid-123",
		},
	}
	j, err := NewJobBuilder("job", "default").
		WithOwner(owner, scheme).
		Build()
	if err != nil {
		t.Fatalf("Build() with owner error: %v", err)
	}
	if j.Name != "job" {
		t.Errorf("Build() job name expected job, got %s", j.Name)
	}
	if j.OwnerReferences == nil || len(j.OwnerReferences) == 0 {
		t.Error("Build() with owner should set OwnerReferences")
	}
	j2, err2 := NewJobBuilder("job2", "default").Build()
	if err2 != nil {
		t.Fatalf("Build() without owner error: %v", err2)
	}
	if j2.Name != "job2" {
		t.Errorf("Build() job name expected job2, got %s", j2.Name)
	}
}

func TestStatefulSetBuilder(t *testing.T) {
	labels := map[string]string{"app": "frappe"}
	selector := map[string]string{"component": "redis"}
	sts, err := NewStatefulSetBuilder("redis", "default").
		WithLabels(labels).
		WithSelector(selector).
		WithServiceName("redis-svc").
		WithReplicas(2).
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if sts.Labels["app"] != "frappe" {
		t.Error("expected label app=frappe")
	}
	if sts.Spec.Selector.MatchLabels["component"] != "redis" {
		t.Error("expected selector component=redis")
	}
	if sts.Spec.ServiceName != "redis-svc" {
		t.Errorf("expected serviceName redis-svc, got %s", sts.Spec.ServiceName)
	}
	if *sts.Spec.Replicas != 2 {
		t.Errorf("expected 2 replicas, got %d", *sts.Spec.Replicas)
	}
}

func TestPVCBuilder(t *testing.T) {
	size := resource.MustParse("10Gi")
	pvc, err := NewPVCBuilder("test-pvc", "default").
		WithLabels(map[string]string{"app": "frappe"}).
		WithAccessMode(corev1.ReadWriteOnce).
		WithStorageRequest(size).
		WithStorageClass("standard").
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if pvc.Name != "test-pvc" {
		t.Errorf("expected name test-pvc, got %s", pvc.Name)
	}
	if pvc.Labels["app"] != "frappe" {
		t.Error("expected label app=frappe")
	}
	if pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("expected access mode ReadWriteOnce, got %v", pvc.Spec.AccessModes[0])
	}
	if pvc.Spec.Resources.Requests[corev1.ResourceStorage] != size {
		t.Errorf("expected size 10Gi, got %v", pvc.Spec.Resources.Requests[corev1.ResourceStorage])
	}
	if *pvc.Spec.StorageClassName != "standard" {
		t.Errorf("expected storage class standard, got %s", *pvc.Spec.StorageClassName)
	}
}

func TestIngressBuilder(t *testing.T) {
	pathType := networkingv1.PathTypePrefix
	ingress, err := NewIngressBuilder("test-ingress", "default").
		WithLabels(map[string]string{"app": "frappe"}).
		WithClassName("nginx").
		WithRule("example.com", "/", pathType, "test-svc", 80).
		WithTLS([]string{"example.com"}, "test-tls").
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if ingress.Labels["app"] != "frappe" {
		t.Error("expected label app=frappe")
	}
	if *ingress.Spec.IngressClassName != "nginx" {
		t.Errorf("expected class name nginx, got %s", *ingress.Spec.IngressClassName)
	}
	if len(ingress.Spec.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(ingress.Spec.Rules))
	}
	if ingress.Spec.Rules[0].Host != "example.com" {
		t.Errorf("expected host example.com, got %s", ingress.Spec.Rules[0].Host)
	}
	if len(ingress.Spec.TLS) != 1 {
		t.Fatalf("expected 1 TLS config, got %d", len(ingress.Spec.TLS))
	}
	if ingress.Spec.TLS[0].SecretName != "test-tls" {
		t.Errorf("expected TLS secret test-tls, got %s", ingress.Spec.TLS[0].SecretName)
	}
}

func TestContainerBuilderAdvanced(t *testing.T) {
	res := ResourceRequirements("100m", "256Mi", "200m", "512Mi")
	secCtx := &corev1.SecurityContext{
		RunAsUser:  Int64Ptr(1000),
		RunAsGroup: Int64Ptr(1000),
	}

	c := NewContainerBuilder("advanced", "frappe/frappe:latest").
		WithCommand("bench", "serve").
		WithArgs("--port", "8000").
		WithResources(res).
		WithSecurityContext(secCtx).
		Build()

	if c.Command[0] != "bench" {
		t.Errorf("expected command bench, got %s", c.Command[0])
	}
	if c.Args[1] != "8000" {
		t.Errorf("expected arg 8000, got %s", c.Args[1])
	}
	if c.Resources.Requests[corev1.ResourceCPU] != res.Requests[corev1.ResourceCPU] {
		t.Error("CPU requests mismatch")
	}
	if *c.SecurityContext.RunAsUser != 1000 {
		t.Error("Security context RunAsUser mismatch")
	}
}

func TestServiceBuilderAdvanced(t *testing.T) {
	s, err := NewServiceBuilder("adv-svc", "default").
		WithAnnotations(map[string]string{"foo": "bar"}).
		WithSelector(map[string]string{"app": "frappe"}).
		WithSessionAffinity(corev1.ServiceAffinityClientIP).
		WithExternalTrafficPolicy(corev1.ServiceExternalTrafficPolicyTypeLocal).
		AsNodePort().
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if s.Annotations["foo"] != "bar" {
		t.Error("Annotation mismatch")
	}
	if s.Spec.Selector["app"] != "frappe" {
		t.Error("Selector mismatch")
	}
	if s.Spec.SessionAffinity != corev1.ServiceAffinityClientIP {
		t.Error("Session affinity mismatch")
	}
	if s.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyTypeLocal {
		t.Error("External traffic policy mismatch")
	}
	if s.Spec.Type != corev1.ServiceTypeNodePort {
		t.Error("Service type mismatch")
	}
}

func TestDeploymentBuilderAdvanced(t *testing.T) {
	initContainer := NewContainerBuilder("init", "busybox").WithCommand("sh", "-c", "echo ok").Build()

	d, err := NewDeploymentBuilder("adv-deploy", "default").
		WithAnnotations(map[string]string{"deploy": "now"}).
		WithInitContainer(initContainer).
		WithNodeSelector(map[string]string{"tier": "worker"}).
		WithStrategy(appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}).
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if d.Annotations["deploy"] != "now" {
		t.Error("Annotation mismatch")
	}
	if len(d.Spec.Template.Spec.InitContainers) != 1 {
		t.Fatal("Expected 1 init container")
	}
	if d.Spec.Template.Spec.NodeSelector["tier"] != "worker" {
		t.Error("Node selector mismatch")
	}
	if d.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Error("Deployment strategy mismatch")
	}
}
