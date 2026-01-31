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

package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := New(DefaultConfig("test"))

	if cb.State() != StateClosed {
		t.Errorf("expected initial state Closed, got %s", cb.State())
	}
	if cb.FailureCount() != 0 {
		t.Errorf("expected initial failure count 0, got %d", cb.FailureCount())
	}
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 3

	var stateChanges []State
	config.OnStateChange = func(name string, from, to State) {
		stateChanges = append(stateChanges, to)
	}

	cb := New(config)
	ctx := context.Background()

	testErr := errors.New("test error")

	// Simulate failures
	for i := 0; i < 3; i++ {
		_ = cb.Execute(ctx, func(ctx context.Context) error {
			return testErr
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state Open after %d failures, got %s", config.MaxFailures, cb.State())
	}

	if len(stateChanges) != 1 || stateChanges[0] != StateOpen {
		t.Errorf("expected state change to Open, got %v", stateChanges)
	}
}

func TestCircuitBreaker_OpenRejectsRequests(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 1
	config.Timeout = time.Hour // Long timeout to keep it open

	cb := New(config)
	ctx := context.Background()

	// Trip the circuit
	_ = cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("error")
	})

	if cb.State() != StateOpen {
		t.Fatal("expected circuit to be open")
	}

	// Try another request
	err := cb.Execute(ctx, func(ctx context.Context) error {
		t.Fatal("function should not be called when circuit is open")
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 1
	config.Timeout = 10 * time.Millisecond

	cb := New(config)
	ctx := context.Background()

	// Trip the circuit
	_ = cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("error")
	})

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Next request should be allowed (half-open)
	err := cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error after timeout, got %v", err)
	}

	// Should be closed now after successful request
	if cb.State() != StateClosed {
		t.Errorf("expected state Closed after successful half-open request, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 1
	config.Timeout = 10 * time.Millisecond

	cb := New(config)
	ctx := context.Background()

	// Trip the circuit
	_ = cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("error")
	})

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Fail again in half-open
	_ = cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("still failing")
	})

	// Should be open again
	if cb.State() != StateOpen {
		t.Errorf("expected state Open after half-open failure, got %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 1

	cb := New(config)
	ctx := context.Background()

	// Trip the circuit
	_ = cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("error")
	})

	if cb.State() != StateOpen {
		t.Fatal("expected circuit to be open")
	}

	// Reset
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("expected state Closed after reset, got %s", cb.State())
	}

	// Should work again
	err := cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 3

	cb := New(config)
	ctx := context.Background()

	// Two failures
	for i := 0; i < 2; i++ {
		_ = cb.Execute(ctx, func(ctx context.Context) error {
			return errors.New("error")
		})
	}

	if cb.FailureCount() != 2 {
		t.Errorf("expected 2 failures, got %d", cb.FailureCount())
	}

	// One success should reset
	_ = cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})

	if cb.FailureCount() != 0 {
		t.Errorf("expected failure count reset to 0 after success, got %d", cb.FailureCount())
	}
}

func TestCircuitBreaker_Concurrent(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 100

	cb := New(config)
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = cb.Execute(ctx, func(ctx context.Context) error {
				if id%2 == 0 {
					return errors.New("error")
				}
				return nil
			})
		}(i)
	}

	wg.Wait()

	// Just verify no panics/races occurred
	_ = cb.State()
	_ = cb.FailureCount()
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tc := range tests {
		if tc.state.String() != tc.expected {
			t.Errorf("expected %s for state %d, got %s", tc.expected, tc.state, tc.state.String())
		}
	}
}
