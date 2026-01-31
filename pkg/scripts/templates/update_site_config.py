# Site configuration update script for Frappe (Python)
# Updates site_config.json with domain and Redis configuration

import json
import os

# Read from secret files mounted at /tmp/site-secrets
def read_secret(name, default=''):
    try:
        with open(f'/tmp/site-secrets/{name}', 'r') as f:
            return f.read().strip()
    except FileNotFoundError:
        return default

site_name = read_secret('site_name')
domain = read_secret('domain')
bench_name = read_secret('bench_name')
db_host = read_secret('db_host')
db_port = read_secret('db_port')
db_name = read_secret('db_name')
db_user = read_secret('db_user')
db_password = read_secret('db_password')
db_provider = read_secret('db_provider')

site_path = f"/home/frappe/frappe-bench/sites/{site_name}"
config_file = os.path.join(site_path, "site_config.json")

# Read or initialize config
try:
    with open(config_file, 'r') as f:
        config = json.load(f)
except FileNotFoundError:
    config = {}

# Update with resolved domain
config['host_name'] = domain

# Add Redis configuration for this site
config['redis_cache'] = f"redis://{bench_name}-redis-cache:6379"
config['redis_queue'] = f"redis://{bench_name}-redis-queue:6379"

# Explicitly add database credentials for self-healing
config['db_name'] = db_name
config['db_user'] = db_user
config['db_password'] = db_password
config['db_host'] = db_host
config['db_type'] = db_provider

# Ensure directory exists
os.makedirs(site_path, exist_ok=True)

# Ensure logs directory exists
os.makedirs(os.path.join(site_path, "logs"), exist_ok=True)

# Write back
with open(config_file, 'w') as f:
    json.dump(config, f, indent=2)

print(f"Updated site_config.json for domain: {domain}")
print(f"Redis cache: {bench_name}-redis-cache:6379")
print(f"Redis queue: {bench_name}-redis-queue:6379")
