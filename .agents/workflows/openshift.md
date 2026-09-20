---
description: Every change must also work on OpenShift (restricted-v2 SCC, Routes, UBI utility images)
---

# OpenShift parity (mandatory for every change)

The operator ships to OpenShift (OLM bundle, `docs/INSTALL_OPENSHIFT.md`,
`deploy-on-openshift.yml`) as a first-class target. There is no OpenShift
cluster in CI, so parity is enforced by construction and by the contract test:
**every object the operator emits must be valid under the `restricted-v2` SCC
and every HTTP surface must have a Route path.**

## Detection and the switch

- `main.go` sets `isOpenShift` when the Route API is served
  (`controllers.IsRouteAPIAvailable`), overridable with `FRAPPE_IS_OPENSHIFT`.
  Every reconciler carries `IsOpenShift`; new reconcilers get it wired the same
  way in `main.go` and in their `SetupWithManager` tests.
- `controllers/utils.go: isOpenShiftPlatform` and
  `controllers/domain_detector.go: detectOpenShiftAppsDomain` (reads the
  cluster `config.openshift.io/v1 Ingress` wildcard) are the only places that
  know the platform's shape; do not add ad-hoc detection.

## Rules for anything that creates a Pod (Deployment, StatefulSet, Job, CronJob)

1. Security contexts come from
   `PodSecurityContextForBench(ctx, client, IsOpenShift, namespace, security)` and
   `ContainerSecurityContextForBench(IsOpenShift, security)` in
   `controllers/security_context.go` — never hand-written. On OpenShift they
   leave `runAsUser` unset (SCC assigns a UID from the project range), set
   `fsGroup` inside the namespace's `openshift.io/sa.scc.supplemental-groups`
   range (`getNamespaceFSGroup`), apply the project's SELinux MCS label
   (`getNamespaceMCSLabel`), drop all capabilities and forbid privilege
   escalation.
2. No `runAsUser: 0`, no `fsGroup: 0`, no `privileged`, no hostPath, no
   `hostNetwork`, no added capabilities. Anything needing root does not ship.
3. Scripts must not assume uid 1000 or a writable `$HOME` — set `HOME=/tmp`,
   `USER=frappe`, and write under the sites volume or `/tmp` only (the SiteApp,
   SiteConfig and SiteCron Jobs are the reference).
4. Utility containers use a UBI image on OpenShift: honour `UTILITY_IMAGE`
   and default to `registry.access.redhat.com/ubi9/ubi-minimal` when
   `IsOpenShift` (see the SiteDomain alias Job) — never bare `busybox`/`alpine`
   only.
5. Volumes: shared bench storage is RWX (ODF CephFS on OpenShift); the mount
   must be writable by the SCC-assigned uid via `fsGroup`, which is why (1) is
   not optional. `subPath` mounts are fine; `mountPropagation` is not.
6. Bench image pull secrets, resources and `jobResources` sizing apply on both
   platforms; a Job that omits them fails only on the stricter cluster.

## Rules for anything HTTP

- Where an Ingress is created, a Route is created on OpenShift
  (`FrappeSiteReconciler.ensureRoute`, edge TLS termination, HTTPS-only).
  A new HTTP surface (alias domains, proxies, webhooks callbacks) implements
  both branches; Ingress-only is a bug.
- Route hosts must resolve: use the detected apps wildcard
  (`*.apps.<cluster>`) via the domain detector, and remember Frappe routes by
  `Host` — a host that is not the site name needs the `sites/<host>` alias.
- Do not set nginx-ingress annotations on Routes; Route-side policy is
  `tls.insecureEdgeTerminationPolicy`.

## RBAC, CRDs, chart, bundle

- New API groups the operator reads or writes on OpenShift
  (`route.openshift.io`, `config.openshift.io`, `security.openshift.io`) need
  kubebuilder RBAC markers *and* the chart's aggregated roles; regenerate with
  `make manifests` and check `helm/frappe-operator/templates` and
  `config/rbac`.
- The chart must install with the default values on OpenShift: no
  `runAsUser`/`fsGroup` in `values.yaml` defaults (see the comment there); the
  console plugin and aggregated RBAC roles ship via Helm.
- A CRD change is followed by `make bundle` when the OLM bundle exists for
  that version (`bundle-validate` must pass).

## Tests you must add

- `controllers/openshift_contract_test.go` asserts on the *emitted* objects
  with the inputs OpenShift really supplies (SCC range annotations on the
  namespace, the cluster Ingress config). New OpenShift-relevant behaviour
  gets a spec there; a fix for an OpenShift-only defect gets a regression spec
  there first.
- Reconciler tests that build Pods/Jobs run twice, `IsOpenShift: false` and
  `true` (pattern: `TestSiteAppReconciler_SecurityContext_OpenShift`).

## Before you call a change done

- `go test ./controllers/ -run 'OpenShift|SecurityContext'` passes.
- `grep -rn "runAsUser\|fsGroup\|Privileged\|busybox" controllers/` shows only
  the helpers and the guarded defaults.
- If you touched anything HTTP, both `ensureIngress` and `ensureRoute` (or
  their SiteDomain equivalents) reflect it.
- When a real OpenShift cluster is available, `deploy-on-openshift.yml` is the
  manual smoke test. The probe currently assumes an Ingress class
  (`--ingress-class`, default `nginx`); teaching it Routes so it can be the
  acceptance test on OpenShift too is an open task in the probe repo.
