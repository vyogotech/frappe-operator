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
var frappebenchlog = logf.Log.WithName("frappebench-resource")

func (r *FrappeBench) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-vyogo-tech-v1alpha1-frappebench,mutating=false,failurePolicy=fail,sideEffects=None,groups=vyogo.tech,resources=frappebenches,verbs=create;update,versions=v1alpha1,name=vfrappebench.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &FrappeBench{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *FrappeBench) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	frappebenchlog.Info("validate create", "name", r.Name)

	if err := r.validateBench(); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *FrappeBench) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	frappebenchlog.Info("validate update", "name", r.Name)

	if err := r.validateBench(); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (r *FrappeBench) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	frappebenchlog.Info("validate delete", "name", r.Name)
	return nil, nil
}

func (r *FrappeBench) validateBench() error {
	// Validate FrappeVersion
	if r.Spec.FrappeVersion == "" {
		return fmt.Errorf("frappeVersion must be specified")
	}

	// Validate apps - at least one app source required if not using AppsJSON
	if len(r.Spec.Apps) == 0 && r.Spec.AppsJSON == "" {
		return fmt.Errorf("at least one app must be specified via apps or appsJSON")
	}

	// Validate replicas if specified
	if r.Spec.ComponentReplicas != nil {
		if r.Spec.ComponentReplicas.Gunicorn < 0 {
			return fmt.Errorf("componentReplicas.gunicorn must be non-negative")
		}
		if r.Spec.ComponentReplicas.Socketio < 0 {
			return fmt.Errorf("componentReplicas.socketio must be non-negative")
		}
	}

	return nil
}
