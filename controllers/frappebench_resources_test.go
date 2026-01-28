package controllers

import (
	"context"
	"testing"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFrappeBenchReconciler_Resources(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))

	namespace := "test-ns"
	benchName := "test-bench"
	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
		},
	}

	t.Run("ensureRedis", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		err := r.ensureRedis(context.TODO(), bench)
		if err != nil {
			t.Fatalf("ensureRedis failed: %v", err)
		}

		// Check Service
		svc := &corev1.Service{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-redis-queue", Namespace: namespace}, svc)
		if err != nil {
			t.Error("Redis queue service not created")
		}

		// Check StatefulSet
		sts := &appsv1.StatefulSet{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-redis-queue", Namespace: namespace}, sts)
		if err != nil {
			t.Error("Redis queue statefulset not created")
		}
	})

	t.Run("ensureRedis Idempotency and Update", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		// First creation
		if err := r.ensureRedis(context.TODO(), bench); err != nil {
			t.Fatalf("First ensureRedis failed: %v", err)
		}

		// Verify existing
		if err := r.ensureRedis(context.TODO(), bench); err != nil {
			t.Fatalf("Second ensureRedis failed: %v", err)
		}

		// Test Update (simulate image change - although image is hardcoded in controller logic for now,
		// we can test if it reconciles other fields if we manually change them on the object effectively)
		// For now, just ensure it doesn't error on existing objects
	})

	t.Run("ensureGunicorn Update", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		// Create initial deployment
		if err := r.ensureGunicorn(context.TODO(), bench); err != nil {
			t.Fatalf("Initial ensureGunicorn failed: %v", err)
		}

		// Manually modify deployment to have wrong image
		deploy := &appsv1.Deployment{}
		client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-gunicorn", Namespace: namespace}, deploy)
		deploy.Spec.Template.Spec.Containers[0].Image = "wrong/image:tag"
		client.Update(context.TODO(), deploy)

		// Reconcile should fix it
		if err := r.ensureGunicorn(context.TODO(), bench); err != nil {
			t.Fatalf("Update ensureGunicorn failed: %v", err)
		}

		updatedDeploy := &appsv1.Deployment{}
		client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-gunicorn", Namespace: namespace}, updatedDeploy)
		if updatedDeploy.Spec.Template.Spec.Containers[0].Image == "wrong/image:tag" {
			t.Error("Gunicorn image not updated")
		}
	})

	t.Run("ensureScheduler", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		err := r.ensureScheduler(context.TODO(), bench)
		if err != nil {
			t.Fatalf("ensureScheduler failed: %v", err)
		}

		deploy := &appsv1.Deployment{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-scheduler", Namespace: namespace}, deploy)
		if err != nil {
			t.Error("Scheduler deployment not created")
		}
	})

	t.Run("ensureSocketIO", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		err := r.ensureSocketIO(context.TODO(), bench)
		if err != nil {
			t.Fatalf("ensureSocketIO failed: %v", err)
		}

		deploy := &appsv1.Deployment{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-socketio", Namespace: namespace}, deploy)
		if err != nil {
			t.Error("SocketIO deployment not created")
		}

		svc := &corev1.Service{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-socketio", Namespace: namespace}, svc)
		if err != nil {
			t.Error("SocketIO service not created")
		}
	})

	t.Run("ensureStorage", func(t *testing.T) {
		sc := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "standard",
				Annotations: map[string]string{
					"storageclass.kubernetes.io/is-default-class": "true",
				},
			},
			Provisioner: "kubernetes.io/no-provisioner",
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench, sc).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		err := r.ensureBenchStorage(context.TODO(), bench)
		if err != nil {
			t.Fatalf("ensureBenchStorage failed: %v", err)
		}

		pvc := &corev1.PersistentVolumeClaim{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-sites", Namespace: namespace}, pvc)
		if err != nil {
			t.Error("PVC not created")
		}
	})

	t.Run("ensureNginx", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		err := r.ensureNginx(context.TODO(), bench)
		if err != nil {
			t.Fatalf("ensureNginx failed: %v", err)
		}

		deploy := &appsv1.Deployment{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-nginx", Namespace: namespace}, deploy)
		if err != nil {
			t.Error("Nginx deployment not created")
		}
	})

	t.Run("ensureWorkers", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		err := r.ensureWorkers(context.TODO(), bench)
		if err != nil {
			t.Fatalf("ensureWorkers failed: %v", err)
		}

		deploy := &appsv1.Deployment{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-worker-default", Namespace: namespace}, deploy)
		if err != nil {
			t.Error("Worker default deployment not created")
		}
	})
}

