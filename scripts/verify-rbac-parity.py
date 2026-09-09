#!/usr/bin/env python3
"""Assert Helm and kustomize grant identical user-facing RBAC.

The two install paths build their manifests independently, so RBAC added to one
can silently miss the other. That already happened once: the aggregated
edit/view ClusterRoles shipped in config/rbac but not in the Helm chart, which
would have left Helm users with a web console whose actions were all greyed out
while `Test Helm deployment` still passed, because nothing compared them.

Compares rules and aggregation labels (not chart-specific metadata labels).
"""
import json
import subprocess
import sys

import yaml


def roles(docs):
    out = {}
    for d in yaml.safe_load_all(docs):
        if not d or d.get("kind") != "ClusterRole":
            continue
        labels = d["metadata"].get("labels") or {}
        agg = {k: v for k, v in labels.items() if "aggregate-to" in k}
        if not agg:
            continue
        out[d["metadata"]["name"]] = {"rules": d.get("rules"), "aggregation": agg}
    return out


def main():
    kustomize = roles(subprocess.run(
        ["./bin/kustomize", "build", "config/default"],
        capture_output=True, text=True, check=True).stdout)
    helm = roles(subprocess.run(
        ["helm", "template", "frappe-operator", "helm/frappe-operator",
         "--set", "mariadb-operator.enabled=false"],
        capture_output=True, text=True, check=True).stdout)

    if not kustomize:
        print("FAIL: kustomize emitted no aggregated ClusterRoles")
        return 1

    failed = False
    for name in sorted(set(kustomize) | set(helm)):
        if name not in helm:
            print(f"FAIL: {name} is in kustomize but missing from the Helm chart")
            failed = True
        elif name not in kustomize:
            print(f"FAIL: {name} is in the Helm chart but missing from kustomize")
            failed = True
        elif json.dumps(kustomize[name], sort_keys=True) != json.dumps(helm[name], sort_keys=True):
            print(f"FAIL: {name} differs between Helm and kustomize")
            print(f"  kustomize: {json.dumps(kustomize[name], sort_keys=True)}")
            print(f"  helm:      {json.dumps(helm[name], sort_keys=True)}")
            failed = True
        else:
            print(f"ok: {name}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
