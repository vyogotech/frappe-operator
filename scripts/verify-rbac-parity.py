#!/usr/bin/env python3
"""Assert Helm and kustomize grant identical user-facing RBAC.

The two install paths build their manifests independently, so RBAC added to one
can silently miss the other. That already happened once: the aggregated
edit/view ClusterRoles shipped in config/rbac but not in the Helm chart, which
would have left Helm users with a web console whose actions were all greyed out
while `Test Helm deployment` still passed, because nothing compared them.

Compares rules and aggregation labels; chart-specific metadata labels differ by
construction and are ignored.
"""
import json
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

import yaml

CHART = Path("helm/frappe-operator")
KUSTOMIZE = "./bin/kustomize"


def run(cmd, **kw):
    p = subprocess.run(cmd, capture_output=True, text=True, **kw)
    if p.returncode != 0:
        print(f"FAIL: {' '.join(str(c) for c in cmd)} exited {p.returncode}")
        print(p.stderr.strip() or p.stdout.strip())
        sys.exit(1)
    return p.stdout


def render_chart():
    """Render the chart's own templates, without fetching its subcharts.

    The aggregated roles live in the parent chart, but `helm template` refuses to
    run while Chart.yaml declares dependencies that are absent from charts/ --
    and those .tgz files are not in git. Rather than make this check depend on
    two external chart repositories being reachable, copy the chart and drop the
    dependencies block, so it stays hermetic.
    """
    with tempfile.TemporaryDirectory() as tmp:
        dest = Path(tmp) / CHART.name
        shutil.copytree(CHART, dest)
        meta = yaml.safe_load((dest / "Chart.yaml").read_text())
        meta.pop("dependencies", None)
        (dest / "Chart.yaml").write_text(yaml.safe_dump(meta, sort_keys=False))
        shutil.rmtree(dest / "charts", ignore_errors=True)
        return run(["helm", "template", "frappe-operator", str(dest)])


def aggregated_roles(docs):
    out = {}
    for d in yaml.safe_load_all(docs):
        if not d or d.get("kind") != "ClusterRole":
            continue
        labels = d["metadata"].get("labels") or {}
        agg = {k: v for k, v in labels.items() if "aggregate-to" in k}
        if agg:
            out[d["metadata"]["name"]] = {"rules": d.get("rules"), "aggregation": agg}
    return out


def main():
    kustomize = aggregated_roles(run([KUSTOMIZE, "build", "config/default"]))
    helm = aggregated_roles(render_chart())

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
