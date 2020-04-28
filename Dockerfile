# This file allows for a "dev" build of the Envoy for selectively deploying on dev/perf systems
FROM golang:1.13 as builder

WORKDIR /build

# cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# ...and bring over everything to build
COPY . .
RUN make build-dev

FROM alpine:latest
RUN apk --update add ca-certificates
COPY --from=builder /build/telemetry-envoy /
ENTRYPOINT ["/telemetry-envoy"]
