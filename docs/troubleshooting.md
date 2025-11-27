# Troubleshooting

Common issues and solutions when running Frappe Operator.

## Table of Contents

- [General Debugging](#general-debugging)
- [Installation Issues](#installation-issues)
- [Bench Issues](#bench-issues)
- [Site Issues](#site-issues)
- [Database Issues](#database-issues)
- [Networking Issues](#networking-issues)
- [Performance Issues](#performance-issues)
- [Storage Issues](#storage-issues)

---

## General Debugging

### Check Operator Logs

```bash
# View operator logs
kubectl logs -n frappe-operator-system \
  deployment/frappe-operator-controller-manager -f

# Check for errors
kubectl logs -n frappe-operator-system \
  deployment/frappe-operator-controller-manager | grep ERROR
```

### Check Resource Status

```bash
# Check all Frappe resources
kubectl get frappebench,frappesite -A

# Describe resources for events
kubectl describe frappebench <bench-name>
kubectl describe frappesite <site-name>

# Check all pods
kubectl get pods -A | grep frappe
```

### Common Commands

```bash
# Get events
kubectl get events --sort-by='.lastTimestamp'

# Check resource status
kubectl get <resource> <name> -o yaml

# View logs from specific component
kubectl logs -l app=<component-name> -f

# Execute commands in pod
kubectl exec -it <pod-name> -- bash
```

---

## Installation Issues

### CRDs Not Installing

**Problem:** CRDs fail to install or are not recognized.

**Solution:**

```bash
# Check if CRDs exist
kubectl get crd | grep vyogo.tech

# Manually install CRDs
kubectl apply -f config/crd/bases/

# Verify CRD installation
kubectl get crd vyogo.tech_frappebenchs.yaml -o yaml
```

### Operator Pod Not Starting

**Problem:** Operator pod is in CrashLoopBackOff or pending.

**Diagnosis:**

```bash
# Check pod status
kubectl get pods -n frappe-operator-system

# Check logs
kubectl logs -n frappe-operator-system <pod-name>

# Describe pod for events
kubectl describe pod -n frappe-operator-system <pod-name>
```

**Common Causes:**

1. **Insufficient Resources:**
   ```bash
   # Check node resources
   kubectl top nodes
   kubectl describe nodes
   ```

2. **Image Pull Errors:**
   ```bash
   # Check image pull status
   kubectl describe pod -n frappe-operator-system <pod-name> | grep -A 10 Events
   
   # Verify image exists
   docker pull <image-name>
   ```

3. **RBAC Issues:**
   ```bash
   # Check service account
   kubectl get serviceaccount -n frappe-operator-system
   
   # Check role bindings
   kubectl get rolebinding,clusterrolebinding | grep frappe-operator
   ```

### Webhook Configuration Issues

**Problem:** Validating/mutating webhook errors.

**Solution:**

```bash
# Delete webhooks
kubectl delete validatingwebhookconfiguration frappe-operator-validating-webhook-configuration
kubectl delete mutatingwebhookconfiguration frappe-operator-mutating-webhook-configuration

# Reinstall operator
kubectl apply -f config/install.yaml
```

---

## Bench Issues

### Bench Not Becoming Ready

**Problem:** FrappeBench status shows `ready: false`.

**Diagnosis:**

```bash
# Check bench status
kubectl get frappebench <bench-name> -o yaml | grep -A 10 status

# Check all bench pods
kubectl get pods -l bench=<bench-name>

# Check init job
kubectl get job <bench-name>-init
kubectl logs job/<bench-name>-init
```

**Common Causes:**

1. **Init Job Failed:**
   ```bash
   # Check init job logs
   kubectl logs -l job-name=<bench-name>-init
   
   # Delete and recreate if needed
   kubectl delete job <bench-name>-init
   kubectl delete frappebench <bench-name>
   kubectl apply -f bench.yaml
   ```

2. **Image Pull Issues:**
   ```bash
   # Check if image exists
   kubectl describe pod <bench-pod> | grep Image
   
   # Create pull secret if needed
   kubectl create secret docker-registry regcred \
     --docker-server=<registry> \
     --docker-username=<username> \
     --docker-password=<password>
   ```

3. **Resource Constraints:**
   ```bash
   # Check if pods are pending
   kubectl get pods -l bench=<bench-name>
   
   # Check resource availability
   kubectl describe nodes
   kubectl top nodes
   ```

### Redis/DragonFly Not Starting

**Problem:** Redis or DragonFly pod failing.

**Solution:**

```bash
# Check Redis logs
kubectl logs -l app=<bench-name>-redis

# Common issues:
# 1. Memory limits too low
kubectl patch frappebench <bench-name> --type=merge -p '{
  "spec": {
    "redisConfig": {
      "resources": {
        "requests": {"memory": "2Gi"},
        "limits": {"memory": "4Gi"}
      }
    }
  }
}'

# 2. Persistence issues
kubectl get pvc | grep redis
kubectl describe pvc <redis-pvc>
```

---

## Site Issues

### Site Stuck in Provisioning

**Problem:** FrappeSite phase remains "Provisioning".

**Diagnosis:**

```bash
# Check site status
kubectl get frappesite <site-name> -o yaml

# Check if bench is ready
kubectl get frappebench <bench-ref-name> -o jsonpath='{.status.ready}'

# Check site init job
kubectl get job <site-name>-init
kubectl logs job/<site-name>-init -f
```

**Common Causes:**

1. **Bench Not Ready:**
   ```bash
   # Wait for bench to be ready first
   kubectl wait --for=condition=ready frappebench/<bench-name> --timeout=600s
   ```

2. **Database Connection Issues:**
   ```bash
   # Check database secret
   kubectl get secret <db-secret-name> -o yaml
   
   # Test database connectivity
   kubectl run mysql-client --rm -it --image=mysql:8 -- \
     mysql -h <db-host> -u <user> -p<password>
   ```

3. **Insufficient Storage:**
   ```bash
   # Check PVC status
   kubectl get pvc | grep <site-name>
   kubectl describe pvc <site-pvc>
   
   # Check storage class
   kubectl get storageclass
   ```

### Site Init Job Fails

**Problem:** Site initialization job fails.

**Solution:**

```bash
# View init job logs
kubectl logs job/<site-name>-init

# Common errors and fixes:

# 1. Database already exists
# Delete the site from database manually
kubectl exec -it mariadb-0 -- \
  mysql -u root -p -e "DROP DATABASE IF EXISTS <site_db>;"

# 2. Database connection refused
# Check database service and credentials
kubectl get svc <db-service>
kubectl get secret <db-secret> -o yaml

# 3. Permission denied
# Check security context and volume permissions
kubectl exec -it <site-pod> -- ls -la /home/frappe/frappe-bench/sites

# Restart init job
kubectl delete job <site-name>-init
kubectl delete frappesite <site-name>
kubectl apply -f site.yaml
```

### Site Not Accessible

**Problem:** Cannot access site via browser.

**Diagnosis:**

```bash
# Check all site components
kubectl get pods -l site=<site-name>

# Check services
kubectl get svc | grep <site-name>

# Check ingress
kubectl get ingress <site-name>-ingress
kubectl describe ingress <site-name>-ingress

# Test internal connectivity
kubectl run curl-test --rm -it --image=curlimages/curl -- \
  curl -H "Host: <site-domain>" http://<bench-nginx-service>:8080
```

**Common Causes:**

1. **Ingress Not Configured:**
   ```bash
   # Check ingress controller
   kubectl get pods -n ingress-nginx
   
   # Check ingress resource
   kubectl describe ingress <site-name>-ingress
   
   # Check ingress class
   kubectl get ingressclass
   ```

2. **DNS Not Configured:**
   ```bash
   # Check DNS resolution
   nslookup <site-domain>
   dig <site-domain>
   
   # For local testing, add to /etc/hosts
   echo "127.0.0.1 <site-domain>" | sudo tee -a /etc/hosts
   ```

3. **TLS Certificate Issues:**
   ```bash
   # Check cert-manager
   kubectl get certificate -A
   kubectl describe certificate <cert-name>
   
   # Check certificate secret
   kubectl get secret <tls-secret> -o yaml
   ```

---

## Database Issues

### Cannot Connect to Database

**Problem:** Site cannot connect to database.

**Solution:**

```bash
# Check database service
kubectl get svc <db-service>

# Check database pods
kubectl get pods -l app=mariadb

# Test connectivity from site pod
kubectl exec -it <site-pod> -- \
  mysql -h <db-host> -u <user> -p<password> -e "SELECT 1;"

# Check database credentials secret
kubectl get secret <db-secret> -o jsonpath='{.data.password}' | base64 -d

# Check network policies
kubectl get networkpolicy -A
```

### Database User/Grants Issues (MariaDB Operator)

**Problem:** Using MariaDB Operator and permissions are incorrect.

**Solution:**

```bash
# Check MariaDB User resources
kubectl get user -A
kubectl describe user <site-db-user>

# Check grants
kubectl get grant -A
kubectl describe grant <site-db-grant>

# Manually fix (if needed)
kubectl exec -it mariadb-0 -- mysql -u root -p << EOF
GRANT ALL PRIVILEGES ON <database>.* TO '<user>'@'%';
FLUSH PRIVILEGES;
EOF
```

### Database Performance Issues

**Problem:** Slow database queries.

**Diagnosis:**

```bash
# Check database resource usage
kubectl top pod <db-pod>

# Check slow query log
kubectl exec -it mariadb-0 -- mysql -u root -p -e "
  SELECT * FROM mysql.slow_log ORDER BY start_time DESC LIMIT 10;
"

# Check connection count
kubectl exec -it mariadb-0 -- mysql -u root -p -e "
  SHOW STATUS LIKE 'Threads_connected';
  SHOW STATUS LIKE 'Max_used_connections';
"
```

**Solutions:**

1. **Increase resources:**
   ```bash
   kubectl patch frappebench <name> --type=merge -p '{
     "spec": {
       "dbConfig": {
         "resources": {
           "requests": {"cpu": "2", "memory": "8Gi"}
         }
       }
     }
   }'
   ```

2. **Optimize queries:** Check Frappe logs for slow queries and optimize.

3. **Add indexes:** Use Frappe's database tools to add appropriate indexes.

---

## Networking Issues

### Services Not Resolving

**Problem:** Cannot resolve service DNS names.

**Solution:**

```bash
# Test DNS resolution
kubectl run dns-test --rm -it --image=busybox -- nslookup <service-name>

# Check CoreDNS
kubectl get pods -n kube-system -l k8s-app=kube-dns
kubectl logs -n kube-system -l k8s-app=kube-dns

# Check service endpoints
kubectl get endpoints <service-name>
```

### Ingress Not Working

**Problem:** External traffic not reaching services.

**Solution:**

```bash
# Check ingress controller
kubectl get pods -n ingress-nginx
kubectl logs -n ingress-nginx -l app.kubernetes.io/component=controller

# Check ingress resource
kubectl get ingress
kubectl describe ingress <ingress-name>

# Verify ingress class
kubectl get ingressclass
kubectl describe ingressclass <class-name>

# Check ingress controller service
kubectl get svc -n ingress-nginx
```

### Network Policies Blocking Traffic

**Problem:** Network policies preventing communication.

**Solution:**

```bash
# List network policies
kubectl get networkpolicy -A

# Describe policy
kubectl describe networkpolicy <policy-name>

# Temporarily remove policy for testing
kubectl delete networkpolicy <policy-name>

# Test connectivity
kubectl run test-pod --rm -it --image=curlimages/curl -- curl <service>
```

---

## Performance Issues

### High Response Times

**Problem:** Slow page loads and API responses.

**Diagnosis:**

```bash
# Check resource usage
kubectl top pods

# Check gunicorn logs
kubectl logs -l app=<site>-gunicorn --tail=100

# Check worker queue lengths
kubectl exec -it <site-pod> -- bench --site <site-name> doctor
```

**Solutions:**

1. **Scale up gunicorn:**
   ```bash
   kubectl patch frappebench <name> --type=merge -p '{
     "spec": {"componentReplicas": {"gunicorn": 5}}
   }'
   ```

2. **Increase resources:**
   ```bash
   kubectl patch frappebench <name> --type=merge -p '{
     "spec": {
       "componentResources": {
         "gunicorn": {
           "requests": {"cpu": "2", "memory": "4Gi"}
         }
       }
     }
   }'
   ```

3. **Check database performance:** See [Database Performance Issues](#database-performance-issues).

### Workers Not Processing Jobs

**Problem:** Background jobs queuing up.

**Solution:**

```bash
# Check worker logs
kubectl logs -l app=<site>-worker-default --tail=100

# Check Redis queue
kubectl exec -it <redis-pod> -- redis-cli LLEN <queue-name>

# Scale up workers
kubectl patch frappebench <name> --type=merge -p '{
  "spec": {
    "componentReplicas": {
      "workerDefault": 5,
      "workerLong": 3
    }
  }
}'

# Check for stuck jobs
kubectl exec -it <site-pod> -- bench --site <site-name> show-pending-jobs
```

### Memory Issues

**Problem:** Pods being OOMKilled.

**Solution:**

```bash
# Check pod events
kubectl describe pod <pod-name> | grep -A 10 Events

# Increase memory limits
kubectl patch frappebench <name> --type=merge -p '{
  "spec": {
    "componentResources": {
      "gunicorn": {
        "limits": {"memory": "8Gi"}
      }
    }
  }
}'

# Check for memory leaks
kubectl top pod <pod-name> --containers
```

---

## Storage Issues

### PVC Not Binding

**Problem:** PersistentVolumeClaim stuck in Pending.

**Solution:**

```bash
# Check PVC status
kubectl describe pvc <pvc-name>

# Check storage class
kubectl get storageclass
kubectl describe storageclass <class-name>

# Check available PVs
kubectl get pv

# Check provisioner logs (depends on storage provider)
kubectl logs -n kube-system -l app=<storage-provisioner>
```

### Disk Full

**Problem:** Storage exhausted.

**Solution:**

```bash
# Check disk usage in pod
kubectl exec -it <pod-name> -- df -h

# Check PVC size
kubectl get pvc <pvc-name>

# Expand PVC (if storage class supports it)
kubectl patch pvc <pvc-name> -p '{
  "spec": {"resources": {"requests": {"storage": "100Gi"}}}
}'

# Clean up old files
kubectl exec -it <site-pod> -- bench --site <site-name> clear-cache
kubectl exec -it <site-pod> -- bench --site <site-name> clear-logs
```

### File Permission Issues

**Problem:** Permission denied errors.

**Solution:**

```bash
# Check file permissions
kubectl exec -it <site-pod> -- ls -la /home/frappe/frappe-bench/sites

# Fix permissions
kubectl exec -it <site-pod> -- \
  chown -R frappe:frappe /home/frappe/frappe-bench/sites

# Check security context
kubectl get pod <pod-name> -o jsonpath='{.spec.securityContext}'
```

---

## Getting Help

### Collecting Debug Information

When reporting issues, collect:

```bash
# 1. Resource definitions
kubectl get frappebench <name> -o yaml > bench.yaml
kubectl get frappesite <name> -o yaml > site.yaml

# 2. Pod status
kubectl get pods -o wide > pods.txt

# 3. Events
kubectl get events --sort-by='.lastTimestamp' > events.txt

# 4. Logs
kubectl logs -l bench=<name> --tail=500 > bench-logs.txt
kubectl logs -l site=<name> --tail=500 > site-logs.txt
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager > operator-logs.txt

# 5. Describe resources
kubectl describe frappebench <name> > bench-describe.txt
kubectl describe frappesite <name> > site-describe.txt
```

### Resources

- **GitHub Issues**: [vyogotech/frappe-operator/issues](https://github.com/vyogotech/frappe-operator/issues)
- **Documentation**: [Frappe Operator Docs](https://vyogotech.github.io/frappe-operator/)
- **Frappe Forum**: [discuss.frappe.io](https://discuss.frappe.io/)

---

## Next Steps

- **[Operations Guide](operations.md)** - Production operations
- **[Examples](examples.md)** - Configuration examples
- **[API Reference](api-reference.md)** - Complete specification

