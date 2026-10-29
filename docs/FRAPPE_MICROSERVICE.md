# <i data-lucide="boxes"></i> Frappe Microservices

A `FrappeMicroservice` runs a service built on
[frappe-microservice-lib](https://github.com/ATXINVOX/frappe-microservice-lib)
next to an existing `FrappeSite`. The library handles authentication, tenant
isolation and DocType logic. The operator does the Kubernetes wiring:

1. **Binds** the image to the site with a generated Secret holding the site
   name, the central-site URL and the site's database credentials.
2. **Runs** it as one of three workload types, set by `spec.type`:
   - `Deployment` (the default): a long-running HTTP service with a ClusterIP
     Service, and path routing on the site's domain when
     `spec.deployment.routing.paths` is set.
   - `Job`: a one-time task, run once per spec change.
   - `CronJob`: a scheduled task.

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
  deployment:
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

## One-time task

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeMicroservice
metadata:
  name: orders-backfill
  namespace: tenant-acme
spec:
  type: Job
  siteRef:
    name: acme-site
  image:
    repository: ghcr.io/myorg/orders-service
    tag: "1.0.0"
  command: ["python", "-m", "orders.backfill"]
  args: ["--since", "2026-01-01"]
  job:
    backoffLimit: 2
    activeDeadlineSeconds: 3600
    ttlSecondsAfterFinished: 86400
```

## Scheduled task

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeMicroservice
metadata:
  name: orders-nightly-sync
  namespace: tenant-acme
spec:
  type: CronJob
  siteRef:
    name: acme-site
  image:
    repository: ghcr.io/myorg/orders-service
    tag: "1.0.0"
  command: ["python", "-m", "orders.nightly_sync"]
  cronJob:
    schedule: "0 2 * * *"
    timeZone: Australia/Sydney
  job:
    activeDeadlineSeconds: 1800
```

---

## Spec

Shared by every type:

| Field | Required | Description |
|---|---|---|
| `siteRef.name` | yes | `FrappeSite` in the same namespace. Immutable. |
| `image` | yes | `repository` (required), `tag` (default `latest`), `pullPolicy`, `pullSecrets`. |
| `type` | no | `Deployment` (default), `Job` or `CronJob`. Immutable. |
| `command`, `args` | `command` is required for `Job` and `CronJob` | Override the image entrypoint. The library's default entrypoint starts a server that never exits, so batch types must set a command. |
| `resources` | no | `requests` and `limits` for the container. |
| `env` | no | Extra environment variables. Binding keys are rejected. |

`deployment`, only valid with `type: Deployment`:

| Field | Description |
|---|---|
| `port` | Port the container listens on and the Service exposes. Default `8000`, range 1–65535. Passed to the container as `PORT`. |
| `replicas` | Default `1`, minimum `0`. With `autoscaling` set it is only used once autoscaling is removed. |
| `routing.paths` | Path prefixes on the site's domain. When omitted, no Ingress or Route is created. |
| `autoscaling` | Scales the Deployment with a HorizontalPodAutoscaler or a KEDA ScaledObject. See [Autoscaling](#autoscaling). |

`job`, valid with `type: Job` and `type: CronJob` (applies to every run):

| Field | Description |
|---|---|
| `backoffLimit` | Retries before a run is marked failed. Kubernetes defaults it to 6. |
| `activeDeadlineSeconds` | Maximum duration of a run. |
| `ttlSecondsAfterFinished` | Lets Kubernetes delete a finished Job. |

`cronJob`, required with `type: CronJob`:

| Field | Description |
|---|---|
| `schedule` | Cron schedule, required. |
| `timeZone` | For example `Australia/Sydney`. |
| `concurrencyPolicy` | `Allow`, `Forbid` (default) or `Replace`. |
| `suspend` | Pauses future runs. |
| `startingDeadlineSeconds` | Skip a missed run after this long. |
| `successfulJobsHistoryLimit`, `failedJobsHistoryLimit` | How many finished Jobs to keep. |

## Autoscaling

`deployment.autoscaling` makes the operator create and own an autoscaler that
targets the generated Deployment. Both kinds carry an owner reference, so they
are deleted with the microservice.

CPU or memory, through a `HorizontalPodAutoscaler`:

```yaml
spec:
  resources:
    requests: { cpu: 200m, memory: 256Mi }
  deployment:
    replicas: 2                  # used on removal of autoscaling
    autoscaling:
      minReplicas: 2             # default 1
      maxReplicas: 10
      targetCPUUtilizationPercentage: 70
      targetMemoryUtilizationPercentage: 80   # optional
```

Event-driven, through a KEDA `ScaledObject` (KEDA must be installed):

```yaml
spec:
  deployment:
    autoscaling:
      minReplicas: 0             # KEDA can scale to zero
      maxReplicas: 20
      keda:
        pollingInterval: 15
        cooldownPeriod: 120
        triggers:
          - type: redis
            metadata:
              address: redis-queue:6379
              listName: rq:queue:default
              listLength: "50"
            authenticationRef: redis-auth   # optional TriggerAuthentication
```

Behaviour:

- The operator sets the Deployment's `replicas` only when it creates the
  Deployment (to `minReplicas`) and never writes it again, so the autoscaler
  owns the count. Every other spec change still applies.
- `Available` compares ready replicas with the count the autoscaler chose, not
  with `deployment.replicas`.
- Removing `autoscaling` deletes the HPA or ScaledObject and returns the
  Deployment to `deployment.replicas`. Switching between HPA and KEDA deletes
  the other kind.
- An HPA or ScaledObject with the microservice's name that the operator does
  not own is left untouched and reported as `AutoscalingReady=False`,
  `NameConflict`.
- If `keda` is set and the KEDA CRDs are missing, the microservice still
  reconciles and `AutoscalingReady` is `False` with reason `KEDANotInstalled`.
  The Deployment stays at the count it has.
- HPAs need a metrics API (metrics-server). Without it `AutoscalingReady` is
  `False` with the HPA's own reason, such as `FailedGetResourceMetric`.
- The operator does not watch ScaledObjects, so a manual edit to one is
  corrected at the next periodic resync (about every 5 minutes) or the next
  change to the microservice.

## Site binding Secret

The operator owns the Secret `<name>-site` and mounts it with `envFrom`. Do not
edit it; it is rewritten on every reconcile.

| Key | Value |
|---|---|
| `FRAPPE_SITE` | The site's `spec.siteName`. |
| `CENTRAL_SITE_URL` | `http://<bench>-nginx.<namespace>.svc.cluster.local:8080`. The library sends `FRAPPE_SITE` as the `Host` header, which is how the bench picks the site, so session checks stay inside the cluster. |
| `PORT` | `spec.deployment.port`, for `type: Deployment` only. The library binds Gunicorn to it. |
| `DB_TYPE` | `mariadb` or `postgres`. |
| `DB_HOST`, `DB_PORT`, `DB_NAME` | The site's database, as recorded in the site's status. |
| `DB_USER`, `DB_PASSWORD` | The site's database user, read through the site's database provider. |

When the site's database password changes, the operator rewrites the Secret.
For a Deployment it also changes the `vyogo.tech/site-binding-version` annotation
on the pod template, so the Deployment rolls. A CronJob gets the same annotation
on its job template, so the next run uses the new credentials. A Job that has
already been created is not re-run. The operator also resyncs every five
minutes.

Anything else the library reads, such as `REDIS_CACHE_HOST`,
`REDIS_QUEUE_HOST`, `ALLOW_CORS` or `LOG_LEVEL`, goes in `spec.env`.

## Deployment and Service

- One container named `microservice`, with the restricted security context the
  operator uses for bench pods.
- Liveness and readiness probes on `GET /health`.
- A ClusterIP Service named `<name>` on `spec.deployment.port`.

## Job

- The operator creates one Job named `<name>-<hash>`, where the hash covers the
  image, command, args, env, resources and `job` settings. Changing any of them
  starts a new run and deletes the Job of the previous spec, stopping it if it
  is still running.
- An unchanged spec never runs twice. If `ttlSecondsAfterFinished` removes the
  finished Job, the operator keeps the recorded result and does not recreate
  it.
- Pods use `restartPolicy: Never`, no ports and no probes. Retries are governed
  by `backoffLimit`; the operator does not retry a failed run.

## CronJob

- One CronJob named `<name>`. The `cronJob` and `job` settings map onto it
  directly; `concurrencyPolicy` defaults to `Forbid`.
- Every run gets the binding Secret and the same pod settings as a Job.

If a Secret, Deployment, Service, Ingress, Route, Job or CronJob with the
operator's chosen name already exists and is not owned by this CR, the operator
leaves it alone and reports a `NameConflict` condition.

## Routing

Routing is only available for `type: Deployment`.

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
- `type` and `siteRef` are immutable.
- `deployment` is only valid for `Deployment`, `job` only for `Job` and
  `CronJob`, and `cronJob` is required exactly when the type is `CronJob`.
- `Job` and `CronJob` require a `command`.
- `autoscaling`: `minReplicas` is at most `maxReplicas`, and at least 1 unless
  `keda` is set. It needs a CPU or memory target, or `keda` with at least one
  trigger, and `keda` cannot be combined with CPU or memory targets (use a
  `cpu` or `memory` KEDA trigger). A CPU or memory target needs the matching
  `resources.requests`.
- A `Deployment` name is at most 63 characters, because it names the Service.
  A `CronJob` name is at most 52 characters, because Kubernetes appends a
  suffix to each run. `Job` names can be longer; the operator shortens the Job
  it creates.

**Path conflicts between microservices are not checked.** Two
`FrappeMicroservice`s claiming the same path on one site are resolved by the
ingress controller (ingress-nginx generally keeps the older Ingress) or by an
API gateway in front of it. On OpenShift the router rejects the later Route,
and that shows up in `Routed`.

## Status

```
$ kubectl get fms -n tenant-acme
NAME                  SITE        TYPE         PHASE       READY   SERVICE URL                                        AGE
orders                acme-site   Deployment   Ready       2       http://orders.tenant-acme.svc.cluster.local:8000   5m
orders-backfill       acme-site   Job          Succeeded                                                              5m
orders-nightly-sync   acme-site   CronJob      Ready                                                                  5m
```

| Field | Meaning |
|---|---|
| `phase` | `Deployment`: `Ready` when `SiteReady` and `Available` are true and, if routed, `Routed` is true; `Pending` otherwise. `Job`: `Pending`, `Running`, `Succeeded` or `Failed`. `CronJob`: `Ready` once the CronJob exists. Any type: `Failed` when the site cannot be supported. |
| `readyReplicas` | From the Deployment. |
| `autoscaling` | `currentReplicas` and `desiredReplicas` while `deployment.autoscaling` is set. |
| `serviceURL` | In-cluster URL, `Deployment` only. |
| `externalURLs` | One per routed path. |
| `jobName` | The Job of the current run, `Job` only. |
| `lastScheduleTime`, `lastSuccessfulTime`, `activeJobs` | From the CronJob. |
| `observedGeneration` | Generation last reconciled. |

Conditions:

| Type | Types | True when | Common reasons when False |
|---|---|---|---|
| `SiteReady` | all | The site is Ready and the binding Secret is in sync. | `SiteNotFound`, `SiteNotReady`, `DatabaseInfoUnavailable`, `CredentialsUnavailable`, `UnsupportedDatabase` |
| `Available` | Deployment | Ready replicas reach `replicas` (or `replicas: 0`). | `ReplicasNotReady`, `NameConflict` |
| `Routed` | Deployment | Kubernetes: the Ingress has a load balancer address. OpenShift: every Route is admitted. Only present when routing is set. | `IngressPending`, `RoutePending`, `RouteNotAdmitted`, `SiteIngressDisabled`, `PassthroughUnsupported`, `NameConflict` |
| `AutoscalingReady` | Deployment | The HPA or ScaledObject exists and is active. Only present when `autoscaling` is set. | `KEDANotInstalled`, `NameConflict`, the HPA's `ScalingActive` reason (for example `FailedGetResourceMetric`), `ScaledObjectNotReady` |
| `Complete` | Job | The run succeeded. | `Running`, `Failed`, `NameConflict` |
| `Scheduled` | CronJob | The CronJob exists. Reason `Suspended` when paused. | `NameConflict` |

On Kubernetes, Ingress status does not report path conflicts, so a path that
loses to another Ingress can still show `Routed=True`.

## Limitations

- The microservice must be in the site's namespace.
- SQLite sites are not supported (`UnsupportedDatabase`).
- The microservice uses the site's own database user, so it has the same
  database access as the monolith.
- Serverless workloads and separate microservice databases are not supported
  yet. Scale-to-zero is available only through KEDA.
- Autoscaling does not apply to `Job` and `CronJob` (no parallelism setting).
