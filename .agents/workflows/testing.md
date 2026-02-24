---
description: Standard workflow for writing and running Frappe Operator tests
---

# Frappe Operator Testing Skill

This skill defines how AI agents should write and execute tests for the `frappe-operator`. It ensures all new code maintains the TDD culture.

## 1. Writing Tests
- **Framework:** The operator uses `ginkgo` and `gomega` for BDD-style testing. Do not use standard `testing.T` unless writing very simple, isolated unit tests.
- **Location:** Place test files directly next to the code being tested (e.g., `controllers/frappesite_controller_test.go`).
- **Mocking:** When writing controller tests, ALWAYS use the `FakeProvider` interface provided in the database package rather than attempting to connect to a real database.
- **Isolation:** Ensure every Ginkgo `It` block uses a uniquely generated Kubernetes namespace to prevent parallel execution collisions.

## 2. Running Local Unit/Integration Tests
Whenever you modify operator logic, you MUST execute the EnvTest suite to verify your changes:

```bash
# Run the standard controller test suite
make test
```

*Requirements:* This requires `kubebuilder` assets to be installed locally (EnvTest).

## 3. Running End-to-End (E2E) Tests
If the user asks you to verify changes against a real cluster, follow these steps to use Kind (Kubernetes in Docker):

```bash
# 1. Start the local kind cluster and registry (if not running)
make kind-cluster

# 2. Build the current image and push to the local Kind registry
make docker-build IMG=localhost:5001/frappe-operator:test
make docker-push IMG=localhost:5001/frappe-operator:test

# 3. Load the image into the Kind cluster
kind load docker-image localhost:5001/frappe-operator:test --name frappe-operator-test

# 4. Run the full e2e suite script
./scripts/e2e-suite.sh
```

## 4. Handling Test Failures (Auto-Correction)
- If `make test` fails, evaluate the Ginkgo failure trace.
- If an E2E test fails, aggressively use `kubectl logs` on the `frappe` / `test` namespaces to extract the operator panic or routing error.
- **Rule:** Do NOT modify `api/v1alpha1/.*_types.go` struct fields just to make a test pass. Fix the faulty logic in the controller.
