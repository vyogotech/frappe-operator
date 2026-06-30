import yaml
import json

with open('/tmp/frappe_b64.txt') as f:
    b64 = f.read().strip()
    
with open('/tmp/alm-examples.json') as f:
    alm = f.read().strip()
    
with open('../community-operators/operators/frappe-operator/4.0.0/manifests/frappe-operator.clusterserviceversion.yaml') as f:
    data = yaml.safe_load(f)

data['metadata'].setdefault('annotations', {})
data['metadata']['annotations']['alm-examples'] = alm
data['metadata']['annotations']['categories'] = "Application Runtime,Database"
data['metadata']['annotations']['capabilities'] = "Basic Install"

desc = """The Frappe Operator brings the robust capabilities of Frappe and ERPNext frameworks natively into Kubernetes using the Operator Pattern.

### Features
* **FrappeBench Provisioning**: Automatically deploy and scale custom Frappe stacks natively.
* **Database Harmonization**: Spin up shared or dedicated MariaDB instances tailored.
* **Automated Replicas**: High Availability web worker routing via Ingress.
* **GitOps Compliance**: Designed to operate smoothly with ArgoCD.

### Setup
Ensure that you have deployed the prerequisites in your cluster before bootstrapping an ERPNext portal using standard `kubectl apply` commands.
"""
data['spec']['description'] = desc

data['spec']['icon'] = [
    {
        "base64data": b64,
        "mediatype": "image/png"
    }
]

with open('../community-operators/operators/frappe-operator/4.0.0/manifests/frappe-operator.clusterserviceversion.yaml', 'w') as f:
    yaml.dump(data, f, default_flow_style=False, sort_keys=False)
