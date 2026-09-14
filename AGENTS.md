# Working on frappe-operator (agents and humans alike)

This file is the checklist. The detailed procedures live in `.agents/workflows/`:

| Workflow | When |
|---|---|
| [`crd-generation.md`](.agents/workflows/crd-generation.md) | any change under `api/` |
| [`testing.md`](.agents/workflows/testing.md) | any code change |
| [`probe-e2e.md`](.agents/workflows/probe-e2e.md) | any change — the probe app is the acceptance test |
| [`openshift.md`](.agents/workflows/openshift.md) | any change — everything must also work on OpenShift |

## Always, for every change

1. **Unit tests, formatting, build**: `gofmt -l . && go build ./... && go test ./...`
   (`make test` runs the envtest suite). A change that touches `api/` also runs
   `make manifests generate sync-helm-crds` and regenerates `install.yaml`
   (`./bin/kustomize build config/default > install.yaml`); CI diffs it.
2. **The probe app must stay green.** Every push/PR/tag runs `probe e2e`
   (`.github/workflows/probe-e2e.yml`): it builds the operator image from the
   commit and exercises every CR on kind with `vyogo_probe` installed from git
   and from its FPM package. Read `.agents/workflows/probe-e2e.md`.
3. **Every CRD is covered by the probe.** `make probe-coverage`
   (`hack/check-probe-coverage.sh`) fails when a Kind in `config/crd/bases` has
   no manifest in the probe repo. A new CRD lands together with its probe
   manifest and phase; only an unimplemented scaffold may be listed in
   `hack/probe-coverage-exceptions.txt`, with a reason.
4. **A bug the probe could have caught gets a probe assertion**, not only a unit
   test. The probe is where install paths, Jobs, Ingress/Routes, DNS and Frappe
   itself meet; unit tests did not catch any of the v5.2.2 fixes.
5. **It works on OpenShift too.** Every workload the operator emits (Deployments,
   Jobs, Routes, Secrets) must be valid under the `restricted-v2` SCC and every
   HTTP surface must have a Route path, not only an Ingress one. Read
   `.agents/workflows/openshift.md`; the emitted-object contract lives in
   `controllers/openshift_contract_test.go` and a new OpenShift-relevant
   behaviour adds a spec there.
6. **Durable fixes over live hacks.** Fix the controller/script/chart, never a
   running cluster by hand; never commit plaintext secrets; never edit generated
   files (`config/crd/bases/`, `helm/frappe-operator/crds/`,
   `api/v1/zz_generated.deepcopy.go`, `install.yaml`) by hand.
7. **Releases**: `./scripts/bump-version.sh X.Y.Z`, CHANGELOG entry,
   `make manifests`, regenerate `install.yaml`, commit `chore: release vX.Y.Z`,
   annotated tag `vX.Y.Z` → `release.yml` publishes image, chart and
   `install.yaml`. Consumers pin release tags, not branch shas.

## Things that have bitten us (do not repeat)

- A CRD `bool` with `omitempty` and `+kubebuilder:default=true` cannot hold
  `false` across a controller `Update` (the zero value is dropped, the API
  server re-defaults it). Use `*bool`, and add finalizers with a merge `Patch`,
  never `Update`. (SiteDomain `tls.enabled`, SiteApp `autoMigrate`.)
- `sites/apps.txt` is the union of image apps and volume apps; rebuilding it
  from the image alone makes `bench migrate` delete a SiteApp's DocTypes.
- `install-app` records patches as executed without running them; an install
  path that skips `bench migrate` ships apps with unapplied patches.
- Ingress TLS sections, `force-ssl-redirect` and cert-manager annotations are
  only emitted when TLS is actually on; a plain-HTTP cluster otherwise gets a
  308 loop (invisible behind an HTTPS-terminating edge such as Cloudflare).
- Frappe routes by `Host`: a Route/Ingress host must equal the site name or
  have a `sites/<host>` alias symlink on the shared PVC.
