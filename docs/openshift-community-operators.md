# Listing on OpenShift OperatorHub (Red Hat community catalogue)

The Red Hat community catalogue is fed from
[redhat-openshift-ecosystem/community-operators-prod](https://github.com/redhat-openshift-ecosystem/community-operators-prod).
It is separate from operatorhub.io (`k8s-operatorhub/community-operators`, where
frappe-operator stopped at 4.1.2 when the licence moved to ELv2). The Red Hat
repo documents no licence requirement beyond the DCO sign-off and already hosts
other source-available operators, but a reviewer may still ask: the first PR is
reviewed by a human either way ("brand-new operator" never auto-merges).

Docs: https://redhat-openshift-ecosystem.github.io/operator-pipelines/

## What a submission contains

```
operators/frappe-operator/
├── ci.yaml                      reviewers + updateGraph: replaces-mode
└── <version>/                   the OLM bundle, verbatim
    ├── manifests/               CSV + CRDs + RBAC + Service + ConfigMaps
    ├── metadata/annotations.yaml  incl. com.redhat.openshift.versions: "v4.14"
    └── tests/scorecard/config.yaml
```

Requirements the pipeline checks (static + preflight install on OCP):
- `operator-sdk bundle validate ./bundle --select-optional name=operatorhub`
  and `--select-optional suite=operatorframework` pass (they do; the
  "example annotation" warnings are fine).
- CSV has `containerImage`, `createdAt`, displayName, description, icon,
  maintainers, provider, links, semver `version` (`make bundle` produces all
  but `containerImage`, which the release job injects).
- `replaces` names a version the catalogue already has, or is absent (first
  submission). Never set `olm.skipRange` without `replaces` (pruned-graph check).
- Every image in the CSV is publicly pullable: `ghcr.io/vyogotech/frappe-operator`
  is public; the kube-rbac-proxy sidecar is `docker.io/kubebuilder/kube-rbac-proxy:v0.13.1`.
- Commits are signed off (`git commit -s`) with a real name; author and
  `Signed-off-by` must match. The identity used on every accepted PR so far is
  `Rakesh Kumar Mallam <60370210+rmallam@users.noreply.github.com>`.

## Manual submission (what the release job automates)

```bash
# 1. bundle for the tagged release
git checkout vX.Y.Z
make bundle VERSION=X.Y.Z IMG=ghcr.io/vyogotech/frappe-operator:vX.Y.Z
# add `containerImage:` to the CSV annotations, and `replaces:` when a previous
# version is already in community-operators-prod
operator-sdk bundle validate ./bundle --select-optional name=operatorhub
operator-sdk bundle validate ./bundle --select-optional suite=operatorframework

# 2. fork branch
gh repo fork redhat-openshift-ecosystem/community-operators-prod --clone
cd community-operators-prod && git fetch upstream && git checkout -b frappe-operator-X.Y.Z upstream/main
mkdir -p operators/frappe-operator/X.Y.Z
cp -r ../bundle/{manifests,metadata,tests} operators/frappe-operator/X.Y.Z/
git add operators/frappe-operator && git commit -s -m "operator frappe-operator (X.Y.Z)"
git push -u origin frappe-operator-X.Y.Z
gh pr create --repo redhat-openshift-ecosystem/community-operators-prod --base main \
  --head "$(gh api user --jq .login):frappe-operator-X.Y.Z" --title "operator frappe-operator (X.Y.Z)"
```

The hosted pipeline posts its results on the PR. Useful PR comments:
`/pipeline restart operator-hosted-pipeline`, `/test skip check_pruned_graph`.

## Automation

`.github/workflows/release.yml` job `submit-to-openshift-community` does the
above on every `v*` tag once these are set on the GitHub repo:

| Kind | Name | Value |
|---|---|---|
| variable | `SUBMIT_TO_OPENSHIFT_COMMUNITY` | `true` |
| variable | `COMMUNITY_OP_AUTHOR_NAME` | `Rakesh Kumar Mallam` |
| variable | `COMMUNITY_OP_AUTHOR_EMAIL` | `60370210+rmallam@users.noreply.github.com` |
| secret | `COMMUNITY_OP_PAT` | a classic PAT with `repo` + `workflow` (fork, push, open PRs) |

`replaces` is derived from the highest version directory already present in
the fork's copy of upstream, so upgrade edges follow what was actually published.

## Later: FBC mode

Red Hat recommends file-based catalogs (`fbc.enabled: true` in `ci.yaml`,
`catalog-templates/`, generated `catalogs/`). 14 of the 17 operators added in
the last 90 days used plain replaces-mode, which is what this submission uses;
convert with `make fbc-onboarding` (needs podman) once the first version is
published, so the catalogue templates have a real bundle pullspec to start from.
