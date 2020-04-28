

FROM alpine:latest
RUN apk --update add ca-certificates
COPY telemetry-envoy /
ENTRYPOINT ["/telemetry-envoy"]
