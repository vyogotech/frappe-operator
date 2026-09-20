---
description: The probe app is the operator's acceptance test; every change runs it and every CRD must be covered by it
---

# Probe e2e (mandatory for every change)

[vyogotech/frappe-operator-probe](https://github.com/vyogotech/frappe-operator-probe)
is a small Frappe app (`vyogo_probe`, with a DocType, server/client scripts,
webhooks, a patch, an `after_migrate` hook, a scheduler task and JSON APIs) plus
a runner (`probe.py`) that applies one manifest per CR kind on a throw-away
namespace and asserts on what Frappe actually did. It runs on kind in CI and
must **never** be pointed at a production cluster.

## What runs automatically

`.github/workflows/probe-e2e.yml` on every push, pull request and tag:

1. `Every CRD has a probe phase` — `hack/check-probe-coverage.sh` against a
   checkout of the probe repo.
2. `Build operator image` — the image from *this* commit, plus this commit's
   Helm chart, uploaded as run artifacts.
3. `probe / probe (git)` and `probe / probe (fpm)` — the probe repo's
   `e2e.yml` called via `workflow_call`; it loads the image into kind, installs
   the chart, a MariaDB CR and ingress-nginx, then runs all phases:
   bench, site, access (SiteRole/SiteUser/SiteAPIKey), siteapp (git or FPM
   package `vyogotech/vyogo_probe` from `ghcr.io/vyogotech/fpm`), config,
   content (custom field, property setter, scripts, webhook, quota), seed,
   cron, migration, backup→wipe→restore, domain.

A red job blocks the change. Do not skip or `continue-on-error` it; fix the
operator (or, if the probe is wrong, the probe — in the same change set).

## When you add or change a CRD

- Add `manifests/NN-<kind>.yaml` and a phase in `probe.py` to the probe repo,
  with an assertion that reads the effect back from Frappe (not just the CR's
  `status.phase`).
- `make probe-coverage` must pass locally (`PROBE_DIR=../frappe-operator-probe`).
- A scaffold CRD with no real controller may be listed in
  `hack/probe-coverage-exceptions.txt` with a reason; implementing it means
  removing that line.

## When you fix a bug

Ask "would the probe have caught this?". If yes, add the assertion to the
probe (it is the regression test that actually runs Frappe); a Go unit test
on the emitted script/object is a complement, not a substitute.

## Running it yourself

```bash
# against a published operator image (operator-image.yml publishes sha-<short> tags)
gh workflow run e2e.yml -R vyogotech/frappe-operator-probe -f operator_image_tag=sha-<sha>

# from a checkout, against any disposable cluster
./probe.py --domain <zone> --mariadb-ref <mariadb>/<namespace> [--app-source fpm] [--only siteapp,domain] [--cleanup]
```

Check names to require on protected branches: `Every CRD has a probe phase`,
`probe / probe (git)`, `probe / probe (fpm)`.
