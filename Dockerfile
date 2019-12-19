FROM scratch

COPY telemetry-envoy /
ENTRYPOINT ["/telemetry-envoy"]
