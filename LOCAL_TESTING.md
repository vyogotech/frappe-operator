# Testing Frappe Sites Locally

## Current Status

✅ **FrappeBench**: Fully operational with all components
✅ **FrappeSite**: Site created successfully  
✅ **Ingress**: Created and routing correctly
✅ **Redis Services**: Separate redis-cache and redis-queue services

## Accessing Sites Locally

### Issue with `.local` Domains

When you create a site with a `.local` domain (e.g., `site1.local`), the Ingress is created correctly, but you can't access it externally without additional setup.

### Solution Options

#### Option 1: Port-Forward to NGINX Service (Recommended for Testing)

```bash
# Port-forward to the bench's NGINX service
kubectl port-forward svc/test-bench-nginx 8080:8080 -n default

# Access the site
curl -H "Host: site1.local" http://localhost:8080
# Or in browser: http://localhost:8080 (may need to add Host header via browser extension)
```

#### Option 2: Port-Forward to Ingress Controller

```bash
# If you have nginx-ingress controller
kubectl port-forward -n ingress-nginx svc/ingress-nginx-controller 8080:80

# Add to /etc/hosts
echo "127.0.0.1 site1.local" | sudo tee -a /etc/hosts

# Access the site
curl http://site1.local:8080
# Or in browser: http://site1.local:8080
```

#### Option 3: Use Real Domain (Production)

For production deployments, use actual domains:

```yaml
spec:
  siteName: erp.mycompany.com
  domain: erp.mycompany.com
```

The Ingress will work automatically with your DNS and Ingress Controller.

## What's Working

### Ingress Configuration
```yaml
NAME            CLASS   HOSTS         ADDRESS   PORTS   AGE
site1-ingress   nginx   site1.local             80      
```

**Routes:**
- Host: `site1.local`
- Path: `/`
- Backend: `test-bench-nginx:8080`

This is **correct**! The Ingress is properly configured.

### Redis Services
```bash
$ kubectl get svc -n default | grep redis
test-bench-redis-cache   ClusterIP   None   <none>   6379/TCP
test-bench-redis-queue   ClusterIP   None   <none>   6379/TCP
```

Both `redis-cache` and `redis-queue` services are created (socket.io removed for v15+).

## Intelligent Domain Handling

The operator has 4-priority domain resolution:

1. **Explicit**: `spec.domain` specified in FrappeSite
2. **Bench Suffix**: Uses bench's `domainSuffix` + siteName
3. **Auto-Detected**: Detects cluster domain from Ingress Controller
4. **Fallback**: Uses `siteName` as-is

For `site1.local`, it used **explicit** domain (priority 1).

## Recommended Test Flow

1. **Create site with real domain** (if available):
   ```yaml
   spec:
     siteName: test.example.com
     domain: test.example.com
   ```

2. **OR use port-forwarding** for local testing:
   ```bash
   kubectl port-forward svc/test-bench-nginx 8080:8080 -n default
   ```

3. **Access via**:
   - Direct: `http://localhost:8080` (if only one site)
   - With Host header: `curl -H "Host: site1.local" http://localhost:8080`

## Testing Multiple Sites

Each site gets its own Ingress with different host rules:
- site1.local → test-bench-nginx:8080
- site2.local → test-bench-nginx:8080
- etc.

NGINX (and Frappe) routes based on the HTTP `Host` header to the correct site.

## Next Steps

1. ✅ Redis services fixed (cache + queue)
2. 📝 Test multiple sites
3. 📝 Document production setup
4. 📝 Create release


## Current Status

✅ **FrappeBench**: Fully operational with all components
✅ **FrappeSite**: Site created successfully  
✅ **Ingress**: Created and routing correctly
✅ **Redis Services**: Separate redis-cache and redis-queue services

## Accessing Sites Locally

### Issue with `.local` Domains

When you create a site with a `.local` domain (e.g., `site1.local`), the Ingress is created correctly, but you can't access it externally without additional setup.

### Solution Options

#### Option 1: Port-Forward to NGINX Service (Recommended for Testing)

```bash
# Port-forward to the bench's NGINX service
kubectl port-forward svc/test-bench-nginx 8080:8080 -n default

# Access the site
curl -H "Host: site1.local" http://localhost:8080
# Or in browser: http://localhost:8080 (may need to add Host header via browser extension)
```

#### Option 2: Port-Forward to Ingress Controller

```bash
# If you have nginx-ingress controller
kubectl port-forward -n ingress-nginx svc/ingress-nginx-controller 8080:80

# Add to /etc/hosts
echo "127.0.0.1 site1.local" | sudo tee -a /etc/hosts

# Access the site
curl http://site1.local:8080
# Or in browser: http://site1.local:8080
```

#### Option 3: Use Real Domain (Production)

For production deployments, use actual domains:

```yaml
spec:
  siteName: erp.mycompany.com
  domain: erp.mycompany.com
```

The Ingress will work automatically with your DNS and Ingress Controller.

## What's Working

### Ingress Configuration
```yaml
NAME            CLASS   HOSTS         ADDRESS   PORTS   AGE
site1-ingress   nginx   site1.local             80      
```

**Routes:**
- Host: `site1.local`
- Path: `/`
- Backend: `test-bench-nginx:8080`

This is **correct**! The Ingress is properly configured.

### Redis Services
```bash
$ kubectl get svc -n default | grep redis
test-bench-redis-cache   ClusterIP   None   <none>   6379/TCP
test-bench-redis-queue   ClusterIP   None   <none>   6379/TCP
```

Both `redis-cache` and `redis-queue` services are created (socket.io removed for v15+).

## Intelligent Domain Handling

The operator has 4-priority domain resolution:

1. **Explicit**: `spec.domain` specified in FrappeSite
2. **Bench Suffix**: Uses bench's `domainSuffix` + siteName
3. **Auto-Detected**: Detects cluster domain from Ingress Controller
4. **Fallback**: Uses `siteName` as-is

For `site1.local`, it used **explicit** domain (priority 1).

## Recommended Test Flow

1. **Create site with real domain** (if available):
   ```yaml
   spec:
     siteName: test.example.com
     domain: test.example.com
   ```

2. **OR use port-forwarding** for local testing:
   ```bash
   kubectl port-forward svc/test-bench-nginx 8080:8080 -n default
   ```

3. **Access via**:
   - Direct: `http://localhost:8080` (if only one site)
   - With Host header: `curl -H "Host: site1.local" http://localhost:8080`

## Testing Multiple Sites

Each site gets its own Ingress with different host rules:
- site1.local → test-bench-nginx:8080
- site2.local → test-bench-nginx:8080
- etc.

NGINX (and Frappe) routes based on the HTTP `Host` header to the correct site.

## Next Steps

1. ✅ Redis services fixed (cache + queue)
2. 📝 Test multiple sites
3. 📝 Document production setup
4. 📝 Create release


