# Redis
kubectl create secret generic external-redis-creds -n frappe \
  --from-literal=password=frappe123

# MariaDB
kubectl create secret generic external-mariadb-creds -n frappe \
  --from-literal=username=frappe-user \
  --from-literal=password=frappe123

kubectl create secret generic external-mariadb-creds -n frappe \
  --from-literal=username=frappe-user \
  --from-literal=password=frappe123 \
  --from-literal=database=vyogotech


