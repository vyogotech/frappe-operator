# <i data-lucide="boxes"></i> Frappe Microservices

A `FrappeMicroservice` runs a service built on
[frappe-microservice-lib](https://github.com/ATXINVOX/frappe-microservice-lib)
next to an existing `FrappeSite`. The library handles authentication, tenant
isolation and DocType logic. The operator does the Kubernetes wiring:

1. **Binds** the service to the site with a generated Secret holding the site
   name, the central-site URL and the site's database credentials.
2. **Runs** it as a Deployment.
3. **Exposes** it in-cluster through a ClusterIP Service. This always happens.
4. **Routes** path prefixes on the site's domain to it, through an Ingress (or
   OpenShift Routes). This only happens when `spec.routing.paths` is set.

There is no API gateway and no change to `site_config.json`.

---

## Internal-only service

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeMicroservice
metadata:
  name: pdf-generator
  namespace: tenant-acme          # must be the FrappeSite's namespace
spec:
  siteRef:
    name: acme-site
  image:
    repository: ghcr.io/myorg/pdf-service
    tag: "1.0.0"
```

The operator creates the Secret `pdf-generator-site`, the Deployment
`pdf-generator` and the Service `pdf-generator`. The monolith, its workers and
other microservices reach it at `http://pdf-generator:8000` from the same
namespace, or `http://pdf-generator.tenant-acme.svc.cluster.local:8000` from
anywhere in the cluster. It is never reachable from outside.

## Routed service

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeMicroservice
metadata:
  name: orders
  namespace: tenant-acme
spec:
  siteRef:
    name: acme-site
  image:
    repository: ghcr.io/myorg/orders-service
    tag: "1.0.0"
    pullSecrets:
      - name: regcred
  port: 8000
  replicas: 2
  routing:
    paths:
      - /orders
      - /api/v1/orders
  resources:
    requests: { cpu: 100m, memory: 256Mi }
    limits:   { cpu: 500m, memory: 512Mi }
  env:
    - name: LOG_LEVEL
      value: INFO
```

This adds the Ingress `orders-ingress` on the site's domain. Requests under
`/orders` and `/api/v1/orders` go to the microservice. Everything else on the
domain stays on the monolith, because Ingress controllers pick the longest
matching prefix:

```
https://erp.acme.com
   ├── /                -> <site>-ingress  -> <bench>-nginx:8080   (monolith)
   ├── /orders          -> orders-ingress  -> orders:8000
   └── /api/v1/orders   -> orders-ingress  -> orders:8000
```

Because the microservice shares the site's host, the browser sends the site's
`sid` cookie with every request. The library validates it against the central
site.

---

## Spec

| Field | Required | Description |
|---|---|---|
| `siteRef.name` | yes | `FrappeSite` in the same namespace. Immutable. |
| `image` | yes | `repository` (required), `tag` (default `latest`), `pullPolicy`, `pullSecrets`. |
| `port` | no | Port the container listens on and the Service exposes. Default `8000`, range 1–65535. Passed to the container as `PORT`. |
| `routing.paths` | no | Path prefixes on the site's domain. When omitted, no Ingress or Route is created. |
| `replicas` | no | Default `1`, minimum `0`. |
| `resources` | no | `requests` and `limits` for the container. |
| `env` | no | Extra environment variables. Binding keys are rejected. |

## Site binding Secret

The operator owns the Secret `<name>-site` and mounts it with `envFrom`. Do not
edit it; it is rewritten on every reconcile.

| Key | Value |
|---|---|
| `FRAPPE_SITE` | The site's `spec.siteName`. |
| `CENTRAL_SITE_URL` | `http://<bench>-nginx.<namespace>.svc.cluster.local:8080`. The library sends `FRAPPE_SITE` as the `Host` header, which is how the bench picks the site, so session checks stay inside the cluster. |
| `PORT` | `spec.port`. The library binds Gunicorn to it. |
| `DB_TYPE` | `mariadb` or `postgres`. |
| `DB_HOST`, `DB_PORT`, `DB_NAME` | The site's database, as recorded in the site's status. |
| `DB_USER`, `DB_PASSWORD` | The site's database user, read through the site's database provider. |

When the site's database password changes, the operator rewrites the Secret
and changes the `vyogo.tech/site-binding-hash` annotation on the pod template,
so the Deployment rolls. It also resyncs every five minutes.

