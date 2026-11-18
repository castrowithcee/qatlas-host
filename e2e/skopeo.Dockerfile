# skopeo of one environment: the base image from deps trusting the test CA of the registry.
ARG BASE
FROM ${BASE}

COPY certs/ca.crt /usr/local/share/ca-certificates/qhoste2e-ca.crt
RUN update-ca-certificates
