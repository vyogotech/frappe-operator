# MariaDB Database Integration

The Frappe Operator orchestrates MariaDB using the [MariaDB Operator](https://mariadb-operator.github.io/mariadb-operator/). This integration ensures secure, isolated, and automated database management for every Frappe site.

## Architectural Overview

### 1. Database Isolation
For security and compliance, the operator enforces strict database isolation:
-   Each **FrappeSite** gets its own dedicated database and a unique database user.
-   The operator manages this by creating `Database`, `User`, and `Grant` CRDs for every site.
-   Site Users only have permissions for their specific database, preventing cross-site data access.

### 2. Deployment Modes
-   **Shared Mode (Default)**: Multiple sites share a single large MariaDB instance (the `MariaDB` CR). This is cost-effective and easy to manage.
-   **Dedicated Mode**: A `FrappeSite` can be configured to use its own dedicated MariaDB instance for maximum isolation.

## Credential Management

### Site Database Credentials
-   **Automatic Generation**: When a site is created, the operator generates a cryptographically secure random password for the site's database user.
-   **Storage**: This password is stored in a Kubernetes secret (e.g., `[site-name]-db-password`).
-   **Injection**: During site initialization, the `SiteInit` job mounts this secret as a volume and reads it to run `bench new-site --no-setup-db`.

### MariaDB Root Credentials
The **root password** is required by the operator to perform administrative tasks that the site-specific user cannot perform.

#### Where is the Root Password Referenced?
The operator retrieves the root password dynamically using the following logic in `controllers/frappesite_controller.go`:

1.  **Shared Mode**:
    -   The operator identifies the central `MariaDB` instance (default name: `frappe-mariadb`).
    -   It reads the `spec.rootPasswordSecretKeyRef` from the `MariaDB` CR.
    -   It then fetches the referenced secret (e.g., `mariadb-root`) and reads the specific key (default: `password`) to get the cleartext password.

2.  **Dedicated Mode**:
    -   The operator looks for a secret named `[site-name]-mariadb-root` in the site's namespace.

#### When is it Used?
-   **Site Deletion**: To completely clean up, the operator uses root credentials to drop the database, drop the user, and remove grants.
-   **Schema Management**: While the MariaDB Operator handles the CRs, the Frappe Operator uses root credentials during initialization if manual database setup steps are required.

## Security Practices
-   **Secret Mounting**: Sensitive credentials are never passed as environment variables. They are mounted as files in `/run/secrets/` to prevent exposure via pod inspection tools.
-   **Least Privilege**: Site containers (Gunicorn, Workers) only have access to their specific site user credentials, never the MariaDB root password.
