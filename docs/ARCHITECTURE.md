# Frappe Operator Architecture

This document provides machine-readable architectural context for AI coding assistants. It defines the strict boundary between the two Custom Resource Definitions (CRDs) mapped in this operator.

## 1. System Components

The operator is split into two distinct lifecycle stages.

### `FrappeBench` (Infrastructure Layer)
The `FrappeBench` is responsible for provisioning all stateless and stateful *shared* infrastructure.

- **Storage:** Creates a `PersistentVolumeClaim` (PVC) structured like a traditional frappe bench (`sites/`, `apps/`, `logs/`).
- **Initialization:** Executes a `Job` (e.g., `<bench-name>-init`) to sync the container `apps/` to the PVC and generate `apps.txt`.
- **Runtime Pods:** Deploys `Gunicorn`, `Nginx`, `Worker`, `Scheduler`, and `SocketIO` Deployments.
- **Cache/Queue:** Deploys dedicated `Redis` statefulsets (Cache and Queue).

*Rule for AI:* When asked to scale pods, update environment variables, or change Frappe versions/images, modify the `FrappeBench` controller/types.

### `FrappeSite` (Tenant Layer)
The `FrappeSite` is a single tenant *within* a FrappeBench. It uses the infrastructure provisioned by the Bench.

- **Database:** Connects to a MariaDB provider (internal or external) and provisions a unique database and user for the site.
- **Initialization:** Executes a `Job` to run `bench new-site <site-name>` creating the `site_config.json` inside the Bench's PVC.
- **Network Routing:** provisions `Ingress` or `Route` (OpenShift) manifests pointing `siteName` Host traffic to the Bench's Nginx service.

*Rule for AI:* When asked to configure custom domains, database credentials, or site endpoints, modify the `FrappeSite` controller/types.

## 2. Directory Layout
- `api/v1alpha1/`: Schema Definitions (Source of truth)
- `controllers/`: Operator Reconciliation loops
- `pkg/scripts/templates/`: Bash scripts executed inside Initialization Jobs
- `helm/frappe-operator/`: The official Helm chart (Synced using `make manifests`)

## 3. Storage Model
Because Frappe uses a shared filesystem for runtime components:
- `ReadWriteMany` (RWX) volumes are highly recommended.
- If the PVC is restricted to `ReadWriteOnce` (RWO), the operator uses a "fallback" mode using pod-affinity to force all Bench pods to schedule on a single node.
