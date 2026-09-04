package controllers

import (
	"context"
	"testing"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetBenchImage_VersionPrefix(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = vyogotechv1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	reconciler := &FrappeBenchReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}
	ctx := context.Background()

	tests := []struct {
		name             string
		frappeVersion    string
		imageConfig      *vyogotechv1.ImageConfig
		expectedImage    string
		expectedContains string
	}{
		{
			name:             "bare major maps to the published version-N tag",
			frappeVersion:    "15",
			imageConfig:      nil,
			expectedContains: ":version-15",
		},
		{
			name:             "version with v prefix stays unchanged",
			frappeVersion:    "v15",
			imageConfig:      nil,
			expectedContains: ":v15",
		},
		{
			name:             "bare major 14 maps to version-14",
			frappeVersion:    "14",
			imageConfig:      nil,
			expectedContains: ":version-14",
		},
		{
			name:          "custom repository ignored if no tag",
			frappeVersion: "15",
			imageConfig: &vyogotechv1.ImageConfig{
				Repository: "custom/repo",
			},
			expectedContains: ":version-15",
		},
		{
			name:          "custom image with full tag used directly",
			frappeVersion: "15",
			imageConfig: &vyogotechv1.ImageConfig{
				Repository: "custom/repo",
				Tag:        "sha-abc123",
			},
			expectedImage: "custom/repo:sha-abc123",
		},
		{
			name:             "development version stays unchanged",
			frappeVersion:    "develop",
			imageConfig:      nil,
			expectedContains: ":develop",
		},
		{
			name:             "custom tag like latest stays unchanged",
			frappeVersion:    "latest",
			imageConfig:      nil,
			expectedContains: ":latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bench := &vyogotechv1.FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bench",
					Namespace: "default",
				},
				Spec: vyogotechv1.FrappeBenchSpec{
					FrappeVersion: tt.frappeVersion,
					ImageConfig:   tt.imageConfig,
				},
			}

			image := reconciler.getBenchImage(ctx, bench)

			if tt.expectedImage != "" {
				if image != tt.expectedImage {
					t.Errorf("getBenchImage() = %v, want %v", image, tt.expectedImage)
				}
			}

			if tt.expectedContains != "" {
				if len(image) == 0 {
					t.Errorf("getBenchImage() returned empty string")
				}
				// Check if the image contains the expected version tag
				found := false
				for i := 0; i < len(image)-len(tt.expectedContains)+1; i++ {
					if image[i:i+len(tt.expectedContains)] == tt.expectedContains {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("getBenchImage() = %v, expected to contain %v", image, tt.expectedContains)
				}
			}
		})
	}
}
