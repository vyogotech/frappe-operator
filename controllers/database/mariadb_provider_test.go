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

package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testScheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(vyogotechv1.AddToScheme(testScheme))
}

func TestMariaDBProviderUnstructured_IsReady_NoDatabaseCR(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(testScheme).Build()
	p := NewMariaDBProvider(vyogotechv1.DatabaseConfig{}, client, testScheme).(*MariaDBProviderUnstructured)
	ctx := context.Background()
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "mysite", Namespace: "default"},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "mysite.local"},
	}
	ready, err := p.IsReady(ctx, site)
	require.NoError(t, err)
	assert.False(t, ready)
}

func TestMariaDBProvider_IsReady_AllReady(t *testing.T) {
	scheme := testScheme
	ns := "default"
	siteName := "mysite"
	dbName := siteName + "-db"
	userName := siteName + "-user"
	grantName := siteName + "-grant"

	dbObj := &unstructured.Unstructured{}
	dbObj.SetGroupVersionKind(DatabaseGVK)
	dbObj.SetName(dbName)
	dbObj.SetNamespace(ns)
	dbObj.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True"},
		},
	}

	userObj := &unstructured.Unstructured{}
	userObj.SetGroupVersionKind(UserGVK)
	userObj.SetName(userName)
	userObj.SetNamespace(ns)
	userObj.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True"},
		},
	}

	grantObj := &unstructured.Unstructured{}
	grantObj.SetGroupVersionKind(GrantGVK)
	grantObj.SetName(grantName)
	grantObj.SetNamespace(ns)
	grantObj.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True"},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(dbObj, userObj, grantObj).Build()
	p := NewMariaDBProvider(vyogotechv1.DatabaseConfig{}, client, testScheme).(*MariaDBProviderUnstructured)
	ctx := context.Background()
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: siteName, Namespace: ns},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "mysite.local"},
	}
	ready, err := p.IsReady(ctx, site)
	require.NoError(t, err)
	assert.True(t, ready)
}

func TestMariaDBProvider_GetCredentials(t *testing.T) {
	scheme := testScheme
	ns := "default"
	siteName := "mysite"
	userCRName := siteName + "-user"
	secretName := siteName + "-db-password"

	userObj := &unstructured.Unstructured{}
	userObj.SetGroupVersionKind(UserGVK)
	userObj.SetName(userCRName)
	userObj.SetNamespace(ns)
	require.NoError(t, unstructured.SetNestedField(userObj.Object, "dbuser123", "spec", "name"))
	require.NoError(t, unstructured.SetNestedField(userObj.Object, secretName, "spec", "passwordSecretKeyRef", "name"))
	require.NoError(t, unstructured.SetNestedField(userObj.Object, "password", "spec", "passwordSecretKeyRef", "key"))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
		Data:       map[string][]byte{"password": []byte("secretpass")},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(userObj, secret).Build()
	p := NewMariaDBProvider(vyogotechv1.DatabaseConfig{}, client, testScheme).(*MariaDBProviderUnstructured)
	ctx := context.Background()
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: siteName, Namespace: ns},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "mysite.local"},
	}
	creds, err := p.GetCredentials(ctx, site)
	require.NoError(t, err)
	assert.Equal(t, "dbuser123", creds.Username)
	assert.Equal(t, "secretpass", creds.Password)
	assert.Equal(t, secretName, creds.SecretName)
}

func TestMariaDBProvider_GetCredentials_UserNotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(testScheme).Build()
	p := NewMariaDBProvider(vyogotechv1.DatabaseConfig{}, client, testScheme).(*MariaDBProviderUnstructured)
	ctx := context.Background()
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "mysite", Namespace: "default"},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "mysite.local"},
	}
	_, err := p.GetCredentials(ctx, site)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "User CR")
}

func TestMariaDBProvider_Cleanup(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(testScheme).Build()
	p := NewMariaDBProvider(vyogotechv1.DatabaseConfig{}, client, testScheme).(*MariaDBProviderUnstructured)
	ctx := context.Background()
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "mysite", Namespace: "default"},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "mysite.local"},
	}
	err := p.Cleanup(ctx, site)
	require.NoError(t, err)
}

func TestMariaDBProvider_EnsureDatabase_MariaDBRef(t *testing.T) {
	scheme := testScheme
	ns := "default"
	siteName := "mysite"
	mariadbName := "shared-mariadb"

	mariadb := &unstructured.Unstructured{}
	mariadb.SetGroupVersionKind(MariaDBGVK)
	mariadb.SetName(mariadbName)
	mariadb.SetNamespace(ns)
	mariadb.Object["spec"] = map[string]interface{}{}

	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: siteName, Namespace: ns},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "mysite.local",
			DBConfig: vyogotechv1.DatabaseConfig{
				MariaDBRef: &vyogotechv1.NamespacedName{Name: mariadbName, Namespace: ns},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(mariadb, site).Build()
	p := NewMariaDBProvider(site.Spec.DBConfig, client, testScheme).(*MariaDBProviderUnstructured)
	ctx := context.Background()

	info, err := p.EnsureDatabase(ctx, site)
	require.NoError(t, err)
	assert.NotEmpty(t, info.Host)
	assert.Equal(t, "3306", info.Port)
	assert.NotEmpty(t, info.Name)
	assert.Equal(t, "mariadb", info.Provider)

	// Verify Database CR created
	dbCR := &unstructured.Unstructured{}
	dbCR.SetGroupVersionKind(DatabaseGVK)
	err = client.Get(ctx, types.NamespacedName{Name: siteName + "-db", Namespace: ns}, dbCR)
	require.NoError(t, err)

	// Verify User CR and secret created
	userCR := &unstructured.Unstructured{}
	userCR.SetGroupVersionKind(UserGVK)
	err = client.Get(ctx, types.NamespacedName{Name: siteName + "-user", Namespace: ns}, userCR)
	require.NoError(t, err)

	secret := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{Name: siteName + "-db-password", Namespace: ns}, secret)
	require.NoError(t, err)
	// Controller creates with StringData; fake client may store as StringData or Data
	assert.True(t, len(secret.Data["password"]) > 0 || len(secret.StringData["password"]) > 0,
		"secret should have password in Data or StringData")

	// Verify Grant CR created
	grantCR := &unstructured.Unstructured{}
	grantCR.SetGroupVersionKind(GrantGVK)
	err = client.Get(ctx, types.NamespacedName{Name: siteName + "-grant", Namespace: ns}, grantCR)
	require.NoError(t, err)
}

