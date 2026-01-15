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

package backoff

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBackoff(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backoff Suite")
}

var _ = Describe("ExponentialBackoff", func() {
	It("should calculate exponential backoff correctly", func() {
		base := 1 * time.Second
		max := 60 * time.Second

		// Test attempt 0
		duration := ExponentialBackoff(base, 0, max)
		Expect(duration).To(Equal(base))

		// Test attempt 1 (should be 2 seconds)
		duration = ExponentialBackoff(base, 1, max)
		Expect(duration).To(Equal(2 * time.Second))

		// Test attempt 2 (should be 4 seconds)
		duration = ExponentialBackoff(base, 2, max)
		Expect(duration).To(Equal(4 * time.Second))

		// Test attempt 10 (should be capped at max)
		duration = ExponentialBackoff(base, 10, max)
		Expect(duration).To(Equal(max))
	})

	It("should handle negative attempt", func() {
		base := 1 * time.Second
		max := 60 * time.Second
		duration := ExponentialBackoff(base, -1, max)
		Expect(duration).To(Equal(base))
	})

	It("should calculate requeue interval", func() {
		base := 1 * time.Second
		max := 60 * time.Second
		interval, shouldRequeue := CalculateRequeueInterval(base, 2, max)
		Expect(shouldRequeue).To(BeTrue())
		Expect(interval).To(Equal(4 * time.Second))
	})
})
