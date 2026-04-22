# External Resource Support

The Frappe Operator supports connecting to externally managed MariaDB and Redis instances. This is useful for production environments using managed services like AWS RDS, ElastiCache, or on-premise shared clusters.

## External MariaDB

To use an external MariaDB instance, configure the `dbConfig` section in your `FrappeSite` custom resource.

### Configuration

Set `provider` to `external` and provide the connection details.

```yaml
dbConfig:
  provider: external
  mode: private
  host: mariadb.external.svc.cluster.local
  port: "3306"
  connectionSecretRef:
    name: external-mariadb-creds
```

The `connectionSecretRef` must point to a secret containing:
- `username`: The database user.
- `password`: The user's password.
- `database`: (Optional) The database name (defaults to site name).
- `host`: (Optional, if not in spec).
- `port`: (Optional, if not in spec).

### How it Works

When `provider` is `external`, the operator:
1. Skips automated provisioning of a MariaDB instance via the MariaDB Operator.
2. Resolves connection details from the spec and the secret.
3. Injects these credentials into the site initialization job and the final `site_config.json`.
4. **Note**: The `bench` version must support the configured username. If the `bench` version uses the database name as the username, ensure both are set identical in the external provider.

---

## External Redis

To use an external Redis instance, configure the `redisConfig` section in your `FrappeBench` custom resource.

### Configuration

Set `external` to `true` and provide the host and port.

```yaml
redisConfig:
  external: true
  host: redis-external.frappe.svc.cluster.local
  port: 6379
  connectionSecretRef:
    name: external-redis-creds
```

The `connectionSecretRef` (optional) should contain:
- `password`: The Redis password.

### How it Works

When `external` is `true`, the operator:
1. Skips creation of the internal Redis StatefulSets (`redis-cache` and `redis-queue`).
2. Resolves the full Redis URL, including authentication if a secret is provided (`redis://:password@host:port`).
3. Injects this URL into `common_site_config.json` via the bench init job.
4. Configures KEDA ScaledObjects (if used) with the authenticated address to monitor background queues.
5. Injects the URL into each site's `site_config.json` during initialization.

---

## Troubleshooting

- **Connectivity**: Ensure the external service is reachable from the worker nodes where Frappe pods are running.
- **Permissions**: Verify that the provided database user has `ALL PRIVILEGES` on the specified database.
- **Auth Failures**: Check that the secret keys (`username`, `password`, `database`) match the expected names exactly.
