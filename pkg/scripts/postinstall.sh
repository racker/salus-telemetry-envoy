#!/bin/bash

setcap CAP_SETFCAP+p /usr/local/bin/telemetry-envoy

# Only adjust top level of data directory since just need to fix up initial, empty dir.
# A recursive chown on a fully populated data directory could become time consuming.
chown telemetry-envoy: /var/lib/telemetry-envoy

# telemetry-envoy owns config
chown -R telemetry-envoy: /etc/salus
# ...and only telemetry-envoy can access config, since there are sensitive fields
chmod -R go-rwx /etc/salus