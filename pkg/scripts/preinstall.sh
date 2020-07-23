#!/bin/bash

USER=telemetry-envoy
DATA_DIR=/var/lib/telemetry-envoy

if ! id ${USER} &>/dev/null; then
    useradd -r -M ${USER} -s /bin/false -d ${DATA_DIR} -U
fi
