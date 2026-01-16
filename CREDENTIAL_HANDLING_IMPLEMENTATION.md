// Implementation update for secure credential handling
// This shows how to update ensureSiteInitialized to use secrets instead of env vars

// Key changes:
// 1. Create a secret containing all database credentials
// 2. Mount the secret as a volume in the init job
// 3. Read credentials from secret files in the init script

// Step 1: Create database credentials secret (before job creation)
initCredentialsSecretName := fmt.Sprintf("%s-init-credentials", site.Name)
initCredentialsSecret := &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      initCredentialsSecretName,
		Namespace: site.Namespace,
		Labels: map[string]string{
			"app":  "frappe",
			"site": site.Name,
		},
	},
	Type: corev1.SecretTypeOpaque,
	StringData: map[string]string{
		"db-host":     dbHost,
		"db-port":     dbPort,
		"db-name":     dbName,
		"db-user":     dbUser,
		"db-password": dbPassword,
		"admin-password": adminPassword,
	},
}

if err := controllerutil.SetControllerReference(site, initCredentialsSecret, r.Scheme); err != nil {
	return false, err
}

if err := r.Create(ctx, initCredentialsSecret); err != nil && !errors.IsAlreadyExists(err) {
	return false, fmt.Errorf("failed to create init credentials secret: %w", err)
}

// Step 2: Update init script to read from secret files
initScript := `#!/bin/bash
set -e

cd /home/frappe/frappe-bench

echo "Creating Frappe site: $SITE_NAME"
echo "Domain: $DOMAIN"

# Validate environment variables
if [[ -z "$SITE_NAME" || -z "$DOMAIN" || -z "$BENCH_NAME" || -z "$DB_PROVIDER" ]]; then
    echo "ERROR: Required environment variables not set"
    exit 1
fi

# Read sensitive data from secret files (not from env vars)
DB_HOST=$(cat /run/secrets/db-host)
DB_PORT=$(cat /run/secrets/db-port)
DB_NAME=$(cat /run/secrets/db-name)
DB_USER=$(cat /run/secrets/db-user)
DB_PASSWORD=$(cat /run/secrets/db-password)
ADMIN_PASSWORD=$(cat /run/secrets/admin-password)

if [[ -z "$DB_HOST" || -z "$DB_PORT" || -z "$DB_NAME" || -z "$DB_USER" || -z "$DB_PASSWORD" || -z "$ADMIN_PASSWORD" ]]; then
    echo "ERROR: Failed to read credentials from secret files"
    exit 1
fi

# Dynamically build the --install-app argument
INSTALL_APP_ARG=""
if [[ -n "$APPS_TO_INSTALL" ]]; then
    for app in $APPS_TO_INSTALL; do
        INSTALL_APP_ARG+=" --install-app=$app"
    done
fi

# Run bench new-site with provider-specific database configuration
if [[ "$DB_PROVIDER" == "mariadb" ]] || [[ "$DB_PROVIDER" == "postgres" ]]; then
    echo "Creating site with $DB_PROVIDER database (pre-provisioned)"
    
    # Check if bench version supports --db-user flag
    DB_USER_FLAG=""
    if bench new-site --help | grep -q " --db-user"; then
        echo "Detected support for --db-user flag"
        DB_USER_FLAG="--db-user=$DB_USER"
    elif [[ "$DB_USER" != "$DB_NAME" ]]; then
        echo "WARNING: Your bench version does not support --db-user. Using DB_NAME as username."
    else
        echo "Bench version does not support --db-user, but DB_USER matches DB_NAME. Proceeding."
    fi

    bench new-site \
      --db-type="$DB_PROVIDER" \
      --db-name="$DB_NAME" \
      --db-host="$DB_HOST" \
      --db-port="$DB_PORT" \
      $DB_USER_FLAG \
      --db-password="$DB_PASSWORD" \
      --no-setup-db \
      --admin-password="$ADMIN_PASSWORD" \
      $INSTALL_APP_ARG \
      --verbose \
      "$SITE_NAME"

elif [[ "$DB_PROVIDER" == "sqlite" ]]; then
    echo "Creating site with SQLite database (file-based)"
    bench new-site "$SITE_NAME" \
      --db-type=sqlite \
      --admin-password="$ADMIN_PASSWORD" \
      $INSTALL_APP_ARG \
      --verbose

else
    echo "ERROR: Unsupported database provider: $DB_PROVIDER"
    exit 1
fi

echo "Site $SITE_NAME created successfully!"

# Update site_config.json with domain and Redis configuration using Python
echo "Updating site_config.json with domain and Redis"
python3 << 'PYTHON_SCRIPT'
import json
import os

# Get values from environment variables (safe, non-sensitive)
site_name = os.environ['SITE_NAME']
domain = os.environ['DOMAIN']
bench_name = os.environ['BENCH_NAME']

site_path = f"/home/frappe/frappe-bench/sites/{site_name}"
config_file = os.path.join(site_path, "site_config.json")

# Read existing config
with open(config_file, 'r') as f:
    config = json.load(f)

# Update with resolved domain
config['host_name'] = domain

# Add Redis configuration for this site
config['redis_cache'] = f"redis://{bench_name}-redis-cache:6379"
config['redis_queue'] = f"redis://{bench_name}-redis-queue:6379"

# Write back
with open(config_file, 'w') as f:
    json.dump(config, f, indent=2)

print(f"Updated site_config.json for domain: {domain}")
print(f"Redis cache: {bench_name}-redis-cache:6379")
print(f"Redis queue: {bench_name}-redis-queue:6379")
PYTHON_SCRIPT

echo "Site initialization complete!"
`

// Step 3: Update job spec to include secret volume
job = &batchv1.Job{
	ObjectMeta: metav1.ObjectMeta{
		Name:      jobName,
		Namespace: site.Namespace,
		Labels: map[string]string{
			"app":  "frappe",
			"site": site.Name,
		},
	},
	Spec: batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				RestartPolicy:   corev1.RestartPolicyNever,
				SecurityContext: r.getPodSecurityContext(bench),
				Containers: []corev1.Container{
					{
						Name:    "site-init",
						Image:   r.getBenchImage(ctx, bench),
						Command: []string{"bash", "-c"},
						Args:    []string{initScript},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "sites",
								MountPath: "/home/frappe/frappe-bench/sites",
							},
							{
								Name:      "credentials",
								MountPath: "/run/secrets",
								ReadOnly:  true,
							},
						},
						SecurityContext: r.getContainerSecurityContext(bench),
						// Only non-sensitive data in env vars
						Env: []corev1.EnvVar{
							{
								Name:  "SITE_NAME",
								Value: site.Spec.SiteName,
							},
							{
								Name:  "DOMAIN",
								Value: domain,
							},
							{
								Name:  "DB_PROVIDER",
								Value: dbProvider,
							},
							{
								Name:  "BENCH_NAME",
								Value: bench.Name,
							},
							{
								Name:  "APPS_TO_INSTALL",
								Value: strings.Join(appNames, " "),
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "sites",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: pvcName,
							},
						},
					},
					{
						Name: "credentials",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: initCredentialsSecretName,
								DefaultMode: &readOnlyMode, // 0400 (read-only)
							},
						},
					},
				},
			},
		},
	},
}
