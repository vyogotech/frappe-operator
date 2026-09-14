---
description: Standard workflow for modifying Custom Resource Definitions (CRDs)
---

# CRD Generation & API Modification

The API lives in `api/v1/` (group `vyogo.tech`, version `v1`): one
`<kind>_types.go` per Kind (FrappeBench, FrappeSite, SiteApp, SiteConfig,
SiteDomain, SiteBackup, SiteRestore, SiteMigration, SiteCron, SiteAPIKey,
SiteRole, SiteUser, SiteUserPermission, SiteCustomField, SitePropertySetter,
SiteServerScript, SiteClientScript, SiteWebhook, SiteQuota, …) plus
`shared_types.go`. Everything under `config/crd/bases/`,
`helm/frappe-operator/crds/`, `api/v1/zz_generated.deepcopy.go` and
`install.yaml` is generated — never edit it by hand.

## 1. Modifying API structs

1. `json:"fieldName,omitempty"` unless the field is strictly required; add
   `+kubebuilder:validation` markers for constrained values.
2. **Tri-state booleans are `*bool`.** A plain `bool` with `omitempty` and a
   `+kubebuilder:default=true` marker cannot hold `false`: the controller's own
   `Update` (finalizer add, spec touch) drops the zero value and the API server
   re-defaults it to `true`. (`SiteDomain.spec.tls.enabled`,
   `SiteApp.spec.autoMigrate` were both bitten.) Add finalizers with
   `client.MergeFrom` + `Patch`, never `Update`.
3. Field renames and type changes are breaking for existing objects; prefer
   adding a field and deprecating the old one.

## 2. Generating

After any change under `api/`:

```bash
make manifests generate sync-helm-crds
./bin/kustomize build config/default > install.yaml   # CI diffs this (validate-manifests.yml)
gofmt -l . && go build ./... && go test ./...
```

Check `git diff config/crd/bases/` shows exactly the fields you added.
`make install` applies the CRDs to the current kubeconfig when working live.

## 3. A new Kind is not done until

- its controller is registered in `main.go` with `IsOpenShift` wired
  (see `.agents/workflows/openshift.md`) and RBAC markers generated;
- the Helm chart carries the CRD (`make sync-helm-crds`) and, if the OLM bundle
  exists for the version, `make bundle` was re-run;
- **the probe app covers it**: a `manifests/NN-<kind>.yaml` and a phase in
  `probe.py` in [frappe-operator-probe](https://github.com/vyogotech/frappe-operator-probe),
  or — only for an unimplemented scaffold — a line with a reason in
  `hack/probe-coverage-exceptions.txt`. `make probe-coverage` must pass
  (see `.agents/workflows/probe-e2e.md`);
- `config/samples/v1_<kind>.yaml` exists and the docs mention the Kind.
