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

// Package circuitbreaker provides a circuit breaker implementation for resilient external service calls
package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed allows requests to pass through
	StateClosed State = iota
	// StateOpen blocks all requests
	StateOpen
	// StateHalfOpen allows limited requests to test if the service has recovered
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests is returned when too many requests are made in half-open state
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// Config defines circuit breaker configuration
type Config struct {
	// Name identifies this circuit breaker instance
	Name string

	// MaxFailures is the number of consecutive failures before opening the circuit
	MaxFailures int

	// Timeout is how long the circuit stays open before moving to half-open
	Timeout time.Duration

	// HalfOpenMaxRequests is the max concurrent requests allowed in half-open state
	HalfOpenMaxRequests int

	// OnStateChange is called when the state changes
	OnStateChange func(name string, from, to State)
}

// DefaultConfig returns default circuit breaker configuration
func DefaultConfig(name string) Config {
	return Config{
		Name:                name,
		MaxFailures:         5,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config Config

	mu              sync.RWMutex
	state           State
	failureCount    int
	successCount    int
	lastFailureTime time.Time
	halfOpenCount   int
}

// New creates a new circuit breaker with the given configuration
func New(config Config) *CircuitBreaker {
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.HalfOpenMaxRequests <= 0 {
		config.HalfOpenMaxRequests = 1
	}

	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Execute runs the given function through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn(ctx)

	cb.afterRequest(err)
	return err
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// FailureCount returns the current failure count
func (cb *CircuitBreaker) FailureCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failureCount
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenCount = 0

	if oldState != StateClosed && cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.config.Name, oldState, StateClosed)
	}
}

func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		// Check if timeout has passed
		if now.Sub(cb.lastFailureTime) >= cb.config.Timeout {
			cb.transitionToHalfOpen()
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		// Limit concurrent requests in half-open state
		if cb.halfOpenCount >= cb.config.HalfOpenMaxRequests {
			return ErrTooManyRequests
		}
		cb.halfOpenCount++
		return nil
	}

	return nil
}

func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.failureCount++
	cb.successCount = 0
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failureCount >= cb.config.MaxFailures {
			cb.transitionToOpen()
		}

	case StateHalfOpen:
		cb.transitionToOpen()
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	switch cb.state {
	case StateClosed:
		cb.failureCount = 0

	case StateHalfOpen:
		cb.successCount++
		cb.halfOpenCount--
		// If we've had enough successes, close the circuit
		if cb.successCount >= cb.config.HalfOpenMaxRequests {
			cb.transitionToClosed()
		}
	}
}

func (cb *CircuitBreaker) transitionToOpen() {
	oldState := cb.state
	cb.state = StateOpen
	cb.halfOpenCount = 0

	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.config.Name, oldState, StateOpen)
	}
}

func (cb *CircuitBreaker) transitionToHalfOpen() {
	oldState := cb.state
	cb.state = StateHalfOpen
	cb.successCount = 0
	cb.halfOpenCount = 0

	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.config.Name, oldState, StateHalfOpen)
	}
}

func (cb *CircuitBreaker) transitionToClosed() {
	oldState := cb.state
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenCount = 0

	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.config.Name, oldState, StateClosed)
	}
}
