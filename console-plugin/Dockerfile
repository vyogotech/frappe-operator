# OpenShift Dynamic Console Plugin & Standalone Kubernetes Dashboard
# Certified for OpenShift restricted-v2 SCC & Vanilla Kubernetes (non-root)

FROM registry.access.redhat.com/ubi9/nginx-120:latest

USER 0

COPY nginx-http.conf /etc/nginx/nginx-http.conf
COPY nginx-ssl.conf /etc/nginx/nginx-ssl.conf
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY plugin-manifest.json /usr/share/nginx/html/plugin-manifest.json
COPY public/index.html /usr/share/nginx/html/index.html

RUN chmod +x /usr/local/bin/entrypoint.sh && \
    chgrp -R 0 /usr/share/nginx/html /etc/nginx /tmp && \
    chmod -R g=u /usr/share/nginx/html /etc/nginx /tmp

USER 1001

EXPOSE 8080 9443

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