func TestFrappeBenchReconciler_Helpers(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))

	namespace := "test-ns"
	benchName := "test-bench"

	t.Run("getRedisImage", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		image := r.getRedisImage(bench)
		if image == "" {
			t.Error("Expected non-empty Redis image")
		}
	})

	t.Run("getGunicornReplicas", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		replicas := r.getGunicornReplicas(bench)
		if replicas <= 0 {
			t.Error("Expected positive replica count")
		}
	})

	t.Run("getNginxReplicas", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		replicas := r.getNginxReplicas(bench)
		if replicas <= 0 {
			t.Error("Expected positive replica count")
		}
	})

	t.Run("getSocketIOReplicas", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		replicas := r.getSocketIOReplicas(bench)
		if replicas <= 0 {
			t.Error("Expected positive replica count")
		}
	})

	t.Run("getWorkerDefaultReplicas", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		replicas := r.getWorkerDefaultReplicas(bench)
		if replicas <= 0 {
			t.Error("Expected positive replica count")
		}
	})

	t.Run("benchLabels", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		labels := r.benchLabels(bench)
		if labels["bench"] != benchName {
			t.Errorf("Expected bench name in labels, got %v", labels)
		}
	})

	t.Run("componentLabels", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		labels := r.componentLabels(bench, "gunicorn")
		if labels["component"] != "gunicorn" {
			t.Errorf("Expected component name in labels, got %v", labels)
		}
	})

	t.Run("getRedisResources", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		res := r.getRedisResources(bench)
		if res.Requests == nil && res.Limits == nil {
			t.Error("Expected some resource requirements")
		}
	})

	t.Run("getGunicornResources", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		res := r.getGunicornResources(bench)
		if res.Requests == nil && res.Limits == nil {
			t.Error("Expected some resource requirements")
		}
	})

	t.Run("getRedisAddress", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		addr := r.getRedisAddress(bench)
		if addr == "" {
			t.Error("Expected non-empty Redis address")
		}
	})

	t.Run("getPodSecurityContext", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		ctx := r.getPodSecurityContext(context.TODO(), bench)
		if ctx == nil {
			t.Error("Expected non-nil security context")
		}
	})

	t.Run("getContainerSecurityContext", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		ctx := r.getContainerSecurityContext(context.TODO(), bench)
		if ctx == nil {
			t.Error("Expected non-nil security context")
		}
	})

	t.Run("getWorkerLongReplicas", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		replicas := r.getWorkerLongReplicas(bench)
		if replicas <= 0 {
			t.Error("Expected positive replica count")
		}
	})

	t.Run("getWorkerShortReplicas", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		replicas := r.getWorkerShortReplicas(bench)
		if replicas <= 0 {
			t.Error("Expected positive replica count")
		}
	})

	t.Run("getNginxResources", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		res := r.getNginxResources(bench)
		if res.Requests == nil && res.Limits == nil {
			t.Error("Expected some resource requirements")
		}
	})

	t.Run("getSocketIOResources", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		res := r.getSocketIOResources(bench)
		if res.Requests == nil && res.Limits == nil {
			t.Error("Expected some resource requirements")
		}
	})

	t.Run("getSchedulerResources", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		res := r.getSchedulerResources(bench)
		if res.Requests == nil && res.Limits == nil {
			t.Error("Expected some resource requirements")
		}
	})

	t.Run("getWorkerDefaultResources", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: benchName, Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench).Build()
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme}

		res := r.getWorkerDefaultResources(bench)
		if res.Requests == nil && res.Limits == nil {
			t.Error("Expected some resource requirements")
		}
	})
}
