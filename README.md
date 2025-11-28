# Frappe Operator

A Kubernetes Operator that makes deploying and managing [Frappe](https://frappeframework.com/) and [ERPNext](https://erpnext.com/) on Kubernetes simple and declarative.

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.19+-blue.svg)](https://kubernetes.io/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vyogotech/frappe-operator)](go.mod)
[![Release](https://img.shields.io/github/v/release/vyogotech/frappe-operator)](https://github.com/vyogotech/frappe-operator/releases/latest)
[![Production Ready](https://img.shields.io/badge/Production-Ready-green.svg)](RELEASE_NOTES_v1.0.0.md)

**📚 [Complete Documentation](https://vyogotech.github.io/frappe-operator/)**

## What is Frappe Operator?

Frappe Operator is a Kubernetes operator that automates the deployment, scaling, and management of Frappe Framework applications (including ERPNext) on Kubernetes clusters. It brings the power of Kubernetes orchestration to Frappe deployments.

### Why Do You Need This?

**Traditional Frappe Deployment Challenges:**
- 🔧 Complex manual setup and configuration
- 🔄 Difficult to scale and manage multiple sites
- 🐛 Hard to maintain consistency across environments
- 📦 Manual updates and migrations are error-prone
- 🏢 Multi-tenancy requires custom scripting

**With Frappe Operator:**
- ✅ **Simple Declarative Configuration** - Define your Frappe infrastructure as YAML files
- ✅ **Automated Management** - Operator handles deployment, scaling, and updates automatically
- ✅ **Multi-Tenancy Built-in** - Easily manage hundreds of sites on shared infrastructure
- ✅ **Production-Ready** - High availability, auto-scaling, and security out of the box
- ✅ **GitOps Compatible** - Manage infrastructure as code with version control

## Key Features

- 🚀 **One-Command Deployment** - Deploy entire Frappe infrastructure with a single kubectl command
- 🏢 **Multi-Tenancy** - Run multiple customer sites efficiently on shared infrastructure
- 🔐 **Secure Database Provisioning** - MariaDB Operator integration with auto-generated credentials
- 🔒 **Zero Hardcoded Secrets** - All passwords auto-generated and stored in Kubernetes Secrets
- 🗄️ **Per-Site DB Isolation** - Each site gets its own database and user for security
- 📦 **Hybrid App Installation** - Install apps from FPM packages, Git repositories, or pre-built images
- ⚡ **Production-Ready Architecture** - Dual Redis (cache + queue), correct entry points
- 📊 **Auto-Scaling** - Automatically scale based on traffic and resource usage
- 🔄 **Automated Updates** - Zero-downtime rolling updates and migrations
- 💾 **Backup Management** - Automated backups with configurable retention policies
- 🔌 **Operator Integrations** - Works with MariaDB Operator, cert-manager, and ingress controllers
- 📈 **Observability** - Built-in Prometheus metrics and logging
- 🌐 **Multi-Platform Support** - ARM64 and AMD64 compatible

## Quick Start (5 Minutes)

### Prerequisites

You need:
- A Kubernetes cluster (v1.19 or newer)
- `kubectl` installed and configured
- Basic understanding of Kubernetes concepts
- **MariaDB Operator** (for database management)

**Don't have a cluster?** Try one of these:
- **Local Development**: [kind](https://kind.sigs.k8s.io/), [minikube](https://minikube.sigs.k8s.io/), or [k3d](https://k3d.io/)
- **Cloud**: [GKE](https://cloud.google.com/kubernetes-engine), [EKS](https://aws.amazon.com/eks/), or [AKS](https://azure.microsoft.com/en-us/services/kubernetes-service/)
- **Managed**: [Civo](https://www.civo.com/), [DigitalOcean Kubernetes](https://www.digitalocean.com/products/kubernetes/)

### Step 1: Install MariaDB Operator

Frappe Operator uses [MariaDB Operator](https://github.com/mariadb-operator/mariadb-operator) for secure database provisioning:

```bash
# Install MariaDB Operator CRDs
kubectl apply -f https://github.com/mariadb-operator/mariadb-operator/releases/latest/download/crds.yaml

# Install MariaDB Operator
kubectl apply -f https://github.com/mariadb-operator/mariadb-operator/releases/latest/download/mariadb-operator.yaml

# Verify installation
kubectl get pods -n mariadb-operator-system
```

### Step 2: Install Frappe Operator

Install Frappe Operator in your Kubernetes cluster:

```bash
kubectl apply -f https://github.com/vyogotech/frappe-operator/releases/download/v1.0.0/install.yaml

# Verify installation
kubectl get pods -n frappe-operator-system
```

This installs:
- Custom Resource Definitions (CRDs) for FrappeBench and FrappeSite
- Operator deployment with RBAC permissions
- Service accounts and roles

### Step 3: Create a Shared MariaDB Instance

For cost-effective multi-tenancy, create a shared MariaDB instance:

```bash
# Download example configuration
curl -O https://raw.githubusercontent.com/vyogotech/frappe-operator/main/examples/mariadb-shared-instance.yaml

# IMPORTANT: Edit the file and change the default password!
# Edit mariadb-shared-instance.yaml and update the password

# Apply the configuration
kubectl apply -f mariadb-shared-instance.yaml

# Wait for MariaDB to be ready
kubectl wait --for=condition=Ready mariadb/frappe-mariadb --timeout=300s

# You should see:
# mariadb.k8s.mariadb.com/frappe-mariadb condition met
```

### Step 4: Deploy Your First Frappe Site

Create a file called `my-first-site.yaml`:

```yaml
---
# Step 1: Create the shared infrastructure (Bench)
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
  namespace: default
spec:
  frappeVersion: "version-15"
  apps:
    - name: erpnext
      source: image  # Use pre-built image for faster deployment

---
# Step 2: Create your site
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: mycompany-site
  namespace: default
spec:
  benchRef:
    name: production-bench
    namespace: default
  siteName: mycompany.example.com
  
  # Database configuration - MariaDB Operator will handle provisioning
  dbConfig:
    provider: mariadb        # Use MariaDB
    mode: shared             # Share MariaDB instance across sites
    mariadbRef:
      name: frappe-mariadb   # Reference the MariaDB we created
      namespace: default
  
  # Domain and Ingress (optional for production)
  domain: mycompany.example.com
  ingress:
    enabled: true
  ingressClassName: nginx
```

**Deploy it:**

```bash
# Apply the configuration
kubectl apply -f my-first-site.yaml

# The operator will automatically:
# 1. Create the bench infrastructure (NGINX, Redis, workers, etc.)
# 2. Provision a database and user via MariaDB Operator
# 3. Generate secure admin password
# 4. Initialize the Frappe site
# 5. Create Ingress for external access
```

### Step 5: Watch Your Site Come Alive

Monitor the deployment:

```bash
# Watch the resources being created
kubectl get frappebench,frappesite

# Check detailed status
kubectl get frappesite mycompany-site -o yaml

# Check MariaDB resources (auto-created by operator)
kubectl get database,user,grant | grep mycompany-site

# Check the pods
kubectl get pods

# View initialization logs
kubectl logs -f job/mycompany-site-init
```

**Wait for site to be ready** (usually 2-3 minutes):

```bash
kubectl get frappesite mycompany-site -w

# When phase shows "Ready", your site is up!
```

### Step 6: Get Your Auto-Generated Credentials

The operator automatically generates secure passwords:

```bash
# Get admin password
kubectl get secret mycompany-site-admin -o jsonpath='{.data.password}' | base64 -d
echo

# Get database password (managed by MariaDB Operator)
kubectl get secret mycompany-site-db-password -o jsonpath='{.data.password}' | base64 -d
echo
```

### Step 7: Access Your Site

**For Local Testing (Port Forward):**

```bash
kubectl port-forward svc/production-bench-nginx 8080:8080
```

Then open http://localhost:8080 in your browser.

**For Production (with Ingress):**

Access directly at: https://mycompany.example.com

**Login credentials:**
- Username: `Administrator`
- Password: The auto-generated password from Step 6

## Understanding the Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                             │
│                                                                       │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │             Frappe Operator v1.0.0                             │  │
│  │    (Manages FrappeBench & FrappeSite Custom Resources)        │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                              │                                        │
│         ┌────────────────────┼────────────────────┐                  │
│         │                    │                    │                  │
│         ▼                    ▼                    ▼                  │
│  ┌─────────────────┐  ┌─────────────┐  ┌──────────────────────┐    │
│  │  FrappeBench    │  │ FrappeSite  │  │  MariaDB Operator    │    │
│  │  (Shared Infra) │  │ Controller  │  │  (DB Provisioning)   │    │
│  └─────────────────┘  └─────────────┘  └──────────────────────┘    │
│         │                    │                    │                  │
│         │  Creates           │  Creates           │  Creates         │
│         ▼                    ▼                    ▼                  │
│  ┌────────────────────────────────────────────────────────────┐     │
│  │ Kubernetes Resources                                       │     │
│  │                                                             │     │
│  │  NGINX ──┬── Gunicorn ───┬── site1.db ◄── Database CR     │     │
│  │  Proxy   │   (Python)    │   (isolated) ◄── User CR       │     │
│  │          │               │              ◄── Grant CR      │     │
│  │  Redis   ├── Scheduler   │                                 │     │
│  │  Cache   │   (Cron)      ├── site2.db ◄── Database CR     │     │
│  │          │               │   (isolated) ◄── User CR       │     │
│  │  Redis   ├── Workers     │              ◄── Grant CR      │     │
│  │  Queue   │   (Queue)     │                                 │     │
│  │          │               ├── siteN.db ◄── Database CR     │     │
│  │  SocketIO├── Init Jobs   │   (isolated) ◄── User CR       │     │
│  │          │               │              ◄── Grant CR      │     │
│  │          └── Ingress     │                                 │     │
│  │              (Routing)   └── Secrets (auto-generated)      │     │
│  └────────────────────────────────────────────────────────────┘     │
│                                                                       │
└──────────────────────────────────────────────────────────────────────┘
```

**Key Concepts:**

1. **FrappeBench** - Shared infrastructure (NGINX, Redis, Socketio, workers) used by multiple sites
2. **FrappeSite** - Individual Frappe application instance with isolated database and auto-generated credentials
3. **MariaDB Operator** - Provisions databases, users, and grants declaratively for each site
4. **Operator** - Watches custom resources and orchestrates all infrastructure automatically

**Security Features:**
- ✅ No hardcoded passwords - all credentials auto-generated
- ✅ Per-site database isolation - each site has its own DB and user
- ✅ Secret-based credential storage - managed by Kubernetes
- ✅ Declarative database provisioning - MariaDB Operator handles grants

## Common Use Cases

### 1. SaaS Platform (Multi-Tenant)

Deploy hundreds of customer sites on shared infrastructure:

```yaml
apiVersion: frappe.io/v1alpha1
kind: FrappeBench
metadata:
  name: saas-bench
spec:
  version: "version-15"
  scaling:
    minReplicas: 3
    maxReplicas: 20
  apps:
    - name: frappe
    - name: erpnext
---
# Customer 1
apiVersion: frappe.io/v1alpha1
kind: FrappeSite
metadata:
  name: customer1
spec:
  benchRef: saas-bench
  siteName: customer1.myerp.com
---
# Customer 2
apiVersion: frappe.io/v1alpha1
kind: FrappeSite
metadata:
  name: customer2
spec:
  benchRef: saas-bench
  siteName: customer2.myerp.com
# ... add more customers
```

### 2. Enterprise Deployment

High-availability setup for a single organization:

```yaml
apiVersion: frappe.io/v1alpha1
kind: FrappeBench
metadata:
  name: enterprise-bench
spec:
  version: "version-15"
  highAvailability:
    enabled: true
    replicas: 3
  resources:
    gunicorn:
      requests:
        memory: "2Gi"
        cpu: "1000m"
      limits:
        memory: "4Gi"
        cpu: "2000m"
  apps:
    - name: frappe
    - name: erpnext
    - name: hrms
```

### 3. Development Environment

Quick setup for local development:

```yaml
apiVersion: frappe.io/v1alpha1
kind: FrappeBench
metadata:
  name: dev-bench
spec:
  version: "develop"
  resources:
    gunicorn:
      requests:
        memory: "256Mi"
        cpu: "100m"
---
apiVersion: frappe.io/v1alpha1
kind: FrappeSite
metadata:
  name: dev-site
spec:
  benchRef: dev-bench
  siteName: dev.local
  developer_mode: true
```

## Hybrid App Installation (New!)

Frappe Operator v2.0 introduces flexible app installation with three sources:

### 1. FPM Packages (Recommended for Enterprise)

Install apps from versioned package repositories:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: enterprise-bench
spec:
  frappeVersion: "version-15"
  
  # Apps from FPM repositories
  apps:
    - name: erpnext
      source: fpm
      org: frappe
      version: "15.0.0"
    - name: hrms
      source: fpm
      org: frappe
      version: "15.0.0"
  
  # FPM repository configuration
  fpmConfig:
    repositories:
      - name: company-private
        url: https://fpm.company.com
        priority: 10
        authSecretRef:
          name: fpm-credentials
      - name: frappe-community
        url: https://fpm.frappe.io
        priority: 50
  
  # Disable Git for enterprise security
  gitConfig:
    enabled: false
```

**Benefits:**
- ✅ Reproducible deployments with exact versions
- ✅ No Git access required (security compliance)
- ✅ Faster deployment (pre-packaged apps)
- ✅ Private package repositories
- ✅ Audit trail for all app versions

### 2. Git Repositories (Development)

Clone apps directly from Git:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: dev-bench
spec:
  frappeVersion: "version-15"
  
  apps:
    - name: custom_app
      source: git
      gitUrl: https://github.com/company/custom_app.git
      gitBranch: develop
  
  # Enable Git for development
  gitConfig:
    enabled: true
```

### 3. Pre-built Images (Fastest)

Use container images with apps pre-installed:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: fast-bench
spec:
  frappeVersion: "version-15"
  
  apps:
    - name: frappe
      source: image
    - name: erpnext
      source: image
  
  imageConfig:
    repository: myregistry.com/frappe-custom
    tag: v1.0.0
```

### 4. Hybrid Approach (Best of All Worlds)

Combine all three methods:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: hybrid-bench
spec:
  frappeVersion: "version-15"
  
  apps:
    # Base framework in image (fastest)
    - name: frappe
      source: image
    
    # Stable apps from FPM (versioned)
    - name: erpnext
      source: fpm
      org: frappe
      version: "15.0.0"
    
    # Development apps from Git
    - name: custom_app
      source: git
      gitUrl: https://github.com/company/custom_app.git
      gitBranch: main
  
  gitConfig:
    enabled: true  # Allow Git for custom apps
```

**See [FPM_MIGRATION.md](FPM_MIGRATION.md) for complete migration guide.**

## Enterprise Features

### Git Access Control

Disable Git cluster-wide for security compliance:

```yaml
# config/manager/operator-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: frappe-operator-config
  namespace: frappe-operator-system
data:
  gitEnabled: "false"  # Disable Git by default
```

Individual benches can override:

```yaml
spec:
  gitConfig:
    enabled: true  # Override for this bench only
```

### Private Package Repositories

Configure authentication for private FPM repositories:

```bash
kubectl create secret generic fpm-credentials \
  --from-literal=username=admin \
  --from-literal=password=changeme
```

```yaml
spec:
  fpmConfig:
    repositories:
      - name: company-private
        url: https://fpm.company.com
        priority: 10
        authSecretRef:
          name: fpm-credentials
```

### Air-Gapped Deployments

Run completely offline with internal FPM repository:

```yaml
spec:
  apps:
    - name: erpnext
      source: fpm
      org: frappe
      version: "15.0.0"
  
  fpmConfig:
    repositories:
      - name: internal
        url: http://fpm.internal.company.com
        priority: 10
  
  gitConfig:
    enabled: false  # No external access
```

## Next Steps

Now that you have your first site running, explore these topics:

### Learn More
- **[Complete Documentation](https://vyogotech.github.io/frappe-operator/)** - Full guide
- **[Concepts](https://vyogotech.github.io/frappe-operator/concepts)** - Understand benches, sites, and architecture
- **[Examples](https://vyogotech.github.io/frappe-operator/examples)** - Production-ready deployment patterns

### Operations
- **[Backup & Restore](https://vyogotech.github.io/frappe-operator/operations#backups)** - Protect your data
- **[Scaling](https://vyogotech.github.io/frappe-operator/operations#scaling)** - Handle growing traffic
- **[Updates](https://vyogotech.github.io/frappe-operator/operations#updates)** - Keep your sites up to date
- **[Monitoring](https://vyogotech.github.io/frappe-operator/operations#monitoring)** - Track performance

### Advanced Topics
- **[Custom Apps](https://vyogotech.github.io/frappe-operator/api-reference#custom-apps)** - Add your own Frappe apps
- **[Database Options](https://vyogotech.github.io/frappe-operator/concepts#databases)** - Shared vs dedicated databases
- **[Security](https://vyogotech.github.io/frappe-operator/operations#security)** - Harden your deployment

## Custom Resources Reference

The operator provides these Custom Resource Definitions:

### Core Resources

| Resource | Purpose | Documentation |
|----------|---------|---------------|
| **FrappeBench** | Shared infrastructure for sites | [Docs](https://vyogotech.github.io/frappe-operator/api-reference#frappebench) |
| **FrappeSite** | Individual Frappe site | [Docs](https://vyogotech.github.io/frappe-operator/api-reference#frappesite) |

### Management Resources

| Resource | Purpose | Documentation |
|----------|---------|---------------|
| **SiteBackup** | Automated backups | [Docs](https://vyogotech.github.io/frappe-operator/api-reference#sitebackup) |
| **SiteJob** | Run bench commands | [Docs](https://vyogotech.github.io/frappe-operator/api-reference#sitejob) |
| **SiteUser** | Manage site users | [Docs](https://vyogotech.github.io/frappe-operator/api-reference#siteuser) |
| **SiteWorkspace** | Create workspaces | [Docs](https://vyogotech.github.io/frappe-operator/api-reference#siteworkspace) |
| **SiteDashboard** | Manage dashboards | [Docs](https://vyogotech.github.io/frappe-operator/api-reference#sitedashboard) |

## Requirements

**Minimum:**
- Kubernetes 1.19 or newer
- kubectl installed and configured
- 2 CPU cores and 4GB RAM available in your cluster

**Recommended for Production:**
- Kubernetes 1.24+
- MariaDB (external or MariaDB Operator)
- Ingress Controller (nginx-ingress, Traefik, etc.)
- cert-manager (for TLS certificates)
- Persistent storage with dynamic provisioning

## Troubleshooting

**Site not coming up?**

```bash
# Check operator logs
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager

# Check site events
kubectl describe frappesite mycompany-site -n frappe

# Check pod logs
kubectl logs -n frappe -l site=mycompany-site
```

**Common Issues:**
- Database connection failed → Check database credentials and connectivity
- ImagePullError → Check image names and registry access
- CrashLoopBackOff → Check pod logs for application errors

See our **[Troubleshooting Guide](https://vyogotech.github.io/frappe-operator/troubleshooting)** for detailed solutions.

## Development

Want to contribute or customize the operator?

### Setup Development Environment

```bash
# Clone the repository
git clone https://github.com/vyogotech/frappe-operator.git
cd frappe-operator

# Install dependencies
go mod download

# Install CRDs
make install

# Run locally (against configured cluster)
make run
```

### Build and Test

```bash
# Run tests
make test

# Build Docker image
make docker-build IMG=myregistry/frappe-operator:dev

# Deploy to cluster
make deploy IMG=myregistry/frappe-operator:dev
```

See **[Contributing Guidelines](CONTRIBUTING.md)** for more details.

## Community & Support

### Get Help
- 💬 **GitHub Discussions**: [Ask questions and share ideas](https://github.com/vyogotech/frappe-operator/discussions)
- 🐛 **GitHub Issues**: [Report bugs and request features](https://github.com/vyogotech/frappe-operator/issues)
- 📖 **Documentation**: [Complete guides](https://vyogotech.github.io/frappe-operator/)
- 🌐 **Frappe Forum**: [discuss.frappe.io](https://discuss.frappe.io/)

### Contributing

We welcome contributions! Here's how you can help:

1. 🌟 **Star the project** - Show your support
2. 🐛 **Report bugs** - Help us improve
3. 💡 **Suggest features** - Share your ideas
4. 📝 **Improve docs** - Help others learn
5. 🔧 **Submit PRs** - Contribute code

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Database Management

### Secure Database Provisioning (v1.0.0+)

Frappe Operator integrates with [MariaDB Operator](https://github.com/mariadb-operator/mariadb-operator) for secure, declarative database management:

**Features:**
- ✅ **Auto-Generated Credentials** - No hardcoded passwords
- ✅ **Per-Site Isolation** - Each site gets its own database and user
- ✅ **Declarative Provisioning** - Database, User, and Grant CRs managed automatically
- ✅ **Multi-Database Support** - MariaDB (v1.0), PostgreSQL (planned), SQLite (planned for v16+)

**Example - Shared MariaDB:**
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: my-site
spec:
  benchRef:
    name: my-bench
  siteName: my-site.example.com
  dbConfig:
    provider: mariadb      # Database type
    mode: shared           # Share MariaDB instance
    mariadbRef:
      name: frappe-mariadb # Existing MariaDB instance
      namespace: default
```

The operator automatically:
1. Creates a Database CR with unique name
2. Creates a User CR with auto-generated password
3. Creates a Grant CR with required privileges
4. Stores credentials in Kubernetes Secret
5. Configures site with database connection

**See [examples/](examples/) for more database configuration patterns.**

## Roadmap

**v1.0.0 (Current) ✅**
- [x] MariaDB Operator integration for secure DB provisioning
- [x] Auto-generated passwords for admin and database
- [x] Per-site database isolation
- [x] Production-ready architecture (dual Redis, correct entry points)
- [x] Multi-platform support (ARM64/AMD64)
- [x] Dynamic storage access mode detection
- [x] Hybrid app installation (FPM, Git, Image)
- [x] Enterprise Git disable feature

**v1.1 (Next)**
- [ ] PostgreSQL provider for database provisioning
- [ ] SQLite provider for Frappe v16+ sites
- [ ] Horizontal Pod Autoscaling (HPA) support
- [ ] Built-in monitoring dashboards
- [ ] Automated migration testing

**v1.2+**
- [ ] Dedicated database mode (one MariaDB per site)
- [ ] Blue-green deployment support
- [ ] Multi-cluster federation
- [ ] Helm chart support
- [ ] GitOps integration (ArgoCD/Flux)

## License

Copyright 2024 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

---

**Built with ❤️ by [Vyogo Technologies](https://vyogo.tech) using [Kubebuilder](https://book.kubebuilder.io/)**

⭐ If you find this project useful, please consider giving it a star!
