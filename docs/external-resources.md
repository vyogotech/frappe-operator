# <i data-lucide="cloud"></i> External Resource Integration

The Frappe Operator supports connecting to externally managed PostgreSQL, MariaDB, and Redis instances. This is useful for production environments using managed services like AWS RDS, Google Cloud SQL, Azure Database, AWS ElastiCache, or existing on-premise database clusters.

---

## <i data-lucide="database"></i> External PostgreSQL & MariaDB

To use an external database instance, configure the `dbConfig` section in your `FrappeSite` custom resource.

### Configuration

Set `provider` to `external` and specify the database credentials secret:

```yaml
dbConfig:
  provider: external
  mode: external
  host: db.external.svc.cluster.local
  port: "5432" # or 3306 for MariaDB/MySQL
  connectionSecretRef:
    name: external-db-credentials
```

The `connectionSecretRef` must point to a Secret containing:
- `username`: The database user
- `password`: The user's password
- `database`: (Optional) The database name (defaults to siteName)
- `host`: (Optional, if omitted from spec)
- `port`: (Optional, if omitted from spec)

### How It Works

When `provider` is `external`, the operator:
1. Skips automated provisioning of local database clusters.
2. Resolves connection details from the spec and the referenced Secret.
3. Injects these credentials into the site initialization job and the final `site_config.json`.

---

## <i data-lucide="layers"></i> External Redis

To use an external Redis instance, configure the `redisConfig` section in your `FrappeBench` custom resource.

### Configuration

Set `external: true` and specify the host and connection details:

```yaml
redisConfig:
  external: true
  host: redis-external.frappe.svc.cluster.local
  port: 6379
  connectionSecretRef:
    name: external-redis-credentials
```

The `connectionSecretRef` (optional) contains:
- `password`: The Redis password (if authentication is required)

### How It Works

When `external` is `true`, the operator:
1. Skips creation of the internal Redis StatefulSets (`redis-cache` and `redis-queue`).
2. Resolves the full authenticated Redis URL (`redis://:password@host:port`).
3. Injects this URL into `common_site_config.json` via the bench init job.
4. Configures KEDA ScaledObjects (if enabled) with the authenticated address to monitor background queues.
5. Injects the URL into each site's `site_config.json` during initialization.

---

## <i data-lucide="alert-circle"></i> Troubleshooting

- **Network Connectivity**: Ensure the external service is reachable from the Kubernetes worker nodes where Frappe pods run (verify Security Groups and Firewalls).
- **Database Grants**: Verify that the provided database user has full administrative privileges (`ALL PRIVILEGES` or schema ownership) on the target database.
- **Authentication Credentials**: Ensure Secret keys (`username`, `password`, `database`) match the expected keys without trailing whitespaces or newlines.
