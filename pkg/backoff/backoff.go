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

package backoff

import (
	"math"
	"time"
)

// ExponentialBackoff calculates exponential backoff duration
// base: base duration (e.g., 1 second)
// attempt: current attempt number (0-indexed)
// max: maximum duration cap
func ExponentialBackoff(base time.Duration, attempt int, max time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// Calculate exponential: base * 2^attempt
	duration := float64(base) * math.Pow(2, float64(attempt))

	// Convert to duration and cap at max
	result := time.Duration(duration)
	if result > max {
		result = max
	}

	return result
}

// CalculateRequeueInterval calculates requeue interval with exponential backoff
// Returns the interval and whether to requeue
func CalculateRequeueInterval(base time.Duration, attempt int, max time.Duration) (time.Duration, bool) {
	interval := ExponentialBackoff(base, attempt, max)
	return interval, true
}
