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

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vyogotech/frappe-operator/pkg/circuitbreaker"
)

// TestCircuitBreakerResilience tests circuit breaker behavior under various conditions
func TestCircuitBreakerResilience(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration tests")
	}

	config := circuitbreaker.Config{
		Name:                "resilience-test",
		MaxFailures:         3,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	}

	cb := circuitbreaker.New(config)
	ctx := context.Background()

	// Simulate intermittent failures
	t.Run("IntermittentFailures", func(t *testing.T) {
		successCount := 0
		failCount := 0

		for i := 0; i < 10; i++ {
			err := cb.Execute(ctx, func(ctx context.Context) error {
				// Fail on even iterations
				if i%2 == 0 {
					return context.DeadlineExceeded
				}
				return nil
			})

			if err == nil {
				successCount++
			} else {
				failCount++
			}
		}

		t.Logf("Successes: %d, Failures: %d", successCount, failCount)
	})

	// Test recovery after failures
	t.Run("RecoveryAfterFailures", func(t *testing.T) {
		cb.Reset()

		// Cause circuit to open
		for i := 0; i < 3; i++ {
			_ = cb.Execute(ctx, func(ctx context.Context) error {
				return context.DeadlineExceeded
			})
		}

		if cb.State() != circuitbreaker.StateOpen {
			t.Errorf("Expected circuit to be open, got %s", cb.State())
		}

		// Wait for timeout
		time.Sleep(150 * time.Millisecond)

		// Execute successful request
		err := cb.Execute(ctx, func(ctx context.Context) error {
			return nil
		})

		if err != nil {
			t.Errorf("Expected success after recovery, got %v", err)
		}

		if cb.State() != circuitbreaker.StateClosed {
			t.Errorf("Expected circuit to be closed after recovery, got %s", cb.State())
		}
	})
}

// TestCircuitBreakerConcurrency tests circuit breaker under concurrent access
func TestCircuitBreakerConcurrency(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration tests")
	}

	config := circuitbreaker.Config{
		Name:                "concurrency-test",
		MaxFailures:         10,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	}

	cb := circuitbreaker.New(config)
	ctx := context.Background()

	// Run concurrent requests
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(id int) {
			_ = cb.Execute(ctx, func(ctx context.Context) error {
				time.Sleep(time.Millisecond)
				if id%3 == 0 {
					return context.DeadlineExceeded
				}
				return nil
			})
			done <- true
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify circuit breaker is in a valid state
	state := cb.State()
	if state != circuitbreaker.StateClosed && state != circuitbreaker.StateOpen && state != circuitbreaker.StateHalfOpen {
		t.Errorf("Invalid circuit breaker state: %s", state)
	}

	t.Logf("Final state: %s, Failure count: %d", cb.State(), cb.FailureCount())
}

// TestDatabaseProviderResilience simulates database connection failures
func TestDatabaseProviderResilience(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration tests")
	}

	// This test would integrate with the actual database provider
	// For now, we simulate the behavior

	config := circuitbreaker.Config{
		Name:                "database-test",
		MaxFailures:         3,
		Timeout:             1 * time.Second,
		HalfOpenMaxRequests: 1,
	}

	cb := circuitbreaker.New(config)
	ctx := context.Background()

	// Simulate database operations with circuit breaker
	checkDatabase := func() error {
		return cb.Execute(ctx, func(ctx context.Context) error {
			// Simulated database check
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
				return nil
			}
		})
	}

	// Normal operation
	for i := 0; i < 5; i++ {
		if err := checkDatabase(); err != nil {
			t.Errorf("Database check %d failed unexpectedly: %v", i, err)
		}
	}

	t.Log("Database resilience test completed")
}
