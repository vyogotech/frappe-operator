#!/bin/bash

# VM Infrastructure Setup Script for Frappe Operator
# Installs and configures MariaDB and Redis on Ubuntu/Debian

set -e

# --- Configuration ---
DB_ROOT_PASSWORD=${DB_ROOT_PASSWORD:-"admin123"}
REDIS_PASSWORD=${REDIS_PASSWORD:-"redis123"}
ALLOWED_IP_RANGE=${ALLOWED_IP_RANGE:-"0.0.0.0/0"} # Change this to your cluster nodes IP range for security

echo "------------------------------------------------"
echo "Starting VM Infrastructure Setup"
echo "------------------------------------------------"

# 1. Update System
echo "[1/5] Updating system packages..."
sudo apt-get update -y
sudo apt-get upgrade -y

# 2. Install MariaDB
echo "[2/5] Installing MariaDB..."
sudo apt-get install -y mariadb-server mariadb-client

# Configure MariaDB for Frappe
echo "Configuring MariaDB for Frappe..."
cat <<EOF | sudo tee /etc/mysql/mariadb.conf.d/99-frappe.cnf
[server]
user = mysql
pid-file = /run/mysqld/mysqld.pid
socket = /run/mysqld/mysqld.sock
basedir = /usr
datadir = /var/lib/mysql
tmpdir = /tmp
lc-messages-dir = /usr/share/mysql
bind-address = 0.0.0.0
query_cache_size = 16M
log_error = /var/log/mysql/error.log

[mysqld]
innodb_file_per_table = 1
innodb_flush_log_at_trx_commit = 1
innodb_log_buffer_size = 64M
innodb_max_dirty_pages_pct = 90
character-set-client-handshake = FALSE
character-set-server = utf8mb4
collation-server = utf8mb4_unicode_ci

[mysql]
default-character-set = utf8mb4
EOF

# Restart MariaDB
sudo systemctl restart mariadb
sudo systemctl enable mariadb

# Secure MariaDB and set root password
echo "Securing MariaDB..."
sudo mysql -e "ALTER USER 'root'@'localhost' IDENTIFIED BY '$DB_ROOT_PASSWORD';"
sudo mysql -u root -p"$DB_ROOT_PASSWORD" -e "DELETE FROM mysql.user WHERE User='';"
sudo mysql -u root -p"$DB_ROOT_PASSWORD" -e "DROP DATABASE IF EXISTS test;"
sudo mysql -u root -p"$DB_ROOT_PASSWORD" -e "DELETE FROM mysql.db WHERE Db='test' OR Db='test\\_%';"
sudo mysql -u root -p"$DB_ROOT_PASSWORD" -e "FLUSH PRIVILEGES;"

# 3. Install Redis
echo "[3/5] Installing Redis..."
sudo apt-get install -y redis-server

# Configure Redis for Production
echo "Configuring Redis..."
sudo sed -i "s/^bind .*/bind 0.0.0.0/" /etc/redis/redis.conf
sudo sed -i "s/^# requirepass .*/requirepass $REDIS_PASSWORD/" /etc/redis/redis.conf
sudo sed -i "s/^protected-mode yes/protected-mode no/" /etc/redis/redis.conf

# Restart Redis
sudo systemctl restart redis-server
sudo systemctl enable redis-server

# 4. Configure Firewall (UFW)
echo "[4/5] Configuring Firewall..."
if command -v ufw > /dev/null; then
    sudo ufw allow ssh
    sudo ufw allow from $ALLOWED_IP_RANGE to any port 3306 comment 'MariaDB external access'
    sudo ufw allow from $ALLOWED_IP_RANGE to any port 6379 comment 'Redis external access'
    echo "y" | sudo ufw enable
else
    echo "UFW not found. Please ensure ports 3306 and 6379 are open in your VM security group/firewall."
fi

# 5. Output Connection Info
echo "------------------------------------------------"
echo "Setup Complete!"
echo "------------------------------------------------"
echo "MariaDB Root Password: $DB_ROOT_PASSWORD"
echo "Redis Password: $REDIS_PASSWORD"
echo ""
echo "You can now use these in your FrappeBench/FrappeSite manifests."
echo "DB Host: $(hostname -I | awk '{print $1}')"
echo "DB Port: 3306"
echo "Redis Host: $(hostname -I | awk '{print $1}')"
echo "Redis Port: 6379"
echo "------------------------------------------------"
