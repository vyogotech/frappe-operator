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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// siteAdminPassword resolves the Administrator password of a FrappeSite the
// same way the site itself was created with it: the Secret named by
// spec.adminPasswordSecretRef first, then the operator-generated
// "<site>-admin" Secret, then the historical "<site>-admin-password"
// convention some controllers used to require. Every controller that logs in
// to the site (custom fields, scripts, webhooks, user permissions, property
// setters) goes through here, so a site declared with adminPasswordSecretRef
// no longer fails those CRs with "Secret <site>-admin-password not found".
func siteAdminPassword(ctx context.Context, c client.Client, site *vyogotechv1.FrappeSite) (string, error) {
	var candidates []types.NamespacedName
	if ref := site.Spec.AdminPasswordSecretRef; ref != nil && ref.Name != "" {
		ns := ref.Namespace
		if ns == "" {
			ns = site.Namespace
		}
		candidates = append(candidates, types.NamespacedName{Name: ref.Name, Namespace: ns})
	}
	candidates = append(candidates,
		types.NamespacedName{Name: fmt.Sprintf("%s-admin", site.Name), Namespace: site.Namespace},
		types.NamespacedName{Name: fmt.Sprintf("%s-admin-password", site.Name), Namespace: site.Namespace},
	)
	var lastErr error
	for _, key := range candidates {
		secret := &corev1.Secret{}
		if err := c.Get(ctx, key, secret); err != nil {
			lastErr = err
			continue
		}
		if pw := string(secret.Data["password"]); pw != "" {
			return pw, nil
		}
		lastErr = fmt.Errorf("password key missing or empty in secret %s", key.Name)
	}
	return "", fmt.Errorf("admin password for site %s: %w", site.Name, lastErr)
}
