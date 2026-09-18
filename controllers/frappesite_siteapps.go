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
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// siteAppReferencesSite reports whether the SiteApp's siteRef resolves to the
// site (an empty siteRef.namespace means the SiteApp's own namespace).
func siteAppReferencesSite(siteApp *vyogotechv1.SiteApp, site *vyogotechv1.FrappeSite) bool {
	if siteApp.Spec.SiteRef == nil || siteApp.Spec.SiteRef.Name != site.Name {
		return false
	}
	siteNamespace := siteApp.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteApp.Namespace
	}
	return siteNamespace == site.Namespace
}

// deleteSiteApps deletes every SiteApp that references the site. Nothing else
// cascades: a SiteApp owns only its Jobs and carries no owner reference to its
// FrappeSite (siteRef may cross namespaces, an owner reference may not), so a
// deleted site used to leave its SiteApps behind, Pending on SiteNotFound and
// requeued every 30s forever. The SiteApp finalizer does not run an uninstall
// Job against a site that is terminating or gone, so these deletes complete
// without waiting on the site.
func (r *FrappeSiteReconciler) deleteSiteApps(ctx context.Context, site *vyogotechv1.FrappeSite) error {
	logger := log.FromContext(ctx)

	siteApps := &vyogotechv1.SiteAppList{}
	if err := r.List(ctx, siteApps); err != nil {
		return fmt.Errorf("failed to list SiteApps of site %s: %w", site.Name, err)
	}
	for i := range siteApps.Items {
		siteApp := &siteApps.Items[i]
		if !siteAppReferencesSite(siteApp, site) || siteApp.DeletionTimestamp != nil {
			continue
		}
		if err := r.Delete(ctx, siteApp); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete SiteApp %s/%s: %w", siteApp.Namespace, siteApp.Name, err)
		}
		logger.Info("Deleted SiteApp of the site being deleted", "siteApp", siteApp.Name, "namespace", siteApp.Namespace, "site", site.Name)
		r.Recorder.Eventf(site, corev1.EventTypeNormal, "SiteAppDeleted", "Deleted SiteApp %s/%s along with the site", siteApp.Namespace, siteApp.Name)
	}
	return nil
}
