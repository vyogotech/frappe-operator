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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// namespaceTerminating reports whether the namespace is being deleted. Once a
// namespace has a deletionTimestamp the API server refuses to create anything
// in it (Jobs, Secrets, ...), so a finalizer whose cleanup needs a Job can
// never finish and the namespace hangs in Terminating forever. Callers use
// this to fall back to "remove the finalizer, keep the data" in that case.
func namespaceTerminating(ctx context.Context, c client.Client, name string) bool {
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: name}, ns); err != nil {
		return false
	}
	return ns.DeletionTimestamp != nil
}
