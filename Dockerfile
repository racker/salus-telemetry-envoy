FROM alpline:latest as certs
RUN apk --update add ca-certificates

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY telemetry-envoy /
ENTRYPOINT ["/telemetry-envoy"]
