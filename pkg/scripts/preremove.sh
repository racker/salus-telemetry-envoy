#!/bin/bash

SERVICE=telemetry-envoy


# debs pass upgrade, remove, or purge
# rpms pass number of versions installed, so 0 means none remain
if [ "$1" == "remove" ] || [ "$1" == "purge" ] || [ "$1" == "0" ]; then

  systemctl stop ${SERVICE}.service

fi

