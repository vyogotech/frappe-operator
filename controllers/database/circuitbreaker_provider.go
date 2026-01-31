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
	"errors"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/circuitbreaker"
)

// CircuitBreakerProvider wraps a Provider and runs all calls through a circuit breaker.
// When the circuit is open, IsReady, EnsureDatabase, GetCredentials, and Cleanup return ErrCircuitOpen
// without calling the underlying provider.
type CircuitBreakerProvider struct {
	inner Provider
	cb    *circuitbreaker.CircuitBreaker
}

// NewCircuitBreakerProvider returns a Provider that wraps inner with the given circuit breaker.
func NewCircuitBreakerProvider(inner Provider, cb *circuitbreaker.CircuitBreaker) Provider {
	return &CircuitBreakerProvider{inner: inner, cb: cb}
}

// EnsureDatabase runs through the circuit breaker.
func (p *CircuitBreakerProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseInfo, error) {
	var info *DatabaseInfo
	err := p.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		info, e = p.inner.EnsureDatabase(ctx, site)
		return e
	})
	if err != nil {
		if errors.Is(err, circuitbreaker.ErrCircuitOpen) || errors.Is(err, circuitbreaker.ErrTooManyRequests) {
			return nil, err
		}
		return info, err
	}
	return info, nil
}

// IsReady runs through the circuit breaker.
func (p *CircuitBreakerProvider) IsReady(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (bool, error) {
	var ready bool
	err := p.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		ready, e = p.inner.IsReady(ctx, site)
		return e
	})
	if err != nil {
		if errors.Is(err, circuitbreaker.ErrCircuitOpen) || errors.Is(err, circuitbreaker.ErrTooManyRequests) {
			return false, err
		}
		return false, err
	}
	return ready, nil
}

// GetCredentials runs through the circuit breaker.
func (p *CircuitBreakerProvider) GetCredentials(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseCredentials, error) {
	var creds *DatabaseCredentials
	err := p.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		creds, e = p.inner.GetCredentials(ctx, site)
		return e
	})
	if err != nil {
		if errors.Is(err, circuitbreaker.ErrCircuitOpen) || errors.Is(err, circuitbreaker.ErrTooManyRequests) {
			return nil, err
		}
		return creds, err
	}
	return creds, nil
}

// Cleanup runs through the circuit breaker.
func (p *CircuitBreakerProvider) Cleanup(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error {
	return p.cb.Execute(ctx, func(ctx context.Context) error {
		return p.inner.Cleanup(ctx, site)
	})
}
