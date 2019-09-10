FROM golang:1.13 as builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o telemetry-envoy .

FROM scratch

COPY --from=builder /build/telemetry-envoy /telemetry-envoy
ENTRYPOINT ["/telemetry-envoy"]