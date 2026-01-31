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

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var frappesitelog = logf.Log.WithName("frappesite-resource")

func (r *FrappeSite) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-vyogo-tech-v1alpha1-frappesite,mutating=false,failurePolicy=fail,sideEffects=None,groups=vyogo.tech,resources=frappesites,verbs=create;update,versions=v1alpha1,name=vfrappesite.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &FrappeSite{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *FrappeSite) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	frappesitelog.Info("validate create", "name", r.Name)

	if err := r.validateSite(); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *FrappeSite) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	frappesitelog.Info("validate update", "name", r.Name)

	if err := r.validateSite(); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (r *FrappeSite) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	frappesitelog.Info("validate delete", "name", r.Name)
	return nil, nil
}

func (r *FrappeSite) validateSite() error {
	// Validate site name
	if r.Spec.SiteName == "" {
		return fmt.Errorf("siteName cannot be empty")
	}

	// Validate bench reference
	if r.Spec.BenchRef == nil {
		return fmt.Errorf("benchRef must be specified")
	}
	if r.Spec.BenchRef.Name == "" {
		return fmt.Errorf("benchRef.name cannot be empty")
	}

	// Validate database mode (empty DBConfig is valid; defaults to shared)
	if r.Spec.DBConfig.Mode != "" {
		if r.Spec.DBConfig.Mode != "shared" && r.Spec.DBConfig.Mode != "dedicated" {
			return fmt.Errorf("dbConfig.mode must be either 'shared' or 'dedicated'")
		}
	}

	// If dedicated mode, validate MariaDB reference
	if r.Spec.DBConfig.Mode == "dedicated" {
		if r.Spec.DBConfig.MariaDBRef == nil {
			return fmt.Errorf("dbConfig.mariaDBRef must be specified for dedicated mode")
		}
		if r.Spec.DBConfig.MariaDBRef.Name == "" {
			return fmt.Errorf("dbConfig.mariaDBRef.name cannot be empty")
		}
	}

	return nil
}
