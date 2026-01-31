package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/circuitbreaker"
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

// TestCircuitBreakerProvider_IsReady_ReturnsErrWhenCircuitOpen verifies that when the circuit is open,
// the wrapped provider returns ErrCircuitOpen instead of calling the underlying provider.
func TestCircuitBreakerProvider_IsReady_ReturnsErrWhenCircuitOpen(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = vyogotechv1alpha1.AddToScheme(scheme)
	ctx := context.Background()
	namespace := "test-ns"

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	inner := NewExternalProvider(client)
	cb := circuitbreaker.New(circuitbreaker.Config{
		Name:        "test-db",
		MaxFailures: 1,
		Timeout:     time.Hour, // keep circuit open for the test
	})
	// Open the circuit by recording MaxFailures (1)
	_ = cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })
	require.Equal(t, circuitbreaker.StateOpen, cb.State(), "circuit should be open")

	wrapped := NewCircuitBreakerProvider(inner, cb)
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			DBConfig: vyogotechv1alpha1.DatabaseConfig{
				ConnectionSecretRef: &corev1.SecretReference{Name: "db-secret", Namespace: namespace},
			},
		},
	}

	_, err := wrapped.IsReady(ctx, site)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, circuitbreaker.ErrCircuitOpen), "expected ErrCircuitOpen, got: %v", err)
}

func TestExternalProvider_EnsureDatabase_HostInSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = vyogotechv1alpha1.AddToScheme(scheme)
	ctx := context.Background()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	provider := NewExternalProvider(client)
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			SiteName: "mysite",
			DBConfig: vyogotechv1alpha1.DatabaseConfig{
				Host: "rds.example.com",
				Port: "3306",
			},
		},
	}
	info, err := provider.EnsureDatabase(ctx, site)
	require.NoError(t, err)
	assert.Equal(t, "rds.example.com", info.Host)
	assert.Equal(t, "3306", info.Port)
	assert.Equal(t, "mysite", info.Name)
	assert.Equal(t, "mariadb", info.Provider)
}

func TestExternalProvider_EnsureDatabase_MissingHost(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = vyogotechv1alpha1.AddToScheme(scheme)
	ctx := context.Background()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	provider := NewExternalProvider(client)
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			SiteName: "mysite",
			DBConfig: vyogotechv1alpha1.DatabaseConfig{},
		},
	}
	_, err := provider.EnsureDatabase(ctx, site)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is required")
}

func TestExternalProvider_Cleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = vyogotechv1alpha1.AddToScheme(scheme)
	ctx := context.Background()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	provider := NewExternalProvider(client)
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
		Spec:       vyogotechv1alpha1.FrappeSiteSpec{},
	}
	err := provider.Cleanup(ctx, site)
	assert.NoError(t, err)
}

func TestCircuitBreakerProvider_GetCredentials_WhenClosed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = vyogotechv1alpha1.AddToScheme(scheme)
	ctx := context.Background()
	namespace := "test-ns"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-secret", Namespace: namespace},
		Data: map[string][]byte{
			"username": []byte("u"),
			"password": []byte("p"),
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	inner := NewExternalProvider(client)
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "db", MaxFailures: 5, Timeout: time.Second})
	wrapped := NewCircuitBreakerProvider(inner, cb)
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			DBConfig: vyogotechv1alpha1.DatabaseConfig{
				ConnectionSecretRef: &corev1.SecretReference{Name: "db-secret", Namespace: namespace},
			},
		},
	}
	creds, err := wrapped.GetCredentials(ctx, site)
	require.NoError(t, err)
	assert.Equal(t, "u", creds.Username)
	assert.Equal(t, "p", creds.Password)
}

func TestCircuitBreakerProvider_EnsureDatabase_WhenClosed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = vyogotechv1alpha1.AddToScheme(scheme)
	ctx := context.Background()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	inner := NewExternalProvider(client)
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "db", MaxFailures: 5, Timeout: time.Second})
	wrapped := NewCircuitBreakerProvider(inner, cb)
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			SiteName: "mysite",
			DBConfig: vyogotechv1alpha1.DatabaseConfig{Host: "rds.example.com", Port: "3306"},
		},
	}
	info, err := wrapped.EnsureDatabase(ctx, site)
	require.NoError(t, err)
	assert.Equal(t, "rds.example.com", info.Host)
	assert.Equal(t, "mysite", info.Name)
}

func TestCircuitBreakerProvider_Cleanup_WhenClosed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = vyogotechv1alpha1.AddToScheme(scheme)
	ctx := context.Background()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	inner := NewExternalProvider(client)
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "db", MaxFailures: 5, Timeout: time.Second})
	wrapped := NewCircuitBreakerProvider(inner, cb)
	site := &vyogotechv1alpha1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}}
	err := wrapped.Cleanup(ctx, site)
	assert.NoError(t, err)
}
