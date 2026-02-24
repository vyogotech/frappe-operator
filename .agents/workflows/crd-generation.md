---
description: Standard workflow for modifying Custom Resource Definitions (CRDs)
---

# CRD Generation & API Modification Skill

This skill outlines the process for changing the Frappe Operator's API schemas (`FrappeBench` and `FrappeSite`).

## 1. Modifying API Structs

The operator uses Kubebuilder. The exact source of truth for all schemas is inside the `api/v1alpha1/` directory.

- **FrappeBench:** Modify `api/v1alpha1/frappebench_types.go`
- **FrappeSite:** Modify `api/v1alpha1/frappesite_types.go`
- **Shared Config:** Modify `api/v1alpha1/shared_types.go`

When adding a new field:
1. Always add `json:"fieldName,omitempty"` unless the field is strictly required.
2. If it is a complex struct, add standard `kubebuilder:validation` markers (e.g., `//+kubebuilder:validation:Minimum=1`).

## 2. Generating Manifests

**CRITICAL:** After making *any* change to the `.go` files in the `api/` directory, you MUST run the generator commands. Do not attempt to manually edit the YAML files in `config/crd/bases/` or `helm/crds/`.

Run this exact command to auto-generate the updated DeepCopy methods and standard YAML CRDs:

```bash
make manifests generate
```

## 3. Applying and Verifying changes

Once `make manifests` runs successfully:

1. Look at `git diff config/crd/bases/` to confirm your new fields appeared in the YAML definitions.
2. If working against a live cluster, apply the CRD directly:
```bash
make install
```

## 4. Helm Chart Synchronization
The `make manifests` command updates the standard kustomize bases. If the Helm chart needs the updated CRDs, you must sync them manually:

```bash
cp config/crd/bases/vyogo.tech_frappesites.yaml helm/frappe-operator/crds/
cp config/crd/bases/vyogo.tech_frappebenches.yaml helm/frappe-operator/crds/
```
