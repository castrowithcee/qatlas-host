# Image registry with HTTPS and a login (htpasswd, bcrypt); e2e/run.sh writes the htpasswd file into the build
# context with the htpasswd command of the fakes, so the build needs no network and no htpasswd program.
FROM registry:2

COPY certs /certs
COPY htpasswd /auth/htpasswd

ENV REGISTRY_HTTP_ADDR=0.0.0.0:443 \
	REGISTRY_HTTP_TLS_CERTIFICATE=/certs/server.crt \
	REGISTRY_HTTP_TLS_KEY=/certs/server.key \
	REGISTRY_AUTH=htpasswd \
	REGISTRY_AUTH_HTPASSWD_REALM=qhost-e2e-private \
	REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd
