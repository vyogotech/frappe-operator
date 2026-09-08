/*
Copyright 2024 Vyogo Technologies.

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

package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"strings"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultDedicatedStorageSize = "2Gi"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func generateDBName(site *vyogotechv1.FrappeSite) string {
	hash := hashString(site.Namespace + "/" + site.Name)[:8]
	safeName := sanitizeName(site.Spec.SiteName)
	dbName := fmt.Sprintf("_%s_%s", hash, safeName)
	if len(dbName) > 63 {
		dbName = dbName[:63]
	}
	return dbName
}

func generateDBUser(site *vyogotechv1.FrappeSite) string {
	return generateDBName(site)
}

// generatePGUserName returns a PostgreSQL role name for dedicated mode.
// The Percona and StackGres CRDs and Secrets work best with a DNS-label
// (^[a-z0-9]([-a-z0-9]*[a-z0-9])?$) user name.
func generatePGUserName(site *vyogotechv1.FrappeSite) string {
	return "u" + hashString(site.Namespace+"/"+site.Name)[:8]
}

// hashString is a stable 8-hex-character digest of s. It MUST be zero-padded:
// generateDBName and generatePGUserName slice [:8], and an unpadded %x drops
// leading zeros.
func hashString(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}

func sanitizeName(name string) string {
	reg := regexp.MustCompile("[^a-zA-Z0-9]+")
	safe := reg.ReplaceAllString(name, "_")
	safe = strings.Trim(safe, "_")
	return safe
}

func generatePassword(length int) string {
	b := make([]byte, length/2)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", length)))
	}
	return hex.EncodeToString(b)
}

func ensurePasswordSecret(ctx context.Context, cl client.Client, site *vyogotechv1.FrappeSite) (string, error) {
	passwordSecretName := fmt.Sprintf("%s-db-password", site.Name)
	passwordSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      passwordSecretName,
			Namespace: site.Namespace,
		},
	}

	err := cl.Get(ctx, types.NamespacedName{Name: passwordSecretName, Namespace: site.Namespace}, passwordSecret)
	var sitePassword string
	if errors.IsNotFound(err) {
		sitePassword = generatePassword(16)
		passwordSecret.StringData = map[string]string{
			"password": sitePassword,
		}
		// NO OwnerReference so Retain works
		if err := cl.Create(ctx, passwordSecret); err != nil {
			return "", err
		}
	} else if err == nil {
		sitePassword = string(passwordSecret.Data["password"])
		if len(passwordSecret.GetOwnerReferences()) > 0 {
			passwordSecret.SetOwnerReferences(nil)
			_ = cl.Update(ctx, passwordSecret)
		}
	} else {
		return "", err
	}
	return sitePassword, nil
}

// ensureDedicatedConfigured runs a one-time, idempotent Job as the superuser that
// fixes PostgreSQL 15+ public schema locking and sets search_path to public.
// Works identically across Percona and StackGres by parameterizing host and superuser secret.
func ensureDedicatedConfigured(ctx context.Context, cl client.Client, site *vyogotechv1.FrappeSite, host, superSecretName string) (bool, error) {
	dbName := generateDBName(site)
	dbUser := generatePGUserName(site)

	// Superuser secret must exist before configuring
	superSecret := &corev1.Secret{}
	if err := cl.Get(ctx, types.NamespacedName{Name: superSecretName, Namespace: site.Namespace}, superSecret); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	jobName := fmt.Sprintf("%s-db-configure", site.Name)
	job := &batchv1.Job{}
	err := cl.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if err == nil {
		if job.Status.Succeeded > 0 {
			return true, nil
		}
		if job.Status.Failed > 0 {
			return false, fmt.Errorf("dedicated postgres configure job failed")
		}
		return false, nil
	}
	if !errors.IsNotFound(err) {
		return false, err
	}

	script := fmt.Sprintf(`set -e
if [ -f /tmp/super/password ]; then
  export PGPASSWORD="$(cat /tmp/super/password)"
elif [ -f /tmp/super/superuser-password ]; then
  export PGPASSWORD="$(cat /tmp/super/superuser-password)"
fi
if [ -f /tmp/super/user ]; then
  SUPER="$(cat /tmp/super/user)"
elif [ -f /tmp/super/superuser-user ]; then
  SUPER="$(cat /tmp/super/superuser-user)"
else
  SUPER="postgres"
fi
psql -v ON_ERROR_STOP=1 -h "%s" -p 5432 -U "$SUPER" -d "%s" <<SQL
ALTER DATABASE "%s" OWNER TO "%s";
GRANT ALL ON SCHEMA public TO "%s";
ALTER ROLE "%s" IN DATABASE "%s" SET search_path TO public;
SQL
`, host, dbName, dbName, dbUser, dbUser, dbUser, dbName)

	container := resources.NewContainerBuilder("pg-configure", "postgres:16-alpine").
		WithCommand("sh", "-c").
		WithResources(vyogotechv1.ResolveJobResources(nil, vyogotechv1.JobKindMaintenance)).
		WithArgs(script).
		WithVolumeMount("super", "/tmp/super").
		Build()

	configureJob := resources.NewJobBuilder(jobName, site.Namespace).
		WithLabels(map[string]string{"app": "frappe", "site": site.Name}).
		WithContainer(container).
		WithSecretVolume("super", superSecretName, resources.Int32Ptr(0444)).
		MustBuild()

	if err := cl.Create(ctx, configureJob); err != nil && !errors.IsAlreadyExists(err) {
		return false, err
	}
	return false, nil
}

func getSharedHostPort(site *vyogotechv1.FrappeSite) (string, string, error) {
	port := "5432"
	if site.Spec.DBConfig.Port != "" {
		port = site.Spec.DBConfig.Port
	}

	if site.Spec.DBConfig.Host != "" {
		return site.Spec.DBConfig.Host, port, nil
	}

	host := "frappe-postgres-pgbouncer" // Default for Percona shared cluster
	if site.Spec.DBConfig.PostgresRef != nil && site.Spec.DBConfig.PostgresRef.Name != "" {
		host = site.Spec.DBConfig.PostgresRef.Name + "-pgbouncer"
	}

	ns := site.Namespace
	if site.Spec.DBConfig.PostgresRef != nil && site.Spec.DBConfig.PostgresRef.Namespace != "" {
		ns = site.Spec.DBConfig.PostgresRef.Namespace
	}

	return fmt.Sprintf("%s.%s.svc.cluster.local", host, ns), port, nil
}

func createIfAbsent(ctx context.Context, cl client.Client, obj *unstructured.Unstructured) error {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	err := cl.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
	if errors.IsNotFound(err) {
		return cl.Create(ctx, obj)
	}
	return err
}