Anything else the library reads, such as `REDIS_CACHE_HOST`,
`REDIS_QUEUE_HOST`, `ALLOW_CORS` or `LOG_LEVEL`, goes in `spec.env`.

## Deployment and Service

- One container named `microservice`, with the restricted security context the
  operator uses for bench pods.
- Liveness and readiness probes on `GET /health`.
- A ClusterIP Service named `<name>` on `spec.port`.

If a Secret, Deployment, Service, Ingress or Route with the operator's chosen
name already exists and is not owned by this CR, the operator leaves it alone
and reports a `NameConflict` condition.

## Routing

**Kubernetes.** One Ingress `<name>-ingress` with:

- host = the site's resolved domain,
- the site's IngressClass,
- `pathType: Prefix` for each path,
- TLS only when the site uses TLS (or HTTPS is enforced operator-wide). It
  references the site's certificate Secret and carries **no** `cert-manager.io`
  annotation, so cert-manager does not issue a second certificate for the same
  Secret.

**OpenShift.** One Route per path, on the site's Route host and with the site's
TLS termination. Path-based Routes cannot use passthrough TLS; a site with
passthrough termination gets `Routed=False` with reason `PassthroughUnsupported`.

Removing `routing` (or a path) deletes the Ingress (or the Route for that path).

If the site has `ingress.enabled: false`, nothing is routed and `Routed=False`
with reason `SiteIngressDisabled`.

`/orders` matches `/orders` and `/orders/...` but not `/orders-report`, which
is standard Ingress `Prefix` behaviour.

## Validation

All rules are CEL on the CRD, so the API server enforces them even when the
operator's admission webhooks are off. They only look at the object being saved.

- Each path is absolute, made of non-empty segments of `[A-Za-z0-9._~%-]`,
  with no trailing slash. `/` is rejected.
- No `.` or `..` segments.
- No duplicate paths, at most 16.
- Paths may not overlap the monolith's reserved routes: `/api/method/login`,
  `/login`, `/app`, `/assets`, `/files`, `/private`, `/socket.io`. A path that
  is a parent of one of them, such as `/api`, is rejected too.
- `env` may not set `FRAPPE_SITE`, `CENTRAL_SITE_URL`, `PORT` or any `DB_*`
  key.

**Path conflicts between microservices are not checked.** Two
`FrappeMicroservice`s claiming the same path on one site are resolved by the
ingress controller (ingress-nginx generally keeps the older Ingress) or by an
API gateway in front of it. On OpenShift the router rejects the later Route,
and that shows up in `Routed`.

## Status

```
$ kubectl get fms -n tenant-acme
NAME            SITE        PHASE   READY   SERVICE URL                                        AGE
orders          acme-site   Ready   2       http://orders.tenant-acme.svc.cluster.local:8000   5m
pdf-generator   acme-site   Ready   1       http://pdf-generator.tenant-acme.svc.cluster.local:8000   5m
```

| Field | Meaning |
|---|---|
| `phase` | `Ready` when `SiteReady` and `Available` are true and, if routed, `Routed` is true. `Pending` otherwise. `Failed` when the site cannot be supported. |
| `readyReplicas` | From the Deployment. |
| `serviceURL` | In-cluster URL. |
| `externalURLs` | One per routed path. |
| `observedGeneration` | Generation last reconciled. |

Conditions:

| Type | True when | Common reasons when False |
|---|---|---|
| `SiteReady` | The site is Ready and the binding Secret is in sync. | `SiteNotFound`, `SiteNotReady`, `DatabaseInfoUnavailable`, `CredentialsUnavailable`, `UnsupportedDatabase` |
| `Available` | Ready replicas reach `replicas` (or `replicas: 0`). | `ReplicasNotReady`, `NameConflict` |
| `Routed` | Kubernetes: the Ingress has a load balancer address. OpenShift: every Route is admitted. Only present when `routing` is set. | `IngressPending`, `RoutePending`, `RouteNotAdmitted`, `SiteIngressDisabled`, `PassthroughUnsupported`, `NameConflict` |

On Kubernetes, Ingress status does not report path conflicts, so a path that
loses to another Ingress can still show `Routed=True`.

## Limitations

- The microservice must be in the site's namespace.
- SQLite sites are not supported (`UnsupportedDatabase`).
- The microservice uses the site's own database user, so it has the same
  database access as the monolith.
- Autoscaling and separate microservice databases are not supported yet.
