#!/bin/bash
# Script to update frappesite_controller.go to use secrets for credentials

# This is a guide for the changes needed to move from environment variables to secret-based credential handling

cat << 'EOF'
================================================================================
Security Enhancement: Move Credentials from Environment Variables to Secrets
================================================================================

PROBLEM:
--------
Currently, database credentials (DB_USER, DB_PASSWORD, ADMIN_PASSWORD) are passed as
environment variables to the init job. This is a security anti-pattern because:

1. Environment variables appear in process listings (ps aux)
2. They may be captured in logs or debugging output
3. They persist in memory and can be dumped
4. Kubernetes shows env vars in pod descriptions

SOLUTION:
---------
Move all sensitive credentials to Kubernetes secrets mounted as read-only volumes.
Credentials will be read from files at /run/secrets/*, never appearing in env vars.

CHANGES REQUIRED:
-----------------

1. Create credential secret before job creation
2. Update init script to read from secret files
3. Mount secret as volume in job pod spec
4. Keep only non-sensitive data in env vars (SITE_NAME, DOMAIN, DB_PROVIDER, BENCH_NAME)

FILES TO MODIFY:
----------------
controllers/frappesite_controller.go

Location: ensureSiteInitialized() function (around line 573)

DETAILED STEPS:
---------------

Step 1: Before creating init job, create a secret with all credentials
  - Name: {site-name}-init-credentials
  - Keys: db-host, db-port, db-name, db-user, db-password, admin-password
  - Owner reference set to site (deleted when site is deleted)

Step 2: Update the initScript bash code
  - Remove credential checks from env vars
  - Add: read credentials from /run/secrets/* files
  - Keep non-sensitive env vars (SITE_NAME, DOMAIN, BENCH_NAME, DB_PROVIDER)

Step 3: Update job.Spec.Template.Spec
  - Add VolumeMounts: mount credentials secret at /run/secrets (ReadOnly: true)
  - Remove DB_USER, DB_PASSWORD, DB_HOST, DB_PORT, ADMIN_PASSWORD from Env
  - Add Volumes: secret volume for credentials

SECURITY BENEFITS:
------------------
✓ Credentials never appear in environment variables
✓ Credentials never appear in pod descriptions or logs
✓ Credentials only accessible to the init container (mounted read-only)
✓ Follows Kubernetes security best practices
✓ Compliant with security scanning tools

BACKWARDS COMPATIBILITY:
-----------------------
This is an internal implementation change with no external API changes.
Existing FrappeSite resources continue to work unchanged.

TESTING:
--------
After implementation:
1. Create a test site and verify it initializes successfully
2. Check that credentials are not in 'kubectl describe pod' output
3. Check that secret files are read-only (chmod 0400)
4. Verify pod environment variables do not contain sensitive data

EOF
