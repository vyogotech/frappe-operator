package controllers

import (
	"context"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func testScheme(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := vyogotechv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func secret(ns, name, pw string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Data: map[string][]byte{"password": []byte(pw)}}
}

func TestSiteAdminPasswordPrefersDeclaredSecret(t *testing.T) {
	site := &vyogotechv1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "erp", Namespace: "t1"},
		Spec: vyogotechv1.FrappeSiteSpec{AdminPasswordSecretRef: &corev1.SecretReference{Name: "erp-secret"}}}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(
		secret("t1", "erp-secret", "declared"), secret("t1", "erp-admin-password", "legacy")).Build()
	pw, err := siteAdminPassword(context.Background(), c, site)
	if err != nil || pw != "declared" {
		t.Fatalf("got %q, %v; want the adminPasswordSecretRef value", pw, err)
	}
}

func TestSiteAdminPasswordFallsBackToGeneratedThenLegacy(t *testing.T) {
	site := &vyogotechv1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "erp", Namespace: "t1"}}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(secret("t1", "erp-admin", "generated")).Build()
	if pw, _ := siteAdminPassword(context.Background(), c, site); pw != "generated" {
		t.Fatalf("got %q; want the operator-generated <site>-admin secret", pw)
	}
	c = fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(secret("t1", "erp-admin-password", "legacy")).Build()
	if pw, _ := siteAdminPassword(context.Background(), c, site); pw != "legacy" {
		t.Fatalf("got %q; want the legacy <site>-admin-password secret", pw)
	}
	c = fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	if _, err := siteAdminPassword(context.Background(), c, site); err == nil {
		t.Fatalf("expected an error when no secret exists")
	}
}

func TestSiteInitJobNameAvoidsTheBenchJob(t *testing.T) {
	site := &vyogotechv1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "probe", Namespace: "t1"}}
	benchOwned := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "probe-init", Namespace: "t1",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "vyogo.tech/v1", Kind: "FrappeBench", Name: "probe", UID: "b"}}}}
	r := &FrappeSiteReconciler{Client: fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(benchOwned).Build()}
	if got := r.siteInitJobName(context.Background(), site); got != "probe-site-init" {
		t.Fatalf("got %q; a Job owned by the bench must not be reused for the site", got)
	}
	siteOwned := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "probe-init", Namespace: "t1",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "vyogo.tech/v1", Kind: "FrappeSite", Name: "probe", UID: "s"}}}}
	r = &FrappeSiteReconciler{Client: fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(siteOwned).Build()}
	if got := r.siteInitJobName(context.Background(), site); got != "probe-init" {
		t.Fatalf("got %q; the site's own Job keeps its name", got)
	}
	r = &FrappeSiteReconciler{Client: fake.NewClientBuilder().WithScheme(testScheme(t)).Build()}
	if got := r.siteInitJobName(context.Background(), site); got != "probe-init" {
		t.Fatalf("got %q; no Job yet keeps the default name", got)
	}
}

func TestNamespaceTerminating(t *testing.T) {
	now := metav1.Now()
	live := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "live"}}
	going := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "going", DeletionTimestamp: &now, Finalizers: []string{"kubernetes"}}}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(live, going).Build()
	if namespaceTerminating(context.Background(), c, "live") {
		t.Fatalf("live namespace reported as terminating")
	}
	if !namespaceTerminating(context.Background(), c, "going") {
		t.Fatalf("terminating namespace not detected")
	}
	if namespaceTerminating(context.Background(), c, "missing") {
		t.Fatalf("unknown namespace must not be treated as terminating")
	}
}

func TestCommonSiteConfigJSON(t *testing.T) {
	if got := commonSiteConfigJSON(nil); got != "{}" {
		t.Fatalf("empty config must render {}, got %s", got)
	}
	got := commonSiteConfigJSON(map[string]string{"server_script_enabled": "1", "mail_server": "smtp.example", "allow": "true"})
	for _, want := range []string{`"server_script_enabled":1`, `"mail_server":"smtp.example"`, `"allow":true`} {
		if !strings.Contains(got, want) {
			t.Fatalf("%s missing from %s", want, got)
		}
	}
}