func TestMariaDBProvider_EnsureDatabase_SharedMode_NoMariaDB(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(testScheme).Build()
	ctx := context.Background()
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "mysite", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "mysite.local",
			DBConfig: vyogotechv1.DatabaseConfig{Mode: "shared"},
		},
	}
	p := NewMariaDBProvider(site.Spec.DBConfig, client, testScheme).(*MariaDBProviderUnstructured)
	_, err := p.EnsureDatabase(ctx, site)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shared MariaDB")
}

// TestEnsureUserCR_HostWildcard verifies that the User CR is created with
// host: "%" so that pods on any cluster node can connect to MariaDB, preventing
// OperationalError 1045 from cross-node pod IPs (10.42.x.x Flannel addresses).
func TestEnsureUserCR_HostWildcard(t *testing.T) {
	ns := "default"
	siteName := "mysite"
	mariadbName := "frappe-mariadb"

	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: siteName, Namespace: ns},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "mysite.local"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(site).Build()
	p := NewMariaDBProvider(vyogotechv1.DatabaseConfig{}, fakeClient, testScheme).(*MariaDBProviderUnstructured)
	ctx := context.Background()

	_, err := p.ensureUserCR(ctx, site, mariadbName, ns, "mysite_user")
	require.NoError(t, err)

	// Retrieve the created User CR and verify host field
	userCR := &unstructured.Unstructured{}
	userCR.SetGroupVersionKind(UserGVK)
	err = fakeClient.Get(ctx, types.NamespacedName{Name: siteName + "-user", Namespace: ns}, userCR)
	require.NoError(t, err)

	host, found, err := unstructured.NestedString(userCR.Object, "spec", "host")
	require.NoError(t, err)
	require.True(t, found, "spec.host must be present in User CR")
	assert.Equal(t, "%", host,
		"User CR host must be '%' for cross-node pod access; node-local host causes OperationalError 1045")
}

// TestEnsureGrantCR_HostWildcard verifies that the Grant CR is created with
// host: "%" matching the User CR, so MySQL resolves the grant for cross-node pods.
func TestEnsureGrantCR_HostWildcard(t *testing.T) {
	ns := "default"
	siteName := "mysite"
	mariadbName := "frappe-mariadb"

	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: siteName, Namespace: ns},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "mysite.local"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(site).Build()
	p := NewMariaDBProvider(vyogotechv1.DatabaseConfig{}, fakeClient, testScheme).(*MariaDBProviderUnstructured)
	ctx := context.Background()

	err := p.ensureGrantCR(ctx, site, mariadbName, ns, siteName+"_db", siteName+"_user")
	require.NoError(t, err)

	// Retrieve the created Grant CR and verify host field
	grantCR := &unstructured.Unstructured{}
	grantCR.SetGroupVersionKind(GrantGVK)
	err = fakeClient.Get(ctx, types.NamespacedName{Name: siteName + "-grant", Namespace: ns}, grantCR)
	require.NoError(t, err)

	host, found, err := unstructured.NestedString(grantCR.Object, "spec", "host")
	require.NoError(t, err)
	require.True(t, found, "spec.host must be present in Grant CR")
	assert.Equal(t, "%", host,
		"Grant CR host must be '%' to match User CR host; mismatch causes MySQL to reject connections")
}

// TestEnsureUserAndGrantCR_HostsMatch verifies that User and Grant CRs
// have the same host value, which is required for MySQL to apply the grant.
func TestEnsureUserAndGrantCR_HostsMatch(t *testing.T) {
	ns := "default"
	siteName := "mysite"
	mariadbName := "frappe-mariadb"

	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: siteName, Namespace: ns},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "mysite.local"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(site).Build()
	p := NewMariaDBProvider(vyogotechv1.DatabaseConfig{}, fakeClient, testScheme).(*MariaDBProviderUnstructured)
	ctx := context.Background()

	_, err := p.ensureUserCR(ctx, site, mariadbName, ns, siteName+"_user")
	require.NoError(t, err)
	err = p.ensureGrantCR(ctx, site, mariadbName, ns, siteName+"_db", siteName+"_user")
	require.NoError(t, err)

	userCR := &unstructured.Unstructured{}
	userCR.SetGroupVersionKind(UserGVK)
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: siteName + "-user", Namespace: ns}, userCR))

	grantCR := &unstructured.Unstructured{}
	grantCR.SetGroupVersionKind(GrantGVK)
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: siteName + "-grant", Namespace: ns}, grantCR))

	userHost, _, _ := unstructured.NestedString(userCR.Object, "spec", "host")
	grantHost, _, _ := unstructured.NestedString(grantCR.Object, "spec", "host")
	assert.Equal(t, userHost, grantHost,
		"User CR host (%q) and Grant CR host (%q) must match", userHost, grantHost)
}
