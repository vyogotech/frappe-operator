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
	"regexp"
	"strconv"
	"strings"
	"time"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/pkg/constants"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Frappe stores its own transient backups under "<site>/private/backups". On
// every `bench backup`, Frappe's delete_temp_backups() lists that directory and
// os.remove()s each entry — and is_file_old() returns True for anything that is
// not a regular file, so a *subdirectory* makes the NEXT backup crash with
// IsADirectoryError. The console/agent addresses each backup by a per-name
// subdirectory ("private/backups/<name>/database.sql.gz"), which is exactly that
// toxic case: the first backup succeeds, its leftover directory breaks all the
// rest.
//
// To keep the console's addressing scheme while staying compatible with Frappe,
// the operator relocates named per-backup artifacts to a sibling directory Frappe
// never scans. Callers still express paths with the conventional
// "private/backups/" segment; the operator rewrites that segment to
// "private/vyogo-backups/" for BOTH the backup output and the restore input, so
// the two always agree on the same physical location.
const (
	frappeBackupsSegment = "private/backups/"
	vyogoBackupsSegment  = "private/vyogo-backups/"
)

// relocateNamedBackupPath rewrites the conventional "private/backups/" segment of
// a backup/restore path to the operator's Frappe-safe "private/vyogo-backups/"
// directory. Paths that don't contain that segment (Frappe's own flat layout,
// custom absolute paths) are returned unchanged, so this is a no-op for anything
// that isn't a console-style named backup.
func relocateNamedBackupPath(p string) string {
	return strings.Replace(p, frappeBackupsSegment, vyogoBackupsSegment, 1)
}

// ensurePreflightBackup guarantees a completed SiteBackup exists before a
// mutating operation (app install/upgrade, bench migrate) proceeds, so there is
// always a rollback point. It is idempotent: the backup is keyed by name (the
// caller derives a per-generation name), so re-reconciles converge instead of
// spawning duplicates.
//
// Returns (done, err):
//   - done=true, err=nil  → a successful backup exists; the caller may proceed.
//   - done=false, err=nil → the backup is still running; the caller should requeue.
//   - err!=nil            → the backup failed (or a client error); the caller
//     should fail the operation rather than mutate an un-backed-up site.
//
// The backup deliberately carries no owner reference: it is a restore point that
// must outlive the SiteApp/SiteMigration that triggered it.
func ensurePreflightBackup(ctx context.Context, c client.Client, namespace, siteName, backupName string) (bool, error) {
	sb := &vyogotechv1.SiteBackup{}
	err := c.Get(ctx, types.NamespacedName{Name: backupName, Namespace: namespace}, sb)
	if apierrors.IsNotFound(err) {
		base := fmt.Sprintf("sites/%s/private/backups/%s", siteName, backupName)
		sb = &vyogotechv1.SiteBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backupName,
				Namespace: namespace,
				Labels: map[string]string{
					"app":                  "frappe",
					"site":                 siteName,
					"vyogo.tech/preflight": "true",
				},
			},
			Spec: vyogotechv1.SiteBackupSpec{
				Site:                   siteName,
				WithFiles:              true,
				Compress:               true,
				BackupPath:             base,
				BackupPathDB:           base + "/database.sql.gz",
				BackupPathConf:         base + "/site_config_backup.json",
				BackupPathFiles:        base + "/files.tar",
				BackupPathPrivateFiles: base + "/private-files.tar",
			},
		}
		if err := c.Create(ctx, sb); err != nil && !apierrors.IsAlreadyExists(err) {
			return false, err
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch sb.Status.Phase {
	case "Succeeded":
		return true, nil
	case "Failed":
		return false, fmt.Errorf("preflight backup %q failed: %s", backupName, sb.Status.Message)
	default:
		return false, nil
	}
}

// benchJobEnv returns the env vars a `bench` job pod (backup, migrate, restore)
// needs to run correctly against a site. The bench deployment/worker pods set
// the same set; job pods historically didn't, which broke two things:
//   - USER/HOME: bench's getpass.getuser() calls getpwuid(); when the pod runs
//     as a uid that isn't in /etc/passwd (the operator pins runAsUser but images
//     may build a different frappe uid, e.g. 1001), that raises
//     "OSError: No username set in the environment". Setting USER avoids it.
//   - PYTHONPATH: SiteApp installs apps into sites/apps on the shared PVC (not
//     the image) and relies on this path + the generated sitecustomize.py to
//     load them. Without it, `bench` crashes with "ModuleNotFoundError: No
//     module named '<app>'" — e.g. a backup of a site with a SiteApp-installed
//     app fails to load that app's commands.
func benchJobEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "USER", Value: "frappe"},
		{Name: "HOME", Value: "/home/frappe"},
		{Name: "PYTHONPATH", Value: "/tmp/pip:/home/frappe/frappe-bench/sites/apps"},
	}
}

// benchImageTag maps a bench's FrappeVersion onto the tag scheme the published
// bench images actually use. A bare numeric major ("16") becomes "version-16"
// (the vyogotech *-for-operator images and upstream frappe/erpnext both publish
// version-N tags); a full semver ("16.0.0") keeps a leading "v" ("v16.0.0"),
// which is what upstream tags point releases with. Anything else ("develop",
// "latest", already-prefixed values) is passed through unchanged. Rewriting a
// bare major to "v16" produced a tag that does not exist on any of these
// registries, so every bench without an explicit imageConfig ImagePullBackOff'd.
func benchImageTag(version string) string {
	if version == "" || version == "latest" {
		return version
	}
	if ok, _ := regexp.MatchString(`^\d+$`, version); ok {
		return "version-" + version
	}
	if ok, _ := regexp.MatchString(`^\d+\.[\d.]+$`, version); ok {
		return "v" + version
	}
	return version
}

// getBenchImage returns the image to use from the bench
// Priority: 1. bench.spec.imageConfig, 2. operator ConfigMap defaults, 3. hardcoded constants
func (r *FrappeSiteReconciler) getBenchImage(ctx context.Context, bench *vyogotechv1.FrappeBench) string {
	// Priority 1: Check bench-level ImageConfig override
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		image := bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			image = fmt.Sprintf("%s:%s", image, bench.Spec.ImageConfig.Tag)
		} else if bench.Spec.FrappeVersion != "" {
			// If tag not specified but version is, use version as tag
			image = fmt.Sprintf("%s:%s", image, benchImageTag(bench.Spec.FrappeVersion))
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
					return fmt.Sprintf("%s:%s", parts[0], benchImageTag(bench.Spec.FrappeVersion))
				}
			}
			return defaultImage
		}
	}

	// Priority 3: Fall back to constants with version
	if bench.Spec.FrappeVersion != "" && bench.Spec.FrappeVersion != "latest" {
		return fmt.Sprintf("docker.io/frappe/erpnext:%s", benchImageTag(bench.Spec.FrappeVersion))
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
		return int64Ptr(1000)
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
		return int64Ptr(0)
	}
	gid, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &gid
}

// getDefaultFSGroup returns the default FSGroup for security contexts
// Defaults to 1001 (Frappe standard) but can be overridden via FRAPPE_DEFAULT_FSGROUP env var
func getDefaultFSGroup() *int64 {
	value := os.Getenv("FRAPPE_DEFAULT_FSGROUP")
	if value == "" {
		return int64Ptr(1000)
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
