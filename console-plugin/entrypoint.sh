#!/bin/sh
set -e

if [ -f /var/serving-cert/tls.crt ] && [ -f /var/serving-cert/tls.key ]; then
    echo "[INFO] Detected OpenShift serving certificates. Starting Nginx with HTTPS on 9443 and HTTP on 8080..."
    cp /etc/nginx/nginx-ssl.conf /tmp/nginx.conf
else
    echo "[INFO] No TLS certificates mounted. Starting Nginx in standard HTTP mode on 8080 (vanilla Kubernetes / Ingress)..."
    cp /etc/nginx/nginx-http.conf /tmp/nginx.conf
fi

exec nginx -c /tmp/nginx.conf -g "daemon off;"
