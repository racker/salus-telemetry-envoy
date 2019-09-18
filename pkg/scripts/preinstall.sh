#!/bin/bash

if ! id telemetry-envoy &>/dev/null; then
    useradd -r -M telemetry-envoy -s /bin/false -d /var/lib/telemetry-envoy -U
fi
