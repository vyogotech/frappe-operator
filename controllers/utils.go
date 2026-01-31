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

package controllers

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/constants"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getBenchImage returns the image to use from the bench
// Priority: 1. bench.spec.imageConfig, 2. operator ConfigMap defaults, 3. hardcoded constants
func (r *FrappeSiteReconciler) getBenchImage(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) string {
	// Priority 1: Check bench-level ImageConfig override
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		image := bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			image = fmt.Sprintf("%s:%s", image, bench.Spec.ImageConfig.Tag)
		} else if bench.Spec.FrappeVersion != "" {
			// If tag not specified but version is, use version as tag
			image = fmt.Sprintf("%s:%s", image, bench.Spec.FrappeVersion)
		}
		return image
	}

	// Priority 2: Check operator ConfigMap defaults
	operatorConfig, err := r.getOperatorConfig(ctx, bench.Namespace)
	if err == nil && operatorConfig != nil {
		if defaultImage, ok := operatorConfig.Data["defaultFrappeImage"]; ok && defaultImage != "" {
			// If version is specified, replace tag in default image
			if bench.Spec.FrappeVersion != "" && bench.Spec.FrappeVersion != "latest" {
				// Extract repository from default image and append version tag
				parts := strings.Split(defaultImage, ":")
				if len(parts) == 2 {
					return fmt.Sprintf("%s:%s", parts[0], bench.Spec.FrappeVersion)
				}
			}
			return defaultImage
		}
	}

	// Priority 3: Fall back to constants with version
	if bench.Spec.FrappeVersion != "" && bench.Spec.FrappeVersion != "latest" {
		return fmt.Sprintf("docker.io/frappe/erpnext:%s", bench.Spec.FrappeVersion)
	}
	return constants.DefaultFrappeImage
}

// getOperatorConfig retrieves the operator configuration ConfigMap
func (r *FrappeSiteReconciler) getOperatorConfig(ctx context.Context, namespace string) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      "frappe-operator-config",
		Namespace: "frappe-operator-system", // Operator namespace
	}, configMap)
	return configMap, err
}

// isLocalDomain checks if a domain is a local development domain
func isLocalDomain(domain string) bool {
	return strings.HasSuffix(domain, ".local") ||
		strings.HasSuffix(domain, ".localhost") ||
		domain == "localhost"
}

// generatePassword generates a random password of specified length
func (r *FrappeSiteReconciler) generatePassword(length int) string {
	// Use alphanumeric only to avoid bash escaping issues
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	password := make([]byte, length)
	for i := range password {
		// Use crypto/rand for secure random generation
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fallback to timestamp-based if crypto/rand fails (shouldn't happen)
			password[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		} else {
			password[i] = charset[n.Int64()]
		}
	}
	return string(password)
}

// IsRouteAPIAvailable checks if the OpenShift route API group is available
func IsRouteAPIAvailable(config *rest.Config) bool {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false
	}

	apiGroupList, err := discoveryClient.ServerGroups()
	if err != nil {
		return false
	}

	for _, group := range apiGroupList.Groups {
		if group.Name == "route.openshift.io" {
			return true
		}
	}

	return false
}

func (r *FrappeSiteReconciler) isOpenShiftPlatform(ctx context.Context) bool {
	return r.IsOpenShift
}

// getDefaultUID returns the default UID for security contexts
// Defaults to 1001 (OpenShift standard) but can be overridden via FRAPPE_DEFAULT_UID env var
func getDefaultUID() *int64 {
	value := os.Getenv("FRAPPE_DEFAULT_UID")
	if value == "" {
		return nil
	}
	uid, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &uid
}

// getDefaultGID returns the default GID for security contexts
// Defaults to 0 (root group for OpenShift arbitrary UID support) but can be overridden via FRAPPE_DEFAULT_GID env var
func getDefaultGID() *int64 {
	value := os.Getenv("FRAPPE_DEFAULT_GID")
	if value == "" {
		return nil
	}
	gid, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &gid
}

// getDefaultFSGroup returns the default FSGroup for security contexts
// Defaults to 0 (root group for OpenShift arbitrary UID support) but can be overridden via FRAPPE_DEFAULT_FSGROUP env var
func getDefaultFSGroup() *int64 {
	value := os.Getenv("FRAPPE_DEFAULT_FSGROUP")
	if value == "" {
		return nil
	}
	fsGroup, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &fsGroup
}

// getEnvAsInt64 retrieves an environment variable as int64 with a default fallback
func getEnvAsInt64(key string, defaultValue int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// getNamespaceMCSLabel fetches the OpenShift MCS label (categories) for a namespace
// This ensures all pods in a bench share the same SELinux context to access shared volumes.
func getNamespaceMCSLabel(ctx context.Context, c client.Client, namespaceName string) string {
	ns := &corev1.Namespace{}
	err := c.Get(ctx, types.NamespacedName{Name: namespaceName}, ns)
	if err != nil {
		return ""
	}

	if ns.Annotations != nil {
		return ns.Annotations["openshift.io/sa.scc.mcs"]
	}
	return ""
}

// Helper functions for pointer types
func boolPtr(b bool) *bool {
	return &b
}

func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}

// applyDefaultJobTTL ensures every batch Job has a TTL to avoid resource leaks (uses pkg/resources constant)
func applyDefaultJobTTL(spec *batchv1.JobSpec) {
	if spec == nil || spec.TTLSecondsAfterFinished != nil {
		return
	}
	spec.TTLSecondsAfterFinished = int32Ptr(resources.DefaultJobTTL)
}
