# Storage Implementation for Frappe Operator

## Overview

The Frappe Operator now includes complete storage provisioning for FrappeBench instances, creating PersistentVolumeClaims (PVCs) to provide shared storage for all bench components.

## What Was Implemented

### 1. PVC Creation (`frappebench_resources.go`)

The operator now creates a PVC named `{bench-name}-sites` for each FrappeBench:

```go
func (r *FrappeBenchReconciler) ensureBenchStorage(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error
```

**Default Configuration:**
- **Size**: 10Gi (configurable via future StorageConfig spec)
- **Access Mode**: ReadWriteMany (RWX) - fallback to ReadWriteOnce (RWO) if cluster doesn't support RWX
- **Storage Class**: Uses cluster default storage class

### 2. Resource Creation Flow

The operator now follows this sequence when a FrappeBench is created:

1. **Storage First**: Create PVC for sites directory
2. **Redis**: Create StatefulSet + Service for Redis/cache
3. **Init Job**: Initialize bench structure on the PVC
4. **Deployments**: Create all application components
   - Gunicorn (web workers)
   - NGINX (reverse proxy + static files)
   - SocketIO (websocket server)
   - Scheduler (cron jobs)
   - Workers (default, long, short queues)
5. **Services**: Create ClusterIP services for each component

### 3. Volume Mounts

All pods mount the shared PVC at `/home/frappe/frappe-bench/sites`:

```yaml
volumeMounts:
  - name: sites
    mountPath: /home/frappe/frappe-bench/sites
volumes:
  - name: sites
    persistentVolumeClaim:
      claimName: {bench-name}-sites
```

## Access Modes Explained

### ReadWriteMany (RWX) - Preferred
- **What**: Multiple pods across multiple nodes can read and write simultaneously
- **When**: Production multi-node clusters
- **Required Storage**: NFS, CephFS, GlusterFS, Azure Files, AWS EFS, etc.
- **Why Needed**: Frappe bench components (gunicorn, nginx, workers) all need to access the same sites directory

### ReadWriteOnce (RWO) - Fallback
- **What**: Single node can read and write, but multiple pods on that node can access
- **When**: Development, Kind clusters, local testing
- **Limitation**: All pods must be scheduled on the same node
- **Storage**: Most cloud providers (AWS EBS, Google PD, Azure Disk), local storage

## Testing Results

### Kind Cluster (Current Test Environment)

✅ **Successfully Tested:**
- PVC created automatically: `test-bench-sites` (20Gi, RWO)
- Init job completed successfully
- All 12 components running:
  - 2x Gunicorn pods
  - 2x NGINX pods
  - 1x SocketIO pod
  - 1x Scheduler pod
  - 2x Worker-default pods
  - 1x Worker-long pod
  - 1x Worker-short pod
  - 1x Redis pod
- All pods can access the shared PVC (verified)
- Bench initialization script executed successfully
- `common_site_config.json` created with Redis connections

### Storage Access Verification

```bash
$ kubectl exec -it test-bench-gunicorn-744475c7cf-drjmg -- ls -la /home/frappe/frappe-bench/sites/
total 8
drwxrwxrwx. 4 root   root    94 Nov 27 03:54 .
drwxr-xr-x. 1 frappe frappe  18 Nov 19 00:27 ..
drwxr-xr-x. 2 frappe frappe   6 Nov 27 03:54 .common_site_config
-rw-r--r--. 1 frappe frappe  15 Nov 27 03:54 apps.txt
drwxr-xr-x. 5 frappe frappe 112 Nov 27 03:54 assets
-rw-r--r--. 1 frappe frappe 569 Nov 27 03:54 common_site_config.json
```

## Production Considerations

### For Production Deployments

#### Option 1: Use RWX-Capable Storage Class (Recommended)

**Prerequisites:**
- Install NFS provisioner, Rook-Ceph, GlusterFS, or use cloud provider RWX storage

**Example with NFS:**

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: nfs-client
provisioner: cluster.local/nfs-client-provisioner
parameters:
  archiveOnDelete: "false"
mountOptions:
  - hard
  - nfsvers=4.1
reclaimPolicy: Retain
```

Then specify in FrappeBench:

```yaml
spec:
  storageConfig:  # Future enhancement
    className: nfs-client
```

#### Option 2: Single-Node with RWO

For single-node or testing:
```yaml
spec:
  nodeSelector:
    kubernetes.io/hostname: specific-node
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule
```

#### Option 3: ReadWriteOncePod (K8s 1.22+)

For isolated workloads:
```yaml
accessModes:
  - ReadWriteOncePod  # Only a single pod can access
```

### Cloud Provider Options

| Provider | RWX Storage | Service |
|----------|------------|---------|
| **AWS** | Amazon EFS | `aws-efs` CSI driver |
| **Azure** | Azure Files | `azurefile` storage class |
| **GCP** | Filestore | `filestore.csi.storage.gke.io` |
| **DigitalOcean** | Volume (RWO only) | Use NFS provisioner |

## Future Enhancements

### Planned for v2.1

1. **StorageConfig in FrappeBench CRD:**
   ```go
   type StorageConfig struct {
       ClassName string                 `json:"className,omitempty"`
       Size      resource.Quantity      `json:"size,omitempty"`
       AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
   }
   ```

2. **Automatic Access Mode Detection:**
   - Query available storage classes
   - Detect RWX support
   - Fall back to RWO with pod anti-affinity

3. **Per-Component PVCs** (optional):
   - Separate PVC for logs
   - Separate PVC for backups
   - Separate PVC for uploads

4. **Volume Snapshots**:
   - Backup/restore using VolumeSnapshot CRD
   - Integration with SiteBackup controller

## Troubleshooting

### Pods Stuck in "ContainerCreating"

**Symptom**: Pods won't start, describe shows volume mount errors

**Check**:
```bash
kubectl describe pod <pod-name>
# Look for: "Multi-Attach error for volume" or "FailedAttachVolume"
```

**Solution**: Your storage class doesn't support RWX. Options:
1. Install NFS provisioner
2. Use RWO and ensure all pods on same node
3. Use cloud provider RWX storage

### PVC Stuck in "Pending"

**Check**:
```bash
kubectl describe pvc test-bench-sites
# Look for events explaining why provisioning failed
```

**Common Causes**:
- No default storage class configured
- Requested size exceeds quota
- Storage class doesn't exist

**Solution**:
```bash
# Check available storage classes
kubectl get storageclass

# If none marked (default), set one:
kubectl patch storageclass <class-name> -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
```

### Permission Denied Errors

**Symptom**: Pods can't write to PVC

**Check**:
```bash
kubectl exec -it <pod-name> -- ls -la /home/frappe/frappe-bench/sites/
```

**Solution**: Add securityContext to init job:
```yaml
securityContext:
  fsGroup: 1000  # frappe user GID
  runAsUser: 1000
```

## Monitoring Storage

### Check PVC Usage

```bash
# Install metrics-server if not already
kubectl top pods -n default

# Check PV usage
kubectl exec -it <pod-name> -- df -h /home/frappe/frappe-bench/sites
```

### Set Up Alerts

Monitor PVC usage and alert when:
- Usage > 80%
- Write errors increase
- Performance degrades

## References

- [Kubernetes Persistent Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
- [Storage Classes](https://kubernetes.io/docs/concepts/storage/storage-classes/)
- [Frappe Bench Structure](https://frappeframework.com/docs/user/en/bench)
- [NFS Dynamic Provisioner](https://github.com/kubernetes-sigs/nfs-subdir-external-provisioner)

