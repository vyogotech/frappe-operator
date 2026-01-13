package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestExternalProvider_GetCredentials(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = vyogotechv1alpha1.AddToScheme(scheme)

	ctx := context.Background()
	namespace := "test-ns"

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db-secret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte("testuser"),
			"password": []byte("testpass"),
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	provider := &ExternalProvider{
		client: client,
	}

	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			DBConfig: vyogotechv1alpha1.DatabaseConfig{
				ConnectionSecretRef: &corev1.SecretReference{
					Name:      "db-secret",
					Namespace: namespace,
				},
			},
		},
	}

	creds, err := provider.GetCredentials(ctx, site)
	assert.NoError(t, err)
	assert.Equal(t, "testuser", creds.Username)
	assert.Equal(t, "testpass", creds.Password)
}

func TestExternalProvider_IsReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = vyogotechv1alpha1.AddToScheme(scheme)

	ctx := context.Background()
	namespace := "test-ns"

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	provider := &ExternalProvider{
		client: client,
	}

	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			DBConfig: vyogotechv1alpha1.DatabaseConfig{
				Host: "rds.example.com",
				Port: "3306",
			},
		},
	}

	ready, err := provider.IsReady(ctx, site)
	assert.NoError(t, err)
	assert.True(t, ready)
}
